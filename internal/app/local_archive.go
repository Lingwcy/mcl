package app

import (
	"context"
	"errors"
	"strings"

	"mcl/internal/core/instance"
	"mcl/internal/core/market"
	"mcl/internal/core/meta"
	"mcl/internal/core/modpack"
)

// InspectLocal copies a chosen archive into the verified digest cache. Only
// fixed metadata is retained; the user's filesystem path is never persisted.
func (s *ArchiveImporter) InspectLocal(ctx context.Context, name string, settings Settings) (ArchiveImportPlan, error) {
	store, ok := s.store.(interface {
		CacheLocalArchive(context.Context, string) (market.File, error)
	})
	if !ok {
		return ArchiveImportPlan{}, errors.New("本地整合包存储未配置")
	}
	file, err := store.CacheLocalArchive(ctx, name)
	if err != nil {
		return ArchiveImportPlan{}, err
	}
	manifest, err := s.store.InspectPack(ctx, file)
	if err != nil {
		return ArchiveImportPlan{}, err
	}
	title := strings.TrimSpace(manifest.Name)
	if instance.ValidateName(title) != nil {
		title = "导入的整合包"
	}
	return s.Inspect(ctx, ArchiveImportRequest{Source: market.Source(manifest.Format), Kind: market.Modpack, ProjectID: "local-" + file.SHA1, VersionID: file.SHA1, Name: title, LocalArchive: &file}, settings, nil)
}

func (s *ArchiveImporter) resolveArchive(ctx context.Context, request ArchiveImportRequest) (market.File, string, market.Version, error) {
	if request.LocalArchive != nil {
		file := *request.LocalArchive
		if request.Kind != market.Modpack || (request.Source != market.Modrinth && request.Source != market.CurseForge && request.Source != "multimc") || file.URL != "" || !market.SafeFilename(file.Filename) || !meta.ValidSHA1(file.SHA1) || file.SHA1 != strings.ToLower(file.SHA1) || file.Size <= 0 || file.Size > modpack.MaxArchiveSize || request.ProjectID != "local-"+file.SHA1 || request.VersionID != file.SHA1 {
			return market.File{}, "", market.Version{}, errors.New("本地整合包固定身份无效")
		}
		return file, "", market.Version{}, ctx.Err()
	}
	client := s.catalog.Client()
	version, err := client.Version(ctx, request.Source, request.ProjectID, request.VersionID)
	if err != nil {
		return market.File{}, "", market.Version{}, err
	}
	if version.Source != request.Source || version.ProjectID != request.ProjectID || version.ID != request.VersionID {
		return market.File{}, "", market.Version{}, errors.New("压缩包版本身份不匹配")
	}
	restricted, page := false, ""
	if request.Source == market.CurseForge {
		project, err := client.ProjectInfo(ctx, request.Source, request.ProjectID, "")
		if err != nil {
			return market.File{}, "", market.Version{}, err
		}
		if project.Kind != request.Kind {
			return market.File{}, "", market.Version{}, errors.New("压缩包项目类别不匹配")
		}
		restricted = project.DistributionRestricted
		if market.SafeURL(project.PageURL) {
			page = project.PageURL
		}
	}
	file, err := version.PrimaryFileMetadata()
	if err != nil {
		return file, page, version, err
	}
	if file.Size > modpack.MaxArchiveSize {
		return file, page, version, errors.New("压缩包超过 2 GiB")
	}
	if restricted || !modpack.AllowedDownload(file.URL) {
		file.URL = ""
	}
	file.SHA1 = strings.ToLower(file.SHA1)
	return file, page, version, nil
}
