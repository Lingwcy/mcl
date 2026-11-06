package app

import (
	"context"
	"errors"
	"fmt"
	"path"
	"slices"
	"strings"
	"time"

	"mcl/internal/core/market"
)

// AssetAssociation checks a fixed release, the listed releases of a project,
// or the source's digest index when both IDs are empty.
type AssetAssociation struct {
	InstanceID string
	File       AssetFile
	Source     market.Source
	ProjectID  string
	VersionID  string
}

func (s *AssetIdentityService) Associate(ctx context.Context, request AssetAssociation) (market.InstallFile, error) {
	var origin market.InstallFile
	if request.Source != market.Modrinth && request.Source != market.CurseForge {
		return origin, errors.New("未知资产来源")
	}
	lookup := request.ProjectID == "" && request.VersionID == ""
	if !lookup && (!market.ValidID(request.ProjectID) || (request.VersionID != "" && !market.ValidID(request.VersionID))) {
		return origin, errors.New("请选择有效的市场项目或版本")
	}
	client := s.market.Client()
	if request.Source == market.CurseForge && !client.CurseForgeAvailable() {
		return origin, market.ErrAPIKey
	}
	if s.registry == nil {
		return origin, errors.New("实例会话保护未配置")
	}
	lease, err := s.registry.Reserve(ctx, request.InstanceID)
	if err != nil {
		return origin, err
	}
	defer lease.Close()
	if _, err := s.instances.Get(ctx, request.InstanceID); err != nil {
		return origin, err
	}
	asset, err := s.store.HashAsset(ctx, request.InstanceID, request.File, lookup && request.Source == market.CurseForge)
	if err != nil {
		return origin, err
	}
	var version market.Version
	switch {
	case lookup:
		versions, err := client.IdentifyFiles(ctx, request.Source, []market.FileDigest{asset.Digest})
		if err != nil {
			return origin, err
		}
		version = versions[asset.Digest.SHA1]
		if version.ID == "" {
			return origin, errors.New("未找到相同文件，可以选择市场版本核对")
		}
	case request.VersionID == "":
		versions, err := client.Versions(ctx, request.Source, request.ProjectID, "", "")
		if err != nil {
			return origin, err
		}
		for _, candidate := range versions {
			if candidate.Source != request.Source || candidate.ProjectID != request.ProjectID || !market.ValidID(candidate.ID) {
				continue
			}
			if _, found := market.FileByDigest(candidate, asset.Digest); found {
				if version.ID != "" && version.ID != candidate.ID {
					return origin, errors.New("多个版本包含同一文件，请选择要关联的版本")
				}
				version = candidate
			}
		}
		if version.ID == "" {
			return origin, errors.New("项目返回的版本中未找到相同文件")
		}
	default:
		version, err = client.Version(ctx, request.Source, request.ProjectID, request.VersionID)
		if err != nil {
			return origin, err
		}
		if version.ID != request.VersionID || version.ProjectID != request.ProjectID {
			return origin, errors.New("市场版本身份不匹配")
		}
	}
	if version.Source != request.Source || !market.ValidID(version.ProjectID) || !market.ValidID(version.ID) {
		return origin, errors.New("市场版本来源无效")
	}
	remote, ok := market.FileByDigest(version, asset.Digest)
	if !ok {
		return origin, errors.New("所选版本的文件摘要或大小与本地文件不符")
	}
	project, err := client.ProjectInfo(ctx, request.Source, version.ProjectID, "")
	if err != nil {
		return origin, err
	}
	if project.ID != version.ProjectID || project.Source != version.Source {
		return origin, errors.New("市场项目身份不匹配")
	}
	kind := market.Kind(asset.File.Kind)
	if version.Source == market.Modrinth && kind == market.DataPack && project.Kind == market.Mod &&
		slices.Contains(version.Loaders, "datapack") {
		project.Kind = market.DataPack
	}
	canonical := strings.TrimSuffix(asset.File.Path, ".disabled")
	if project.Kind != kind || !strings.EqualFold(path.Ext(remote.Filename), path.Ext(canonical)) {
		return origin, errors.New("市场文件类别与实例中的资产不符")
	}
	if project.DistributionRestricted {
		remote.URL = ""
	}
	metadata := market.MetadataFromVersion(version)
	metadata.WithProject(project)
	metadata.IdentifiedAt = time.Now().UTC()
	origin = market.InstallFile{Source: request.Source, ProjectID: project.ID, VersionID: version.ID, Path: canonical, File: remote, Metadata: metadata}
	count, err := s.store.RecordIdentities(ctx, request.InstanceID, request.Source, []AssetIdentification{{
		Asset: asset, Origin: &origin, RequireUniquePath: true,
	}})
	if err != nil {
		return market.InstallFile{}, fmt.Errorf("保存文件来源: %w", err)
	}
	if count != 1 {
		return market.InstallFile{}, errors.New("资产来源未保存")
	}
	return origin, nil
}
