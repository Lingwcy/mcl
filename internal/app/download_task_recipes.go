package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"slices"
	"strings"

	"mcl/internal/core/download"
	"mcl/internal/core/instance"
	"mcl/internal/core/java"
	"mcl/internal/core/market"
	"mcl/internal/core/meta"
	"mcl/internal/core/tasks"
)

func taskInstanceFingerprint(value instance.Instance) string {
	required, _ := value.RequiredLoader()
	archive, source, project, version := "", "", "", ""
	if value.Pack != nil {
		archive, source, project, version = value.Pack.ArchiveSHA1, value.Pack.Source, value.Pack.ProjectID, value.Pack.VersionID
	}
	data, _ := json.Marshal(struct {
		ID, Directory, Game, SHA1, Archive, Source, Project, Version string
		Loader                                                       instance.Loader
	}{value.ID, value.GameDirectoryID, value.Version.ID, value.Version.SHA1, archive, source, project, version, required})
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
func taskGameIntent(ctx context.Context, instances InstanceReader, id, kind string) (tasks.Intent, error) {
	value, err := instances.Get(ctx, id)
	if err != nil {
		return tasks.Intent{}, err
	}
	prefix := "安装 / 修复"
	if kind == "preparation" {
		prefix = "启动准备"
	}
	return tasks.Intent{Kind: kind, Name: prefix + " · " + value.Name, InstanceID: id, Fingerprint: taskInstanceFingerprint(value), Payload: taskPayload(struct{ ID string }{id})}, nil
}
func checkTaskTarget(ctx context.Context, instances InstanceReader, intent tasks.Intent) error {
	if intent.InstanceID == "" || intent.Fingerprint == "" {
		return nil
	}
	value, err := instances.Get(ctx, intent.InstanceID)
	if err != nil {
		return err
	}
	if taskInstanceFingerprint(value) != intent.Fingerprint {
		return errors.New("目标实例的位置、版本或加载器已改变，请重新确认计划")
	}
	return nil
}
func taskPlanIntent(ctx context.Context, instances InstanceReader, kind, name, id string, plan any) (tasks.Intent, error) {
	intent := tasks.Intent{Kind: kind, Name: name, InstanceID: id, Payload: taskPayload(plan)}
	if id != "" {
		value, err := instances.Get(ctx, id)
		if err != nil {
			return intent, err
		}
		intent.Fingerprint = taskInstanceFingerprint(value)
	}
	return intent, nil
}
func (s *MarketInstaller) refreshTaskPlan(ctx context.Context, old market.InstallPlan) (market.InstallPlan, error) {
	var plan market.InstallPlan
	data, _ := json.Marshal(old)
	if err := json.Unmarshal(data, &plan); err != nil {
		return plan, err
	}
	client := s.catalog.Client()
	restricted := map[string]bool{}
	for index := range plan.Files {
		file := &plan.Files[index]
		version, err := client.Version(ctx, file.Source, file.ProjectID, file.VersionID)
		if err != nil {
			return plan, err
		}
		if version.Source != file.Source || version.ID != file.VersionID || version.ProjectID != file.ProjectID {
			return plan, errors.New("恢复任务的版本身份已变化")
		}
		fresh, err := market.VersionFileMetadata(version, file.ProjectID, plan.Request.GameVersion)
		if err != nil {
			return plan, err
		}
		if fresh.SHA1 != file.File.SHA1 || fresh.Size != file.File.Size || fresh.Filename != file.File.Filename {
			return plan, errors.New("固定资产文件身份已变化，不能自动改为其他文件")
		}
		if file.Source == market.CurseForge {
			blocked, known := restricted[file.ProjectID]
			if !known {
				project, err := client.ProjectInfo(ctx, file.Source, file.ProjectID, "")
				if err != nil {
					return plan, err
				}
				if project.Kind != plan.Request.Kind {
					return plan, errors.New("恢复任务的项目类别已变化")
				}
				blocked = project.DistributionRestricted
				restricted[file.ProjectID] = blocked
			}
			if blocked {
				fresh.URL = ""
			}
		}
		file.File.URL = fresh.URL
	}
	return plan, nil
}
func PackTaskIdentity(plan ArchiveImportPlan) string {
	files := slices.Clone(plan.ActiveFiles())
	for index := range files {
		files[index].URLs = nil
		files[index].PageURL = ""
	}
	required, _ := plan.Instance.RequiredLoader()
	data, _ := json.Marshal(struct {
		SHA1      string
		Size      int64
		Game, SHA string
		Loader    instance.Loader
		Files     any
		Optional  []string
		World     any
	}{plan.Archive.SHA1, plan.Archive.Size, plan.Instance.Version.ID, plan.Instance.Version.SHA1, required, files, plan.SelectedOptional, plan.WorldFiles})
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
func (s *PackImportService) refreshTaskPlan(ctx context.Context, old ArchiveImportPlan) (ArchiveImportPlan, error) {
	if old.Request.Kind == market.World {
		fresh, _, _, err := s.archives.resolveArchive(ctx, old.Request)
		if err != nil {
			return old, err
		}
		if fresh.SHA1 != old.Archive.SHA1 || fresh.Size != old.Archive.Size || fresh.Filename != old.Archive.Filename {
			return old, errors.New("固定地图压缩包已改变")
		}
		old.Archive.URL = fresh.URL
		return old, nil
	}
	settings := DefaultSettings()
	settings.GameDirectoryID = old.Instance.GameDirectoryID
	fresh, err := s.archives.Inspect(ctx, old.Request, settings, nil)
	if err != nil {
		return fresh, err
	}
	fresh.SelectedOptional = slices.Clone(old.SelectedOptional)
	if PackTaskIdentity(fresh) != PackTaskIdentity(old) {
		return fresh, errors.New("整合包固定清单已改变，请重新确认导入")
	}
	fresh.Instance = old.Instance
	return fresh, nil
}

// Replay reconstructs short-lived URLs only through the original official
// services. It never restores accounts, API keys, proxy credentials or launch
// callbacks; an interrupted launch resumes resource preparation, not a game.
type TaskReplayer struct {
	Installations *InstallationService
	Java          *JavaService
	Market        *MarketInstaller
	Packs         *PackImportService
	Updates       *AssetUpdateService
	Downloads     *download.Client
	Instances     InstanceReader
	Settings      func(context.Context) (Settings, error)
}

func (r *TaskReplayer) Replay(ctx context.Context, intent tasks.Intent) error {
	if err := checkTaskTarget(ctx, r.Instances, intent); err != nil {
		return err
	}
	switch intent.Kind {
	case "game", "preparation":
		var request struct{ ID string }
		if err := json.Unmarshal(intent.Payload, &request); err != nil {
			return err
		}
		if request.ID != intent.InstanceID {
			return errors.New("任务目标身份不符")
		}
		resolver := r.Java.Ensure
		if r.Settings != nil {
			settings, err := r.Settings(ctx)
			if err != nil {
				return err
			}
			resolver = func(task context.Context, major int, target meta.Platform, preferred []string, report func(java.Progress)) (java.Runtime, error) {
				return r.Java.Ensure(task, major, target, append(slices.Clone(preferred), settings.JavaPath), report)
			}
		}
		_, err := r.Installations.RunAutomatic(ctx, request.ID, resolver, nil)
		return err
	case "java":
		var p java.Package
		if err := json.Unmarshal(intent.Payload, &p); err != nil {
			return err
		}
		_, err := r.Java.Install(ctx, p, nil)
		return err
	case "market":
		var plan market.InstallPlan
		if err := json.Unmarshal(intent.Payload, &plan); err != nil {
			return err
		}
		if plan.Request.InstanceID != intent.InstanceID {
			return errors.New("任务目标身份不符")
		}
		fresh, err := r.Market.refreshTaskPlan(ctx, plan)
		if err != nil {
			return err
		}
		return r.Market.Install(ctx, fresh, nil)
	case "pack":
		var plan ArchiveImportPlan
		if err := json.Unmarshal(intent.Payload, &plan); err != nil {
			return err
		}
		if plan.Instance.ID != intent.InstanceID {
			return errors.New("整合包任务身份不符")
		}
		if _, err := r.Instances.Get(ctx, plan.Instance.ID); errors.Is(err, os.ErrNotExist) || plan.Request.Kind == market.World {
			fresh, e := r.Packs.refreshTaskPlan(ctx, plan)
			if e != nil {
				return e
			}
			plan = fresh
		} else if err != nil {
			return err
		}
		_, err := r.Packs.Run(ctx, plan, nil)
		return err
	case "update":
		var plan market.UpdatePlan
		if err := json.Unmarshal(intent.Payload, &plan); err != nil {
			return err
		}
		if plan.Request.InstanceID != intent.InstanceID {
			return errors.New("更新任务身份不符")
		}
		if result, done, err := r.Updates.recoverTask(ctx, plan); err != nil {
			return err
		} else if done && result.Applied {
			return nil
		}
		fresh, err := r.Updates.refreshTaskPlan(ctx, plan)
		if err != nil {
			return err
		}
		_, err = r.Updates.Apply(ctx, fresh, nil)
		return err
	case "archive":
		var saved archiveTask
		if err := json.Unmarshal(intent.Payload, &saved); err != nil {
			return err
		}
		settings := DefaultSettings()
		settings.GameDirectoryID = saved.Directory
		_, err := r.Packs.archives.Inspect(ctx, saved.Request, settings, nil)
		return err
	case "repair":
		var saved repairTask
		if err := json.Unmarshal(intent.Payload, &saved); err != nil {
			return err
		}
		if saved.Plan.InstanceID != intent.InstanceID {
			return errors.New("修复任务实例身份不符")
		}
		current, selected, err := r.Packs.archives.repairTaskPlan(ctx, saved.Plan, saved.Paths)
		if err != nil {
			return err
		}
		if len(selected) == 0 {
			return nil
		}
		_, err = r.Packs.archives.RestorePack(ctx, current, selected, nil)
		return err
	case "files":
		var files []download.File
		if err := json.Unmarshal(intent.Payload, &files); err != nil {
			return err
		}
		if len(files) > tasks.MaxFiles {
			return errors.New("文件任务数量越界")
		}
		for _, f := range files {
			if !strings.HasPrefix(f.Path, "cache/") {
				return errors.New("此下载需要返回原功能重建固定计划")
			}
			if err := f.Validate(); err != nil {
				return errors.New("下载地址需要重新授权，请回原来源页面读取并继续")
			}
		}
		return r.Downloads.Run(ctx, files, nil)
	default:
		return errors.New("这个任务需要返回对应功能重新确认")
	}
}
func (s *AssetUpdateService) refreshTaskPlan(ctx context.Context, old market.UpdatePlan) (market.UpdatePlan, error) {
	if len(old.Changes) == 0 || len(old.Changes) > 128 {
		return market.UpdatePlan{}, errors.New("固定更新计划数量无效")
	}
	var plan market.UpdatePlan
	data, _ := json.Marshal(old)
	if err := json.Unmarshal(data, &plan); err != nil {
		return plan, err
	}
	files := market.InstallPlan{Request: market.InstallRequest{Kind: old.Request.Kind, InstanceID: old.Request.InstanceID, GameVersion: old.Request.GameVersion, Loader: old.Request.Loader, World: old.Request.World}}
	for _, change := range plan.Changes {
		files.Files = append(files.Files, change.After)
	}
	installer := &MarketInstaller{catalog: s.market}
	fresh, err := installer.refreshTaskPlan(ctx, files)
	if err != nil {
		return plan, err
	}
	for index := range plan.Changes {
		plan.Changes[index].After = fresh.Files[index]
	}
	return plan, nil
}

func checkTaskInstance(ctx context.Context, value instance.Instance) error {
	scope := taskScope(ctx)
	if scope == nil {
		return ctx.Err()
	}
	scope.execution.mu.Lock()
	intent := scope.execution.group.Intent
	scope.execution.mu.Unlock()
	if intent.InstanceID == value.ID && intent.Fingerprint != "" && intent.Fingerprint != taskInstanceFingerprint(value) {
		return errors.New("目标实例的位置、版本或加载器已改变，请重新确认任务")
	}
	return ctx.Err()
}

func taskBaseIdentity(ctx context.Context, value instance.Instance, version meta.Version) error {
	if taskScope(ctx) == nil {
		return ctx.Err()
	}
	identity, err := meta.OfficialChainDigest(version, value.Version.SHA1)
	if err != nil {
		return err
	}
	if identity == "" {
		sum := sha256.Sum256([]byte(value.Version.ID + "\n" + value.Version.SHA1))
		identity = hex.EncodeToString(sum[:])
	}
	return TaskIdentity(ctx, "mojang:"+value.Version.ID, identity)
}
