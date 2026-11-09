package app

import (
	"context"
	"errors"
	"fmt"
	"path"
	"slices"
	"strings"
	"time"

	"mcl/internal/core/download"
	"mcl/internal/core/market"
	"mcl/internal/core/meta"
)

type AssetUpdateSnapshot struct {
	Files         []market.InstalledFile
	ReceiptSHA256 string
	Unknown       int
}
type AssetUpdateRecord struct {
	ID, State string
	CreatedAt time.Time
	Pending   bool
}
type AssetUpdateResult struct {
	ID                        string
	Applied, RecoveryRequired bool
}
type AssetUpdateProgress struct {
	Stage string
	download.Progress
}
type AssetUpdateRepository interface {
	UpdateSnapshot(context.Context, string, market.Kind, string) (AssetUpdateSnapshot, error)
	CheckUpdate(context.Context, market.UpdatePlan) error
	ReadyUpdateFiles(context.Context, market.UpdatePlan) ([]string, error)
	SupplyUpdateFiles(context.Context, market.UpdatePlan, []string) (LocalFileSupply, error)
	CommitUpdate(context.Context, market.UpdatePlan, func(AssetUpdateProgress)) (AssetUpdateResult, error)
	RecoverUpdate(context.Context, string, string) (AssetUpdateResult, error)
}
type AssetUpdateRegistry interface {
	SessionRegistry
	ReserveForRecovery(context.Context, string) (SessionLease, error)
}
type AssetUpdateService struct {
	Tasks     *TaskService
	instances InstanceReader
	registry  AssetUpdateRegistry
	store     AssetUpdateRepository
	market    *MarketService
	downloads *download.Client
}

func NewAssetUpdateService(instances InstanceReader, registry AssetUpdateRegistry, store AssetUpdateRepository, catalog *MarketService, downloads *download.Client) *AssetUpdateService {
	return &AssetUpdateService{instances: instances, registry: registry, store: store, market: catalog, downloads: downloads}
}

func (s *AssetUpdateService) Versions(ctx context.Context, id string, file AssetFile, source market.Source) ([]market.Version, error) {
	value, err := s.instances.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	var origin *market.InstallFile
	for _, candidate := range file.Origins {
		if candidate.Source == source {
			copy := candidate
			origin = &copy
			break
		}
	}
	if origin == nil && file.Origin != nil && file.Origin.Source == source {
		origin = file.Origin
	}
	if origin == nil {
		return nil, errors.New("请先识别此文件的来源")
	}
	loader := ""
	if AssetKind(market.Mod) == file.Kind {
		if required, ok := value.RequiredLoader(); ok {
			loader = required.Type
		} else if origin.Metadata != nil && len(origin.Metadata.Loaders) == 1 {
			loader = origin.Metadata.Loaders[0]
		}
	}
	versions, err := s.market.Client().Versions(ctx, source, origin.ProjectID, value.Version.ID, loader)
	if err != nil {
		return nil, err
	}
	versions = slices.DeleteFunc(versions, func(version market.Version) bool {
		return !slices.Contains(version.GameVersions, value.Version.ID) || (loader != "" && !slices.Contains(version.Loaders, loader))
	})
	return versions, nil
}

