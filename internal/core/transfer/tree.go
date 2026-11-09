// Package transfer plans verified copies without following links or deleting sources.
package transfer

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"slices"
	"strings"
	"time"

	"mcl/internal/core/archive"
	"mcl/internal/fileio"
)

const MaxEntries = 100000
const MaxBytes int64 = 128 << 30
const MaxFileBytes int64 = 16 << 30
const OwnerFile = ".mcl-directory-owner.json"

type File struct {
	Path       string
	Size       int64
	SHA256     string
	ModTime    int64
	Executable bool
}
type Tree struct {
	Roots, Directories []string
	Files              []File
	Bytes              int64
	Skipped            []string
}
type Progress struct {
	Stage             string
	Files, TotalFiles int
	Bytes, TotalBytes int64
}

var ImportGroups = []string{"saves", "mods", "config", "resourcepacks", "shaderpacks", "screenshots", "texturepacks", "options.txt", "optionsof.txt", "servers.dat"}

func (t Tree) Fingerprint() string {
	data, _ := json.Marshal(t)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
func OpenDirectory(root *os.Root, name string) (*os.Root, error) {
	if name != "." && !archive.SafePath(name) {
		return nil, errors.New("复制目录路径不安全")
	}
	current, err := root.OpenRoot(".")
	if err != nil {
		return nil, err
	}
	if name == "." {
		return current, nil
	}
	for part := range strings.SplitSeq(name, "/") {
		before, err := current.Lstat(part)
		if err != nil {
			current.Close()
			return nil, err
		}
		if !before.IsDir() || before.Mode()&os.ModeSymlink != 0 {
			current.Close()
			return nil, errors.New("复制目录包含链接或特殊文件")
		}
		next, err := current.OpenRoot(part)
		if err != nil {
			current.Close()
			return nil, err
		}
		actual, e := next.Stat(".")
		after, aErr := current.Lstat(part)
		current.Close()
		if e != nil || aErr != nil || !after.IsDir() || !os.SameFile(before, actual) || !os.SameFile(before, after) {
			next.Close()
			return nil, errors.New("复制目录在打开时发生变化")
		}
		current = next
	}
	return current, nil
}

// MakeDirectories opens and checks every component before descending; a root-
// confined symlink is still not an approved destination for copy operations.
func MakeDirectories(root *os.Root, name string) error {
	if name != "." && !archive.SafePath(name) {
		return errors.New("目标目录路径不安全")
	}
	current, err := root.OpenRoot(".")
	if err != nil {
		return err
	}
	defer func() { current.Close() }()
	if name == "." {
		return nil
	}
	for part := range strings.SplitSeq(name, "/") {
		if err := current.Mkdir(part, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
			return err
		}
		next, err := OpenDirectory(current, part)
		if err != nil {
			return err
		}
		current.Close()
		current = next
	}
	return nil
}
func Hash(ctx context.Context, root *os.Root, name string) (File, error) {
	var value File
	if !archive.SafePath(name) {
		return value, errors.New("文件路径不安全")
	}
	directory, err := OpenDirectory(root, path.Dir(name))
	if err != nil {
		return value, err
	}
	defer directory.Close()
	before, err := directory.Lstat(path.Base(name))
	if err != nil {
		return value, err
	}
	if !before.Mode().IsRegular() || before.Size() < 0 || before.Size() > MaxFileBytes {
		return value, errors.New("复制文件不是普通文件或超过16GiB")
	}
	file, err := directory.Open(path.Base(name))
	if err != nil {
		return value, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(before, opened) {
		return value, errors.New("打开时文件身份改变")
	}
	hash := sha256.New()
	n, err := io.Copy(hash, &contextReader{ctx, io.LimitReader(file, before.Size()+1)})
	if err != nil {
		return value, err
	}
	after, err := file.Stat()
	if err != nil {
		return value, err
	}
	current, err := directory.Lstat(path.Base(name))
	if err != nil {
		return value, err
	}
	if n != before.Size() || !current.Mode().IsRegular() || !os.SameFile(before, current) || !os.SameFile(before, after) || current.Size() != before.Size() || after.Size() != before.Size() || !current.ModTime().Equal(before.ModTime()) || !after.ModTime().Equal(before.ModTime()) {
		return value, errors.New("读取期间源文件发生变化")
	}
	return File{Path: name, Size: n, SHA256: hex.EncodeToString(hash.Sum(nil)), ModTime: before.ModTime().UnixNano(), Executable: before.Mode()&0o100 != 0}, ctx.Err()
}
func Scan(ctx context.Context, root *os.Root, roots []string, importing bool, report func(Progress)) (Tree, error) {
	result := Tree{Roots: slices.Clone(roots)}
	seen := map[string]string{}
	nodes := 0
	metadataBytes := 0
	var walk func(string, int) error
	walk = func(name string, depth int) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		nodes++
		metadataBytes += len(name) + 128
		if metadataBytes > 8<<20 {
			return errors.New("复制清单路径与元信息超过8MiB预算")
		}
		if nodes > MaxEntries || depth > 64 {
			return errors.New("复制计划超过10万项或64层")
		}
		if !archive.SafePath(name) {
			return errors.New("文件名不适合安全跨平台复制")
		}
		key := archive.Key(name)
		if old, ok := seen[key]; ok && old != name {
			return errors.New("文件名存在大小写或Unicode别名")
		}
		seen[key] = name
		if path.Base(name) == OwnerFile && strings.Count(name, "/") == 1 && !importing {
			return nil
		}
		if importing && !allowedImportFile(name) {
			if len(result.Skipped) < 512 {
				result.Skipped = append(result.Skipped, name)
			}
			return nil
		}
		parent, err := OpenDirectory(root, path.Dir(name))
		if err != nil {
			return err
		}
		info, err := parent.Lstat(path.Base(name))
		parent.Close()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() && !info.Mode().IsRegular() {
			return fmt.Errorf("拒绝链接或特殊文件: %s", name)
		}
		if info.IsDir() {
			result.Directories = append(result.Directories, name)
			directory, err := OpenDirectory(root, name)
			if err != nil {
				return err
			}
			defer directory.Close()
			f, err := directory.Open(".")
			if err != nil {
				return err
			}
			defer f.Close()
			entries, err := f.ReadDir(MaxEntries - nodes + 1)
			if err != nil && !errors.Is(err, io.EOF) {
				return err
			}
			if len(entries) > MaxEntries-nodes {
				return errors.New("目录项超过复制计划边界")
			}
			slices.SortFunc(entries, func(a, b os.DirEntry) int { return strings.Compare(a.Name(), b.Name()) })
			for _, entry := range entries {
				if err := walk(path.Join(name, entry.Name()), depth+1); err != nil {
					return err
				}
			}
			return nil
		}
		value, err := Hash(ctx, root, name)
		if err != nil {
			return err
		}
		result.Bytes += value.Size
		if result.Bytes > MaxBytes {
			return errors.New("复制计划超过128GiB")
		}
		if importing {
			value.Executable = false
		}
		result.Files = append(result.Files, value)
		if report != nil {
			report(Progress{Stage: "检查文件", Files: len(result.Files), Bytes: result.Bytes})
		}
		return nil
	}
	rootKeys := map[string]bool{}
	for _, name := range roots {
		if rootKeys[archive.Key(name)] {
			return result, errors.New("复制分类重复或存在别名")
		}
		rootKeys[archive.Key(name)] = true
		if !archive.SafePath(name) || path.Base(name) != name {
			return result, errors.New("复制分类必须是受控顶层名称")
		}
		if importing && !slices.Contains(ImportGroups, name) {
			return result, errors.New("不允许复制这个游戏资料分类")
		}
		if _, err := root.Lstat(name); errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return result, err
		}
		if err := walk(name, 0); err != nil {
			return result, err
		}
	}
	slices.Sort(result.Directories)
	slices.Sort(result.Skipped)
	slices.SortFunc(result.Files, func(a, b File) int { return strings.Compare(a.Path, b.Path) })
	return result, ctx.Err()
}
func allowedImportFile(name string) bool {
	base := strings.ToLower(path.Base(name))
	if base == "session.lock" || base == "launcher_profiles.json" || base == "servers.dat_old" || strings.HasPrefix(base, ".mcl-") || strings.HasPrefix(base, ".env") || strings.HasPrefix(base, "account") || strings.HasPrefix(base, "credential") || strings.HasPrefix(base, "auth") || strings.Contains(base, "access_token") || strings.Contains(base, "refresh_token") {
		return false
	}
	switch strings.ToLower(path.Ext(base)) {
	case ".key", ".pem", ".p12", ".pfx", ".jks", ".sh", ".bat", ".cmd", ".ps1", ".exe", ".dll", ".dylib", ".so", ".py", ".js", ".lua", ".groovy", ".zs":
		return false
	case ".jar":
		return strings.HasPrefix(name, "mods/")
	}
	return true
}
func Verify(ctx context.Context, root *os.Root, expected Tree, importing bool) error {
	actual, err := Scan(ctx, root, expected.Roots, importing, nil)
	if err != nil {
		return err
	}
	if actual.Fingerprint() != expected.Fingerprint() {
		return errors.New("文件清单、目录或内容发生变化，请重新预览")
	}
	return nil
}
func VerifyCopied(ctx context.Context, root *os.Root, expected Tree) error {
	actual, err := Scan(ctx, root, expected.Roots, false, nil)
	if err != nil {
		return err
	}
	expected.Skipped = nil
	for index := range actual.Files {
		if index < len(expected.Files) && actual.Files[index].Path == expected.Files[index].Path {
			actual.Files[index].ModTime = expected.Files[index].ModTime
			actual.Files[index].Executable = expected.Files[index].Executable
		}
	}
	if actual.Fingerprint() != expected.Fingerprint() {
		return errors.New("目标副本与固定清单不一致")
	}
	return nil
}
func Copy(ctx context.Context, source, target *os.Root, expected Tree, report func(Progress)) error {
	for _, directory := range expected.Directories {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := MakeDirectories(target, directory); err != nil {
			return err
		}
	}
	progress := Progress{Stage: "复制并校验", TotalFiles: len(expected.Files), TotalBytes: expected.Bytes}
	for _, file := range expected.Files {
		if err := ctx.Err(); err != nil {
			return err
		}
		existing, err := Hash(ctx, target, file.Path)
		if err == nil {
			existing.ModTime = file.ModTime
			existing.Executable = file.Executable
			if existing != file {
				return errors.New("目标出现不匹配文件，未覆盖")
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		} else {
			if err := copyFile(ctx, source, target, file); err != nil {
				return err
			}
		}
		progress.Files++
		progress.Bytes += file.Size
		if report != nil {
			report(progress)
		}
	}
	for index := len(expected.Directories) - 1; index >= 0; index-- {
		dir, err := OpenDirectory(target, expected.Directories[index])
		if err != nil {
			return err
		}
		err = fileio.SyncDirectory(dir)
		dir.Close()
		if err != nil {
			return err
		}
	}
	if err := fileio.SyncDirectory(target); err != nil {
		return err
	}
	return ctx.Err()
}
func copyFile(ctx context.Context, source, target *os.Root, expected File) error {
	original, err := Hash(ctx, source, expected.Path)
	if err != nil {
		return err
	}
	original.Executable = expected.Executable
	if original != expected {
		return errors.New("源文件与预览摘要不符")
	}
	src, err := OpenDirectory(source, path.Dir(expected.Path))
	if err != nil {
		return err
	}
	defer src.Close()
	input, err := src.Open(path.Base(expected.Path))
	if err != nil {
		return err
	}
	defer input.Close()
	if err := MakeDirectories(target, path.Dir(expected.Path)); err != nil {
		return err
	}
	dst, err := OpenDirectory(target, path.Dir(expected.Path))
	if err != nil {
		return err
	}
	defer dst.Close()
	temporary := ".mcl-copy-" + rand.Text()
	output, err := dst.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer dst.Remove(temporary)
	hash := sha256.New()
	n, copyErr := io.Copy(io.MultiWriter(output, hash), &contextReader{ctx, io.LimitReader(input, expected.Size+1)})
	mode := os.FileMode(0o600)
	if expected.Executable {
		mode |= 0o100
	}
	metaErr := output.Chmod(mode)
	if metaErr == nil {
		metaErr = fileio.SetFileTimes(output, time.Now(), time.Unix(0, expected.ModTime))
	}
	syncErr := output.Sync()
	closeErr := output.Close()
	if err := errors.Join(copyErr, metaErr, syncErr, closeErr); err != nil {
		return err
	}
	if n != expected.Size || hex.EncodeToString(hash.Sum(nil)) != expected.SHA256 {
		return errors.New("复制期间文件摘要改变")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return fileio.MoveNoReplace(dst, temporary, dst, path.Base(expected.Path))
}

type contextReader struct {
	ctx context.Context
	io.Reader
}

func (r *contextReader) Read(b []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.Reader.Read(b)
}
