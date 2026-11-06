package storage

import (
	"context"
	"os"
	"path"

	"mcl/internal/app"
	"mcl/internal/core/market"
	"mcl/internal/core/modpack"
)

func (s *MarketStore) ReadyLocalFiles(ctx context.Context, plan market.InstallPlan) ([]string, error) {
	var files []market.InstallFile
	for _, file := range plan.Files {
		if file.File.URL == "" {
			files = append(files, file)
		}
	}
	if len(files) == 0 {
		return nil, ctx.Err()
	}
	root, err := os.OpenRoot(s.dir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	game, err := s.assets.openGame(ctx, plan.Request.InstanceID)
	if err != nil {
		return nil, err
	}
	defer game.Close()
	var ready []string
	for _, file := range files {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		sha1, size := file.File.SHA1, file.File.Size
		cachePath := path.Join("cache", "market", sha1)
		if verifyLocalAsset(ctx, root, cachePath, sha1, size) == nil ||
			verifyLocalAsset(ctx, game, file.Path, sha1, size) == nil {
			ready = append(ready, file.Path)
		}
	}
	return ready, ctx.Err()
}

func (s *MarketStore) SupplyFiles(ctx context.Context, plan market.InstallPlan, paths []string) (app.LocalFileSupply, error) {
	files := manualMarketFiles(plan.Files)
	if len(files) == 0 {
		return app.LocalFileSupply{}, nil
	}
	return supplyLocalFiles(ctx, s.dir, files, paths)
}

func manualMarketFiles(entries []market.InstallFile) []modpack.File {
	var files []modpack.File
	for _, f := range entries {
		if f.File.URL == "" {
			files = append(files, modpack.File{Path: f.Path, Size: f.File.Size, SHA1: f.File.SHA1})
		}
	}
	return files
}
