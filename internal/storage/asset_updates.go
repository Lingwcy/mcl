package storage

import (
	"bytes"
	"context"
	"crypto/sha256"
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
	"mcl/internal/core/instance"
	"mcl/internal/core/market"
)

const updatePendingFile = "asset-update.pending.json"
const maxUpdateJournalSize = 12 << 20

type AssetUpdateStore struct {
	dir    string
	assets *AssetStore
}

func NewAssetUpdateStore(root string) *AssetUpdateStore {
	return &AssetUpdateStore{dir: root, assets: NewAssetStore(filepath.Join(root, "instances"))}
}

func (s *AssetUpdateStore) ReadyUpdateFiles(ctx context.Context, plan market.UpdatePlan) ([]string, error) {
	return cachedManualFiles(ctx, s.dir, manualMarketFiles(plan.LocalFiles()))
}

func (s *AssetUpdateStore) SupplyUpdateFiles(ctx context.Context, plan market.UpdatePlan, paths []string) (app.LocalFileSupply, error) {
	return supplyLocalFiles(ctx, s.dir, manualMarketFiles(plan.LocalFiles()), paths)
}

func receiptDigest(receipt marketReceipt) (string, error) {
	data, err := json.Marshal(receipt)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func (s *AssetUpdateStore) UpdateSnapshot(ctx context.Context, id string, kind market.Kind, world string) (app.AssetUpdateSnapshot, error) {
	result := app.AssetUpdateSnapshot{}
	directory, _, err := market.AssetDirectory(kind, world)
	if err != nil {
		return result, err
	}
	snapshot, err := s.assets.List(ctx, id)
	if err != nil {
		return result, err
	}
	if snapshot.Warning != "" {
		return result, errors.New(snapshot.Warning)
	}
	if snapshot.Truncated {
		return result, errors.New("资产列表超过读取上限，不能确认完整更新关系")
	}
	root, err := os.OpenRoot(s.dir)
	if err != nil {
		return result, err
	}
	defer root.Close()
	receipt, err := readMarketReceipt(ctx, root, id)
	if err != nil {
		return result, err
	}
	result.ReceiptSHA256, err = receiptDigest(receipt)
	if err != nil {
		return result, err
	}
	for _, file := range snapshot.Files {
		if path.Dir(file.Path) != directory {
			continue
		}
		if file.Directory || len(file.Origins) == 0 {
			result.Unknown++
			continue
		}
		hasMetadata := false
		for _, origin := range file.Origins {
			hasMetadata = hasMetadata || origin.Metadata != nil
			result.Files = append(result.Files, market.InstalledFile{File: origin, Path: file.Path, Enabled: file.Enabled})
		}
		if !hasMetadata {
			result.Unknown++
		}
	}
	return result, nil
}

func (s *AssetUpdateStore) CheckUpdate(ctx context.Context, plan market.UpdatePlan) error {
	if err := plan.Validate(); err != nil {
		return err
	}
	snapshot, err := s.UpdateSnapshot(ctx, plan.Request.InstanceID, plan.Request.Kind, plan.Request.World)
	if err != nil {
		return err
	}
	if snapshot.ReceiptSHA256 != plan.ReceiptSHA256 {
		return errors.New("资产来源记录已经改变，请重新生成更新计划")
	}
	game, err := s.assets.openGame(ctx, plan.Request.InstanceID)
	if err != nil {
		return err
	}
	defer game.Close()
	verified := make(map[string]bool)
	var read int64
	verify := func(file market.InstalledFile) error {
		key := file.Path + ":" + file.File.File.SHA1
		if verified[key] {
			return nil
		}
		if read+file.File.File.Size > 8<<30 {
			return errors.New("单次更新校验超过 8 GiB")
		}
		if err := verifyLocalAsset(ctx, game, file.Path, file.File.File.SHA1, file.File.File.Size); err != nil {
			return err
		}
		read += file.File.File.Size
		verified[key] = true
		return nil
	}
	known := func(expected market.InstalledFile) bool {
		for _, file := range snapshot.Files {
			if file.Path == expected.Path && file.File.Source == expected.File.Source && file.File.ProjectID == expected.File.ProjectID && file.File.VersionID == expected.File.VersionID && file.File.File.SHA1 == expected.File.File.SHA1 && file.File.File.Size == expected.File.File.Size {
				return true
			}
		}
		return false
	}
	oldPaths := make(map[string]bool)
	for _, change := range plan.Changes {
		if change.Before != nil {
			if !known(*change.Before) {
				return errors.New("旧文件来源与更新计划不符")
			}
			if err := verify(*change.Before); err != nil {
				return fmt.Errorf("旧文件已改变，请重新识别: %w", err)
			}
			oldPaths[archive.Key(change.Before.Path)] = true
		}
	}
	for _, file := range plan.Reused {
		if !known(file) {
			return errors.New("复用依赖来源已改变")
		}
		if err := verify(file); err != nil {
			return fmt.Errorf("复用依赖不可用: %w", err)
		}
	}
	for _, change := range plan.Changes {
		current := ""
		for _, part := range strings.Split(path.Dir(change.Path()), "/") {
			current = path.Join(current, part)
			info, err := game.Lstat(current)
			if errors.Is(err, os.ErrNotExist) {
				break
			}
			if err != nil {
				return err
			}
			if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				return errors.New("更新目录不是普通目录")
			}
		}
		if _, err := game.Lstat(change.Path()); err == nil && !oldPaths[archive.Key(change.Path())] {
			return fmt.Errorf("目标文件 %s 已存在，未覆盖", path.Base(change.Path()))
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		alternate := change.After.Path
		if change.Enabled {
			alternate += ".disabled"
		}
		if alternate != change.Path() {
			if _, err := game.Lstat(alternate); err == nil && !oldPaths[archive.Key(alternate)] {
				return errors.New("新版本已有启用或停用的同名文件")
			} else if err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
	}
	var trusted []market.InstalledFile
	for _, file := range snapshot.Files {
		if !file.Enabled {
			trusted = append(trusted, file)
			continue
		}
		if err := verify(file); err != nil {
			return fmt.Errorf("无法校验已有资产 %s，请先重新识别或处理该文件: %w", path.Base(file.Path), err)
		}
		trusted = append(trusted, file)
	}
	return market.CheckUpdateRelations(trusted, plan)
}

func readUpdateJSON(ctx context.Context, root *os.Root, name string, limit int64, out any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	info, err := root.Lstat(name)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("更新记录不是普通文件")
	}
	f, err := root.Open(name)
	if err != nil {
		return err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return err
	}
	if int64(len(data)) > limit {
		return errors.New("更新记录超出大小上限")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return errors.New("更新记录包含尾随内容")
	}
	return ctx.Err()
}

type updatePending struct {
	Schema int    `json:"schema"`
	ID     string `json:"id"`
}

func readUpdatePending(ctx context.Context, managed *os.Root) (updatePending, error) {
	var pending updatePending
	if err := readUpdateJSON(ctx, managed, updatePendingFile, 1024, &pending); err != nil {
		return pending, err
	}
	if pending.Schema != 1 || !instance.ValidID(pending.ID) {
		return pending, errors.New("待恢复更新标识无效")
	}
	return pending, nil
}

func (s *AssetStore) pendingUpdate(ctx context.Context, id string) (*app.AssetUpdateRecord, error) {
	root, err := os.OpenRoot(s.dir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	managed, err := root.OpenRoot(id)
	if err != nil {
		return nil, err
	}
	defer managed.Close()
	pending, err := readUpdatePending(ctx, managed)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &app.AssetUpdateRecord{ID: pending.ID, State: "pending", Pending: true}, nil
}
