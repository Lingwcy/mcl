package java

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"mcl/internal/core/meta"
)

// Package pins an official vendor binary and its SHA-256. Empty Provider is
// retained as Temurin for existing durable tasks and managed installations.
type Package struct {
	Name, Version, Kind, OS, Arch string
	Provider                      string
	URL, SHA256                   string
	Size                          int64
	Major                         int
}

func (p Package) ID() string { return p.SHA256 + "-" + p.Kind }

type Catalog struct {
	Majors   []int
	Packages []Package
}

type CatalogClient struct{ HTTP *http.Client }

func NewCatalogClient() *CatalogClient {
	return &CatalogClient{HTTP: &http.Client{
		Timeout:       25 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}
}

// Catalog uses official Adoptium v3 metadata, never a hardcoded binary URL.
func (c *CatalogClient) temurinCatalog(ctx context.Context, major int, platform meta.Platform) (Catalog, error) {
	result := Catalog{}
	var info struct {
		Available []int `json:"available_releases"`
	}
	if err := c.get(ctx, "/info/available_releases", &info); err != nil {
		return result, err
	}
	for _, version := range info.Available {
		if version >= 8 && version <= 100 {
			result.Majors = append(result.Majors, version)
		}
	}
	sort.Sort(sort.Reverse(sort.IntSlice(result.Majors)))
	osName, arch := platform.OS, meta.NormalizeArch(platform.Arch)
	if osName == "osx" {
		osName = "mac"
	}
	switch arch {
	case "amd64":
		arch = "x64"
	case "arm64":
		arch = "aarch64"
	case "386":
		arch = "x32"
	default:
		return result, errors.New("当前架构尚无可下载的 Java")
	}
	if major < 8 || major > 100 {
		return result, errors.New("无效的 Java 主版本")
	}
	for _, kind := range []string{"jdk", "jre"} {
		var assets []struct {
			Binary struct {
				Architecture string `json:"architecture"`
				OS           string `json:"os"`
				ImageType    string `json:"image_type"`
				Package      struct {
					Checksum string `json:"checksum"`
					Link     string `json:"link"`
					Size     int64  `json:"size"`
				} `json:"package"`
			} `json:"binary"`
			Version struct {
				Semver string `json:"semver"`
			} `json:"version"`
		}
		query := url.Values{"architecture": {arch}, "os": {osName}, "image_type": {kind}, "vendor": {"eclipse"}}
		err := c.get(ctx, "/assets/latest/"+strconv.Itoa(major)+"/hotspot?"+query.Encode(), &assets)
		if errors.Is(err, errNoJavaPackage) {
			continue
		}
		if err != nil {
			return result, err
		}
		for _, asset := range assets {
			binary := asset.Binary
			if binary.OS != osName || binary.Architecture != arch || binary.ImageType != kind {
				continue
			}
			p := Package{
				Provider: "temurin", Name: "Eclipse Temurin", Version: asset.Version.Semver, Major: major,
				Kind: kind, OS: platform.OS, Arch: meta.NormalizeArch(platform.Arch),
				URL: binary.Package.Link, SHA256: binary.Package.Checksum, Size: binary.Package.Size,
			}
			if err := p.Validate(); err != nil {
				return result, err
			}
			result.Packages = append(result.Packages, p)
			break
		}
	}
	return result, ctx.Err()
}

var errNoJavaPackage = errors.New("该平台暂无对应 Java 包")

func (p Package) Validate() error {
	if p.Major < 8 || p.Major > 100 || p.Version == "" ||
		(p.OS != "osx" && p.OS != "linux" && p.OS != "windows") ||
		(p.Arch != "amd64" && p.Arch != "arm64" && p.Arch != "386") {
		return errors.New("Java 下载版本或目标平台无效")
	}
	digest, err := hex.DecodeString(p.SHA256)
	u, urlErr := url.Parse(p.URL)
	if err != nil || len(digest) != 32 || p.SHA256 != fmt.Sprintf("%x", digest) ||
		p.Size <= 0 || p.Size > 1<<30 || urlErr != nil || u.Scheme != "https" ||
		u.User != nil || u.Port() != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || (u.RawPath != "" && p.Vendor() != "temurin") {
		return errors.New("Java 下载元数据缺少有效摘要、大小或官方地址")
	}
	if p.Kind != "jdk" && p.Kind != "jre" {
		return errors.New("Java 包类型无效")
	}
	switch p.Vendor() {
	case "temurin":
		prefix := "/adoptium/temurin" + strconv.Itoa(p.Major) + "-binaries/releases/download/"
		if u.Host != "github.com" || !strings.HasPrefix(u.Path, prefix) {
			return errors.New("Java 下载来源不是对应版本的官方发行仓库")
		}
	case "zulu":
		if u.Host != "cdn.azul.com" || !strings.HasPrefix(u.Path, "/zulu/bin/") || !validZuluFilename(p) {
			return errors.New("Java 下载来源不是对应平台的 Azul 官方发行包")
		}
	default:
		return errors.New("未知 Java 发行商")
	}
	return nil
}

func (c *CatalogClient) get(ctx context.Context, endpoint string, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.adoptium.net/v3"+endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "mcl/0.1 (Meow Craft Launcher)")
	req.Header.Set("Accept", "application/json")
	response, err := c.HTTP.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("读取 Java 下载目录失败")
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		return errNoJavaPackage
	}
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("Java 官方目录返回 HTTP %d，请稍后重试", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, (2<<20)+1))
	if err != nil {
		return err
	}
	if len(data) > 2<<20 {
		return errors.New("Java 下载目录超过大小限制")
	}
	if err := json.Unmarshal(data, target); err != nil {
		return errors.New("无法读取 Java 官方目录内容")
	}
	return ctx.Err()
}
