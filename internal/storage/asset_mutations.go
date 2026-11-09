package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"slices"
	"strings"
	"time"

	"mcl/internal/app"
	"mcl/internal/core/archive"
	"mcl/internal/core/instance"
	"mcl/internal/core/market"
	"mcl/internal/core/meta"
	"mcl/internal/fileio"
)

type removedAsset struct {
	Schema     int       `json:"schema"`
	Path       string    `json:"path"`
	Size       int64     `json:"size"`
	ModifiedAt time.Time `json:"modifiedAt"`
	SHA1       string    `json:"sha1,omitempty"`
	URL        string    `json:"url,omitempty"`
	RemovedAt  time.Time `json:"removedAt"`
}

// assetPath accepts immediate file assets only. Renaming a pack directory to
// .disabled does not reliably deactivate it in Minecraft, so directories remain
// read-only. Worlds and arbitrary game/configuration paths cannot enter here.
func assetPath(name string) bool {
	if !meta.ValidPath(name) {
		return false
	}
	parts := strings.Split(name, "/")
	directory := path.Dir(name)
	if directory != "mods" && directory != "shaderpacks" && directory != "resourcepacks" {
		if len(parts) != 4 || parts[0] != "saves" || !market.SafeFilename(parts[1]) || parts[2] != "datapacks" {
			return false
		}
	}
	file := strings.TrimSuffix(path.Base(name), ".disabled")
	if !market.SafeFilename(file) {
		return false
	}
	extension := strings.ToLower(path.Ext(file))
	return directory == "mods" && extension == ".jar" || directory != "mods" && extension == ".zip"
}

func (s *AssetStore) Change(ctx context.Context, request app.AssetMutation) (app.AssetChange, error) {
	file := request.File
	if !assetPath(file.Path) || file.Directory {
		return app.AssetChange{}, errors.New("仅支持管理实例中的 JAR / ZIP 资产文件")
	}
	if request.Action != app.AssetEnable && request.Action != app.AssetDisable && request.Action != app.AssetRemove {
		return app.AssetChange{}, errors.New("未知资产操作")
	}
	game, err := s.openGame(ctx, request.InstanceID)
	if err != nil {
		return app.AssetChange{}, err
	}
	defer game.Close()
	folder, err := localDirectory(game, path.Dir(file.Path))
	if err != nil {
		return app.AssetChange{}, err
	}
	folder.Close()
	parent, err := game.OpenRoot(path.Dir(file.Path))
	if err != nil {
		return app.AssetChange{}, err
	}
	defer parent.Close()
	info, err := parent.Lstat(path.Base(file.Path))
	if err != nil {
		return app.AssetChange{}, err
	}
	if !info.Mode().IsRegular() || info.Size() != file.Size || !info.ModTime().Equal(file.ModifiedAt) {
		return app.AssetChange{}, errors.New("文件已改变，请刷新后重试")
	}
	root, err := os.OpenRoot(s.dir)
	if err != nil {
		return app.AssetChange{}, err
	}
	defer root.Close()
	receipt, err := readMarketReceiptAt(ctx, root, path.Join(request.InstanceID, "market.json"))
	if err != nil {
		return app.AssetChange{}, err
	}
	var hash, address string
	for _, origin := range receipt.Files {
		if archive.Key(origin.Path) != archive.Key(strings.TrimSuffix(file.Path, ".disabled")) {
			continue
		}
		if err := verifyLocalAsset(ctx, parent, path.Base(file.Path), origin.File.SHA1, origin.File.Size); err != nil {
			return app.AssetChange{}, errors.New("文件与安装记录不符，已保留原文件；请先检查本地修改")
		}
		hash = origin.File.SHA1
		address = origin.File.URL
	}
	if err := ctx.Err(); err != nil {
		return app.AssetChange{}, err
	}
	if err := guardAssetChange(ctx, game, receipt, file.Path, request.Action); err != nil {
		return app.AssetChange{}, err
	}
	if request.Action == app.AssetRemove {
		if err := s.guardLocalMods(ctx, game, request.InstanceID, "", []localModEdit{{before: file.Path}}); err != nil {
			return app.AssetChange{}, err
		}
		if err := confirmAssetEntry(parent, path.Base(file.Path), info); err != nil {
			return app.AssetChange{}, err
		}
		return s.removeAsset(ctx, request.InstanceID, parent, path.Base(file.Path), removedAsset{Schema: 1, Path: file.Path, Size: info.Size(), ModifiedAt: info.ModTime(), SHA1: hash, URL: address, RemovedAt: time.Now().UTC()})
	}
	enabled := !strings.HasSuffix(file.Path, ".disabled")
	wantEnabled := request.Action == app.AssetEnable
	if enabled == wantEnabled {
		return app.AssetChange{}, nil
	}
	target := strings.TrimSuffix(path.Base(file.Path), ".disabled")
	if !wantEnabled {
		target += ".disabled"
	}
	if err := s.guardLocalMods(ctx, game, request.InstanceID, "", []localModEdit{{before: file.Path, after: path.Join(path.Dir(file.Path), target), enabled: wantEnabled}}); err != nil {
		return app.AssetChange{}, err
	}
	if err := confirmAssetEntry(parent, path.Base(file.Path), info); err != nil {
		return app.AssetChange{}, err
	}
	if err := ctx.Err(); err != nil {
		return app.AssetChange{}, err
	}
	if err := fileio.MoveNoReplace(parent, path.Base(file.Path), parent, target); err != nil {
		return app.AssetChange{}, fmt.Errorf("切换资产失败，未覆盖已有文件: %w", err)
	}
	return app.AssetChange{}, nil
}

