package storage

import (
	"context"
	"crypto/rand"
	"errors"
	"os"
	"path"
	"path/filepath"

	"mcl/internal/app"
	"mcl/internal/core/archive"
	"mcl/internal/core/instance"
	"mcl/internal/core/transfer"
)

func (s *ArchiveStore) CheckTaskRepairEntries(ctx context.Context, value instance.Instance, plan app.PackRepairPlan, entries []app.PackRepairEntry) error {
	game, err := s.assets.openGame(ctx, value.ID)
	if err != nil {
		return err
	}
	defer game.Close()
	var overrides []app.PackRepairEntry
	for _, entry := range entries {
		if entry.Override {
			overrides = append(overrides, entry)
			continue
		}
		name := entry.Path
		if _, err := game.Lstat(name); errors.Is(err, os.ErrNotExist) {
			if path.Ext(name) == ".jar" {
				name += ".disabled"
			}
		} else if err != nil {
			return err
		}
		if err := verifyLocalAsset(ctx, game, name, entry.File.SHA1, entry.File.Size); err != nil {
			return errors.New("修复路径已被不同内容占用，请核对实例文件；未将其视为修复完成")
		}
	}
	if len(overrides) == 0 {
		return ctx.Err()
	}
	parent, err := os.OpenRoot(filepath.Join(s.dir, "instances"))
	if err != nil {
		return err
	}
	defer parent.Close()
	managed, err := openRealDirectory(parent, value.ID)
	if err != nil {
		return err
	}
	defer managed.Close()
	origin, receipt, err := readPackOriginMust(ctx, s, value)
	if err != nil {
		return err
	}
	if receipt != plan.Receipt {
		return errors.New("整合包来源记录已变化")
	}
	name := ".mcl-repair-verify-" + rand.Text()
	if err := managed.Mkdir(name, 0700); err != nil {
		return err
	}
	defer managed.RemoveAll(name)
	stage, err := openRealDirectory(managed, name)
	if err != nil {
		return err
	}
	defer stage.Close()
	file, z, err := s.open(ctx, plan.Archive)
	if err != nil {
		return err
	}
	defer file.Close()
	var mappings []archive.Mapping
	for _, entry := range overrides {
		found := false
		for _, mapping := range origin.Manifest.Overrides {
			if mapping.Target == entry.Path {
				mappings = append(mappings, mapping)
				found = true
				break
			}
		}
		if !found {
			return errors.New("覆盖项不在原始整合包清单")
		}
	}
	if err := z.Extract(ctx, stage, mappings); err != nil {
		return err
	}
	for _, entry := range overrides {
		expected, err := transfer.Hash(ctx, stage, entry.Path)
		if err != nil {
			return err
		}
		actual, err := transfer.Hash(ctx, game, entry.Path)
		if err != nil {
			return err
		}
		if expected.Size != actual.Size || expected.SHA256 != actual.SHA256 {
			return errors.New("已有覆盖项与原包不符，请核对实例文件")
		}
	}
	return ctx.Err()
}
func readPackOriginMust(ctx context.Context, s *ArchiveStore, value instance.Instance) (packOrigin, string, error) {
	root, err := os.OpenRoot(s.dir)
	if err != nil {
		return packOrigin{}, "", err
	}
	defer root.Close()
	if err := ctx.Err(); err != nil {
		return packOrigin{}, "", err
	}
	return readPackOrigin(root, value)
}
