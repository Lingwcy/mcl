package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"sync"
	"time"

	"mcl/internal/core/diagnostics"
	"mcl/internal/core/download"
	"mcl/internal/core/install"
	"mcl/internal/core/instance"
	"mcl/internal/core/java"
	"mcl/internal/core/loader"
	"mcl/internal/core/meta"
)

type InstallationRepository interface {
	Load(context.Context, string) (install.Receipt, error)
	Save(context.Context, install.Receipt) error
}
type InstanceReader interface {
	Get(context.Context, string) (instance.Instance, error)
	List(context.Context) ([]instance.Instance, error)
}

// InstallationService serializes preparation of an instance and caps active
// installations at one. Downloads within it are concurrent; UI work and library
// navigation remain independent. LaunchService transfers the same instance
// reservation to a live game. The registry combines an OS lock with persisted
// kernel process identity to protect sessions left by a previous launcher.
type InstallationService struct {
	RuntimeEvidence        diagnostics.Facts
	Tasks                  *TaskService
	CheckExecutionPlatform func(context.Context, meta.Platform, meta.Platform) error
	openGameDirectory      func(context.Context, string) (*os.Root, error)
	gameDirectory          GameDirectoryResolver
	catalog                VersionCatalog
	instances              InstanceReader
	receipts               InstallationRepository
	client                 *download.Client
	platform               meta.Platform
	registry               SessionRegistry
	loaders                *LoaderService
	mu                     sync.Mutex
	active                 map[string]bool
	slot                   chan struct{}
}

func NewInstallationService(catalog VersionCatalog, instances InstanceReader, receipts InstallationRepository, client *download.Client, platform meta.Platform, registry SessionRegistry, loaders *LoaderService, directories ...GameDirectoryResolver) *InstallationService {
	var directory GameDirectoryResolver
	if len(directories) > 0 {
		directory = directories[0]
	}
	return &InstallationService{gameDirectory: directory, catalog: catalog, instances: instances, receipts: receipts, client: client, platform: platform, registry: registry, loaders: loaders, active: make(map[string]bool), slot: make(chan struct{}, 1)}
}

// NewInstallationServiceWithGameDirectory adds the ownership-validating opener
// before the service is shared with workers. Existing internal-only users keep
// the original constructor; external directories never use a pathname fallback.
func NewInstallationServiceWithGameDirectory(catalog VersionCatalog, instances InstanceReader, receipts InstallationRepository, client *download.Client, platform meta.Platform, registry SessionRegistry, loaders *LoaderService, directory GameDirectoryResolver, open func(context.Context, string) (*os.Root, error)) *InstallationService {
	s := NewInstallationService(catalog, instances, receipts, client, platform, registry, loaders, directory)
	s.openGameDirectory = open
	return s
}

func (s *InstallationService) Load(ctx context.Context) (map[string]install.Receipt, error) {
	values, err := s.instances.List(ctx)
	if err != nil {
		return nil, err
	}
	result := make(map[string]install.Receipt)
	var failures []error
	for _, value := range values {
		receipt, err := s.receipts.Load(ctx, value.ID)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			failures = append(failures, fmt.Errorf("实例 %s 的安装记录无法读取: %w", value.Name, err))
			continue
		}
		target := s.platform
		if target.OS == "osx" && meta.NormalizeArch(target.Arch) == "arm64" && receipt.OS == "osx" && receipt.Arch == "amd64" && s.CheckExecutionPlatform != nil {
			candidate := target
			candidate.Arch = "amd64"
			if s.CheckExecutionPlatform(ctx, s.platform, candidate) == nil {
				target = candidate
			}
		}
		if receipt.Matches(value, target) {
			result[value.ID] = receipt
		}
	}
	return result, errors.Join(failures...)
}

func (s *InstallationService) Run(ctx context.Context, id string, report func(install.Progress)) (install.Receipt, error) {
	return s.RunWithJava(ctx, id, "", report)
}

