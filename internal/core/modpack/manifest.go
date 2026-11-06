// Package modpack parses client modpack manifests and plans their archive
// overrides. It does not execute pack-supplied scripts or JVM arguments.
package modpack

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path"
	"slices"
	"sort"
	"strconv"
	"strings"

	"mcl/internal/core/archive"
	"mcl/internal/core/meta"
)

const MaxFiles = 5000

type Loader struct{ Type, Version string }
type File struct {
	Path, SHA1, SHA512   string
	Size                 int64
	URLs                 []string
	ProjectID, VersionID string
	Optional             bool
	PageURL              string `json:",omitempty"`
}
type Manifest struct {
	Format, Name, Version, Minecraft string
	Loaders                          []Loader
	Files                            []File
	Overrides                        []archive.Mapping
}

func Read(ctx context.Context, z *archive.Zip) (Manifest, error) {
	var mr, cf bool
	for _, entry := range z.Entries() {
		if entry.Name == "modrinth.index.json" {
			mr = true
		}
		if entry.Name == "manifest.json" {
			cf = true
		}
	}
	if mr == cf {
		return Manifest{}, errors.New("整合包缺少清单，或同时包含多种清单")
	}
	var m Manifest
	var prefixes []string
	var err error
	if mr {
		m, err = readModrinth(ctx, z)
		prefixes = []string{"overrides/", "client-overrides/"}
	} else {
		m, prefixes, err = readCurseForge(ctx, z)
	}
	if err != nil {
		return Manifest{}, err
	}
	if !meta.ValidID(m.Minecraft) || len(m.Files) > MaxFiles {
		return Manifest{}, errors.New("整合包游戏版本或文件数量无效")
	}
	for _, loader := range m.Loaders {
		if loader.Version == "" || len(loader.Version) > 100 || strings.ContainsAny(loader.Version, "/\\\x00\r\n") {
			return Manifest{}, errors.New("整合包加载器版本无效")
		}
	}
	if len(m.Loaders) > 1 {
		return Manifest{}, errors.New("暂不支持同一整合包包含多个加载器")
	}
	files := make(map[string]string)
	for _, file := range m.Files {
		if !archive.SafePath(file.Path) && file.Path != "" {
			return Manifest{}, errors.New("整合包下载目标路径不安全")
		}
		if file.Path != "" {
			key := archive.Key(file.Path)
			if _, ok := files[key]; ok {
				return Manifest{}, errors.New("整合包文件路径冲突")
			}
			files[key] = file.Path
		}
	}
	selected := make(map[string]archive.Mapping)
	for _, prefix := range prefixes {
		for _, entry := range z.Entries() {
			if entry.Directory || !strings.HasPrefix(entry.Name, prefix) {
				continue
			}
			target := strings.TrimPrefix(entry.Name, prefix)
			if !archive.SafePath(target) {
				return Manifest{}, errors.New("整合包覆盖路径不安全")
			}
			key := archive.Key(target)
			if old, ok := selected[key]; ok && old.Target != target {
				return Manifest{}, errors.New("整合包覆盖路径大小写冲突")
			}
			if old, ok := files[key]; ok && old != target {
				return Manifest{}, errors.New("整合包下载与覆盖路径大小写冲突")
			}
			selected[key] = archive.Mapping{Source: entry.Name, Target: target}
		}
	}
	for _, entry := range selected {
		m.Overrides = append(m.Overrides, entry)
	}
	sort.Slice(m.Overrides, func(i, j int) bool { return m.Overrides[i].Target < m.Overrides[j].Target })
	return m, ctx.Err()
}
func readModrinth(ctx context.Context, z *archive.Zip) (Manifest, error) {
	data, err := z.Read(ctx, "modrinth.index.json", 8<<20)
	if err != nil {
		return Manifest{}, err
	}
	var raw struct {
		FormatVersion int               `json:"formatVersion"`
		Game          string            `json:"game"`
		Name          string            `json:"name"`
		Version       string            `json:"versionId"`
		Dependencies  map[string]string `json:"dependencies"`
		Files         []struct {
			Path      string            `json:"path"`
			Hashes    map[string]string `json:"hashes"`
			Size      int64             `json:"fileSize"`
			Downloads []string          `json:"downloads"`
			Env       map[string]string `json:"env"`
		} `json:"files"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return Manifest{}, errors.New("MRPack 清单无效")
	}
	if raw.FormatVersion != 1 || raw.Game != "minecraft" {
		return Manifest{}, errors.New("不支持此 MRPack 格式或游戏")
	}
	m := Manifest{Format: "modrinth", Name: raw.Name, Version: raw.Version, Minecraft: raw.Dependencies["minecraft"]}
	for name, version := range raw.Dependencies {
		if name == "minecraft" {
			continue
		}
		kind := map[string]string{"fabric-loader": "fabric", "quilt-loader": "quilt", "forge": "forge", "neoforge": "neoforge"}[name]
		if kind == "" {
			return Manifest{}, fmt.Errorf("整合包要求尚未支持的组件 %s", name)
		}
		m.Loaders = append(m.Loaders, Loader{Type: kind, Version: version})
	}
	if len(raw.Files) > MaxFiles {
		return Manifest{}, errors.New("整合包文件过多")
	}
	for _, entry := range raw.Files {
		client := entry.Env["client"]
		if client != "" && !slices.Contains([]string{"required", "optional", "unsupported"}, client) {
			return Manifest{}, errors.New("整合包客户端环境无效")
		}
		if client == "unsupported" {
			continue
		}
		sha512, err := hex.DecodeString(entry.Hashes["sha512"])
		if !archive.SafePath(entry.Path) || !meta.ValidSHA1(entry.Hashes["sha1"]) || err != nil || len(sha512) != 64 || entry.Size < 0 || entry.Size > 2<<30 {
			return Manifest{}, errors.New("整合包文件路径、大小或摘要无效")
		}
		if len(entry.Downloads) == 0 || len(entry.Downloads) > 8 {
			return Manifest{}, errors.New("整合包文件没有有效下载来源")
		}
		for _, address := range entry.Downloads {
			if !AllowedDownload(address) {
				return Manifest{}, errors.New("整合包文件使用不允许的下载来源")
			}
		}
		m.Files = append(m.Files, File{Path: entry.Path, SHA1: strings.ToLower(entry.Hashes["sha1"]), SHA512: strings.ToLower(entry.Hashes["sha512"]), Size: entry.Size, URLs: entry.Downloads, Optional: client == "optional"})
	}
	return m, nil
}
func readCurseForge(ctx context.Context, z *archive.Zip) (Manifest, []string, error) {
	data, err := z.Read(ctx, "manifest.json", 8<<20)
	if err != nil {
		return Manifest{}, nil, err
	}
	var raw struct {
		ManifestType    string `json:"manifestType"`
		ManifestVersion int    `json:"manifestVersion"`
		Name            string `json:"name"`
		Version         string `json:"version"`
		Overrides       string `json:"overrides"`
		Minecraft       struct {
			Version string `json:"version"`
			Loaders []struct {
				ID string `json:"id"`
			} `json:"modLoaders"`
		} `json:"minecraft"`
		Files []struct {
			ProjectID int64 `json:"projectID"`
			FileID    int64 `json:"fileID"`
			Required  bool  `json:"required"`
		} `json:"files"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return Manifest{}, nil, errors.New("CurseForge 整合包清单无效")
	}
	if raw.ManifestType != "minecraftModpack" || raw.ManifestVersion != 1 {
		return Manifest{}, nil, errors.New("不支持此 CurseForge 整合包格式")
	}
	m := Manifest{Format: "curseforge", Name: raw.Name, Version: raw.Version, Minecraft: raw.Minecraft.Version}
	for _, loader := range raw.Minecraft.Loaders {
		kind, version, ok := strings.Cut(loader.ID, "-")
		if !ok || !slices.Contains([]string{"fabric", "quilt", "forge", "neoforge"}, kind) {
			return Manifest{}, nil, errors.New("整合包加载器类型不支持")
		}
		m.Loaders = append(m.Loaders, Loader{Type: kind, Version: strings.TrimPrefix(version, m.Minecraft+"-")})
	}
	if len(raw.Files) > MaxFiles {
		return Manifest{}, nil, errors.New("整合包文件过多")
	}
	for _, f := range raw.Files {
		if f.ProjectID <= 0 || f.FileID <= 0 {
			return Manifest{}, nil, errors.New("CurseForge 文件身份无效")
		}
		m.Files = append(m.Files, File{ProjectID: strconv.FormatInt(f.ProjectID, 10), VersionID: strconv.FormatInt(f.FileID, 10), Optional: !f.Required})
	}
	prefix := raw.Overrides
	if prefix == "" {
		prefix = "overrides"
	}
	if !archive.SafePath(prefix) {
		return Manifest{}, nil, errors.New("整合包覆盖目录无效")
	}
	return m, []string{prefix + "/"}, nil
}
func AllowedDownload(address string) bool {
	u, err := url.Parse(address)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" || strings.ContainsAny(address, " \r\n") {
		return false
	}
	return slices.Contains([]string{"cdn.modrinth.com", "github.com", "raw.githubusercontent.com", "gitlab.com", "edge.forgecdn.net", "mediafilez.forgecdn.net", "media.forgecdn.net"}, strings.ToLower(u.Hostname()))
}
func DownloadHosts() []string {
	return []string{"cdn.modrinth.com", "github.com", "raw.githubusercontent.com", "gitlab.com", "edge.forgecdn.net", "mediafilez.forgecdn.net", "media.forgecdn.net", "release-assets.githubusercontent.com", "objects.githubusercontent.com", "codeload.github.com"}
}

// WorldFiles locates a single level.dat and strips its containing directory,
// handling both a bare world ZIP and a ZIP with one enclosing world folder.
func WorldFiles(z *archive.Zip) ([]archive.Mapping, error) {
	var roots []string
	for _, entry := range z.Entries() {
		if !entry.Directory && path.Base(entry.Name) == "level.dat" {
			if entry.Size == 0 {
				return nil, errors.New("地图的 level.dat 为空")
			}
			roots = append(roots, path.Dir(entry.Name))
		}
	}
	if len(roots) != 1 {
		return nil, errors.New("地图压缩包必须包含且仅包含一个 level.dat 存档")
	}
	prefix := roots[0]
	if prefix == "." {
		prefix = ""
	} else {
		prefix += "/"
	}
	var result []archive.Mapping
	for _, entry := range z.Entries() {
		if entry.Directory || !strings.HasPrefix(entry.Name, prefix) {
			continue
		}
		if strings.HasPrefix(entry.Name, "__MACOSX/") {
			continue
		}
		target := strings.TrimPrefix(entry.Name, prefix)
		if !archive.SafePath(target) {
			return nil, errors.New("地图路径无效")
		}
		result = append(result, archive.Mapping{Source: entry.Name, Target: target})
	}
	return result, nil
}
