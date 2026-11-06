package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"mcl/internal/app"
	"mcl/internal/core/archive"
	"mcl/internal/core/instance"
	"mcl/internal/core/market"
)

const maxLocalAssets = 3000

// AssetStore reads only immediate asset directories; it neither opens archives
// nor follows symbolic links. World data packs are read from saves/*/datapacks.
type AssetStore struct{ dir string }

func NewAssetStore(instancesDir string) *AssetStore { return &AssetStore{dir: instancesDir} }

func (s *AssetStore) openGame(ctx context.Context, id string) (*os.Root, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !instance.ValidID(id) {
		return nil, errors.New("invalid instance ID")
	}
	root, err := os.OpenRoot(s.dir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	for _, name := range []string{id, path.Join(id, "game")} {
		info, err := root.Lstat(name)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil, errors.New("实例游戏目录不是受管的本地目录")
		}
	}
	return root.OpenRoot(path.Join(id, "game"))
}

func (s *AssetStore) Directory(ctx context.Context, id string) (string, error) {
	root, err := s.openGame(ctx, id)
	if err != nil {
		return "", err
	}
	defer root.Close()
	return filepath.Join(s.dir, id, "game"), ctx.Err()
}

func (s *AssetStore) List(ctx context.Context, id string) (app.AssetSnapshot, error) {
	result := app.AssetSnapshot{InstanceID: id, ScannedAt: time.Now().UTC()}
	game, err := s.openGame(ctx, id)
	if err != nil {
		return result, err
	}
	defer game.Close()
	for _, directory := range []struct {
		name string
		kind app.AssetKind
	}{{"mods", app.AssetMod}, {"shaderpacks", app.AssetShader}, {"resourcepacks", app.AssetResourcePack}} {
		if err := scanAssets(ctx, game, directory.name, directory.kind, "", &result); err != nil {
			return result, fmt.Errorf("读取 %s: %w", directory.name, err)
		}
	}
	worlds, err := localDirectory(game, "saves")
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return result, fmt.Errorf("读取存档目录: %w", err)
	}
	if worlds != nil {
		defer worlds.Close()
		entries, err := worlds.ReadDir(129)
		if err != nil && !errors.Is(err, io.EOF) {
			return result, err
		}
		if len(entries) > 128 {
			result.Truncated, entries = true, entries[:128]
		}
		for _, entry := range entries {
			if !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
				continue
			}
			if err := scanAssets(ctx, game, path.Join("saves", entry.Name(), "datapacks"), app.AssetDataPack, entry.Name(), &result); err != nil {
				return result, fmt.Errorf("读取存档数据包: %w", err)
			}
		}
	}
	slices.SortFunc(result.Files, func(a, b app.AssetFile) int { return strings.Compare(strings.ToLower(a.Path), strings.ToLower(b.Path)) })
	if err := s.attachOrigins(ctx, id, &result); err != nil {
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		result.Warning = "来源记录无法读取，文件已保留：" + err.Error()
	}
	if pending, err := s.pendingUpdate(ctx, id); err != nil {
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		result.Warning += " 更新恢复记录无法读取：" + err.Error()
	} else {
		result.PendingUpdate = pending
	}
	return result, ctx.Err()
}

func (s *AssetStore) attachOrigins(ctx context.Context, id string, result *app.AssetSnapshot) error {
	root, err := os.OpenRoot(s.dir)
	if err != nil {
		return err
	}
	defer root.Close()
	receipt, err := readMarketReceiptAt(ctx, root, path.Join(id, "market.json"))
	if err != nil {
		return err
	}
	origins := make(map[string][]market.InstallFile, len(receipt.Files))
	for _, origin := range receipt.Files {
		key := archive.Key(origin.Path)
		origins[key] = append(origins[key], origin)
	}
	for index := range result.Files {
		file := &result.Files[index]
		if file.Directory {
			continue
		}
		for _, origin := range origins[archive.Key(strings.TrimSuffix(file.Path, ".disabled"))] {
			if origin.File.Size != file.Size {
				continue
			}
			file.Origins = append(file.Origins, origin)
			if file.Origin == nil || origin.Source == market.Modrinth {
				file.Origin = &origin
			}
		}
	}
	return nil
}

func localDirectory(root *os.Root, name string) (*os.File, error) {
	// Check each path component, including the world directory. Root confines
	// resolution even if a directory changes while a scan is in progress.
	current := ""
	for _, part := range strings.Split(name, "/") {
		current = path.Join(current, part)
		info, err := root.Lstat(current)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil, errors.New("资产目录不能是符号链接或普通文件")
		}
	}
	return root.Open(name)
}

func scanAssets(ctx context.Context, root *os.Root, directory string, kind app.AssetKind, world string, result *app.AssetSnapshot) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(result.Files) >= maxLocalAssets {
		result.Truncated = true
		return nil
	}
	folder, err := localDirectory(root, directory)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer folder.Close()
	// Bound inspected entries as well as displayed assets; unrelated files
	// cannot turn a frame-triggered refresh into an unbounded directory scan.
	entries, err := folder.ReadDir(maxLocalAssets + 1)
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	if len(entries) > maxLocalAssets {
		result.Truncated, entries = true, entries[:maxLocalAssets]
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		if len(result.Files) >= maxLocalAssets {
			result.Truncated = true
			break
		}
		if strings.HasPrefix(entry.Name(), ".") || entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ".disabled")
		ext := strings.ToLower(path.Ext(name))
		if kind == app.AssetMod && (ext != ".jar" || entry.IsDir()) {
			continue
		}
		if kind != app.AssetMod && !entry.IsDir() && ext != ".zip" {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.IsDir() && !info.Mode().IsRegular() {
			continue
		}
		size := info.Size()
		if info.IsDir() {
			size = 0 // Do not recursively read or invent the size of an unpacked pack.
		}
		result.Files = append(result.Files, app.AssetFile{
			Path: path.Join(directory, entry.Name()), Name: name, World: world,
			Kind: kind, Size: size, ModifiedAt: info.ModTime(), Directory: info.IsDir(), Enabled: name == entry.Name(),
		})
	}
	return nil
}
