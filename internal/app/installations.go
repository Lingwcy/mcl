package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"sync"
	"time"

	"mcl/internal/core/download"
	"mcl/internal/core/install"
	"mcl/internal/core/instance"
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
	catalog   VersionCatalog
	instances InstanceReader
	receipts  InstallationRepository
	client    *download.Client
	platform  meta.Platform
	registry  SessionRegistry
	loaders   *LoaderService
	mu        sync.Mutex
	active    map[string]bool
	slot      chan struct{}
}

func NewInstallationService(catalog VersionCatalog, instances InstanceReader, receipts InstallationRepository, client *download.Client, platform meta.Platform, registry SessionRegistry, loaders *LoaderService) *InstallationService {
	return &InstallationService{catalog: catalog, instances: instances, receipts: receipts, client: client, platform: platform, registry: registry, loaders: loaders, active: make(map[string]bool), slot: make(chan struct{}, 1)}
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
		if receipt.Matches(value, s.platform) {
			result[value.ID] = receipt
		}
	}
	return result, errors.Join(failures...)
}

func (s *InstallationService) Run(ctx context.Context, id string, report func(install.Progress)) (install.Receipt, error) {
	return s.RunWithJava(ctx, id, "", report)
}

func (s *InstallationService) RunWithJava(ctx context.Context, id, fallbackJava string, report func(install.Progress)) (install.Receipt, error) {
	reserved, err := s.reserve(ctx, id)
	if err != nil {
		return install.Receipt{}, err
	}
	defer reserved.Close()
	return s.run(ctx, id, fallbackJava, report)
}

// reserve is shared by installation and launch preparation. The owner may
// transfer the release to a live session, which prevents repair/native cleanup
// while that instance's process is running.
func (s *InstallationService) reserve(ctx context.Context, id string) (*reservation, error) {
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
	resolved, err := s.resolveVersion(ctx, value, fallbackJava, report)
	if err != nil {
		return install.Receipt{}, err
	}
	return s.execute(ctx, value, resolved, report)
}

func (s *InstallationService) resolveVersion(ctx context.Context, value instance.Instance, fallbackJava string, report func(install.Progress)) (resolvedVersion, error) {
	if report != nil {
		report(install.Progress{Stage: "metadata"})
	}
	version, err := s.catalog.Version(ctx, value.Version)
	if err != nil {
		return resolvedVersion{}, err
	}
	return s.resolveLoader(ctx, value, version, fallbackJava, report)
}

func (s *InstallationService) resolveLoader(ctx context.Context, value instance.Instance, version meta.Version, fallbackJava string, report func(install.Progress)) (resolvedVersion, error) {
	if _, required := value.RequiredLoader(); !required {
		return resolvedVersion{version: version}, nil
	}
	if s.loaders == nil {
		return resolvedVersion{}, errors.New("此实例要求的加载器服务未配置")
	}
	if report != nil {
		report(install.Progress{Stage: "loader"})
	}
	return s.loaders.resolve(ctx, value, version, fallbackJava, s.platform, report)
}

// execute receives the exact metadata already used for Java validation and
// launch preview. The caller holds the instance reservation throughout.
func (s *InstallationService) execute(ctx context.Context, value instance.Instance, resolved resolvedVersion, report func(install.Progress)) (install.Receipt, error) {
	id := value.ID
	if report != nil {
		report(install.Progress{Stage: "queued"})
	}
	select {
	case s.slot <- struct{}{}:
		defer func() { <-s.slot }()
	case <-ctx.Done():
		return install.Receipt{}, ctx.Err()
	}
	previous, err := s.receipts.Load(ctx, id)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return install.Receipt{}, fmt.Errorf("安装记录损坏，原文件已保留: %w", err)
	}
	plan, err := install.Build(resolved.version, s.platform)
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
	result, err := install.Execute(ctx, s.client, plan, parent, report)
	if err != nil {
		return install.Receipt{}, err
	}
	receipt := install.Receipt{InstanceID: id, VersionID: value.Version.ID, VersionSHA1: value.Version.SHA1, OS: s.platform.OS, Arch: meta.NormalizeArch(s.platform.Arch), NativeDirectory: result.NativeDirectory, CompletedAt: time.Now().UTC()}
	if required, ok := value.RequiredLoader(); ok {
		receipt.Loader, receipt.LoaderFingerprint = &required, resolved.profileFingerprint
	}
	if err := s.receipts.Save(ctx, receipt); err != nil {
		install.RemoveNativeDirectory(root, parent, result.NativeDirectory)
		return install.Receipt{}, fmt.Errorf("保存安装结果: %w", err)
	}
	// Only a fully committed generation replaces the old one. This application's
	// reservation excludes its live sessions from preparation/native cleanup.
	if previous.NativeDirectory != "" && previous.NativeDirectory != receipt.NativeDirectory {
		install.RemoveNativeDirectory(root, parent, previous.NativeDirectory)
	}
	return receipt, nil
}
