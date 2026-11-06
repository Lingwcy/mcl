package storage

import (
	"context"
	"encoding/json"
	"errors"
	"image"
	"os"
	"path"
	"strconv"
	"strings"

	"mcl/internal/app"
	"mcl/internal/core/archive"
	"mcl/internal/core/thumbnail"

	"github.com/pelletier/go-toml/v2"
)

// Icon reads only bounded artwork and declaration entries from a local asset.
// It keeps the file snapshot used by the list and does not extract the archive.
func (s *AssetStore) Icon(ctx context.Context, id string, asset app.AssetFile) (image.Image, error) {
	game, err := s.openGame(ctx, id)
	if err != nil {
		return nil, err
	}
	defer game.Close()
	if !archive.SafePath(asset.Path) {
		return nil, errors.New("资产图标路径无效")
	}
	parent, err := localDirectory(game, path.Dir(asset.Path))
	if err != nil {
		return nil, err
	}
	parent.Close()
	if asset.Directory {
		directory, err := localDirectory(game, asset.Path)
		if err != nil {
			return nil, err
		}
		directory.Close()
		pack, err := game.OpenRoot(asset.Path)
		if err != nil {
			return nil, err
		}
		defer pack.Close()
		info, err := pack.Lstat("pack.png")
		if err != nil {
			return nil, err
		}
		if !info.Mode().IsRegular() || info.Size() > thumbnail.MaxBytes {
			return nil, errors.New("包图标不是受支持的文件")
		}
		f, err := pack.Open("pack.png")
		if err != nil {
			return nil, err
		}
		defer f.Close()
		opened, err := f.Stat()
		if err != nil || !os.SameFile(info, opened) {
			return nil, errors.New("包图标已改变")
		}
		return thumbnail.Read(&packSupplyReader{ctx: ctx, reader: f})
	}
	if !assetPath(asset.Path) || asset.Size <= 0 || asset.Size > 2<<30 {
		return nil, errors.New("资产图标类型不受支持")
	}
	before, err := game.Lstat(asset.Path)
	if err != nil {
		return nil, err
	}
	if !before.Mode().IsRegular() || before.Size() != asset.Size || !before.ModTime().Equal(asset.ModifiedAt) {
		return nil, errors.New("资产在读取图标前改变")
	}
	f, err := game.Open(asset.Path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !os.SameFile(before, opened) {
		return nil, errors.New("资产在打开时改变")
	}
	z, err := archive.Open(f, asset.Size)
	if err != nil {
		return nil, err
	}
	candidates := assetIconPaths(ctx, z)
	for _, name := range candidates {
		if !archive.SafePath(name) || !z.Has(name) {
			continue
		}
		data, err := z.Read(ctx, name, thumbnail.MaxBytes)
		if err != nil {
			continue
		}
		icon, err := thumbnail.Decode(data)
		if err != nil {
			continue
		}
		current, err := game.Lstat(asset.Path)
		if err != nil || !os.SameFile(before, current) || current.Size() != before.Size() || !current.ModTime().Equal(before.ModTime()) {
			return nil, errors.New("资产在读取图标时改变")
		}
		return icon, ctx.Err()
	}
	return nil, errors.New("本地资源没有可读取的图标")
}

func assetIconPaths(ctx context.Context, z *archive.Zip) []string {
	candidates := []string{"pack.png"}
	const maxDeclaration = 256 << 10
	if z.Has("fabric.mod.json") {
		if data, err := z.Read(ctx, "fabric.mod.json", maxDeclaration); err == nil {
			var raw struct {
				Icon json.RawMessage `json:"icon"`
			}
			if json.Unmarshal(data, &raw) == nil {
				candidates = append(candidates, jsonAssetIcon(raw.Icon))
			}
		}
	}
	if z.Has("quilt.mod.json") {
		if data, err := z.Read(ctx, "quilt.mod.json", maxDeclaration); err == nil {
			var raw struct {
				Loader struct {
					Metadata struct {
						Icon json.RawMessage `json:"icon"`
					} `json:"metadata"`
				} `json:"quilt_loader"`
			}
			if json.Unmarshal(data, &raw) == nil {
				candidates = append(candidates, jsonAssetIcon(raw.Loader.Metadata.Icon))
			}
		}
	}
	for _, declaration := range []string{"META-INF/neoforge.mods.toml", "META-INF/mods.toml"} {
		if !z.Has(declaration) {
			continue
		}
		data, err := z.Read(ctx, declaration, maxDeclaration)
		if err != nil {
			continue
		}
		var raw struct {
			LogoFile, IconFile string
			Mods               []struct{ LogoFile, IconFile string }
		}
		if toml.Unmarshal(data, &raw) != nil {
			continue
		}
		candidates = append(candidates, raw.IconFile, raw.LogoFile)
		for _, mod := range raw.Mods[:min(8, len(raw.Mods))] {
			candidates = append(candidates, mod.IconFile, mod.LogoFile)
		}
	}
	for index, name := range candidates {
		if namespace, relative, found := strings.Cut(name, ":"); found && archive.SafePath(namespace) && !strings.Contains(namespace, "/") && archive.SafePath(relative) {
			candidates[index] = path.Join("assets", namespace, relative)
		}
	}
	return candidates
}

func jsonAssetIcon(raw json.RawMessage) string {
	var name string
	if json.Unmarshal(raw, &name) == nil {
		return name
	}
	var icons map[string]string
	if json.Unmarshal(raw, &icons) != nil || len(icons) > 32 {
		return ""
	}
	chosen, best := "", 0
	for text, value := range icons {
		size, err := strconv.Atoi(text)
		if err == nil && size > 0 && size <= 2048 && (best == 0 || (size >= thumbnail.Size && (best < thumbnail.Size || size < best)) || (best < thumbnail.Size && size > best)) {
			chosen, best = value, size
		}
	}
	return chosen
}
