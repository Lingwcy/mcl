package storage

import (
	"context"
	"crypto/rand"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"mcl/internal/core/market"
	"mcl/internal/core/modpack"
)

func (s *ArchiveStore) CacheLocalArchive(ctx context.Context, name string) (market.File, error) {
	var result market.File
	ext := strings.ToLower(filepath.Ext(name))
	if !filepath.IsAbs(name) || (ext != ".zip" && ext != ".mrpack") {
		return result, errors.New("请选择 MRPack 或带清单的 ZIP 整合包")
	}
	before, err := os.Lstat(name)
	if err != nil {
		return result, err
	}
	if !before.Mode().IsRegular() || before.Size() <= 0 || before.Size() > modpack.MaxArchiveSize {
		return result, errors.New("请选择 2 GiB 以内的整合包文件")
	}
	file, err := os.Open(name)
	if err != nil {
		return result, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(before, opened) {
		return result, errors.New("所选整合包已改变")
	}
	if err := os.MkdirAll(filepath.Join(s.dir, "cache", "market"), 0o700); err != nil {
		return result, err
	}
	cache, err := os.OpenRoot(filepath.Join(s.dir, "cache", "market"))
	if err != nil {
		return result, err
	}
	defer cache.Close()
	temp := ".mcl-local-pack-" + rand.Text()
	output, err := cache.OpenFile(temp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return result, err
	}
	defer cache.Remove(temp)
	defer output.Close()
	hash := sha1.New()
	n, err := io.CopyBuffer(io.MultiWriter(output, hash), io.LimitReader(&packSupplyReader{ctx: ctx, reader: file}, before.Size()+1), make([]byte, 64<<10))
	if err != nil {
		return result, err
	}
	after, err := file.Stat()
	current, pathErr := os.Lstat(name)
	if err != nil || pathErr != nil || n != before.Size() || !os.SameFile(before, after) || !os.SameFile(before, current) || after.Size() != before.Size() || current.Size() != before.Size() || !after.ModTime().Equal(before.ModTime()) || !current.ModTime().Equal(before.ModTime()) {
		return result, errors.New("整合包读取期间发生改变")
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if err := output.Sync(); err != nil {
		return result, err
	}
	if err := output.Close(); err != nil {
		return result, err
	}
	digest := hex.EncodeToString(hash.Sum(nil))
	if err := cache.Rename(temp, digest); err != nil {
		return result, err
	}
	filename := filepath.Base(name)
	if !market.SafeFilename(filename) {
		filename = "modpack" + ext
	}
	result = market.File{Filename: filename, SHA1: digest, Size: n}
	// Re-open through the ordinary bounded ZIP/manifest reader before the
	// caller sees the file as an import candidate.
	if _, err := s.InspectPack(ctx, result); err != nil {
		return market.File{}, err
	}
	return result, nil
}
