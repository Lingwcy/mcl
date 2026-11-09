package app

import (
	"context"
	"fmt"
	"strings"

	"mcl/internal/core/diagnostics"
	"mcl/internal/core/download"
	"mcl/internal/core/install"
	"mcl/internal/core/instance"
	"mcl/internal/core/loader"
	"mcl/internal/core/meta"
)

type LoaderRepository interface {
	Load(context.Context, loader.Request) (loader.Profile, error)
	Save(context.Context, loader.Profile) error
}

type LoaderCatalog interface {
	Resolve(context.Context, loader.Request) (loader.Profile, error)
}

type LoaderService struct {
	catalog LoaderCatalog
	store   LoaderRepository
	forge   *loader.ForgeInstaller
}

func NewLoaderService(catalog LoaderCatalog, store LoaderRepository, forge *loader.ForgeInstaller) *LoaderService {
	return &LoaderService{catalog: catalog, store: store, forge: forge}
}

type resolvedVersion struct {
	evidence           diagnostics.Facts
	platform           meta.Platform
	version            meta.Version
	profileFingerprint string
	libraryPaths       map[string]bool
}

func (s *LoaderService) resolve(ctx context.Context, value instance.Instance, base meta.Version, fallbackJava string, target meta.Platform, report func(install.Progress)) (resolvedVersion, error) {
	required, ok := value.RequiredLoader()
	if !ok {
		return resolvedVersion{version: base}, nil
	}
	diagnostics.SetFacts(ctx, diagnostics.Facts{Game: value.Version.ID, Loader: required.Type, LoaderVersion: required.Version})
	ctx, evidence := diagnostics.Start(ctx, "work", required.Type+":"+required.Version)
	defer evidence.End()
	request, err := (loader.Request{Type: required.Type, Version: required.Version, GameVersion: value.Version.ID, BaseSHA1: strings.ToLower(value.Version.SHA1)}).WithBase(base)
	if err != nil {
		return resolvedVersion{}, err
	}
	request, err = request.WithPlatform(target)
	if err != nil {
		return resolvedVersion{}, err
	}
	requested := request
	knownAlias := false
	if aliases, ok := s.store.(interface {
		LoadRequestAlias(context.Context, loader.Request) (loader.Request, error)
	}); ok && request.Type == "forge" {
		canonical, err := aliases.LoadRequestAlias(ctx, request)
		if err != nil {
			return resolvedVersion{}, fmt.Errorf("读取 Forge 固定构建映射: %w", err)
		}
		if err := loader.ValidateForgeAlias(request, canonical); err != nil {
			return resolvedVersion{}, err
		}
		knownAlias = canonical != request
		request = canonical
	}
	profile, cacheErr := s.store.Load(ctx, request)
	if cacheErr != nil && request.Type == "forge" {
		if canonicalizer, ok := s.catalog.(interface {
			CanonicalForgeRequest(context.Context, loader.Request) (loader.Request, error)
		}); ok {
			canonical, err := canonicalizer.CanonicalForgeRequest(ctx, request)
			if err != nil {
				return resolvedVersion{}, fmt.Errorf("解析 Forge %s 完整发行编号: %w", required.Version, err)
			}
			if err := loader.ValidateForgeAlias(request, canonical); err != nil {
				return resolvedVersion{}, err
			}
			if canonical != request {
				request = canonical
				profile, cacheErr = s.store.Load(ctx, request)
			}
		}
	}
	diagnostics.SetFacts(ctx, diagnostics.Facts{Loader: request.Type, LoaderVersion: request.Version})
	if cacheErr != nil {
		if err := ctx.Err(); err != nil {
			return resolvedVersion{}, err
		}
		var err error
		if loader.UsesProcessors(required.Type) {
			if s.forge == nil {
				return resolvedVersion{}, fmt.Errorf("%s 处理器服务未配置", required.Type)
			}
			javaPath := value.JavaPath
			if javaPath == "" {
				javaPath = fallbackJava
			}
			profile, err = s.forge.Prepare(ctx, request, base, javaPath, target, func(stage string, p download.Progress) {
				if report != nil {
					report(install.Progress{Stage: stage, Progress: p})
				}
			})
		} else {
			profile, err = s.catalog.Resolve(ctx, request)
		}
		if err != nil {
			return resolvedVersion{}, fmt.Errorf("配置 %s %s: %w", required.Type, required.Version, err)
		}
		if profile.Request != request {
			return resolvedVersion{}, fmt.Errorf("%s 配置身份不匹配", required.Type)
		}
		if err := s.store.Save(ctx, profile); err != nil {
			return resolvedVersion{}, fmt.Errorf("保存加载器配置: %w", err)
		}
	}
	if profile.Request != request {
		return resolvedVersion{}, fmt.Errorf("加载器缓存与当前完整基底身份不匹配")
	}

	effective, err := profile.Merge(base)
	if err != nil {
		return resolvedVersion{}, err
	}
	if requested != request && !knownAlias {
		if aliases, ok := s.store.(interface {
			SaveRequestAlias(context.Context, loader.Request, loader.Request) error
		}); ok {
			if err := aliases.SaveRequestAlias(ctx, requested, request); err != nil {
				return resolvedVersion{}, fmt.Errorf("保存 Forge 固定构建映射: %w", err)
			}
		}
	}
	paths := make(map[string]bool, len(profile.Libraries))
	for _, library := range profile.Libraries {
		paths["cache/libraries/"+library.Downloads.Artifact.Path] = true
	}
	return resolvedVersion{version: effective, profileFingerprint: profile.Fingerprint(), libraryPaths: paths, evidence: diagnostics.Facts{Loader: request.Type, LoaderVersion: request.Version}}, nil
}
