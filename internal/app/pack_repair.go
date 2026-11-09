package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"mcl/internal/core/download"
	"mcl/internal/core/instance"
	"mcl/internal/core/market"
	"mcl/internal/core/modpack"
)

const MaxPackRepairSelection = 64

type PackRepairEntry struct {
	Path       string
	Override   bool
	File       modpack.File
	NeedsLocal bool
}

type PackRepairPlan struct {
	InstanceID string
	Receipt    string
	Archive    market.File
	Entries    []PackRepairEntry
	Preserved  int
	Legacy     bool
}

func (s *ArchiveImporter) InspectRepair(ctx context.Context, id string) (PackRepairPlan, error) {
	value, err := s.instances.Get(ctx, id)
	if err != nil {
		return PackRepairPlan{}, err
	}
	return s.inspectRepair(ctx, value)
}

func (s *ArchiveImporter) inspectRepair(ctx context.Context, value instance.Instance) (PackRepairPlan, error) {
	plan, err := s.store.InspectPackRepair(ctx, value)
	if err != nil {
		return plan, err
	}
	var files []modpack.File
	for _, entry := range plan.Entries {
		if !entry.Override {
			files = append(files, entry.File)
		}
	}
	if err := validatePackTargets(ArchiveImportPlan{Files: files}); err != nil {
		return plan, err
	}
	cached, err := s.store.CachedManualFiles(ctx, files)
	if err != nil {
		return plan, err
	}
	available := make(map[string]bool, len(cached))
	for _, name := range cached {
		available[name] = true
	}
	for i := range plan.Entries {
		entry := &plan.Entries[i]
		entry.NeedsLocal = !entry.Override && entry.File.Size > 0 && len(entry.File.URLs) == 0 && !available[entry.Path]
	}
	return plan, nil
}

func (s *ArchiveImporter) SupplyRepair(ctx context.Context, plan PackRepairPlan, paths []string) (LocalFileSupply, error) {
	var files []modpack.File
	for _, entry := range plan.Entries {
		if !entry.Override {
			files = append(files, entry.File)
		}
	}
	if err := validatePackTargets(ArchiveImportPlan{Files: files}); err != nil {
		return LocalFileSupply{}, err
	}
	return s.store.SupplyPackFiles(ctx, files, paths)
}

func (s *ArchiveImporter) RestorePack(ctx context.Context, plan PackRepairPlan, paths []string, report func(download.Progress)) (int, error) {
	if s.Tasks != nil && taskScope(ctx) == nil {
		return s.taskRestore(ctx, plan, paths, func(task context.Context, current PackRepairPlan, selected []string) (int, error) {
			return s.RestorePack(task, current, selected, report)
		})
	}
	if len(paths) == 0 || len(paths) > MaxPackRepairSelection {
		return 0, fmt.Errorf("请选择 1 至 %d 个文件", MaxPackRepairSelection)
	}
	lease, err := s.registry.Reserve(ctx, plan.InstanceID)
	if err != nil {
		return 0, err
	}
	defer lease.Close()
	value, err := s.instances.Get(ctx, plan.InstanceID)
	if err != nil {
		return 0, err
	}
	if err := checkTaskInstance(ctx, value); err != nil {
		return 0, err
	}
	current, err := s.inspectRepair(ctx, value)
	if err != nil {
		return 0, err
	}
	if current.Receipt != plan.Receipt {
		return 0, errors.New("整合包记录已改变，请重新检查")
	}
	byPath := make(map[string]PackRepairEntry, len(current.Entries))
	for _, entry := range current.Entries {
		byPath[entry.Path] = entry
	}
	var selected []PackRepairEntry
	var downloads []download.File
	overrides := false
	for _, name := range paths {
		entry, ok := byPath[name]
		if !ok {
			return 0, fmt.Errorf("目标 %s 已改变，请重新检查", name)
		}
		delete(byPath, name)
		if entry.NeedsLocal {
			return 0, fmt.Errorf("%s 需要从本地补齐", name)
		}
		selected = append(selected, entry)
		overrides = overrides || entry.Override
		if !entry.Override && entry.File.Size > 0 && len(entry.File.URLs) > 0 {
			downloads = append(downloads, packFileDownload(entry.File))
		}
	}
	original := current.Archive
	if overrides {
		if original.Size > 0 {
			if _, err := s.store.InspectPack(ctx, original); err == nil {
				original.URL = ""
			} else {
				if ctx.Err() != nil {
					return 0, ctx.Err()
				}
				if original.URL == "" {
					original = market.File{}
				}
			}
		}
		source := market.Source(value.Pack.Source)
		client := s.catalog.Client()
		if value.Pack.Local && original.Size == 0 {
			return 0, errors.New("本地整合包原始缓存已缺失，请先从本地补齐原包")
		}
		if !value.Pack.Local && source == market.CurseForge && (original.Size == 0 || original.URL != "") {
			project, err := client.ProjectInfo(ctx, source, value.Pack.ProjectID, "")
			if err != nil {
				return 0, fmt.Errorf("读取原始整合包分发设置: %w", err)
			}
			if project.Kind != market.Modpack {
				return 0, errors.New("原始整合包项目类别不匹配")
			}
			if project.DistributionRestricted {
				return 0, errors.New("原始整合包需要本地副本，请在市场的原版本导入页核验压缩包后重试")
			}
		}
		if original.Size == 0 {
			version, err := client.Version(ctx, source, value.Pack.ProjectID, value.Pack.VersionID)
			if err != nil {
				return 0, fmt.Errorf("读取原始整合包: %w", err)
			}
			if version.Source != market.Source(value.Pack.Source) || version.ID != value.Pack.VersionID || version.ProjectID != value.Pack.ProjectID {
				return 0, errors.New("原始整合包版本身份不匹配")
			}
			original, err = version.PrimaryFile()
			if err != nil {
				return 0, err
			}
		}
		if !strings.EqualFold(original.SHA1, value.Pack.ArchiveSHA1) || original.Size <= 0 || original.Size > 512<<20 ||
			(original.URL != "" && !modpack.AllowedDownload(original.URL)) {
			return 0, errors.New("原始整合包的摘要、大小或来源不匹配")
		}
		if original.URL != "" {
			downloads = append(downloads, archiveDownload(original))
		}
	}
	if err := s.downloads.Run(ctx, downloads, report); err != nil {
		return 0, err
	}
	if err := TaskStage(ctx, "发布"); err != nil {
		return 0, err
	}
	if err := s.store.CommitPackRepair(ctx, value, current.Receipt, original, selected); err != nil {
		return 0, err
	}
	return len(selected), nil
}
