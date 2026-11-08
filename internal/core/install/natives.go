package install

import (
	"archive/zip"
	"context"
	"crypto/rand"
	"crypto/sha1"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"

	assetarchive "mcl/internal/core/archive"
	"mcl/internal/core/meta"
	"mcl/internal/fileio"
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
	parentRoot, err := realDirectory(root, parent, true)
	if err != nil {
		return "", err
	}
	defer parentRoot.Close()
	id := rand.Text()
	temporary, final := path.Join(parent, ".prepare-"+id), path.Join(parent, "ready-"+id)
	if err := parentRoot.Mkdir(path.Base(temporary), 0o700); err != nil {
		return "", err
	}
	stage, err := realDirectory(parentRoot, path.Base(temporary), false)
	if err != nil {
		return "", err
	}
	defer stage.Close()
	descriptor, err := json.Marshal(archives)
	if err != nil {
		return "", err
	}
	if err := writeAssetOwner(ctx, stage, assetOwner{1, "native", parent, id, fmt.Sprintf("%x", sha1.Sum(descriptor))}); err != nil {
		return "", err
	}
	defer func() {
		if err != nil {
			stage.Close()
			parentRoot.RemoveAll(path.Base(temporary))
		}
	}()
	claimed := make(map[string]bool)
	var expanded uint64
	var entries int
	for _, archive := range archives {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if archive.File.Size > 128<<20 {
			return "", errors.New("native archive exceeds 128 MiB")
		}
		if err := extractArchive(ctx, root, stage, archive, claimed, &expanded, &entries); err != nil {
			return "", err
		}
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	stage.Close()
	if err := fileio.PublishDirectory(parentRoot, path.Base(temporary), path.Base(final)); err != nil {
		return "", err
	}
	return final, nil
}

func extractArchive(ctx context.Context, root, stage *os.Root, archive NativeArchive, claimed map[string]bool, expanded *uint64, entries *int) error {
	if err := validateNativeExclusions(archive.Exclude); err != nil {
		return err
	}
	f, identity, err := openAssetFile(root, archive.File.Path, archive.File.Size)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := streamAsset(ctx, f, io.Discard, archive.File.Size, archive.File.SHA1); err != nil {
		return err
	}
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
		if !meta.ValidPath(name) || !assetarchive.SafePath(name) {
			return fmt.Errorf("unsafe native archive entry %q", entry.Name)
		}
		if entry.Mode()&os.ModeType != 0 && !entry.FileInfo().IsDir() {
			return errors.New("native archive contains a link or special file")
		}
		if entry.FileInfo().IsDir() {
			continue
		}
		excluded := name == "META-INF" || strings.HasPrefix(name, "META-INF/")
		for _, prefix := range archive.Exclude {
			excluded = excluded || strings.HasPrefix(name, prefix)
		}
		if excluded {
			continue
		}
		key := assetarchive.Key(name)
		if claimed[key] {
			return fmt.Errorf("duplicate native entry %q", name)
		}
		claimed[key] = true
		if entry.UncompressedSize64 > 64<<20 || *expanded+entry.UncompressedSize64 > 256<<20 {
			return errors.New("native archive expansion limit exceeded")
		}
		*expanded += entry.UncompressedSize64
		target, err := realDirectory(stage, path.Dir(name), true)
		if err != nil {
			return err
		}
		err = extractEntry(ctx, target, path.Base(name), entry)
		target.Close()
		if err != nil {
			return err
		}
	}
	if err := streamAsset(ctx, f, io.Discard, archive.File.Size, archive.File.SHA1); err != nil {
		return err
	}
	return sameAssetFile(root, archive.File.Path, f, identity)
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
