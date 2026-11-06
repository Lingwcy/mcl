// Package archive reads and extracts bounded ZIP content to an explicitly owned
// directory. It never extracts links or cleans unsafe paths into valid ones.
package archive

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

const (
	MaxEntries          = 50000
	MaxFileSize  uint64 = 512 << 20
	MaxTotalSize uint64 = 4 << 30
)

// SafePath accepts portable relative paths. In particular, it rejects Windows
// aliases and case/normalization collisions before a write can occur on macOS.
func SafePath(name string) bool {
	if name == "" || len(name) > 1024 || !utf8.ValidString(name) || strings.ContainsAny(name, "\\:\x00") || path.IsAbs(name) || path.Clean(name) != name {
		return false
	}
	parts := strings.Split(name, "/")
	if len(parts) > 64 {
		return false
	}
	for _, part := range parts {
		if part == "" || part == "." || part == ".." || len(part) > 255 || strings.HasSuffix(part, ".") || strings.HasSuffix(part, " ") || strings.ContainsAny(part, "*?\"<>|") || strings.IndexFunc(part, unicode.IsControl) >= 0 {
			return false
		}
		base := strings.ToUpper(strings.SplitN(part, ".", 2)[0])
		if base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" || (len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '0' && base[3] <= '9') {
			return false
		}
	}
	return true
}
func Key(name string) string { return strings.ToLower(norm.NFC.String(name)) }

type Entry struct {
	Name      string
	Size      uint64
	Directory bool
}
type Zip struct {
	files   map[string]*zip.File
	entries []Entry
}

func Open(reader io.ReaderAt, size int64) (*Zip, error) {
	if size <= 0 || size > 2<<30 {
		return nil, errors.New("压缩包大小超出限制")
	}
	r, err := zip.NewReader(reader, size)
	if err != nil {
		return nil, errors.New("无法读取 ZIP 压缩包")
	}
	if len(r.File) > MaxEntries {
		return nil, errors.New("压缩包文件数量过多")
	}
	z := &Zip{files: make(map[string]*zip.File)}
	seen := make(map[string]bool)
	nodes := make(map[string]bool)
	var total uint64
	for _, file := range r.File {
		directory := file.FileInfo().IsDir()
		name := file.Name
		if directory {
			name = strings.TrimSuffix(name, "/")
		}
		if !SafePath(name) {
			return nil, fmt.Errorf("压缩包路径不安全: %q", file.Name)
		}
		if file.Mode()&os.ModeSymlink != 0 || (!directory && !file.Mode().IsRegular()) {
			return nil, errors.New("压缩包包含链接或特殊文件")
		}
		key := Key(name)
		if seen[key] {
			return nil, errors.New("压缩包包含重复或大小写冲突路径")
		}
		seen[key] = true
		if oldDirectory, ok := nodes[key]; ok && !oldDirectory {
			return nil, errors.New("压缩包文件与目录路径冲突")
		}
		if _, ok := nodes[key]; ok && !directory {
			return nil, errors.New("压缩包文件与子目录冲突")
		}
		nodes[key] = directory
		for parent := path.Dir(name); parent != "."; parent = path.Dir(parent) {
			key := Key(parent)
			if dir, ok := nodes[key]; ok && !dir {
				return nil, errors.New("压缩包父路径不是目录")
			}
			nodes[key] = true
		}
		if file.UncompressedSize64 > MaxFileSize {
			return nil, errors.New("压缩包单文件过大")
		}
		if total > MaxTotalSize-file.UncompressedSize64 {
			return nil, errors.New("压缩包展开体积过大")
		}
		total += file.UncompressedSize64
		if file.UncompressedSize64 > 64<<20 && (file.CompressedSize64 == 0 || file.UncompressedSize64/file.CompressedSize64 > 1000) {
			return nil, errors.New("压缩包压缩比异常")
		}
		z.files[name] = file
		z.entries = append(z.entries, Entry{Name: name, Size: file.UncompressedSize64, Directory: directory})
	}
	return z, nil
}
func (z *Zip) Entries() []Entry     { return append([]Entry(nil), z.entries...) }
func (z *Zip) Has(name string) bool { file, ok := z.files[name]; return ok && !file.FileInfo().IsDir() }
func (z *Zip) Read(ctx context.Context, name string, limit int64) ([]byte, error) {
	f, ok := z.files[name]
	if !ok || f.FileInfo().IsDir() {
		return nil, fmt.Errorf("压缩包缺少 %s", name)
	}
	if limit <= 0 || f.UncompressedSize64 > uint64(limit) {
		return nil, errors.New("压缩包清单过大")
	}
	r, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer r.Close()
	data, err := io.ReadAll(io.LimitReader(&contextReader{ctx, r}, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit || uint64(len(data)) != f.UncompressedSize64 {
		return nil, errors.New("清单大小不匹配")
	}
	return data, ctx.Err()
}

type Mapping struct{ Source, Target string }

// Extract writes only the explicitly mapped files, into a fresh staging root.
// No existing file is replaced, even if a malicious mapping contains aliases.
func (z *Zip) Extract(ctx context.Context, root *os.Root, mapping []Mapping) error {
	if len(mapping) > MaxEntries {
		return errors.New("解压文件过多")
	}
	seen := make(map[string]bool)
	for _, entry := range mapping {
		if !SafePath(entry.Target) || seen[Key(entry.Target)] {
			return errors.New("解压目标无效或冲突")
		}
		seen[Key(entry.Target)] = true
		if _, ok := z.files[entry.Source]; !ok {
			return errors.New("解压源文件不存在")
		}
	}
	for _, entry := range mapping {
		if err := ctx.Err(); err != nil {
			return err
		}
		f := z.files[entry.Source]
		if f.FileInfo().IsDir() {
			continue
		}
		if err := root.MkdirAll(path.Dir(entry.Target), 0o700); err != nil {
			return err
		}
		if err := extractFile(ctx, root, entry.Target, f); err != nil {
			return fmt.Errorf("解压 %s: %w", entry.Target, err)
		}
	}
	return ctx.Err()
}
func extractFile(ctx context.Context, root *os.Root, name string, file *zip.File) error {
	r, err := file.Open()
	if err != nil {
		return err
	}
	defer r.Close()
	w, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	defer w.Close()
	n, err := io.Copy(w, io.LimitReader(&contextReader{ctx, r}, int64(file.UncompressedSize64)+1))
	if err != nil {
		return err
	}
	if uint64(n) != file.UncompressedSize64 {
		return errors.New("展开文件大小不匹配")
	}
	if err := w.Sync(); err != nil {
		return err
	}
	return w.Close()
}

type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r *contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(p)
}
