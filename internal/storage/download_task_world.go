package storage

import (
	"context"
	"crypto/rand"
	"errors"
	"io"
	"os"
	"path"

	"mcl/internal/app"
	"mcl/internal/core/archive"
	"mcl/internal/core/instance"
	"mcl/internal/core/transfer"
)

func (s *ArchiveStore) TaskWorldReady(ctx context.Context, plan app.ArchiveImportPlan) (bool, error) {
	if !instance.ValidID(plan.Request.InstanceID) || !archive.SafePath(plan.Request.World) || path.Base(plan.Request.World) != plan.Request.World || len(plan.WorldFiles) > archive.MaxEntries {
		return false, errors.New("地图任务目标无效")
	}
	game, err := s.assets.openGame(ctx, plan.Request.InstanceID)
	if err != nil {
		return false, err
	}
	defer game.Close()
	if _, err := game.Lstat(path.Join("saves", plan.Request.World)); errors.Is(err, os.ErrNotExist) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	parent, err := openRealDirectory(game, "saves")
	if err != nil {
		return false, err
	}
	defer parent.Close()
	world, err := openRealDirectory(parent, plan.Request.World)
	if err != nil {
		return false, err
	}
	defer world.Close()
	if err := readPackPublication(ctx, world, plan); err != nil {
		return false, errors.New("同名存档不属于此固定地图任务，已保留")
	}
	file, z, err := s.open(ctx, plan.Archive)
	if err != nil {
		return false, err
	}
	defer file.Close()
	temporary := ".mcl-world-verify-" + rand.Text()
	if err := parent.Mkdir(temporary, 0700); err != nil {
		return false, err
	}
	defer parent.RemoveAll(temporary)
	stage, err := openRealDirectory(parent, temporary)
	if err != nil {
		return false, err
	}
	defer stage.Close()
	if err := z.Extract(ctx, stage, plan.WorldFiles); err != nil {
		return false, err
	}
	f, err := stage.Open(".")
	if err != nil {
		return false, err
	}
	entries, err := f.ReadDir(50001)
	f.Close()
	if err != nil && !errors.Is(err, io.EOF) {
		return false, err
	}
	if len(entries) > 50000 {
		return false, errors.New("地图校验文件数量越界")
	}
	roots := make([]string, 0, len(entries))
	for _, entry := range entries {
		roots = append(roots, entry.Name())
	}
	expected, err := transfer.Scan(ctx, stage, roots, false, nil)
	if err != nil {
		return false, err
	}
	if err := transfer.VerifyCopied(ctx, world, expected); err != nil {
		return false, errors.New("已发布地图内容发生变化，请核对存档，未覆盖")
	}
	f, err = world.Open(".")
	if err != nil {
		return false, err
	}
	actual, err := f.ReadDir(50002)
	f.Close()
	if err != nil && !errors.Is(err, io.EOF) {
		return false, err
	}
	if len(actual) != len(entries)+1 {
		return false, errors.New("已发布地图出现额外内容，请核对存档")
	}
	return true, ctx.Err()
}