func (s *InstallationService) RunWithJava(ctx context.Context, id, fallbackJava string, report func(install.Progress)) (install.Receipt, error) {
	if s.Tasks != nil && taskScope(ctx) == nil {
		intent, err := taskGameIntent(ctx, s.instances, id, "game")
		if err != nil {
			return install.Receipt{}, err
		}
		return performTask(ctx, s.Tasks, intent, func(task context.Context) (install.Receipt, error) {
			if err := checkTaskTarget(task, s.instances, intent); err != nil {
				return install.Receipt{}, err
			}
			return s.RunWithJava(task, id, fallbackJava, report)
		})
	}
	reserved, err := s.reserve(ctx, id)
	if err != nil {
		return install.Receipt{}, err
	}
	defer reserved.Close()
	value, err := s.instances.Get(ctx, id)
	if err != nil {
		return install.Receipt{}, err
	}
	if err := checkTaskInstance(ctx, value); err != nil {
		return install.Receipt{}, err
	}
	return s.run(ctx, id, fallbackJava, report)
}

// reserve is shared by installation and launch preparation. The owner may
// transfer the release to a live session, which prevents repair/native cleanup
// while that instance's process is running.
func (s *InstallationService) reserve(ctx context.Context, id string) (*reservation, error) {
	diagnostics.SetFacts(ctx, s.RuntimeEvidence)
	if !instance.ValidID(id) {
		return nil, errors.New("invalid instance ID")
	}
	s.mu.Lock()
	if s.active[id] {
		s.mu.Unlock()
		return nil, errors.New("此实例已有安装、启动准备或游戏进程")
	}
	s.active[id] = true
	s.mu.Unlock()
	if s.registry == nil {
		s.mu.Lock()
		delete(s.active, id)
		s.mu.Unlock()
		return nil, errors.New("实例会话保护未配置")
	}
	lease, err := s.registry.Reserve(ctx, id)
	if err != nil {
		s.mu.Lock()
		delete(s.active, id)
		s.mu.Unlock()
		return nil, err
	}
	var once sync.Once
	return &reservation{lease: lease, release: func() { once.Do(func() { lease.Close(); s.mu.Lock(); delete(s.active, id); s.mu.Unlock() }) }}, nil
}

func (s *InstallationService) run(ctx context.Context, id, fallbackJava string, report func(install.Progress)) (install.Receipt, error) {
	value, err := s.instances.Get(ctx, id)
	if err != nil {
		return install.Receipt{}, err
	}
	if err := checkTaskInstance(ctx, value); err != nil {
		return install.Receipt{}, err
	}
	if s.gameDirectory != nil {
		if _, err := s.gameDirectory(ctx, id); err != nil {
			return install.Receipt{}, fmt.Errorf("读取实例游戏目录: %w", err)
		}
	} else if value.GameDirectoryID != "" {
		return install.Receipt{}, errors.New("外部游戏目录解析未配置")
	}
	resolved, err := s.resolveVersion(ctx, value, fallbackJava, report)
	if err != nil {
		return install.Receipt{}, err
	}
	return s.execute(ctx, value, resolved, report)
}

func (s *InstallationService) resolveVersion(ctx context.Context, value instance.Instance, fallbackJava string, report func(install.Progress)) (resolvedVersion, error) {
	diagnostics.SetFacts(ctx, diagnostics.Facts{Provider: "mojang", Dialect: "official", Game: value.Version.ID, LauncherArch: meta.CurrentPlatform().Arch})
	if err := TaskStage(ctx, "读取官方元数据"); err != nil {
		return resolvedVersion{}, err
	}
	if report != nil {
		report(install.Progress{Stage: "metadata"})
	}
	version, err := s.catalog.Version(ctx, value.Version)
	if err != nil {
		return resolvedVersion{}, err
	}
	if err := taskBaseIdentity(ctx, value, version); err != nil {
		return resolvedVersion{}, err
	}
	return s.resolveLoader(ctx, value, version, fallbackJava, report)
}

