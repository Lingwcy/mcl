package java

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"mcl/internal/core/download"
	"mcl/internal/core/meta"
)

func Providers() []string { return []string{"temurin", "zulu"} }

func (p Package) Vendor() string {
	if p.Provider == "" {
		return "temurin"
	}
	return p.Provider
}
func (c *CatalogClient) CatalogFor(ctx context.Context, provider string, major int, target meta.Platform) (Catalog, error) {
	if err := ctx.Err(); err != nil {
		return Catalog{}, err
	}
	if major < 8 || major > 100 {
		return Catalog{}, errors.New("无效的 Java 主版本")
	}
	switch provider {
	case "temurin":
		return c.temurinCatalog(ctx, major, target)
	case "zulu":
		return c.zuluCatalog(ctx, major, target)
	}
	return Catalog{}, errors.New("未知 Java 目录来源")
}

// The management page lists all providers. Automatic selection queries each
// provider lazily, so a working primary does not wait for unrelated endpoints.
func (c *CatalogClient) Catalog(ctx context.Context, major int, target meta.Platform) (Catalog, error) {
	result := Catalog{Majors: []int{25, 21, 17, 11, 8}}
	var failures []error
	for _, provider := range Providers() {
		catalog, err := c.CatalogFor(ctx, provider, major, target)
		if ctx.Err() != nil {
			return Catalog{}, ctx.Err()
		}
		if err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", provider, err))
			if len(catalog.Packages) == 0 {
				continue
			}
		}
		for _, m := range catalog.Majors {
			if !slices.Contains(result.Majors, m) {
				result.Majors = append(result.Majors, m)
			}
		}
		result.Packages = append(result.Packages, catalog.Packages...)
	}
	slices.SortFunc(result.Majors, func(a, b int) int { return b - a })
	if len(result.Packages) == 0 && len(failures) > 0 {
		return result, fmt.Errorf("Java 下载目录均未提供可用包: %w", errors.Join(failures...))
	}
	return result, nil
}

var zuluUUID = regexp.MustCompile("^[0-9a-f]{8}(-[0-9a-f]{4}){3}-[0-9a-f]{12}$")

