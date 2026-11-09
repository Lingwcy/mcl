package discovery

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"

	"mcl/internal/core/meta"
)

// Every directory component is checked before and after opening. Even links
// pointing elsewhere inside the chosen source are not treated as instance data.
func openDirectory(root *os.Root, name string) (*os.Root, error) {
	if name != "." && !meta.ValidPath(name) {
		return nil, errors.New("目录路径不安全")
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
			return nil, errors.New("目录是链接或不是普通目录")
		}
		next, err := current.OpenRoot(part)
		if err != nil {
			current.Close()
			return nil, err
		}
		opened, openErr := next.Stat(".")
		after, pathErr := current.Lstat(part)
		current.Close()
		if openErr != nil || pathErr != nil || !after.IsDir() || !os.SameFile(before, opened) || !os.SameFile(before, after) {
			next.Close()
			return nil, errors.New("扫描期间目录身份改变")
		}
		current = next
	}
	return current, nil
}
func readDocument(ctx context.Context, root *os.Root, name string, limit int64, consumed *int64) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	before, err := root.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !before.Mode().IsRegular() || before.Size() <= 0 || before.Size() > limit {
		return nil, errors.New("版本文档不是普通文件或超过读取边界")
	}
	file, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(before, opened) {
		return nil, errors.New("版本文档已被替换")
	}
	data, err := io.ReadAll(&cancelReader{ctx: ctx, Reader: io.LimitReader(file, limit+1), consumed: consumed})
	if err != nil {
		return nil, err
	}
	after, err := file.Stat()
	if err != nil {
		return nil, err
	}
	current, err := root.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !current.Mode().IsRegular() || int64(len(data)) != before.Size() || len(data) > int(limit) || !os.SameFile(before, current) || !os.SameFile(before, after) || !before.ModTime().Equal(after.ModTime()) || !before.ModTime().Equal(current.ModTime()) {
		return nil, errors.New("扫描期间版本文档发生变化")
	}
	return data, ctx.Err()
}

type cancelReader struct {
	ctx context.Context
	io.Reader
	consumed *int64
}

func (r *cancelReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	n, err := r.Reader.Read(p)
	if r.consumed != nil {
		*r.consumed += int64(n)
	}
	return n, err
}
