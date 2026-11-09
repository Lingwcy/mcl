package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"time"

	"mcl/internal/core/archive"
	"mcl/internal/core/meta"
	"mcl/internal/platform/instancelock"
)

type DownloadCoordination struct{ root string }

func NewDownloadCoordination(root string) *DownloadCoordination { return &DownloadCoordination{root} }
func (s *DownloadCoordination) Target(ctx context.Context, name string) (io.Closer, error) {
	if !meta.ValidPath(name) {
		return nil, errors.New("下载目标路径无效")
	}
	hash := sha256.Sum256([]byte(archive.Key(name)))
	key := hex.EncodeToString(hash[:])
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		lock, err := instancelock.AcquireDownload(s.root, key)
		if err == nil {
			return lock, nil
		}
		if !errors.Is(err, instancelock.ErrBusy) {
			return nil, err
		}
		timer := time.NewTimer(250 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}
func (s *DownloadCoordination) Parts(ctx context.Context, exclusive bool) (io.Closer, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(s.root)
	if err != nil {
		return nil, err
	}
	return instancelock.AcquireDownloadParts(root, exclusive)
}
