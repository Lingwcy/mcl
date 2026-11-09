package storage

import (
	"context"
	"crypto/rand"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"mcl/internal/core/download"
	"mcl/internal/platform/instancelock"
)

const cacheMarker = "mcl-download-cache-v1\n"
const cacheChild = "mcl-download-cache-v1"

var cacheObjectName = regexp.MustCompile("^object-(sha1-[0-9a-f]{40}|sha256-[0-9a-f]{64})(-[0-9a-f]{128})?-[0-9]{1,10}\\.blob$")
var cacheTempName = regexp.MustCompile("^part-[A-Z2-7]{26}$")

// DownloadCache never manages runtime libraries/assets, local/manual packages,
// transaction backups, credentials or metadata. Only its marked flat namespace.
type DownloadCache struct {
	mu                sync.RWMutex
	defaultBase, base string
	copySlots         chan struct{}
}

func NewDownloadCache(defaultBase, selectedBase string) *DownloadCache {
	return &DownloadCache{defaultBase: defaultBase, base: selectedBase, copySlots: make(chan struct{}, 4)}
}
func (c *DownloadCache) DirectoryFor(base string) string {
	if base == "" {
		base = c.defaultBase
	}
	return filepath.Join(base, cacheChild)
}
func (c *DownloadCache) Directory() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.DirectoryFor(c.base)
}
func (c *DownloadCache) SetDirectory(base string) { c.mu.Lock(); defer c.mu.Unlock(); c.base = base }

func (c *DownloadCache) open(ctx context.Context, directory string) (*os.Root, error) {
	// Apart from the exact default child, the new cache must live outside all
	// application data. Resolve selected-parent aliases before checking scope.
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	appRoot, err := resolveCacheScope(c.defaultBase)
	if err != nil {
		return nil, err
	}
	parent, err := resolveCacheScope(filepath.Dir(directory))
	if err != nil {
		return nil, err
	}
	candidate := filepath.Join(parent, filepath.Base(directory))
	within := func(base, name string) bool {
		rel, err := filepath.Rel(base, name)
		return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
	}
	defaultPath := filepath.Join(appRoot, cacheChild)
	rel, relErr := filepath.Rel(defaultPath, candidate)
	if (relErr != nil || rel != ".") && (within(appRoot, candidate) || within(candidate, appRoot)) {
		return nil, errors.New("自定义下载缓存应位于应用数据目录外，不能包含实例、运行环境或备份")
	}
	return openDownloadCache(ctx, directory)
}

// Resolve missing tails without creating them. A dangling symlink is not a
// missing plain directory and must not be reinterpreted during the scope check.
func resolveCacheScope(name string) (string, error) {
	var tail []string
	for {
		resolved, err := filepath.EvalSymlinks(name)
		if err == nil {
			for i := len(tail) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, tail[i])
			}
			return resolved, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		if info, statErr := os.Lstat(name); statErr == nil && info.Mode()&os.ModeSymlink != 0 {
			return "", errors.New("缓存路径包含失效链接")
		}
		parent := filepath.Dir(name)
		if parent == name {
			return "", err
		}
		tail = append(tail, filepath.Base(name))
		name = parent
	}
}

