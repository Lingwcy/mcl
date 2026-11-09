package download

import (
	"context"
	"crypto/rand"
	"os"
	"path"

	"mcl/internal/fileio"
)

func objectForFile(f File) Object {
	return Object{SHA1: f.SHA1, SHA256: f.SHA256, SHA512: f.SHA512, Size: f.Size}
}
func restoreCachedFile(ctx context.Context, cache CacheLease, root *os.Root, f File) (bool, error) {
	directory, err := targetDirectory(root, path.Dir(f.Path), true)
	if err != nil {
		return false, err
	}
	defer directory.Close()
	name := ".mcl-cache-" + rand.Text()
	output, err := directory.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return false, err
	}
	defer directory.Remove(name)
	defer output.Close()
	found, err := cache.Restore(ctx, objectForFile(f), output)
	if err != nil || !found {
		return false, err
	}
	if err = output.Sync(); err != nil {
		return false, err
	}
	if err = output.Close(); err != nil {
		return false, err
	}
	if err = ctx.Err(); err != nil {
		return false, err
	}
	if err := fileio.ReplaceManagedFile(directory, name, directory, path.Base(f.Path)); err != nil {
		return false, err
	}
	return true, fileio.SyncDirectory(directory)
}
