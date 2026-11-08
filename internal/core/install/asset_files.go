package install

import (
	"context"
	"crypto/sha1"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"

	"mcl/internal/core/meta"
)

// Open real components one by one. os.Root confines symlinks, but a link to a
// different directory inside that root must not redirect managed writes either.
func realDirectory(root *os.Root, name string, create bool) (*os.Root, error) {
	if name != "." && !meta.ValidPath(name) {
		return nil, errors.New("invalid directory path")
	}
	current, err := root.OpenRoot(".")
	if err != nil {
		return nil, err
	}
	if name == "." {
		return current, nil
	}
	for _, part := range strings.Split(name, "/") {
		if create {
			if err := current.Mkdir(part, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
				current.Close()
				return nil, err
			}
		}
		info, err := current.Lstat(part)
		if err != nil {
			current.Close()
			return nil, err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			current.Close()
			return nil, errors.New("directory is a link or is not a directory")
		}
		next, err := current.OpenRoot(part)
		if err == nil {
			opened, e := next.Stat(".")
			if e != nil || !os.SameFile(info, opened) {
				next.Close()
				err = errors.New("directory changed while opening")
			}
		}
		current.Close()
		if err != nil {
			return nil, err
		}
		current = next
	}
	return current, nil
}

func openAssetFile(root *os.Root, name string, size int64) (*os.File, os.FileInfo, error) {
	if !meta.ValidPath(name) {
		return nil, nil, errors.New("unsafe asset file path")
	}
	parent, err := realDirectory(root, path.Dir(name), false)
	if err != nil {
		return nil, nil, err
	}
	defer parent.Close()
	info, err := parent.Lstat(path.Base(name))
	if err != nil {
		return nil, nil, err
	}
	if !info.Mode().IsRegular() || info.Size() != size {
		return nil, nil, errors.New("asset is not a regular file of the expected size")
	}
	f, err := parent.Open(path.Base(name))
	if err != nil {
		return nil, nil, err
	}
	opened, err := f.Stat()
	if err != nil || !os.SameFile(info, opened) {
		f.Close()
		return nil, nil, errors.New("asset file identity changed")
	}
	return f, opened, nil
}

func sameAssetFile(root *os.Root, name string, f *os.File, before os.FileInfo) error {
	after, err := f.Stat()
	if err != nil {
		return err
	}
	current, err := root.Lstat(name)
	if err != nil {
		return err
	}
	if !current.Mode().IsRegular() || !os.SameFile(before, current) || !os.SameFile(before, after) || after.Size() != before.Size() || !after.ModTime().Equal(before.ModTime()) {
		return errors.New("asset changed while reading")
	}
	return nil
}

type assetReader struct {
	ctx context.Context
	r   io.Reader
}

func (r assetReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(p)
}

// Hash exactly the bytes read through this descriptor, including copies. Never
// trust a previous verification of a pathname that can subsequently be replaced.
func streamAsset(ctx context.Context, f *os.File, dest io.Writer, size int64, digest string) error {
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	hash := sha1.New()
	n, err := io.Copy(io.MultiWriter(dest, hash), io.LimitReader(assetReader{ctx, f}, size+1))
	if err != nil {
		return err
	}
	if n != size || !strings.EqualFold(fmt.Sprintf("%x", hash.Sum(nil)), digest) {
		return errors.New("asset size or SHA-1 mismatch")
	}
	return ctx.Err()
}

func verifyAsset(ctx context.Context, root *os.Root, name string, size int64, digest string) error {
	f, info, err := openAssetFile(root, name, size)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := streamAsset(ctx, f, io.Discard, size, digest); err != nil {
		return err
	}
	return sameAssetFile(root, name, f, info)
}

func copyAsset(ctx context.Context, source *os.Root, name string, target *os.Root, dest string, size int64, digest string) (err error) {
	f, before, err := openAssetFile(source, name, size)
	if err != nil {
		return err
	}
	defer f.Close()
	out, err := target.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer func() {
		out.Close()
		if err != nil {
			target.Remove(dest)
		}
	}()
	if err := streamAsset(ctx, f, out, size, digest); err != nil {
		return err
	}
	if err := sameAssetFile(source, name, f, before); err != nil {
		return err
	}
	if err := out.Sync(); err != nil {
		return err
	}
	return out.Close()
}
