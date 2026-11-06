package market

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"mcl/internal/core/meta"
)

// FileMetadata is public catalog data, not a proof that a local file remains
// intact. SHA-1 and size must be verified before a destructive file operation.
type FileMetadata struct {
	Title        string       `json:"title,omitempty"`
	Description  string       `json:"description,omitempty"`
	IconURL      string       `json:"iconUrl,omitempty"`
	VersionName  string       `json:"versionName,omitempty"`
	GameVersions []string     `json:"gameVersions,omitempty"`
	Loaders      []string     `json:"loaders,omitempty"`
	Dependencies []Dependency `json:"dependencies,omitempty"`
	Published    time.Time    `json:"published,omitempty"`
	IdentifiedAt time.Time    `json:"identifiedAt,omitempty"`
}

func MetadataFromVersion(version Version) *FileMetadata {
	return &FileMetadata{VersionName: shortText(version.Name, 320), GameVersions: slices.Clone(version.GameVersions), Loaders: slices.Clone(version.Loaders), Dependencies: slices.Clone(version.Dependencies), Published: version.Published}
}

func (m *FileMetadata) WithProject(project Project) {
	m.Title, m.Description = shortText(project.Title, 320), shortText(project.Description, 800)
	if len(project.IconURL) <= 2048 && SafeURL(project.IconURL) {
		m.IconURL = project.IconURL
	}
}

func (m *FileMetadata) Validate() error {
	if m == nil {
		return nil
	}
	if len(m.Title) > 320 || len(m.Description) > 800 || len(m.VersionName) > 320 || len(m.IconURL) > 2048 || (m.IconURL != "" && !SafeURL(m.IconURL)) || len(m.GameVersions) > 1000 || len(m.Loaders) > 40 || len(m.Dependencies) > 128 {
		return errors.New("资产来源元数据超出限制")
	}
	for _, id := range append(slices.Clone(m.GameVersions), m.Loaders...) {
		if !meta.ValidID(id) {
			return errors.New("资产来源包含无效兼容标签")
		}
	}
	for _, dependency := range m.Dependencies {
		if (dependency.ProjectID != "" && !ValidID(dependency.ProjectID)) || (dependency.VersionID != "" && !ValidID(dependency.VersionID)) {
			return errors.New("资产来源包含无效依赖标识")
		}
	}
	return nil
}

func shortText(value string, limit int) string {
	value = strings.Join(strings.Fields(value), " ")
	if len(value) <= limit {
		return value
	}
	end := limit
	for end > 0 && !utf8.RuneStart(value[end]) {
		end--
	}
	return value[:end]
}

type FileDigest struct {
	SHA1        string
	Size        int64
	Fingerprint uint32
}

// FileByDigest selects the exact known file, which need not be the primary
// download. A renamed local file is still identified by its contents.
func FileByDigest(version Version, digest FileDigest) (File, bool) {
	for _, file := range version.Files {
		if strings.EqualFold(file.SHA1, digest.SHA1) && file.Size == digest.Size && SafeFilename(file.Filename) && (file.URL == "" || SafeURL(file.URL)) {
			return file, true
		}
	}
	return File{}, false
}

