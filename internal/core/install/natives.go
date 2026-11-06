package install

import (
	"archive/zip"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"

	"mcl/internal/core/download"
	"mcl/internal/core/meta"
)

// PrepareNatives publishes a fresh directory only after all legacy archives
// have passed hash, path, type, CRC and expansion checks. Modern natives remain
// in their JARs on the classpath and are loaded by LWJGL itself.
func PrepareNatives(ctx context.Context, root *os.Root, parent string, archives []NativeArchive) (directory string, err error) {
	if !meta.ValidPath(parent) {
		return "", errors.New("unsafe native parent directory")
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := root.MkdirAll(parent, 0o700); err != nil {
		return "", err
	}
	id := rand.Text()
	temporary, final := path.Join(parent, ".prepare-"+id), path.Join(parent, "ready-"+id)
	if err := root.Mkdir(temporary, 0o700); err != nil {
		return "", err
	}
	defer func() {
		if err != nil {
			root.RemoveAll(temporary)
		}
	}()
	claimed := make(map[string]bool)
	var expanded uint64
	var entries int
	for _, archive := range archives {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if err := download.Verify(ctx, root, archive.File); err != nil {
			return "", fmt.Errorf("verify native archive: %w", err)
		}
		if archive.File.Size > 128<<20 {
			return "", errors.New("native archive exceeds 128 MiB")
		}
		if err := extractArchive(ctx, root, temporary, archive, claimed, &expanded, &entries); err != nil {
			return "", err
		}
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := root.Rename(temporary, final); err != nil {
		return "", err
	}
	return final, nil
}

func extractArchive(ctx context.Context, root *os.Root, directory string, archive NativeArchive, claimed map[string]bool, expanded *uint64, entries *int) error {
	f, err := root.Open(archive.File.Path)
	if err != nil {
		return err
	}
	defer f.Close()
	z, err := zip.NewReader(f, archive.File.Size)
	if err != nil {
		return fmt.Errorf("open native archive: %w", err)
	}
	*entries += len(z.File)
	if *entries > 10_000 {
		return errors.New("too many native archive entries")
	}
	for _, entry := range z.File {
		if err := ctx.Err(); err != nil {
			return err
		}
		name := strings.TrimSuffix(entry.Name, "/")
		if !meta.ValidPath(name) {
			return fmt.Errorf("unsafe native archive entry %q", entry.Name)
		}
		if entry.Mode()&os.ModeType != 0 && !entry.FileInfo().IsDir() {
			return errors.New("native archive contains a link or special file")
		}
		if entry.FileInfo().IsDir() {
			continue
		}
		excluded := strings.HasPrefix(name, "META-INF/")
		for _, prefix := range archive.Exclude {
			excluded = excluded || strings.HasPrefix(name, prefix)
		}
		if excluded {
			continue
		}
		key := strings.ToLower(name)
		if claimed[key] {
			return fmt.Errorf("duplicate native entry %q", name)
		}
		claimed[key] = true
		if entry.UncompressedSize64 > 64<<20 || *expanded+entry.UncompressedSize64 > 256<<20 {
			return errors.New("native archive expansion limit exceeded")
		}
		*expanded += entry.UncompressedSize64
		target := path.Join(directory, name)
		if err := root.MkdirAll(path.Dir(target), 0o700); err != nil {
			return err
		}
		if err := extractEntry(ctx, root, target, entry); err != nil {
			return err
		}
	}
	return nil
}

func extractEntry(ctx context.Context, root *os.Root, target string, entry *zip.File) error {
	reader, err := entry.Open()
	if err != nil {
		return err
	}
	defer reader.Close()
	f, err := root.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	// A block loop makes cancellation effective even for a local ZIP bomb.
	buffer := make([]byte, 32<<10)
	var count uint64
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		n, readErr := reader.Read(buffer)
		count += uint64(n)
		if count > entry.UncompressedSize64 {
			return errors.New("native entry exceeds declared size")
		}
		if _, err := f.Write(buffer[:n]); err != nil {
			return err
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return readErr
		}
	}
	if count != entry.UncompressedSize64 {
		return errors.New("native entry size mismatch")
	}
	if err := f.Sync(); err != nil {
		return err
	}
	return f.Close()
}