func (s *AssetUpdateService) Plan(ctx context.Context, request market.UpdateRequest) (market.UpdatePlan, error) {
	lease, err := s.registry.Reserve(ctx, request.InstanceID)
	if err != nil {
		return market.UpdatePlan{}, err
	}
	defer lease.Close()
	value, err := s.instances.Get(ctx, request.InstanceID)
	if err != nil {
		return market.UpdatePlan{}, err
	}
	request.GameVersion, request.VersionSHA1 = value.Version.ID, value.Version.SHA1
	if required, ok := value.RequiredLoader(); ok && request.Kind == market.Mod {
		if request.Loader != "" && request.Loader != required.Type {
			return market.UpdatePlan{}, errors.New("所选加载器与实例不匹配")
		}
		request.Loader = required.Type
	}
	snapshot, err := s.store.UpdateSnapshot(ctx, value.ID, request.Kind, request.World)
	if err != nil {
		return market.UpdatePlan{}, err
	}
	if request.Kind == market.Mod && request.Loader == "" {
		for _, selection := range request.Selections {
			for _, file := range snapshot.Files {
				if file.Path == selection.Path && file.File.Source == selection.Source && file.File.Metadata != nil && len(file.File.Metadata.Loaders) == 1 {
					candidate := file.File.Metadata.Loaders[0]
					if request.Loader != "" && request.Loader != candidate {
						return market.UpdatePlan{}, errors.New("所选文件使用不同加载器")
					}
					request.Loader = candidate
				}
			}
		}
	}
	client := s.market.Client()
	plan, err := market.BuildUpdate(ctx, client, request, snapshot.Files)
	if err != nil {
		return market.UpdatePlan{}, err
	}
	plan.ReceiptSHA256 = snapshot.ReceiptSHA256
	if request.Kind == market.Mod {
		plan.Warnings = append(plan.Warnings, "应用前检查模组声明。未确认的范围和内嵌候选可在资产检查页查看。")
	}
	if _, configured := value.RequiredLoader(); request.Kind == market.Mod && !configured {
		plan.Warnings = append(plan.Warnings, "实例尚未配置加载器，此操作只更新文件，不会安装运行环境")
	}
	if request.Kind == market.Shader {
		plan.Warnings = append(plan.Warnings, "光影仍需要匹配的运行环境，并在游戏内选择")
	}
	if snapshot.Unknown > 0 {
		plan.Warnings = append(plan.Warnings, fmt.Sprintf("%d 个未识别文件的依赖与版本尚未检查", snapshot.Unknown))
	}
	for _, source := range []market.Source{market.Modrinth, market.CurseForge} {
		var ids []string
		for _, change := range plan.Changes {
			if change.After.Source == source {
				ids = append(ids, change.After.ProjectID)
			}
		}
		slices.Sort(ids)
		ids = slices.Compact(ids)
		for offset := 0; offset < len(ids); offset += 20 {
			batch := ids[offset:min(offset+20, len(ids))]
			projects, err := client.ProjectSummaries(ctx, source, batch)
			if err != nil {
				return market.UpdatePlan{}, err
			}
			for index := range plan.Changes {
				file := &plan.Changes[index].After
				if source == market.CurseForge && file.Source == source && slices.Contains(batch, file.ProjectID) {
					project, found := projects[file.ProjectID]
					if !found || project.Kind != request.Kind {
						return market.UpdatePlan{}, fmt.Errorf("无法确认项目 %s 的类别或分发设置", file.ProjectID)
					}
					if project.DistributionRestricted {
						file.File.URL = ""
					}
				}
				if file.Source == source && file.Metadata != nil {
					if project, ok := projects[file.ProjectID]; ok {
						copy := *file.Metadata
						copy.WithProject(project)
						file.Metadata = &copy
						if file.File.URL == "" && market.SafeURL(project.PageURL) {
							if plan.LocalPages == nil {
								plan.LocalPages = make(map[string]string)
							}
							plan.LocalPages[file.Path] = project.PageURL
						}
					}
				}
			}
		}
	}
	if err := s.store.CheckUpdate(ctx, plan); err != nil {
		return market.UpdatePlan{}, err
	}
	plan.ReadyLocal, err = s.store.ReadyUpdateFiles(ctx, plan)
	if err != nil {
		return market.UpdatePlan{}, err
	}
	return plan, nil
}

func (s *AssetUpdateService) Supply(ctx context.Context, plan market.UpdatePlan, paths []string) (LocalFileSupply, error) {
	if err := s.store.CheckUpdate(ctx, plan); err != nil {
		return LocalFileSupply{}, err
	}
	return s.store.SupplyUpdateFiles(ctx, plan, paths)
}

