package app

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"mcl/internal/core/download"
	"mcl/internal/core/market"
)

type AssetDigest struct {
	File   AssetFile
	Digest market.FileDigest
}
type AssetHashSnapshot struct {
	Files     []AssetDigest
	Skipped   int
	Truncated bool
	Warnings  []string
}
type AssetIdentification struct {
	Asset             AssetDigest
	Origin            *market.InstallFile
	RequireUniquePath bool
}
type AssetIdentityProgress struct {
	Stage string
	download.Progress
}
type AssetIdentityResult struct {
	InstanceID                          string
	Source                              market.Source
	Hashed, Matched, Unmatched, Skipped int
	Truncated                           bool
	Warnings                            []string
}

type AssetIdentityRepository interface {
	HashAssets(context.Context, string, market.Source, func(download.Progress)) (AssetHashSnapshot, error)
	HashAsset(context.Context, string, AssetFile, bool) (AssetDigest, error)
	RecordIdentities(context.Context, string, market.Source, []AssetIdentification) (int, error)
}

type AssetIdentityService struct {
	instances InstanceReader
	registry  SessionRegistry
	store     AssetIdentityRepository
	market    *MarketService
}

func NewAssetIdentityService(instances InstanceReader, registry SessionRegistry, store AssetIdentityRepository, catalog *MarketService) *AssetIdentityService {
	return &AssetIdentityService{instances: instances, registry: registry, store: store, market: catalog}
}

// Identify is explicit, cancellable work. It never writes a game file and
// commits completed lookup batches independently; a later failure retains
// earlier verified identities rather than report the whole scan as successful.
func (s *AssetIdentityService) Identify(ctx context.Context, id string, source market.Source, report func(AssetIdentityProgress)) (AssetIdentityResult, error) {
	result := AssetIdentityResult{InstanceID: id, Source: source}
	if source != market.Modrinth && source != market.CurseForge {
		return result, errors.New("未知识别来源")
	}
	client := s.market.Client()
	if source == market.CurseForge && !client.CurseForgeAvailable() {
		return result, market.ErrAPIKey
	}
	if s.registry == nil {
		return result, errors.New("实例会话保护未配置")
	}
	lease, err := s.registry.Reserve(ctx, id)
	if err != nil {
		return result, err
	}
	defer lease.Close()
	if _, err := s.instances.Get(ctx, id); err != nil {
		return result, err
	}
	publish := func(stage string, p download.Progress) {
		if report != nil {
			report(AssetIdentityProgress{Stage: stage, Progress: p})
		}
	}
	snapshot, err := s.store.HashAssets(ctx, id, source, func(p download.Progress) { publish("identify-hash", p) })
	result.Hashed, result.Skipped, result.Truncated = len(snapshot.Files), snapshot.Skipped, snapshot.Truncated
	result.Warnings = slices.Clone(snapshot.Warnings)
	if err != nil {
		return result, err
	}
	projects := make(map[string]market.Project)
	for start := 0; start < len(snapshot.Files); start += 100 {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		batch := snapshot.Files[start:min(start+100, len(snapshot.Files))]
		publish("identify-lookup", download.Progress{Files: start, TotalFiles: len(snapshot.Files)})
		digests := make([]market.FileDigest, 0, len(batch))
		for _, file := range batch {
			digests = append(digests, file.Digest)
		}
		versions, err := client.IdentifyFiles(ctx, source, digests)
		if err != nil {
			return result, fmt.Errorf("查询文件来源: %w", err)
		}
		var ids []string
		for _, version := range versions {
			if _, known := projects[version.ProjectID]; !known {
				ids = append(ids, version.ProjectID)
			}
		}
		slices.Sort(ids)
		ids = slices.Compact(ids)
		for offset := 0; offset < len(ids); offset += 20 {
			publish("identify-projects", download.Progress{Files: start, TotalFiles: len(snapshot.Files)})
			found, err := client.ProjectSummaries(ctx, source, ids[offset:min(offset+20, len(ids))])
			if err != nil {
				return result, fmt.Errorf("读取来源项目: %w", err)
			}
			for key, project := range found {
				projects[key] = project
			}
		}
		observations := make([]AssetIdentification, 0, len(batch))
		for _, asset := range batch {
			observation := AssetIdentification{Asset: asset}
			if version, found := versions[asset.Digest.SHA1]; found {
				file, matched := market.FileByDigest(version, asset.Digest)
				if !matched {
					return result, errors.New("文件来源与本地摘要不匹配")
				}
				metadata := market.MetadataFromVersion(version)
				if project, found := projects[version.ProjectID]; found {
					metadata.WithProject(project)
				}
				metadata.IdentifiedAt = time.Now().UTC()
				if err := metadata.Validate(); err != nil {
					return result, err
				}
				observation.Origin = &market.InstallFile{Source: source, ProjectID: version.ProjectID, VersionID: version.ID, Path: strings.TrimSuffix(asset.File.Path, ".disabled"), File: file, Metadata: metadata}
			}
			observations = append(observations, observation)
		}
		publish("identify-save", download.Progress{Files: start, TotalFiles: len(snapshot.Files)})
		matched, err := s.store.RecordIdentities(ctx, id, source, observations)
		if err != nil {
			return result, fmt.Errorf("保存文件来源: %w", err)
		}
		result.Matched += matched
		result.Unmatched += len(batch) - matched
		publish("identify-save", download.Progress{Files: start + len(batch), TotalFiles: len(snapshot.Files)})
	}
	return result, nil
}