func openDownloadCache(ctx context.Context, directory string) (*os.Root, error) {
	parent, name := filepath.Dir(directory), filepath.Base(directory)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return nil, err
	}
	base, err := os.OpenRoot(parent)
	if err != nil {
		return nil, err
	}
	defer base.Close()
	wait, finish := context.WithTimeout(ctx, 30*time.Second)
	defer finish()
	for {
		if err := wait.Err(); err != nil {
			return nil, err
		}
		child, err := base.OpenRoot(".")
		if err != nil {
			return nil, err
		}
		lock, err := instancelock.AcquireDownloadCacheInit(child)
		if err == nil {
			defer lock.Close()
			break
		}
		if !errors.Is(err, instancelock.ErrBusy) {
			return nil, err
		}
		timer := time.NewTimer(20 * time.Millisecond)
		select {
		case <-wait.Done():
			timer.Stop()
			return nil, wait.Err()
		case <-timer.C:
		}
	}
	created := false
	if err = base.Mkdir(name, 0o700); err == nil {
		created = true
	} else if !errors.Is(err, os.ErrExist) {
		return nil, err
	}
	info, err := base.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("缓存子目录不能是链接或普通文件")
	}
	root, err := base.OpenRoot(name)
	if err != nil {
		return nil, err
	}
	fail := func(err error) (*os.Root, error) { root.Close(); return nil, err }
	openedDirectory, err := root.Stat(".")
	if err != nil || !os.SameFile(info, openedDirectory) {
		return fail(errors.New("缓存目录读取期间改变"))
	}
	if created {
		f, err := root.OpenFile("owner", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			return fail(err)
		}
		_, err = f.WriteString(cacheMarker)
		if err == nil {
			err = f.Sync()
		}
		err = errors.Join(err, f.Close())
		if err != nil {
			return fail(err)
		}
	}
	mark, err := root.Lstat("owner")
	if err != nil {
		return fail(errors.New("目录缺少 mcl 缓存标记；请选择其他位置"))
	}
	if !mark.Mode().IsRegular() || mark.Size() != int64(len(cacheMarker)) {
		return fail(errors.New("缓存所有权标记无效"))
	}
	f, err := root.Open("owner")
	if err != nil {
		return fail(err)
	}
	opened, err := f.Stat()
	if err != nil || !os.SameFile(mark, opened) {
		f.Close()
		return fail(errors.New("缓存标记已改变"))
	}
	data, err := io.ReadAll(io.LimitReader(f, 128))
	f.Close()
	if err != nil || string(data) != cacheMarker {
		return fail(errors.New("缓存所有权标记无效"))
	}
	return root, nil
}

// Prepare validates ownership and writeability before a preference is saved.
func (c *DownloadCache) Prepare(ctx context.Context, base string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	root, err := c.open(ctx, c.DirectoryFor(base))
	if err != nil {
		return err
	}
	defer root.Close()
	locked, err := root.OpenRoot(".")
	if err != nil {
		return err
	}
	lock, err := instancelock.AcquireDownloadCache(locked, false)
	if err != nil {
		return errors.New("缓存目录正被维护，请稍后重试")
	}
	defer lock.Close()
	name := "part-" + rand.Text()
	f, err := root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer root.Remove(name)
	err = errors.Join(f.Sync(), f.Close())
	return errors.Join(err, ctx.Err())
}

type downloadCacheLease struct {
	root      *os.Root
	lock      *instancelock.Lock
	copySlots chan struct{}
}

func (l *downloadCacheLease) Close() error { return errors.Join(l.root.Close(), l.lock.Close()) }
func (c *DownloadCache) lease(ctx context.Context, exclusive bool) (*downloadCacheLease, error) {
	directory := c.Directory() // A complete job pins this snapshot despite later saves.
	wait, finish := context.WithTimeout(ctx, 30*time.Second)
	defer finish()
	for {
		if err := wait.Err(); err != nil {
			return nil, err
		}
		root, err := c.open(wait, directory)
		if err != nil {
			return nil, err
		}
		locked, err := root.OpenRoot(".")
		if err != nil {
			root.Close()
			return nil, err
		}
		lock, err := instancelock.AcquireDownloadCache(locked, exclusive)
		if err == nil {
			return &downloadCacheLease{root: root, lock: lock, copySlots: c.copySlots}, nil
		}
		root.Close()
		if !errors.Is(err, instancelock.ErrBusy) || exclusive {
			return nil, errors.New("下载缓存正被其他操作使用，请稍后重试")
		}
		timer := time.NewTimer(100 * time.Millisecond)
		select {
		case <-wait.Done():
			timer.Stop()
			return nil, wait.Err()
		case <-timer.C:
		}
	}
}
func (c *DownloadCache) Location(ctx context.Context) (string, error) {
	lease, err := c.lease(ctx, false)
	if err != nil {
		return "", err
	}
	defer lease.Close()
	return lease.root.Name(), nil
}
func (c *DownloadCache) Acquire(ctx context.Context) (download.CacheLease, error) {
	return c.lease(ctx, false)
}

