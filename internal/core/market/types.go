// Package market models public Minecraft asset catalogs without UI dependencies.
package market

import (
	"errors"
	"fmt"
	"net/url"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"mcl/internal/core/meta"
)

type Source string

const (
	Modrinth   Source = "modrinth"
	CurseForge Source = "curseforge"
)

type Kind string

const (
	Mod          Kind = "mod"
	Shader       Kind = "shader"
	ResourcePack Kind = "resource-pack"
	DataPack     Kind = "data-pack"
	Modpack      Kind = "modpack"
	World        Kind = "world"
	PageSize          = 20
)

type Query struct {
	Source                            Source
	Kind                              Kind
	Text, GameVersion, Sort           string
	Loaders, Categories, Environments []string
	OpenSource                        bool
	Page                              int
}

func (q Query) Validate() error {
	if q.Source != Modrinth && q.Source != CurseForge {
		return errors.New("未知市场来源")
	}
	if !slices.Contains([]Kind{Mod, Shader, ResourcePack, DataPack, Modpack, World}, q.Kind) {
		return errors.New("未知资产类型")
	}
	if q.Kind == World && q.Source == Modrinth {
		return errors.New("Modrinth 不提供地图分类")
	}
	if q.Page < 0 || q.Page > 1000000 || (q.Source == CurseForge && q.Page > 499) || len(q.Text) > 300 || len(q.GameVersion) > 80 {
		return errors.New("搜索条件超出范围")
	}
	if q.GameVersion != "" && !meta.ValidID(q.GameVersion) {
		return errors.New("游戏版本无效")
	}
	loaderLimit, categoryLimit := 16, 32
	if q.Source == CurseForge {
		loaderLimit, categoryLimit = 5, 10
	}
	for _, group := range []struct {
		values []string
		limit  int
	}{{q.Loaders, loaderLimit}, {q.Categories, categoryLimit}, {q.Environments, 2}} {
		if len(group.values) > group.limit {
			return errors.New("筛选项超出来源允许的数量")
		}
		seen := make(map[string]bool, len(group.values))
		for _, value := range group.values {
			if !ValidID(value) || seen[value] {
				return errors.New("筛选项无效或重复")
			}
			seen[value] = true
		}
	}
	for _, environment := range q.Environments {
		if environment != "client" && environment != "server" {
			return errors.New("未知运行环境")
		}
	}
	if !slices.Contains([]string{"", "relevance", "downloads", "follows", "newest", "updated", "name", "popular"}, q.Sort) {
		return errors.New("未知排序")
	}
	if q.Source == CurseForge {
		if len(q.Environments) > 0 || q.OpenSource {
			return errors.New("CurseForge 不支持此环境或开源筛选")
		}
		for _, category := range q.Categories {
			id, err := strconv.ParseInt(category, 10, 64)
			if err != nil || id <= 0 {
				return errors.New("CurseForge 类别标识无效")
			}
		}
		for _, loader := range q.Loaders {
			if loaderID(loader) == 0 {
				return errors.New("CurseForge 不支持此加载器")
			}
		}
		if len(q.Loaders) > 0 && q.GameVersion == "" {
			return errors.New("请先选择 Minecraft 版本，再筛选 CurseForge 加载器")
		}
	}
	return nil
}

type Project struct {
	Source                                                       Source
	Kind                                                         Kind
	ID, Slug, Title, Description, Author, IconURL, PageURL, Body string
	Categories, Loaders                                          []string
	Downloads, Follows                                           int64
	DistributionRestricted                                       bool
}
type Results struct {
	Projects      []Project
	Total, Offset int
}
type Category struct{ ID, Name, Header, IconSVG, IconURL string }

type LoaderIcon struct{ Name, SVG string }
type File struct {
	URL, Filename, SHA1 string
	Size                int64
	Primary             bool
}
type Dependency struct {
	ProjectID, VersionID   string
	Required, Incompatible bool
}
type Version struct {
	Source                                       Source
	ID, ProjectID, Name, Number, Type, Changelog string
	GameVersions, Loaders                        []string
	Published                                    time.Time
	Downloads                                    int64
	Files                                        []File
	Dependencies                                 []Dependency
	Environment                                  string
}

func (v Version) PrimaryFile() (File, error) {
	file, err := v.PrimaryFileMetadata()
	if err != nil {
		return File{}, err
	}
	if !SafeURL(file.URL) {
		return File{}, errors.New("此版本未提供有效的自动下载地址")
	}
	return file, nil
}

// PrimaryFileMetadata also accepts files that must be obtained manually.
func (v Version) PrimaryFileMetadata() (File, error) {
	if len(v.Files) == 0 {
		return File{}, errors.New("此版本没有可下载文件")
	}
	file := v.Files[0]
	for _, candidate := range v.Files {
		if candidate.Primary {
			file = candidate
			break
		}
	}
	if !SafeFilename(file.Filename) || !meta.ValidSHA1(file.SHA1) || file.Size <= 0 || file.Size > 2<<30 {
		return File{}, errors.New("文件摘要、大小或名称无效，无法确认身份")
	}
	return file, nil
}

var identifier = regexp.MustCompile(`^[A-Za-z0-9_-]{1,100}$`)

func ValidID(id string) bool { return identifier.MatchString(id) }
func SafeFilename(name string) bool {
	return name != "" && len(name) <= 180 && path.Base(name) == name && name != "." && name != ".." && !strings.ContainsAny(name, "\\/:*?\"<>|\x00\r\n") && !strings.HasPrefix(name, ".") && !strings.HasSuffix(name, ".") && !strings.HasSuffix(name, " ")
}
func SafeURL(value string) bool {
	u, err := url.Parse(value)
	return err == nil && u.Scheme == "https" && u.Hostname() != "" && u.User == nil && u.Port() == "" && u.Fragment == ""
}
func projectType(kind Kind) string {
	switch kind {
	case ResourcePack:
		return "resourcepack"
	case DataPack:
		return "datapack"
	default:
		return string(kind)
	}
}
func ValidateVersion(v Version, projectID, gameVersion string) error {
	file, err := VersionFileMetadata(v, projectID, gameVersion)
	if err != nil {
		return err
	}
	if !SafeURL(file.URL) {
		return errors.New("此版本未提供有效的自动下载地址")
	}
	return nil
}

// VersionFileMetadata checks the fixed release and file independently of
// download availability. Restricted files still have a verifiable identity.
func VersionFileMetadata(v Version, projectID, gameVersion string) (File, error) {
	if !ValidID(v.ID) || v.ProjectID != projectID {
		return File{}, errors.New("项目版本身份不匹配")
	}
	if gameVersion != "" && !slices.Contains(v.GameVersions, gameVersion) {
		return File{}, fmt.Errorf("此文件不支持 Minecraft %s", gameVersion)
	}
	return v.PrimaryFileMetadata()
}
