package storage

import (
	"context"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"mcl/internal/app"
	"mcl/internal/core/download"
	"mcl/internal/core/modpack"
)

func (s *ArchiveStore) CachedManualFiles(ctx context.Context, files []modpack.File) ([]string, error) {
	return cachedManualFiles(ctx, s.dir, files)
}

func cachedManualFiles(ctx context.Context, dir string, files []modpack.File) ([]string, error) {
	if len(files) == 0 {
		return nil, ctx.Err()
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	var supplied []string
	for _, file := range files {
		if len(file.URLs) > 0 || file.Size <= 0 || file.SHA512 != "" {
			continue
		}
		// CurseForge supplies SHA-1 for manually obtained files. MRPack cache
		// reuse goes through the downloader's dual-hash verification instead.
		err := verifyLocalAsset(ctx, root, "cache/market/"+file.SHA1, file.SHA1, file.Size)
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if err == nil {
			supplied = append(supplied, file.Path)
		}
	}
	return supplied, ctx.Err()
}

func (s *ArchiveStore) SupplyPackFiles(ctx context.Context, files []modpack.File, paths []string) (app.LocalFileSupply, error) {
	return supplyLocalFiles(ctx, s.dir, files, paths)
}

func supplyLocalFiles(ctx context.Context, dir string, files []modpack.File, paths []string) (app.LocalFileSupply, error) {
	result := app.LocalFileSupply{}
	if len(paths) == 0 {
		return result, ctx.Err()
	}
	if len(paths) > 256 {
		return result, errors.New("一次最多选择 256 个文件")
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return result, err
	}
	defer root.Close()
	if err := root.MkdirAll("cache/market", 0o700); err != nil {
		return result, err
	}
	cache, err := root.OpenRoot("cache/market")
	if err != nil {
		return result, err
	}
	defer cache.Close()
	bySize := make(map[int64][]modpack.File)
	for _, file := range files {
		if file.Size > 0 {
			bySize[file.Size] = append(bySize[file.Size], file)
		}
	}
	var total int64
	matched := make(map[string]bool)
	for _, name := range paths {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		matches, err := supplyPackFile(ctx, cache, bySize, name, &total)
		if err != nil {
			if ctx.Err() != nil {
				return result, ctx.Err()
			}
			result.Skipped++
			if len(result.Warnings) < 6 {
				result.Warnings = append(result.Warnings, filepath.Base(name)+" · "+err.Error())
			}
			continue
		}
		for _, match := range matches {
			if !matched[match] {
				result.Matched = append(result.Matched, match)
				matched[match] = true
			}
		}
	}
	slices.Sort(result.Matched)
	return result, ctx.Err()
}

func supplyPackFile(ctx context.Context, cache *os.Root, bySize map[int64][]modpack.File, name string, total *int64) ([]string, error) {
	if !filepath.IsAbs(name) {
		return nil, errors.New("请选择本地文件")
	}
	before, err := os.Lstat(name)
	if err != nil {
		return nil, errors.New("无法读取所选文件")
	}
	if !before.Mode().IsRegular() || before.Size() <= 0 || before.Size() > download.MaxFileSize {
		return nil, errors.New("文件类型或大小不符合要求")
	}
	if len(bySize[before.Size()]) == 0 {
		return nil, errors.New("文件大小与清单不符")
	}
	if *total+before.Size() > 8<<30 {
		return nil, errors.New("本批读取达到 8 GiB 上限")
	}
	*total += before.Size()
	file, err := os.Open(name)
	if err != nil {
		return nil, errors.New("无法打开所选文件")
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(before, opened) {
		return nil, errors.New("所选文件已改变")
	}
	temp := ".mcl-supply-" + rand.Text()
	destination, err := cache.OpenFile(temp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, errors.New("无法创建补齐文件缓存")
	}
	defer cache.Remove(temp)
	defer destination.Close()
	h1, h512 := sha1.New(), sha512.New()
	output := io.MultiWriter(destination, h1, h512)
	reader := &packSupplyReader{ctx: ctx, reader: file}
	n, err := io.CopyBuffer(output, io.LimitReader(reader, before.Size()+1), make([]byte, 64<<10))
	if err != nil || n != before.Size() {
		return nil, errors.New("文件读取失败或大小已改变")
	}
	after, err := file.Stat()
	if err != nil {
		return nil, errors.New("无法重查所选文件")
	}
	current, err := os.Lstat(name)
	if err != nil || !os.SameFile(before, after) || !os.SameFile(before, current) || after.Size() != before.Size() || current.Size() != before.Size() || !after.ModTime().Equal(before.ModTime()) || !current.ModTime().Equal(before.ModTime()) {
		return nil, errors.New("文件在校验期间改变")
	}
	digest1, digest512 := hex.EncodeToString(h1.Sum(nil)), hex.EncodeToString(h512.Sum(nil))
	var matches []string
	for _, expected := range bySize[n] {
		if strings.EqualFold(expected.SHA1, digest1) && (expected.SHA512 == "" || strings.EqualFold(expected.SHA512, digest512)) {
			matches = append(matches, expected.Path)
		}
	}
	if len(matches) == 0 {
		return nil, errors.New("摘要与清单不匹配")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := destination.Sync(); err != nil {
		return nil, fmt.Errorf("保存补齐文件缓存: %w", err)
	}
	if err := destination.Close(); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := cache.Rename(temp, digest1); err != nil {
		return nil, fmt.Errorf("提交补齐文件缓存: %w", err)
	}
	return matches, nil
}

type packSupplyReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *packSupplyReader) Read(buffer []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(buffer)
}