func (s *AssetUpdateService) Apply(ctx context.Context, plan market.UpdatePlan, report func(AssetUpdateProgress)) (AssetUpdateResult, error) {
	if s.Tasks != nil && taskScope(ctx) == nil {
		intent, err := taskPlanIntent(ctx, s.instances, "update", "更新市场资源", plan.Request.InstanceID, plan)
		if err != nil {
			return AssetUpdateResult{}, err
		}
		return performTask(ctx, s.Tasks, intent, func(task context.Context) (AssetUpdateResult, error) {
			if err := checkTaskTarget(task, s.instances, intent); err != nil {
				return AssetUpdateResult{}, err
			}
			current := plan
			if TaskAttempt(task) > 1 {
				if result, done, err := s.recoverTask(task, plan); err != nil || done {
					return result, err
				}
				fresh, err := s.refreshTaskPlan(task, plan)
				if err != nil {
					return AssetUpdateResult{}, err
				}
				current = fresh
			}
			return s.Apply(task, current, report)
		})
	}
	if taskScope(ctx) != nil {
		result, done, err := s.recoverTask(ctx, plan)
		if err != nil || done {
			return result, err
		}
	}
	lease, err := s.registry.Reserve(ctx, plan.Request.InstanceID)
	if err != nil {
		return AssetUpdateResult{}, err
	}
	defer lease.Close()
	value, err := s.instances.Get(ctx, plan.Request.InstanceID)
	if err != nil {
		return AssetUpdateResult{}, err
	}
	if err := checkTaskInstance(ctx, value); err != nil {
		return AssetUpdateResult{}, err
	}
	if value.Version.ID != plan.Request.GameVersion || !strings.EqualFold(value.Version.SHA1, plan.Request.VersionSHA1) {
		return AssetUpdateResult{}, errors.New("实例版本已改变，请重新确认更新计划")
	}
	if required, ok := value.RequiredLoader(); ok && plan.Request.Kind == market.Mod && required.Type != plan.Request.Loader {
		return AssetUpdateResult{}, errors.New("实例加载器已改变")
	}
	if err := s.store.CheckUpdate(ctx, plan); err != nil {
		return AssetUpdateResult{}, err
	}
	if len(plan.Changes) == 0 {
		return AssetUpdateResult{}, errors.New("没有需要应用的更新")
	}
	plan.ReadyLocal, err = s.store.ReadyUpdateFiles(ctx, plan)
	if err != nil {
		return AssetUpdateResult{}, err
	}
	if missing := plan.MissingLocal(); missing > 0 {
		return AssetUpdateResult{}, &MissingMarketFilesError{Ready: plan.ReadyLocal, Count: missing}
	}
	var files []download.File
	for _, change := range plan.Changes {
		if change.ReusesContent() || change.After.File.URL == "" {
			continue
		}
		file := change.After.File
		files = append(files, download.File{Path: path.Join("cache", "market", file.SHA1), Download: meta.Download{URL: file.URL, SHA1: file.SHA1, Size: file.Size}})
	}
	if len(files) > 0 {
		if err := s.downloads.Run(ctx, files, func(progress download.Progress) {
			if report != nil {
				report(AssetUpdateProgress{Stage: "update-download", Progress: progress})
			}
		}); err != nil {
			return AssetUpdateResult{}, err
		}
	}
	if err := TaskStage(ctx, "发布"); err != nil {
		return AssetUpdateResult{}, err
	}
	return s.store.CommitUpdate(ctx, plan, report)
}

func (s *AssetUpdateService) Recover(ctx context.Context, id, transaction string) (AssetUpdateResult, error) {
	lease, err := s.registry.ReserveForRecovery(ctx, id)
	if err != nil {
		return AssetUpdateResult{}, err
	}
	defer lease.Close()
	if _, err := s.instances.Get(ctx, id); err != nil {
		return AssetUpdateResult{}, err
	}
	return s.store.RecoverUpdate(ctx, id, transaction)
}
