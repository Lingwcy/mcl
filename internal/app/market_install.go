package app

import (
	"context"
	"errors"
	"fmt"
	"path"
	"slices"

	"mcl/internal/core/download"
	"mcl/internal/core/market"
	"mcl/internal/core/meta"
)

type MarketFileStore interface {
	Check(context.Context, market.InstallPlan) error
	Commit(context.Context, market.InstallPlan) error
	Worlds(context.Context, string) ([]string, error)
	ReadyLocalFiles(context.Context, market.InstallPlan) ([]string, error)
	SupplyFiles(context.Context, market.InstallPlan, []string) (LocalFileSupply, error)
}

type LocalFileSupply struct {
	Matched  []string
	Skipped  int
	Warnings []string
}

type MissingMarketFilesError struct {
	Ready []string
	Count int
}

func (e *MissingMarketFilesError) Error() string {
	return fmt.Sprintf("仍有 %d 个文件需要从本地补齐", e.Count)
}

type MarketInstaller struct {
	Tasks     *TaskService
	catalog   *MarketService
	instances InstanceReader
	registry  SessionRegistry
	files     MarketFileStore
	downloads *download.Client
}

func NewMarketInstaller(catalog *MarketService, instances InstanceReader, registry SessionRegistry, files MarketFileStore, downloads *download.Client) *MarketInstaller {
	return &MarketInstaller{catalog: catalog, instances: instances, registry: registry, files: files, downloads: downloads}
}
func (s *MarketInstaller) Worlds(ctx context.Context, id string) ([]string, error) {
	if _, err := s.instances.Get(ctx, id); err != nil {
		return nil, err
	}
	return s.files.Worlds(ctx, id)
}
func (s *MarketInstaller) Plan(ctx context.Context, request market.InstallRequest) (market.InstallPlan, error) {
	value, err := s.instances.Get(ctx, request.InstanceID)
	if err != nil {
		return market.InstallPlan{}, err
	}
	request.GameVersion = value.Version.ID
	if required, ok := value.RequiredLoader(); ok && request.Kind == market.Mod {
		if request.Loader != "" && request.Loader != required.Type {
			return market.InstallPlan{}, fmt.Errorf("目标实例使用 %s，所选模组要求 %s", required.Type, request.Loader)
		}
		request.Loader = required.Type
	}
	client := s.catalog.Client()
	plan, err := market.BuildInstall(ctx, client, request)
	if err != nil {
		return plan, err
	}
	if request.Kind == market.Mod {
		plan.Warnings = append(plan.Warnings, "下载后检查模组声明。未确认的范围和内嵌候选可在资产检查页查看。")
	}
	var ids []string
	for _, file := range plan.Files {
		ids = append(ids, file.ProjectID)
	}
	slices.Sort(ids)
	ids = slices.Compact(ids)
	for start := 0; start < len(ids); start += 20 {
		batch := ids[start:min(start+20, len(ids))]
		projects, err := client.ProjectSummaries(ctx, request.Source, batch)
		if err != nil {
			return market.InstallPlan{}, fmt.Errorf("读取资产项目: %w", err)
		}
		for index := range plan.Files {
			file := &plan.Files[index]
			project, found := projects[file.ProjectID]
			if request.Source == market.CurseForge && slices.Contains(batch, file.ProjectID) {
				if !found || project.Source != request.Source || project.Kind != request.Kind {
					return market.InstallPlan{}, fmt.Errorf("无法确认项目 %s 的类别或分发设置", file.ProjectID)
				}
				if project.DistributionRestricted {
					file.File.URL = ""
				}
			}
			if !found {
				continue
			}
			file.Metadata.WithProject(project)
			if file.File.URL == "" && market.SafeURL(project.PageURL) {
				if plan.LocalPages == nil {
					plan.LocalPages = make(map[string]string)
				}
				plan.LocalPages[file.Path] = project.PageURL
			}
		}
	}
	if err := s.files.Check(ctx, plan); err != nil {
		return market.InstallPlan{}, err
	}
	plan.ReadyLocal, err = s.files.ReadyLocalFiles(ctx, plan)
	if err != nil {
		return market.InstallPlan{}, err
	}
	return plan, nil
}

func (s *MarketInstaller) Supply(ctx context.Context, plan market.InstallPlan, paths []string) (LocalFileSupply, error) {
	if err := s.files.Check(ctx, plan); err != nil {
		return LocalFileSupply{}, err
	}
	return s.files.SupplyFiles(ctx, plan, paths)
}

func (s *MarketInstaller) Install(ctx context.Context, plan market.InstallPlan, report func(download.Progress)) error {
	if s.Tasks != nil && taskScope(ctx) == nil {
		intent, err := taskPlanIntent(ctx, s.instances, "market", "安装市场资源", plan.Request.InstanceID, plan)
		if err != nil {
			return err
		}
		return s.Tasks.Execute(ctx, intent, func(task context.Context) error {
			if err := checkTaskTarget(task, s.instances, intent); err != nil {
				return err
			}
			current := plan
			if TaskAttempt(task) > 1 {
				fresh, err := s.refreshTaskPlan(task, plan)
				if err != nil {
					return err
				}
				current = fresh
			}
			return s.Install(task, current, report)
		})
	}
	request := plan.Request
	lease, err := s.registry.Reserve(ctx, request.InstanceID)
	if err != nil {
		return err
	}
	defer lease.Close()
	value, err := s.instances.Get(ctx, request.InstanceID)
	if err != nil {
		return err
	}
	if err := checkTaskInstance(ctx, value); err != nil {
		return err
	}
	if value.Version.ID != request.GameVersion {
		return errors.New("目标实例版本已改变，请重新确认安装计划")
	}
	if required, ok := value.RequiredLoader(); ok && request.Kind == market.Mod && required.Type != request.Loader {
		return errors.New("目标实例加载器已改变，请重新确认安装计划")
	}
	if err := s.files.Check(ctx, plan); err != nil {
		return err
	}
	plan.ReadyLocal, err = s.files.ReadyLocalFiles(ctx, plan)
	if err != nil {
		return err
	}
	if missing := plan.MissingLocal(); missing > 0 {
		return &MissingMarketFilesError{Ready: plan.ReadyLocal, Count: missing}
	}
	var files []download.File
	for _, f := range plan.Files {
		if f.File.URL == "" {
			continue
		}
		files = append(files, download.File{Path: path.Join("cache", "market", f.File.SHA1), Download: meta.Download{URL: f.File.URL, SHA1: f.File.SHA1, Size: f.File.Size}})
	}
	if len(files) > 0 {
		if err := s.downloads.Run(ctx, files, report); err != nil {
			return fmt.Errorf("下载资产: %w", err)
		}
	}
	if err := TaskStage(ctx, "发布"); err != nil {
		return err
	}
	if err := s.files.Commit(ctx, plan); err != nil {
		return fmt.Errorf("写入实例资产: %w", err)
	}
	return nil
}