func (l *downloadCacheLease) acquireCopy(ctx context.Context) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	select {
	case l.copySlots <- struct{}{}:
		return func() { <-l.copySlots }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
func (l *downloadCacheLease) Restore(ctx context.Context, o download.Object, dst io.Writer) (bool, error) {
	release, err := l.acquireCopy(ctx)
	if err != nil {
		return false, err
	}
	defer release()
	key, err := o.Key()
	if err != nil {
		return false, err
	}
	before, err := l.root.Lstat(key)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !before.Mode().IsRegular() || before.Size() != o.Size {
		return false, nil
	}
	f, err := l.root.Open(key)
	if err != nil {
		return false, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !os.SameFile(before, info) {
		return false, errors.New("缓存副本已改变")
	}
	// Invalid data is a miss; leave caller's private output untouched until a
	// complete validation succeeds, then reverify while copying from the same FD.
	if err = o.Copy(ctx, io.Discard, f); err != nil {
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
		return false, nil
	}
	if _, err = f.Seek(0, io.SeekStart); err != nil {
		return false, err
	}
	if err = o.Copy(ctx, dst, f); err != nil {
		return false, err
	}
	after, err := f.Stat()
	if err != nil || !os.SameFile(before, after) || after.Size() != before.Size() || !after.ModTime().Equal(before.ModTime()) {
		return false, errors.New("缓存副本读取期间改变")
	}
	return true, nil
}
func (l *downloadCacheLease) Store(ctx context.Context, o download.Object, src io.Reader) error {
	release, err := l.acquireCopy(ctx)
	if err != nil {
		return err
	}
	defer release()
	key, err := o.Key()
	if err != nil {
		return err
	}
	name := "part-" + rand.Text()
	f, err := l.root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer l.root.Remove(name)
	defer f.Close()
	if err = o.Copy(ctx, f, src); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	return l.root.Rename(name, key)
}

func (c *DownloadCache) Inspect(ctx context.Context) (download.CacheInfo, error) {
	return c.inspect(ctx, false, "")
}
func (c *DownloadCache) Clear(ctx context.Context, expected string) (download.CacheInfo, error) {
	return c.inspect(ctx, true, expected)
}
func (c *DownloadCache) inspect(ctx context.Context, clear bool, expected string) (download.CacheInfo, error) {
	var result download.CacheInfo
	lease, err := c.lease(ctx, clear)
	if err != nil {
		return result, err
	}
	defer lease.Close()
	result.Directory = lease.root.Name()
	if clear && result.Directory != expected {
		return result, errors.New("缓存目录已改变，请重新检查后再清理")
	}
	dir, err := lease.root.Open(".")
	if err != nil {
		return result, err
	}
	defer dir.Close()
	seen := 0
	for {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		entries, err := dir.ReadDir(128)
		if err != nil && err != io.EOF {
			return result, err
		}
		for _, entry := range entries {
			seen++
			if seen > 100000 {
				return result, errors.New("缓存条目超过扫描上限，未继续处理")
			}
			if err := ctx.Err(); err != nil {
				return result, err
			}
			name := entry.Name()
			if name == "owner" || name == "cache.lock" {
				continue
			}
			if !cacheObjectName.MatchString(name) && !cacheTempName.MatchString(name) {
				result.Skipped++
				continue
			}
			info, err := lease.root.Lstat(name)
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			if err != nil {
				return result, err
			}
			if !info.Mode().IsRegular() {
				result.Skipped++
				continue
			}
			if clear {
				if err := lease.root.Remove(name); err != nil {
					return result, err
				}
			}
			if cacheTempName.MatchString(name) {
				result.TemporaryFiles++
				result.TemporaryBytes += info.Size()
			} else {
				result.Files++
				result.Bytes += info.Size()
			}
		}
		if err == io.EOF {
			break
		}
	}
	return result, nil
}

// Ensure no raw local archive/manual-supply cache is included in this namespace.
var _ download.ObjectCache = (*DownloadCache)(nil)