func zuluPlatform(target meta.Platform) (string, string, int, error) {
	osName := map[string]string{"osx": "macos", "windows": "windows", "linux": "linux"}[target.OS]
	arch, bits := "", 64
	switch meta.NormalizeArch(target.Arch) {
	case "amd64":
		arch = "x86"
	case "arm64":
		arch = "arm"
	case "386":
		arch = "x86"
		bits = 32
	}
	if osName == "" || arch == "" {
		return "", "", 0, errors.New("Zulu 尚不支持目标平台")
	}
	return osName, arch, bits, nil
}
func (c *CatalogClient) zuluCatalog(ctx context.Context, major int, target meta.Platform) (Catalog, error) {
	result := Catalog{}
	osName, arch, bits, err := zuluPlatform(target)
	if err != nil {
		return result, err
	}
	queryArch := arch
	if arch == "arm" && bits == 64 {
		queryArch = "aarch64"
	} else if arch == "x86" && bits == 64 {
		queryArch = "x86_64"
	}
	archiveType := "tar.gz"
	if target.OS == "windows" {
		archiveType = "zip"
	}
	var failures []error
	for _, kind := range []string{"jre", "jdk"} {
		query := url.Values{"java_version": {strconv.Itoa(major)}, "os": {osName}, "arch": {queryArch}, "archive_type": {archiveType}, "java_package_type": {kind}, "javafx_bundled": {"false"}, "release_status": {"ga"}, "availability_types": {"CA"}, "latest": {"true"}, "page_size": {"4"}}
		var entries []struct {
			UUID string `json:"package_uuid"`
		}
		if err := c.zuluGet(ctx, "/?"+query.Encode(), &entries); err != nil {
			failures = append(failures, err)
			continue
		}
		for index, entry := range entries {
			if index >= 4 {
				break
			}
			if !zuluUUID.MatchString(entry.UUID) {
				continue
			}
			var detail struct {
				UUID              string `json:"package_uuid"`
				Product, OS, Arch string
				Bits              int    `json:"hw_bitness"`
				Kind              string `json:"java_package_type"`
				Archive           string `json:"archive_type"`
				Status            string `json:"release_status"`
				Availability      string `json:"availability_type"`
				JavaFX            bool   `json:"javafx_bundled"`
				Version           []int  `json:"java_version"`
				URL               string `json:"download_url"`
				SHA256            string `json:"sha256_hash"`
				Size              int64
			}
			if err := c.zuluGet(ctx, "/"+entry.UUID, &detail); err != nil {
				failures = append(failures, err)
				continue
			}
			if detail.UUID != entry.UUID || detail.Product != "zulu" || detail.OS != osName || detail.Arch != arch || detail.Bits != bits || detail.Kind != kind || detail.Archive != archiveType || detail.Status != "ga" || detail.Availability != "CA" || detail.JavaFX || len(detail.Version) == 0 || detail.Version[0] != major {
				continue
			}
			version := make([]string, 0, len(detail.Version))
			for _, part := range detail.Version {
				version = append(version, strconv.Itoa(part))
			}
			p := Package{Provider: "zulu", Name: "Azul Zulu", Version: strings.Join(version, "."), Major: major, Kind: kind, OS: target.OS, Arch: meta.NormalizeArch(target.Arch), URL: detail.URL, SHA256: detail.SHA256, Size: detail.Size}
			if err := p.Validate(); err != nil {
				failures = append(failures, err)
				continue
			}
			result.Packages = append(result.Packages, p)
			break
		}
	}
	if ctx.Err() != nil {
		return result, ctx.Err()
	}
	if len(result.Packages) == 0 && len(failures) > 0 {
		return result, errors.Join(failures...)
	}
	return result, nil
}
func validZuluFilename(p Package) bool {
	u, err := url.Parse(p.URL)
	if err != nil {
		return false
	}
	name := path.Base(u.Path)
	osSuffix := map[string]string{"osx": "macosx", "linux": "linux", "windows": "win"}[p.OS]
	archSuffix := map[string]string{"amd64": "x64", "arm64": "aarch64", "386": "i686"}[p.Arch]
	extension := ".tar.gz"
	if p.OS == "windows" {
		extension = ".zip"
	}
	return path.Dir(u.Path) == "/zulu/bin" && strings.HasPrefix(name, "zulu") && strings.Contains(name, "-ca-"+p.Kind+strconv.Itoa(p.Major)+".") && strings.HasSuffix(name, "-"+osSuffix+"_"+archSuffix+extension) && !strings.Contains(name, "..")
}
func (c *CatalogClient) zuluGet(ctx context.Context, endpoint string, target any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.azul.com/metadata/v1/zulu/packages"+endpoint, nil)
	if err != nil {
		return err
	}
	request.Header.Set("User-Agent", "mcl/0.0.4")
	request.Header.Set("Accept", "application/json")
	response, err := c.HTTP.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("读取 Azul 官方目录失败")
	}
	defer response.Body.Close()
	if response.StatusCode == 404 {
		return nil
	}
	if response.StatusCode != 200 {
		return fmt.Errorf("Azul 官方目录返回 HTTP %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, (2<<20)+1))
	if err != nil {
		return errors.New("读取 Azul 元数据中断")
	}
	if len(data) > 2<<20 {
		return errors.New("Azul 元数据超出限制")
	}
	if err := json.Unmarshal(data, target); err != nil {
		return errors.New("Azul 官方目录内容无效")
	}
	return ctx.Err()
}

// DownloadFile binds every transport and task event to the same fixed archive.
func (p Package) DownloadFile() download.File {
	return download.File{Path: path.Join("cache", "java", p.SHA256+".archive"), SHA256: p.SHA256, Download: meta.Download{URL: p.URL, Size: p.Size}, AllowedHosts: []string{"github.com", "release-assets.githubusercontent.com", "objects.githubusercontent.com", "cdn.azul.com"}}
}