func (s *InstallationService) resolveLoader(ctx context.Context, value instance.Instance, version meta.Version, fallbackJava string, report func(install.Progress)) (resolvedVersion, error) {
	if err := TaskStage(ctx, "准备加载器"); err != nil {
		return resolvedVersion{}, err
	}
	target, err := s.targetPlatform(ctx, version)
	if err != nil {
		return resolvedVersion{}, err
	}
	diagnostics.SetFacts(ctx, diagnostics.Facts{TargetOS: target.OS, TargetArch: target.Arch})
	if _, required := value.RequiredLoader(); !required {
		diagnostics.SetFacts(ctx, diagnostics.Facts{Loader: "vanilla"})
		return resolvedVersion{version: version, platform: target}, nil
	}
	if s.loaders == nil {
		return resolvedVersion{}, errors.New("此实例要求的加载器服务未配置")
	}
	if report != nil {
		report(install.Progress{Stage: "loader"})
	}
	resolved, err := s.loaders.resolve(ctx, value, version, fallbackJava, target, report)
	resolved.platform = target
	return resolved, err
}

// execute receives the exact metadata already used for Java validation and
// launch preview. The caller holds the instance reservation throughout.
func (s *InstallationService) execute(ctx context.Context, value instance.Instance, resolved resolvedVersion, report func(install.Progress)) (install.Receipt, error) {
	id := value.ID
	if report != nil {
		report(install.Progress{Stage: "queued"})
	}
	_, waiting := diagnostics.Start(ctx, "installation-slot", "installation-budget")
	select {
	case s.slot <- struct{}{}:
		waiting.End()
		defer func() { <-s.slot }()
	case <-ctx.Done():
		waiting.End()
		return install.Receipt{}, ctx.Err()
	}
	previous, err := s.receipts.Load(ctx, id)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return install.Receipt{}, fmt.Errorf("安装记录损坏，原文件已保留: %w", err)
	}
	plan, err := install.Build(resolved.version, s.resolvedPlatform(resolved))
	if err != nil {
		return install.Receipt{}, err
	}
	for index := range plan.Files {
		if resolved.libraryPaths[plan.Files[index].Path] {
			plan.Files[index].AllowedHosts = loader.LibraryHosts()
		}
	}
	parent := path.Join("instances", id, "natives")
	root, err := os.OpenRoot(s.client.Root)
	if err != nil {
		return install.Receipt{}, err
	}
	defer root.Close()
	if err := install.CleanupNatives(root, parent, previous.NativeDirectory); err != nil {
		return install.Receipt{}, fmt.Errorf("清理未完成的本地库: %w", err)
	}
	if err := install.CleanupLegacyAssets(ctx, root, id, previous.LegacyAssetsDirectory); err != nil {
		return install.Receipt{}, fmt.Errorf("清理未完成历史资源: %w", err)
	}
	var game *os.Root
	if s.openGameDirectory != nil {
		game, err = s.openGameDirectory(ctx, id)
		if err != nil {
			return install.Receipt{}, err
		}
		defer game.Close()
	}
	// map_to_resources requires the injected, ownership-validated game root.
	// A missing opener is an explicit failure, never dataRoot fallback.
	result, err := install.Execute(ctx, s.client, plan, parent, report, game)
	if err != nil {
		return install.Receipt{}, err
	}
	receipt := install.Receipt{InstanceID: id, VersionID: value.Version.ID, VersionSHA1: value.Version.SHA1, OS: plan.Platform.OS, Arch: meta.NormalizeArch(plan.Platform.Arch), NativeDirectory: result.NativeDirectory, LegacyAssetsDirectory: result.LegacyAssetsDirectory, CompletedAt: time.Now().UTC()}
	if required, ok := value.RequiredLoader(); ok {
		receipt.Loader, receipt.LoaderFingerprint = &required, resolved.profileFingerprint
	}
	if err := TaskStage(ctx, "发布"); err != nil {
		return install.Receipt{}, err
	}
	if err := s.receipts.Save(ctx, receipt); err != nil {
		install.RemoveNativeDirectory(root, parent, result.NativeDirectory)
		install.RemoveLegacyAssets(context.WithoutCancel(ctx), root, id, result.LegacyAssetsDirectory)
		return install.Receipt{}, fmt.Errorf("保存安装结果: %w", err)
	}
	// Only a fully committed generation replaces the old one. This application's
	// reservation excludes its live sessions from preparation/native cleanup.
	if previous.NativeDirectory != "" && previous.NativeDirectory != receipt.NativeDirectory {
		install.RemoveNativeDirectory(root, parent, previous.NativeDirectory)
	}
	if previous.LegacyAssetsDirectory != "" && previous.LegacyAssetsDirectory != receipt.LegacyAssetsDirectory {
		install.RemoveLegacyAssets(context.WithoutCancel(ctx), root, id, previous.LegacyAssetsDirectory)
	}
	return receipt, nil
}