// IdentifyFiles accepts at most 100 objects per request. CurseForge's 32-bit
// fingerprint is only a lookup hint; exact SHA-1 and size are still required.
func (c *Client) IdentifyFiles(ctx context.Context, source Source, digests []FileDigest) (map[string]Version, error) {
	if len(digests) == 0 || len(digests) > 100 {
		return nil, errors.New("文件识别批次无效")
	}
	wanted := make(map[string]FileDigest, len(digests))
	for _, digest := range digests {
		if !meta.ValidSHA1(digest.SHA1) || digest.Size <= 0 || digest.Size > 2<<30 {
			return nil, errors.New("文件识别摘要或大小无效")
		}
		digest.SHA1 = strings.ToLower(digest.SHA1)
		if old, ok := wanted[digest.SHA1]; ok && old.Size != digest.Size {
			return nil, errors.New("同一摘要的文件大小冲突")
		}
		wanted[digest.SHA1] = digest
	}
	result := make(map[string]Version)
	if source == Modrinth {
		hashes := make([]string, 0, len(wanted))
		for hash := range wanted {
			hashes = append(hashes, hash)
		}
		slices.Sort(hashes)
		var raw map[string]mrVersion
		if err := c.post(ctx, source, "/version_files", struct {
			Hashes    []string `json:"hashes"`
			Algorithm string   `json:"algorithm"`
		}{hashes, "sha1"}, &raw); err != nil {
			return nil, err
		}
		if len(raw) > len(wanted) {
			return nil, errors.New("识别服务返回了未请求的文件")
		}
		for hash, entry := range raw {
			hash = strings.ToLower(hash)
			digest, ok := wanted[hash]
			if !ok {
				return nil, errors.New("识别结果摘要不匹配")
			}
			version := entry.version()
			if !ValidID(version.ID) || !ValidID(version.ProjectID) {
				return nil, errors.New("识别结果版本身份无效")
			}
			if _, found := FileByDigest(version, digest); !found {
				return nil, errors.New("识别结果没有包含匹配的文件")
			}
			result[hash] = version
		}
		return result, nil
	}
	if source != CurseForge {
		return nil, errors.New("未知识别来源")
	}
	prints := make([]uint32, 0, len(wanted))
	for _, digest := range wanted {
		prints = append(prints, digest.Fingerprint)
	}
	slices.Sort(prints)
	prints = slices.Compact(prints)
	var raw struct {
		Data struct {
			CacheBuilt *bool `json:"isCacheBuilt"`
			Exact      []struct {
				File cfFile `json:"file"`
			} `json:"exactMatches"`
		} `json:"data"`
	}
	if err := c.post(ctx, source, "/fingerprints/432", struct {
		Fingerprints []uint32 `json:"fingerprints"`
	}{prints}, &raw); err != nil {
		return nil, err
	}
	if len(raw.Data.Exact) > 500 {
		return nil, errors.New("指纹匹配结果过多")
	}
	if raw.Data.CacheBuilt != nil && !*raw.Data.CacheBuilt {
		return nil, errors.New("CurseForge 指纹索引尚未就绪，请稍后重试")
	}
	ambiguous := make(map[string]bool)
	for _, match := range raw.Data.Exact {
		version := match.File.version()
		if !ValidID(version.ID) || !ValidID(version.ProjectID) {
			continue
		}
		for _, file := range version.Files {
			hash := strings.ToLower(file.SHA1)
			digest, ok := wanted[hash]
			if !ok || digest.Fingerprint != match.File.Fingerprint {
				continue
			}
			if _, found := FileByDigest(version, digest); !found {
				continue
			}
			if old, exists := result[hash]; exists && (old.ProjectID != version.ProjectID || old.ID != version.ID) {
				ambiguous[hash] = true
			}
			result[hash] = version
		}
	}
	for hash := range ambiguous {
		delete(result, hash)
	}
	return result, nil
}

// ProjectSummaries batches public project metadata; bodies are never persisted
// in local asset records. Callers limit each batch to twenty project IDs.
func (c *Client) ProjectSummaries(ctx context.Context, source Source, ids []string) (map[string]Project, error) {
	if len(ids) == 0 || len(ids) > 20 {
		return nil, errors.New("项目批次无效")
	}
	wanted := make(map[string]bool, len(ids))
	for _, id := range ids {
		if !ValidID(id) {
			return nil, errors.New("项目标识无效")
		}
		wanted[id] = true
	}
	result := make(map[string]Project, len(ids))
	if source == Modrinth {
		encoded, _ := json.Marshal(ids)
		var raw []mrProject
		if err := c.get(ctx, source, "/projects", url.Values{"ids": {string(encoded)}}, &raw); err != nil {
			return nil, err
		}
		if len(raw) > len(ids) {
			return nil, errors.New("项目批次响应过多")
		}
		for _, entry := range raw {
			project := entry.project("")
			project.Body = ""
			if wanted[project.ID] {
				result[project.ID] = project
			}
		}
		return result, nil
	}
	if source != CurseForge {
		return nil, errors.New("未知项目来源")
	}
	numbers := make([]int64, 0, len(ids))
	for _, id := range ids {
		value, err := strconv.ParseInt(id, 10, 64)
		if err != nil || value <= 0 {
			return nil, errors.New("CurseForge 项目标识无效")
		}
		numbers = append(numbers, value)
	}
	var raw struct {
		Data []cfProject `json:"data"`
	}
	if err := c.post(ctx, source, "/mods", struct {
		IDs []int64 `json:"modIds"`
	}{numbers}, &raw); err != nil {
		return nil, err
	}
	if len(raw.Data) > len(ids) {
		return nil, errors.New("项目批次响应过多")
	}
	for _, entry := range raw.Data {
		project := entry.project("")
		if wanted[project.ID] {
			result[project.ID] = project
		}
	}
	return result, nil
}
