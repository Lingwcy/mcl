package market

import (
	"context"
	"errors"
	"fmt"
	"path"
	"slices"
	"strings"
)

type Catalog interface {
	Version(context.Context, Source, string, string) (Version, error)
	Versions(context.Context, Source, string, string, string) ([]Version, error)
}
type InstallRequest struct {
	Source                                                       Source
	Kind                                                         Kind
	ProjectID, VersionID, InstanceID, GameVersion, Loader, World string
}
type InstallFile struct {
	Source                     Source
	ProjectID, VersionID, Path string
	File                       File
	Metadata                   *FileMetadata `json:"metadata,omitempty"`
}
type InstallPlan struct {
	Request    InstallRequest
	Files      []InstallFile
	Warnings   []string
	LocalPages map[string]string
	ReadyLocal []string
}

func (p InstallPlan) MissingLocal() int {
	count := 0
	for _, file := range p.Files {
		if file.File.URL == "" && !slices.Contains(p.ReadyLocal, file.Path) {
			count++
		}
	}
	return count
}

// BuildInstall resolves the selected release and required dependencies without
// writing anything. Its caller supplies the authoritative instance version.
func BuildInstall(ctx context.Context, catalog Catalog, request InstallRequest) (InstallPlan, error) {
	plan := InstallPlan{Request: request}
	if request.Source != Modrinth && request.Source != CurseForge {
		return plan, errors.New("未知安装来源")
	}
	if !ValidID(request.ProjectID) || !ValidID(request.VersionID) || request.GameVersion == "" {
		return plan, errors.New("缺少安装项目、版本或目标游戏版本")
	}
	var directory, extension string
	switch request.Kind {
	case Mod:
		directory, extension = "mods", ".jar"
		if request.Loader == "" {
			return plan, errors.New("请选择模组加载器")
		}
		plan.Warnings = append(plan.Warnings, "模组需要匹配实例的加载器与游戏版本")
	case Shader:
		directory, extension = "shaderpacks", ".zip"
		plan.Warnings = append(plan.Warnings, "需要 Iris 等光影运行环境，并在游戏内启用")
	case ResourcePack:
		directory, extension = "resourcepacks", ".zip"
	case DataPack:
		if !SafeFilename(request.World) {
			return plan, errors.New("请选择已有存档")
		}
		directory, extension = path.Join("saves", request.World, "datapacks"), ".zip"
	case Modpack:
		return plan, errors.New("整合包必须安装到新实例，不能作为单文件加入现有实例")
	case World:
		return plan, errors.New("地图需要安全解压到独立存档")
	default:
		return plan, errors.New("不支持此资产类型")
	}
	seen := make(map[string]string)
	visiting := make(map[string]bool)
	paths := make(map[string]bool)
	incompatible := make(map[string]bool)
	var visit func(string, string, int) error
	visit = func(projectID, versionID string, depth int) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if depth > 24 || len(seen) >= 64 {
			return errors.New("依赖树超出限制")
		}
		if previous, ok := seen[projectID]; ok {
			if versionID != "" && previous != versionID {
				return errors.New("必需依赖要求不同版本，无法自动合并")
			}
			return nil
		}
		if visiting[projectID] {
			return errors.New("检测到循环依赖")
		}
		visiting[projectID] = true
		defer delete(visiting, projectID)
		var v Version
		var err error
		if versionID != "" {
			v, err = catalog.Version(ctx, request.Source, projectID, versionID)
		} else {
			versions, fetchErr := catalog.Versions(ctx, request.Source, projectID, request.GameVersion, request.Loader)
			if fetchErr != nil {
				return fetchErr
			}
			for _, candidate := range versions {
				if candidate.Type == "release" && slices.Contains(candidate.GameVersions, request.GameVersion) && (request.Loader == "" || slices.Contains(candidate.Loaders, request.Loader)) {
					v = candidate
					break
				}
			}
			if v.ID == "" {
				return fmt.Errorf("依赖 %s 没有兼容的正式版本", projectID)
			}
		}
		if err != nil {
			return err
		}
		if v.Source != request.Source || (versionID != "" && v.ID != versionID) {
			return errors.New("依赖来源不匹配")
		}
		f, err := VersionFileMetadata(v, projectID, request.GameVersion)
		if err != nil {
			return err
		}
		if request.Kind == Mod && !slices.Contains(v.Loaders, request.Loader) {
			return fmt.Errorf("版本 %s 不支持 %s", v.Name, request.Loader)
		}
		if v.Environment == "dedicated_server_only" {
			plan.Warnings = append(plan.Warnings, v.Name+" 仅支持独立服务端，不适用于客户端或单人游戏")
		}
		f.SHA1 = strings.ToLower(f.SHA1)
		if !SafeURL(f.URL) {
			f.URL = ""
		}
		if strings.ToLower(path.Ext(f.Filename)) != extension {
			return errors.New("下载文件类型与目标资产不一致")
		}
		for _, d := range v.Dependencies {
			if d.Incompatible && d.ProjectID != "" {
				incompatible[d.ProjectID] = true
			}
			if !d.Required {
				continue
			}
			depID := d.ProjectID
			if depID == "" && d.VersionID != "" {
				dep, err := catalog.Version(ctx, request.Source, projectID, d.VersionID)
				if err != nil {
					return err
				}
				depID = dep.ProjectID
			}
			if !ValidID(depID) {
				return errors.New("依赖没有可解析的项目身份")
			}
			if err := visit(depID, d.VersionID, depth+1); err != nil {
				return err
			}
		}
		key := strings.ToLower(path.Join(directory, f.Filename))
		if paths[key] {
			return errors.New("依赖文件名冲突")
		}
		paths[key] = true
		seen[projectID] = v.ID
		plan.Files = append(plan.Files, InstallFile{Source: request.Source, ProjectID: projectID, VersionID: v.ID, Path: path.Join(directory, f.Filename), File: f, Metadata: MetadataFromVersion(v)})
		return nil
	}
	if err := visit(request.ProjectID, request.VersionID, 0); err != nil {
		return InstallPlan{}, err
	}
	for id := range incompatible {
		if _, found := seen[id]; found {
			return InstallPlan{}, errors.New("所选版本与必需依赖声明不兼容")
		}
	}
	return plan, nil
}