// RunAutomatic installs a pack's client and fixed loader using a verified Java
// runtime. No game/account is launched and no heap budget is needed to install.
func (s *InstallationService) RunAutomatic(ctx context.Context, id string, resolveJava JavaResolver, report func(install.Progress)) (install.Receipt, error) {
	if s.Tasks != nil && taskScope(ctx) == nil {
		intent, err := taskGameIntent(ctx, s.instances, id, "game")
		if err != nil {
			return install.Receipt{}, err
		}
		return performTask(ctx, s.Tasks, intent, func(task context.Context) (install.Receipt, error) {
			if err := checkTaskTarget(task, s.instances, intent); err != nil {
				return install.Receipt{}, err
			}
			return s.RunAutomatic(task, id, resolveJava, report)
		})
	}
	if resolveJava == nil {
		return install.Receipt{}, errors.New("Java 自动安装服务尚未配置")
	}
	reserved, err := s.reserve(ctx, id)
	if err != nil {
		return install.Receipt{}, err
	}
	defer reserved.Close()
	value, err := s.instances.Get(ctx, id)
	if err != nil {
		return install.Receipt{}, err
	}
	if err := checkTaskInstance(ctx, value); err != nil {
		return install.Receipt{}, err
	}
	if s.gameDirectory != nil {
		if _, err := s.gameDirectory(ctx, id); err != nil {
			return install.Receipt{}, err
		}
	} else if value.GameDirectoryID != "" {
		return install.Receipt{}, errors.New("外部游戏目录未配置")
	}
	if report != nil {
		report(install.Progress{Stage: "metadata"})
	}
	if err := TaskStage(ctx, "读取官方元数据"); err != nil {
		return install.Receipt{}, err
	}
	base, err := s.catalog.Version(ctx, value.Version)
	if err != nil {
		return install.Receipt{}, err
	}
	if err := taskBaseIdentity(ctx, value, base); err != nil {
		return install.Receipt{}, err
	}
	target, err := s.targetPlatform(ctx, base)
	if err != nil {
		return install.Receipt{}, err
	}
	preferred := []string{value.JavaPath}
	javaProgress := func(p java.Progress) {
		if report != nil {
			report(install.Progress{Stage: p.Stage, Progress: download.Progress{Bytes: p.Bytes, TotalBytes: p.Total}})
		}
	}
	resolved, err := s.resolveLoader(ctx, value, base, "", report)
	if err != nil {
		return install.Receipt{}, err
	}
	requirement, err := fixedGameRequirement(ctx, resolved.version.Java.Major, target)
	if err != nil {
		return install.Receipt{}, err
	}
	runtime, err := resolveRequiredGameJava(ctx, resolveJava, resolved.version.Java.Major, requirement, preferred, javaProgress)
	if err != nil {
		return install.Receipt{}, err
	}
	if err := requirement.CheckRuntime(runtime); err != nil {
		return install.Receipt{}, err
	}
	return s.execute(ctx, value, resolved, report)
}