func (s *AssetStore) removeAsset(ctx context.Context, id string, source *os.Root, name string, record removedAsset) (app.AssetChange, error) {
	backup := instance.NewID()
	parent, err := s.removedAssetsRoot(ctx, id, true)
	if err != nil {
		return app.AssetChange{}, err
	}
	defer parent.Close()
	if err := parent.Mkdir(backup, 0o700); err != nil {
		return app.AssetChange{}, err
	}
	committed := false
	defer func() {
		if !committed {
			parent.RemoveAll(backup)
		}
	}()
	target, err := parent.OpenRoot(backup)
	if err != nil {
		return app.AssetChange{}, err
	}
	defer target.Close()
	data, err := json.Marshal(record)
	if err != nil {
		return app.AssetChange{}, err
	}
	// Save recovery information before moving the payload. A crash at either
	// side leaves the file at its original path or in a named recoverable backup.
	if err := fileio.WriteAt(ctx, target, "asset.json", data, 0o600); err != nil {
		return app.AssetChange{}, err
	}
	if err := ctx.Err(); err != nil {
		return app.AssetChange{}, err
	}
	if err := fileio.MoveNoReplace(source, name, target, "payload"); err != nil {
		return app.AssetChange{}, fmt.Errorf("移除失败，原文件已保留: %w", err)
	}
	committed = true
	return app.AssetChange{BackupID: backup}, nil
}

func (s *AssetStore) Restore(ctx context.Context, id, backup string) error {
	if !instance.ValidID(id) || !instance.ValidID(backup) {
		return errors.New("资产恢复标识无效")
	}
	game, err := s.openGame(ctx, id)
	if err != nil {
		return err
	}
	defer game.Close()
	root, err := os.OpenRoot(s.dir)
	if err != nil {
		return err
	}
	defer root.Close()
	parent, err := s.removedAssetsRoot(ctx, id, false)
	if err != nil {
		return err
	}
	defer parent.Close()
	removed, err := parent.OpenRoot(backup)
	if err != nil {
		return err
	}
	defer removed.Close()
	file, err := removed.Open("asset.json")
	if err != nil {
		return err
	}
	data, readErr := io.ReadAll(io.LimitReader(file, (16<<10)+1))
	file.Close()
	if readErr != nil {
		return readErr
	}
	if len(data) > 16<<10 {
		return errors.New("资产恢复记录过大")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var record removedAsset
	if err := decoder.Decode(&record); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return errors.New("资产恢复记录包含尾随内容")
	}
	if record.Schema != 1 || !assetPath(record.Path) || record.Size < 0 || record.RemovedAt.IsZero() || record.ModifiedAt.IsZero() || (record.SHA1 != "" && !meta.ValidSHA1(record.SHA1)) || (record.URL != "" && !market.SafeURL(record.URL)) {
		return errors.New("资产恢复记录无效")
	}
	info, err := removed.Lstat("payload")
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() != record.Size || !info.ModTime().Equal(record.ModifiedAt) {
		return errors.New("备份文件已改变，未恢复")
	}
	if record.SHA1 != "" {
		if err := verifyLocalAsset(ctx, removed, "payload", record.SHA1, record.Size); err != nil {
			return fmt.Errorf("恢复文件校验失败: %w", err)
		}
	}
	if !strings.HasSuffix(record.Path, ".disabled") {
		receipt, err := readMarketReceiptAt(ctx, root, path.Join(id, "market.json"))
		if err != nil {
			return err
		}
		key := archive.Key(record.Path)
		receipt.Files = slices.DeleteFunc(receipt.Files, func(file market.InstallFile) bool {
			return archive.Key(file.Path) == key && !strings.EqualFold(file.File.SHA1, record.SHA1)
		})
		if err := guardAssetChange(ctx, game, receipt, record.Path, app.AssetEnable); err != nil {
			return err
		}
		if err := s.guardLocalMods(ctx, game, id, "", []localModEdit{{after: record.Path, enabled: true, root: removed, source: "payload", size: record.Size, sha1: record.SHA1}}); err != nil {
			return err
		}
	}
	folder, err := localDirectory(game, path.Dir(record.Path))
	if err != nil {
		return err
	}
	folder.Close()
	target, err := game.OpenRoot(path.Dir(record.Path))
	if err != nil {
		return err
	}
	defer target.Close()
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := confirmAssetEntry(removed, "payload", info); err != nil {
		return err
	}
	if err := fileio.MoveNoReplace(removed, "payload", target, path.Base(record.Path)); err != nil {
		return fmt.Errorf("恢复失败，未覆盖已有文件: %w", err)
	}
	parent.RemoveAll(backup)
	return nil
}

func confirmAssetEntry(root *os.Root, name string, expected os.FileInfo) error {
	current, err := root.Lstat(name)
	if err != nil {
		return err
	}
	if !current.Mode().IsRegular() || !os.SameFile(current, expected) || current.Size() != expected.Size() || !current.ModTime().Equal(expected.ModTime()) {
		return errors.New("文件在检查期间改变，请刷新后重试")
	}
	return nil
}

func (s *AssetStore) removedAssetsRoot(ctx context.Context, id string, create bool) (*os.Root, error) {
	managed, err := s.openPayload(ctx, id)
	if err != nil {
		return nil, err
	}
	defer managed.Close()
	if create {
		if err := managed.Mkdir("removed-assets", 0o700); err != nil && !errors.Is(err, os.ErrExist) {
			return nil, err
		}
	}
	info, err := managed.Lstat("removed-assets")
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("资产备份目录不能是链接或普通文件")
	}
	return managed.OpenRoot("removed-assets")
}
