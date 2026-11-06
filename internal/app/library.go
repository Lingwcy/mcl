package app

import (
	"bytes"
	"context"
	"fmt"
	"image/png"
	"slices"
	"strings"
	"sync"
	"time"

	"mcl/internal/core/instance"
	"mcl/internal/core/java"
	"mcl/internal/core/loader"
	"mcl/internal/core/meta"
	"mcl/internal/core/thumbnail"
)

type VersionCatalog interface {
	Manifest(context.Context, bool) (meta.Catalog, error)
	Version(context.Context, meta.ManifestEntry) (meta.Version, error)
}

type InstanceRepository interface {
	List(context.Context) ([]instance.Instance, error)
	Create(context.Context, instance.Instance) error
	Rename(context.Context, string, string) (instance.Instance, error)
}

type LibrarySnapshot struct {
	Instances       []instance.Instance
	InstancesLoaded bool
	Releases        []meta.ManifestEntry
	Cached          bool
	Stale           bool
	Warning         string
}

type CreateInstance struct {
	Name      string
	VersionID string
	Settings  Settings
	Loader    *instance.Loader
	Icon      []byte
	IconStyle string
}

type LoaderReleaseCatalog interface {
	Releases(context.Context, string, string) ([]loader.Release, error)
}

type LibraryService struct {
	catalog  VersionCatalog
	store    InstanceRepository
	platform meta.Platform
	mu       sync.Mutex
	loaders  LoaderReleaseCatalog
}

func NewLibraryService(catalog VersionCatalog, store InstanceRepository, platform meta.Platform) *LibraryService {
	return &LibraryService{catalog: catalog, store: store, platform: platform}
}

func NewLibraryServiceWithLoaders(catalog VersionCatalog, store InstanceRepository, platform meta.Platform, loaders LoaderReleaseCatalog) *LibraryService {
	service := NewLibraryService(catalog, store, platform)
	service.loaders = loaders
	return service
}

func (s *LibraryService) Load(ctx context.Context, refresh bool) (LibrarySnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	instances, err := s.store.List(ctx)
	if err != nil {
		return LibrarySnapshot{}, err
	}
	snapshot := LibrarySnapshot{Instances: instances, InstancesLoaded: true}
	catalog, err := s.catalog.Manifest(ctx, refresh)
	if err != nil {
		return snapshot, err
	}
	snapshot.Releases = catalog.Manifest.Releases()
	snapshot.Cached, snapshot.Stale = catalog.Cached, catalog.Stale
	if catalog.Warning != nil {
		if catalog.Stale {
			snapshot.Warning = "版本服务暂不可用，正在显示已缓存的版本列表。"
		} else {
			snapshot.Warning = "版本列表已获取，但未能写入本机缓存。"
		}
	}
	return snapshot, nil
}

func (s *LibraryService) Create(ctx context.Context, request CreateInstance) (instance.Instance, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	request.Name = strings.TrimSpace(request.Name)
	request.Settings.JavaPath = strings.TrimSpace(request.Settings.JavaPath)
	if err := instance.ValidateName(request.Name); err != nil {
		return instance.Instance{}, err
	}
	if err := request.Settings.Validate(); err != nil {
		return instance.Instance{}, err
	}
	catalog, err := s.catalog.Manifest(ctx, false)
	if err != nil {
		return instance.Instance{}, err
	}
	entry, ok := catalog.Manifest.Find(request.VersionID)
	if !ok || entry.Type != "release" {
		return instance.Instance{}, fmt.Errorf("找不到原版正式版 %q", request.VersionID)
	}
	version, err := s.catalog.Version(ctx, entry)
	if err != nil {
		return instance.Instance{}, err
	}
	if err := version.ValidateVanilla(); err != nil {
		return instance.Instance{}, err
	}
	if request.Loader != nil {
		if !instance.ValidLoader(*request.Loader) || s.loaders == nil {
			return instance.Instance{}, fmt.Errorf("加载器选择无效或版本服务未配置")
		}
		releases, err := s.loaders.Releases(ctx, request.Loader.Type, entry.ID)
		if err != nil {
			return instance.Instance{}, fmt.Errorf("核对加载器版本: %w", err)
		}
		if !slices.ContainsFunc(releases, func(release loader.Release) bool { return release.Version == request.Loader.Version }) {
			return instance.Instance{}, fmt.Errorf("所选加载器版本不在该 Minecraft 版本的官方目录中")
		}
	}
	if !instance.ValidIconStyle(request.IconStyle) {
		return instance.Instance{}, fmt.Errorf("实例图标选择无效")
	}
	var icon []byte
	if len(request.Icon) > 0 {
		image, err := thumbnail.Decode(request.Icon)
		if err != nil {
			return instance.Instance{}, err
		}
		var normalized bytes.Buffer
		if err := png.Encode(&normalized, image); err != nil {
			return instance.Instance{}, err
		}
		icon = normalized.Bytes()
	}
	if request.Settings.JavaPath != "" {
		runtime, err := java.Probe(ctx, request.Settings.JavaPath)
		if err != nil {
			return instance.Instance{}, err
		}
		if err := runtime.Check(version.Java.Major, s.platform); err != nil {
			return instance.Instance{}, err
		}
	}
	value := instance.Instance{
		ID: instance.NewID(), Name: request.Name, Version: entry, RequiredJava: version.Java.Major,
		JavaPath: request.Settings.JavaPath, CreatedAt: time.Now().UTC(),
	}
	value.Icon, value.IconStyle = icon, request.IconStyle
	if request.Loader != nil {
		copy := *request.Loader
		value.Loader = &copy
	}
	if err := s.store.Create(ctx, value); err != nil {
		return instance.Instance{}, fmt.Errorf("创建实例失败: %w", err)
	}
	return value, nil
}

func (s *LibraryService) Rename(ctx context.Context, id, name string) (instance.Instance, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.store.Rename(ctx, id, strings.TrimSpace(name))
}
