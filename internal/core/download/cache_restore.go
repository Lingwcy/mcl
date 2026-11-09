package download

import (
	"context"
	"crypto/rand"
	"os"
	"path"
)

func objectForFile(f File) Object { return Object{SHA1: f.SHA1, SHA512: f.SHA512, Size: f.Size} }
func restoreCachedFile(ctx context.Context, cache CacheLease, root *os.Root, f File) (bool, error) {
	name := path.Join(path.Dir(f.Path), ".mcl-cache-"+rand.Text())
	output, err := root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return false, err
	}
	defer root.Remove(name)
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
	return true, root.Rename(name, f.Path)
}
