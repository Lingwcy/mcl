package storage

import (
	"context"
	"crypto/rand"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"mcl/internal/app"
	"mcl/internal/core/archive"
	"mcl/internal/core/market"
	"mcl/internal/core/modpack"
	"mcl/internal/fileio"
)

type ArchiveStore struct {
	dir    string
	assets *AssetStore
}

func NewArchiveStore(root string) *ArchiveStore {
	return &ArchiveStore{dir: root, assets: NewAssetStore(filepath.Join(root, "instances"))}
}
func (s *ArchiveStore) open(ctx context.Context, expected market.File) (*os.File, *archive.Zip, error) {
	if expected.Size <= 0 || expected.Size > 512<<20 {
		return nil, nil, errors.New("压缩包大小无效")
	}
	root, err := os.OpenRoot(s.dir)
	if err != nil {
		return nil, nil, err
	}
	defer root.Close()
	if len(expected.SHA1) != 40 {
		return nil, nil, errors.New("压缩包摘要无效")
	}
	if _, err := hex.DecodeString(expected.SHA1); err != nil {
		return nil, nil, err
	}
	name := path.Join("cache", "market", expected.SHA1)
	before, err := root.Lstat(name)
	if err != nil {
		return nil, nil, err
	}
	if !before.Mode().IsRegular() || before.Size() != expected.Size {
		return nil, nil, errors.New("压缩包缓存大小或类型无效")
	}
	file, err := root.Open(name)
	if err != nil {
		return nil, nil, err
	}
	fail := func(err error) (*os.File, *archive.Zip, error) { file.Close(); return nil, nil, err }
	info, err := file.Stat()
	if err != nil {
		return fail(err)
	}
	if !os.SameFile(before, info) || info.Size() != expected.Size {
		return fail(errors.New("压缩包缓存大小或类型无效"))
	}
	hash := sha1.New()
	source := io.LimitReader(file, expected.Size+1)
	buffer := make([]byte, 64<<10)
	var size int64
	for {
		if err := ctx.Err(); err != nil {
			return fail(err)
		}
		n, err := source.Read(buffer)
		if n > 0 {
			hash.Write(buffer[:n])
			size += int64(n)
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fail(err)
		}
	}
	if size != expected.Size || !strings.EqualFold(hex.EncodeToString(hash.Sum(nil)), expected.SHA1) {
		return fail(errors.New("压缩包缓存校验失败"))
	}
	z, err := archive.Open(file, info.Size())
	if err != nil {
		return fail(err)
	}
	return file, z, nil
}
func (s *ArchiveStore) InspectPack(ctx context.Context, file market.File) (modpack.Manifest, error) {
	f, z, err := s.open(ctx, file)
	if err != nil {
		return modpack.Manifest{}, err
	}
	defer f.Close()
	return modpack.Read(ctx, z)
}
func (s *ArchiveStore) InspectWorld(ctx context.Context, file market.File) ([]archive.Mapping, error) {
	f, z, err := s.open(ctx, file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return modpack.WorldFiles(z)
}
func (s *ArchiveStore) CheckWorld(ctx context.Context, id, name string) error {
	if !archive.SafePath(name) || path.Base(name) != name || strings.HasPrefix(name, ".") {
		return errors.New("请输入安全的新存档名称")
	}
	game, err := s.assets.openGame(ctx, id)
	if err != nil {
		return err
	}
	defer game.Close()
	info, err := game.Lstat("saves")
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("存档目录不是本地普通目录")
	}
	if _, err := game.Lstat(path.Join("saves", name)); err == nil {
		return errors.New("同名存档已存在，请换一个名称")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return ctx.Err()
}
func (s *ArchiveStore) CommitArchive(ctx context.Context, plan app.ArchiveImportPlan) error {
	f, z, err := s.open(ctx, plan.Archive)
	if err != nil {
		return err
	}
	defer f.Close()
	if plan.Request.Kind == market.World {
		return s.commitWorld(ctx, z, plan)
	}
	if plan.Request.Kind != market.Modpack || plan.Instance.Pack == nil {
		return errors.New("无效整合包提交")
	}
	if err := plan.Instance.Validate(); err != nil {
		return err
	}
	// Re-read the checked archive; never extract source paths supplied only by
	// UI state. The digest fixes the manifest and overrides for this import.
	manifest, err := modpack.Read(ctx, z)
	if err != nil {
		return err
	}
	if manifest.Minecraft != plan.Instance.Version.ID {
		return errors.New("整合包版本与新实例不匹配")
	}
	pack := plan.Instance.Pack
	if manifest.Format != pack.Source || plan.Archive.SHA1 != pack.ArchiveSHA1 || len(manifest.Loaders) != len(pack.Loaders) || (plan.Request.LocalArchive != nil) != pack.Local {
		return errors.New("整合包固定来源或运行环境不匹配")
	}
	for index, required := range manifest.Loaders {
		if required.Type != pack.Loaders[index].Type || required.Version != pack.Loaders[index].Version {
			return errors.New("整合包加载器配置已改变")
		}
	}
	if err := checkPackManifestFiles(manifest, plan.Files); err != nil {
		return err
	}
	plan.Manifest = manifest
	if err := os.MkdirAll(filepath.Join(s.dir, "instances"), 0o700); err != nil {
		return err
	}
	parent, err := os.OpenRoot(filepath.Join(s.dir, "instances"))
	if err != nil {
		return err
	}
	defer parent.Close()
	temporary := ".mcl-import-" + rand.Text()
	if err := parent.Mkdir(temporary, 0o700); err != nil {
		return err
	}
	defer parent.RemoveAll(temporary)
	stage, err := parent.OpenRoot(temporary)
	if err != nil {
		return err
	}
	defer stage.Close()
	if err := stage.Mkdir("game", 0o700); err != nil {
		return err
	}
	game, err := stage.OpenRoot("game")
	if err != nil {
		return err
	}
	defer game.Close()
	cache, err := os.OpenRoot(s.dir)
	if err != nil {
		return err
	}
	defer cache.Close()
	activeFiles := plan.ActiveFiles()
	for _, file := range activeFiles {
		if !archive.SafePath(file.Path) {
			return errors.New("整合包安装路径不安全")
		}
		if err := game.MkdirAll(path.Dir(file.Path), 0o700); err != nil {
			return err
		}
		if err := copyVerifiedCache(ctx, cache, game, file.Path, file.SHA1, file.SHA512, file.Size); err != nil {
			return fmt.Errorf("准备 %s: %w", file.Path, err)
		}
	}
	if err := z.Extract(ctx, game, manifest.Overrides); err != nil {
		return err
	}
	if err := guardStagedPackMods(ctx, game, plan.Instance); err != nil {
		return err
	}
	data, err := json.MarshalIndent(instanceDocument{Version: 1, Instance: plan.Instance}, "", "  ")
	if err != nil {
		return err
	}
	if err := fileio.WriteAt(ctx, stage, "instance.json", data, 0o600); err != nil {
		return err
	}
	// Store origin separately from game-controlled pack overrides.
	origin, err := json.Marshal(packOrigin{
		Schema: 1, Request: plan.Request, Manifest: manifest, Optional: plan.SelectedOptional,
		Resolved: append([]modpack.File{}, activeFiles...), Archive: &plan.Archive,
	})
	if err != nil {
		return err
	}
	if len(origin) > 16<<20 {
		return errors.New("整合包来源记录过大")
	}
	if err := fileio.WriteAt(ctx, stage, "modpack.json", origin, 0o600); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// Close stage handles before rename for Windows sharing semantics.
	game.Close()
	stage.Close()
	return fileio.PublishDirectory(parent, temporary, plan.Instance.ID)
}

func checkPackManifestFiles(manifest modpack.Manifest, files []modpack.File) error {
	identity := func(file modpack.File) string {
		if manifest.Format == "curseforge" {
			return file.ProjectID + "/" + file.VersionID
		}
		return file.Path
	}
	expected := make(map[string]modpack.File, len(manifest.Files))
	for _, file := range manifest.Files {
		key := identity(file)
		if _, duplicate := expected[key]; duplicate {
			return errors.New("整合包清单包含重复文件身份")
		}
		expected[key] = file
	}
	for _, file := range files {
		key := identity(file)
		entry, ok := expected[key]
		if !ok || entry.Optional != file.Optional {
			return errors.New("导入文件与固定清单不匹配")
		}
		if manifest.Format == "modrinth" && (entry.Size != file.Size || entry.SHA1 != file.SHA1 || entry.SHA512 != file.SHA512) {
			return errors.New("导入文件摘要与 MRPack 清单不匹配")
		}
		delete(expected, key)
	}
	for _, file := range expected {
		if !file.Optional {
			return errors.New("固定清单仍缺少必需文件")
		}
	}
	return nil
}
func (s *ArchiveStore) commitWorld(ctx context.Context, z *archive.Zip, plan app.ArchiveImportPlan) error {
	if err := s.CheckWorld(ctx, plan.Request.InstanceID, plan.Request.World); err != nil {
		return err
	}
	mapping, err := modpack.WorldFiles(z)
	if err != nil {
		return err
	}
	game, err := s.assets.openGame(ctx, plan.Request.InstanceID)
	if err != nil {
		return err
	}
	defer game.Close()
	if err := game.MkdirAll("saves", 0o700); err != nil {
		return err
	}
	parent, err := game.OpenRoot("saves")
	if err != nil {
		return err
	}
	defer parent.Close()
	temporary := ".mcl-world-" + rand.Text()
	if err := parent.Mkdir(temporary, 0o700); err != nil {
		return err
	}
	defer parent.RemoveAll(temporary)
	stage, err := parent.OpenRoot(temporary)
	if err != nil {
		return err
	}
	defer stage.Close()
	if err := z.Extract(ctx, stage, mapping); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	stage.Close()
	return fileio.PublishDirectory(parent, temporary, plan.Request.World)
}
