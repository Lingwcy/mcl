package download

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"time"

	"mcl/internal/fileio"
)

const partsDirectory = "download-parts-v1"

var ErrCheckpoint = errors.New("下载检查点未能持久化")

type partialState struct {
	Schema                            int
	Key, Path, Source, ETag, Modified string
	Object                            Object
	Offset                            int64
	PrefixSHA256                      string
	UpdatedAt                         time.Time
}

func shaText(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

// Coordination is provided by the storage/platform adapter in production.
// Core fixtures can omit it; the native app injects it before any work starts.
type Coordination interface {
	Target(context.Context, string) (io.Closer, error)
	Parts(context.Context, bool) (io.Closer, error)
}

func (c *Client) targetLease(ctx context.Context, name string) (func(), error) {
	if c.Coordination == nil {
		return func() {}, ctx.Err()
	}
	lease, err := c.Coordination.Target(ctx, name)
	if err != nil {
		return nil, err
	}
	return func() { lease.Close() }, nil
}
func realChild(root *os.Root, name string) (*os.Root, error) {
	info, err := root.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("下载目录不是普通目录")
	}
	child, err := root.OpenRoot(name)
	if err != nil {
		return nil, err
	}
	actual, err := child.Stat(".")
	after, e := root.Lstat(name)
	if err != nil || e != nil || !os.SameFile(info, actual) || !os.SameFile(info, after) {
		child.Close()
		return nil, errors.New("下载目录读取时发生变化")
	}
	return child, nil
}
func openParts(root *os.Root) (*os.Root, error) {
	if err := root.Mkdir(partsDirectory, 0700); err != nil && !errors.Is(err, os.ErrExist) {
		return nil, err
	}
	return realChild(root, partsDirectory)
}
func readPartial(root *os.Root, name string, state *partialState) error {
	info, err := root.Lstat(name)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() > 8192 {
		return errors.New("下载分片标记无效")
	}
	f, err := root.Open(name)
	if err != nil {
		return err
	}
	defer f.Close()
	actual, err := f.Stat()
	if err != nil || !os.SameFile(info, actual) {
		return errors.New("下载分片标记被替换")
	}
	data, err := io.ReadAll(io.LimitReader(f, 8193))
	if err != nil {
		return err
	}
	if len(data) > 8192 {
		return errors.New("下载分片标记越界")
	}
	after, err := f.Stat()
	if err != nil {
		return err
	}
	if after.Size() != info.Size() || !after.ModTime().Equal(info.ModTime()) || int64(len(data)) != info.Size() {
		return errors.New("下载分片标记读取时发生变化")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(state); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return errors.New("下载分片标记包含多余内容")
	}
	return nil
}
func savePartial(ctx context.Context, root *os.Root, name string, state partialState) error {
	state.UpdatedAt = time.Now().UTC()
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return fileio.WriteAt(ctx, root, name, data, 0600)
}
func prefix(ctx context.Context, file *os.File, size int64) (string, error) {
	if _, err := file.Seek(0, 0); err != nil {
		return "", err
	}
	hash := sha256.New()
	n, err := io.Copy(hash, &contextReader{ctx, io.LimitReader(file, size)})
	if err != nil {
		return "", err
	}
	if n != size {
		return "", errors.New("下载分片长度发生变化")
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

type partial struct {
	root         *os.Root
	file         *os.File
	state        partialState
	marker, blob string
	offset       int64
	reset        bool
	lease        io.Closer
}

func openPartial(ctx context.Context, root *os.Root, f File, address string, coordination Coordination) (*partial, error) {
	parts, err := openParts(root)
	if err != nil {
		return nil, err
	}
	var lease io.Closer
	if coordination != nil {
		lease, err = coordination.Parts(ctx, false)
		if err != nil {
			parts.Close()
			return nil, err
		}
	}
	key := FileKey(f)
	p := &partial{lease: lease, root: parts, marker: key + ".json", blob: key + ".part", state: partialState{Schema: 1, Key: key, Path: f.Path, Object: objectForFile(f)}}
	fail := func(err error) (*partial, error) { p.close(); return nil, err }
	err = readPartial(parts, p.marker, &p.state)
	if err == nil {
		if p.state.Schema != 1 || p.state.Key != key || p.state.Path != f.Path || p.state.Object != objectForFile(f) || p.state.Offset < 0 || p.state.Offset > f.Size || len(p.state.Source) > 64 || len(p.state.ETag) > 256 || len(p.state.Modified) > 64 || strings.ContainsAny(p.state.ETag+p.state.Modified, "\r\n\x00") {
			return fail(errors.New("下载分片身份不匹配，未接管"))
		}
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fail(err)
	}
	if errors.Is(err, os.ErrNotExist) {
		if _, e := parts.Lstat(p.blob); e == nil {
			return fail(errors.New("下载分片缺少所属标记，未接管"))
		} else if !errors.Is(e, os.ErrNotExist) {
			return fail(e)
		}
		if e := savePartial(ctx, parts, p.marker, p.state); e != nil {
			return fail(e)
		}
	}
	var before os.FileInfo
	if info, e := parts.Lstat(p.blob); e == nil {
		if !info.Mode().IsRegular() {
			return fail(errors.New("下载分片不是普通文件"))
		}
		before = info
	} else if !errors.Is(e, os.ErrNotExist) {
		return fail(e)
	}
	p.file, err = parts.OpenFile(p.blob, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return fail(err)
	}
	info, err := p.file.Stat()
	if err != nil {
		return fail(err)
	}
	if before != nil && !os.SameFile(before, info) {
		return fail(errors.New("下载分片打开时发生变化"))
	}
	p.offset = info.Size()
	p.reset = p.offset > f.Size || p.state.Offset > p.offset || p.state.Source != "" && p.state.Source != shaText(address)
	if p.state.Offset > 0 {
		decoded, e := hex.DecodeString(p.state.PrefixSHA256)
		if e != nil || len(decoded) != 32 {
			p.reset = true
		}
	}
	if !p.reset && p.state.Offset > 0 {
		actual, e := prefix(ctx, p.file, p.state.Offset)
		if e != nil {
			return fail(e)
		}
		p.reset = actual != p.state.PrefixSHA256
	}
	return p, nil
}
func (p *partial) close() {
	if p.file != nil {
		p.file.Close()
	}
	if p.root != nil {
		p.root.Close()
	}
	if p.lease != nil {
		p.lease.Close()
	}
}
func (p *partial) checkpoint(ctx context.Context) error {
	if err := p.file.Sync(); err != nil {
		return errors.Join(ErrCheckpoint, err)
	}
	info, err := p.file.Stat()
	if err != nil {
		return errors.Join(ErrCheckpoint, err)
	}
	if info.Size() > p.state.Object.Size {
		return errors.Join(ErrCheckpoint, errors.New("下载分片超过固定大小"))
	}
	hash, err := prefix(context.WithoutCancel(ctx), p.file, info.Size())
	if err != nil {
		return errors.Join(ErrCheckpoint, err)
	}
	p.state.Offset, p.state.PrefixSHA256 = info.Size(), hash
	return checkpointError(savePartial(context.WithoutCancel(ctx), p.root, p.marker, p.state))
}
func (p *partial) publish(ctx context.Context, root *os.Root, f File) (bool, time.Duration, error) {
	if err := Observe(ctx, eventFor(f, "verifying", f.Size)); err != nil {
		return false, 0, err
	}
	if _, err := p.file.Seek(0, 0); err != nil {
		return false, 0, err
	}
	before, err := p.file.Stat()
	if err != nil {
		return false, 0, err
	}
	if err := objectForFile(f).Copy(ctx, io.Discard, p.file); err != nil {
		if ctx.Err() != nil {
			return false, 0, ctx.Err()
		}
		if e := p.file.Truncate(0); e != nil {
			return false, 0, e
		}
		p.state.Offset = 0
		p.state.PrefixSHA256 = ""
		if e := savePartial(context.WithoutCancel(ctx), p.root, p.marker, p.state); e != nil {
			return false, 0, e
		}
		if errors.Is(err, ErrContentMismatch) {
			return true, 0, fmt.Errorf("下载摘要不符，已丢弃无效分片，可重试: %w", err)
		}
		return true, 0, errors.New("下载校验未通过，已丢弃无效分片，可重试")
	}
	after, err := p.file.Stat()
	if err != nil {
		return false, 0, err
	}
	current, err := p.root.Lstat(p.blob)
	if err != nil {
		return false, 0, err
	}
	if !current.Mode().IsRegular() || !os.SameFile(before, current) || !os.SameFile(before, after) || before.Size() != after.Size() || before.Size() != current.Size() || !before.ModTime().Equal(after.ModTime()) || !before.ModTime().Equal(current.ModTime()) {
		return false, 0, errors.New("下载分片在校验期间发生变化，未发布")
	}
	if err := p.file.Sync(); err != nil {
		return false, 0, err
	}
	if err := p.file.Close(); err != nil {
		return false, 0, err
	}
	if err := Observe(ctx, eventFor(f, "publishing", f.Size)); err != nil {
		return false, 0, err
	}
	if err := ctx.Err(); err != nil {
		return false, 0, err
	}
	directory, err := targetDirectory(root, path.Dir(f.Path), true)
	if err != nil {
		return false, 0, err
	}
	defer directory.Close()
	if err := fileio.ReplaceManagedFile(p.root, p.blob, directory, path.Base(f.Path)); err != nil {
		return false, 0, err
	}
	if err := fileio.SyncDirectory(p.root); err != nil {
		return false, 0, err
	}
	if err := fileio.SyncDirectory(directory); err != nil {
		return false, 0, err
	}
	if err := Verify(ctx, root, f); err != nil {
		return false, 0, err
	}
	if err := p.root.Remove(p.marker); err != nil {
		return false, 0, err
	}
	return false, 0, nil
}

func checkpointError(err error) error {
	if err != nil {
		return errors.Join(ErrCheckpoint, err)
	}
	return nil
}
