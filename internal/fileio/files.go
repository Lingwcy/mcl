// Package fileio provides bounded reads and transactional writes for application
// documents. Callers own path validation and serialize writes to the same file.
package fileio

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

func Read(ctx context.Context, path string, limit int64) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", filepath.Base(path), err)
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("%s exceeds %d bytes", filepath.Base(path), limit)
	}
	return data, ctx.Err()
}

// Write replaces a document only after its complete contents have been synced.
// The temporary file is in the same directory so replacement stays on one volume.
func Write(ctx context.Context, path string, data []byte, mode fs.FileMode) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create document directory: %w", err)
	}
	f, err := os.CreateTemp(dir, ".mcl-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary document: %w", err)
	}
	defer os.Remove(f.Name())
	defer f.Close()
	return commit(ctx, f, data, mode, func() error { return os.Rename(f.Name(), path) })
}

// WriteAt confines both the temporary document and replacement to an open root,
// including when another process replaces a path component with a symlink.
func WriteAt(ctx context.Context, root *os.Root, name string, data []byte, mode fs.FileMode) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := root.MkdirAll(filepath.Dir(name), 0o700); err != nil {
		return err
	}
	temporary := filepath.Join(filepath.Dir(name), ".mcl-"+rand.Text()+".tmp")
	f, err := root.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	defer root.Remove(temporary)
	defer f.Close()
	return commit(ctx, f, data, mode, func() error { return root.Rename(temporary, name) })
}

func commit(ctx context.Context, f *os.File, data []byte, mode fs.FileMode, replace func() error) error {
	if err := f.Chmod(mode); err != nil {
		return fmt.Errorf("set document permissions: %w", err)
	}
	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("write document: %w", err)
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("sync document: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close document: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := replace(); err != nil {
		return fmt.Errorf("replace document: %w", err)
	}
	return nil
}
