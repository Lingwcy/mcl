package java

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"os"
	"path"
	"strings"

	"mcl/internal/core/archive"
)

// Java archives need executable modes and internal symbolic links on Unix.
// Links are deferred until all regular files have been written into os.Root.
func extractPackage(ctx context.Context, file *os.File, directory string, zip bool) error {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return err
	}
	defer root.Close()
	if zip {
		info, err := file.Stat()
		if err != nil {
			return err
		}
		z, err := archive.Open(file, info.Size())
		if err != nil {
			return err
		}
		var mappings []archive.Mapping
		for _, entry := range z.Entries() {
			if !entry.Directory {
				mappings = append(mappings, archive.Mapping{Source: entry.Name, Target: entry.Name})
			}
		}
		return z.Extract(ctx, root, mappings)
	}
	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return errors.New("无法读取 Java 压缩包")
	}
	defer gzipReader.Close()
	reader := tar.NewReader(gzipReader)
	type link struct{ name, target string }
	var links []link
	seen := make(map[string]bool)
	var total int64
	for count := 0; ; count++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		if count >= archive.MaxEntries {
			return errors.New("Java 压缩包文件数量过多")
		}
		name := strings.TrimSuffix(header.Name, "/")
		if !archive.SafePath(name) || seen[archive.Key(name)] {
			return errors.New("Java 压缩包包含不安全或重复的路径")
		}
		seen[archive.Key(name)] = true
		if header.Size < 0 || header.Size > 512<<20 || total > (4<<30)-header.Size {
			return errors.New("Java 压缩包展开大小超过限制")
		}
		total += header.Size
		switch header.Typeflag {
		case tar.TypeDir:
			if err := root.MkdirAll(name, 0o700); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := root.MkdirAll(path.Dir(name), 0o700); err != nil {
				return err
			}
			if err := extractJavaFile(ctx, root, name, header, reader); err != nil {
				return err
			}
		case tar.TypeSymlink:
			if !safeLink(name, header.Linkname) {
				return errors.New("Java 压缩包包含越界链接")
			}
			links = append(links, link{name, header.Linkname})
		default:
			return errors.New("Java 压缩包包含不支持的特殊文件")
		}
	}
	for _, link := range links {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := root.MkdirAll(path.Dir(link.name), 0o700); err != nil {
			return err
		}
		if err := root.Symlink(link.target, link.name); err != nil {
			return err
		}
	}
	for _, link := range links {
		// Resolve through os.Root to reject escaping chains and broken links.
		if _, err := root.Stat(link.name); err != nil {
			return errors.New("Java 压缩包中的链接无法安全解析")
		}
	}
	// Consume the gzip trailer so truncated data and checksum errors surface.
	trailing, err := io.Copy(io.Discard, io.LimitReader(gzipReader, (1<<20)+1))
	if err != nil {
		return err
	}
	if trailing > 1<<20 {
		return errors.New("Java 压缩包尾部数据过大")
	}
	return ctx.Err()
}

func extractJavaFile(ctx context.Context, root *os.Root, name string, header *tar.Header, reader io.Reader) error {
	mode := os.FileMode(0o600)
	if header.Mode&0o111 != 0 {
		mode = 0o700
	}
	writer, err := root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	defer writer.Close()
	buffer := make([]byte, 64<<10)
	var written int64
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		n, readErr := reader.Read(buffer)
		if n > 0 {
			if _, err := writer.Write(buffer[:n]); err != nil {
				return err
			}
			written += int64(n)
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return readErr
		}
	}
	if written != header.Size {
		return errors.New("Java 解压文件大小不匹配")
	}
	if err := writer.Sync(); err != nil {
		return err
	}
	return writer.Close()
}
