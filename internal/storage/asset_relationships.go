package storage

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"strings"

	"mcl/internal/app"
	"mcl/internal/core/archive"
	"mcl/internal/core/market"
)

// liveOrigins only treats an active, hash-verified plain file as evidence. A
// stale metadata record alone cannot block a mutation or assert compatibility.
type liveOrigins struct {
	ctx     context.Context
	game    *os.Root
	checked map[string]bool
}

func (l *liveOrigins) active(file market.InstallFile) (bool, error) {
	key := file.Path + ":" + file.File.SHA1
	if active, ok := l.checked[key]; ok {
		return active, nil
	}
	if !assetPath(file.Path) || strings.HasSuffix(file.Path, ".disabled") {
		return false, nil
	}
	err := verifyLocalAsset(l.ctx, l.game, file.Path, file.File.SHA1, file.File.Size)
	if l.ctx.Err() != nil {
		return false, l.ctx.Err()
	}
	l.checked[key] = err == nil
	return err == nil, nil
}

func originName(file market.InstallFile) string {
	if file.Metadata != nil && file.Metadata.Title != "" {
		return file.Metadata.Title
	}
	return path.Base(file.Path)
}

func guardAssetChange(ctx context.Context, game *os.Root, receipt marketReceipt, name string, action app.AssetAction) error {
	canonical := strings.TrimSuffix(name, ".disabled")
	key := archive.Key(canonical)
	var targets []market.InstallFile
	for _, file := range receipt.Files {
		if archive.Key(file.Path) == key {
			targets = append(targets, file)
		}
	}
	if len(targets) == 0 {
		return nil
	}
	live := liveOrigins{ctx: ctx, game: game, checked: make(map[string]bool)}
	for _, other := range receipt.Files {
		if archive.Key(other.Path) == key || archive.Key(path.Dir(other.Path)) != archive.Key(path.Dir(canonical)) {
			continue
		}
		if action == app.AssetEnable {
			for _, target := range targets {
				if !market.Incompatible(target, other) {
					continue
				}
				active, err := live.active(other)
				if err != nil {
					return err
				}
				if active {
					return fmt.Errorf("此资产与已启用的 %s 声明不兼容，请先停用其中一个", originName(other))
				}
			}
			continue
		}
		if strings.HasSuffix(name, ".disabled") || other.Metadata == nil {
			continue
		}
		for _, dependency := range other.Metadata.Dependencies {
			if !dependency.Required {
				continue
			}
			needed := false
			for _, target := range targets {
				needed = needed || market.DependencyMatches(other.Source, dependency, target)
			}
			if !needed {
				continue
			}
			active, err := live.active(other)
			if err != nil {
				return err
			}
			if !active {
				continue
			}
			alternative := false
			for _, candidate := range receipt.Files {
				if archive.Key(candidate.Path) == key || archive.Key(path.Dir(candidate.Path)) != archive.Key(path.Dir(canonical)) || !market.DependencyMatches(other.Source, dependency, candidate) {
					continue
				}
				active, err := live.active(candidate)
				if err != nil {
					return err
				}
				alternative = alternative || active
				if alternative {
					break
				}
			}
			if !alternative {
				return fmt.Errorf("%s 仍依赖此资产，请先停用或移除依赖它的文件", originName(other))
			}
		}
	}
	return nil
}

func guardPlannedConflicts(ctx context.Context, game *os.Root, receipt marketReceipt, plan market.InstallPlan) error {
	live := liveOrigins{ctx: ctx, game: game, checked: make(map[string]bool)}
	for _, file := range plan.Files {
		for _, old := range receipt.Files {
			if old.Path == file.Path || path.Dir(old.Path) != path.Dir(file.Path) || !market.Incompatible(file, old) {
				continue
			}
			active, err := live.active(old)
			if err != nil {
				return err
			}
			if active {
				return fmt.Errorf("%s 与实例中已启用的 %s 声明不兼容", originName(file), originName(old))
			}
		}
	}
	for i, left := range plan.Files {
		for _, right := range plan.Files[i+1:] {
			if market.Incompatible(left, right) {
				return errors.New("安装计划中的资产声明不兼容")
			}
		}
	}
	return nil
}
