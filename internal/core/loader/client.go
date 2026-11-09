package loader

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"mcl/internal/core/diagnostics"
	"mcl/internal/core/meta"
)

type Release struct {
	Version     string `json:"version"`
	Stable      bool   `json:"stable"`
	Recommended bool   `json:"recommended,omitempty"`
}
type Client struct {
	HTTP                  *http.Client
	FabricURL, QuiltURL   string
	ForgeURL, NeoForgeURL string
}

func NewClient() *Client {
	return &Client{HTTP: &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 4 || req.URL.Scheme != "https" || req.URL.Host != via[0].URL.Host {
			return errors.New("拒绝跨站加载器元数据重定向")
		}
		return nil
	}}, FabricURL: "https://meta.fabricmc.net/v2", QuiltURL: "https://meta.quiltmc.org/v3",
		ForgeURL: "https://files.minecraftforge.net/net/minecraftforge/forge", NeoForgeURL: "https://maven.neoforged.net/releases/net/neoforged"}
}
func (c *Client) base(kind string) (string, error) {
	switch kind {
	case "fabric":
		return c.FabricURL, nil
	case "quilt":
		return c.QuiltURL, nil
	default:
		return "", fmt.Errorf("%s 加载器安装处理器尚未接入", kind)
	}
}
func (c *Client) bytes(ctx context.Context, address string, limit int64) ([]byte, error) {
	ctx, evidence := diagnostics.Start(ctx, "metadata", "loader-response")
	defer evidence.End()
	response, err := c.request(ctx, http.MethodGet, address)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return nil, &metadataStatusError{status: response.StatusCode}
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errors.New("加载器响应超过限制")
	}
	evidence.Work()
	return data, ctx.Err()
}

// Only transient transport/status failures are retried. A long Retry-After is
// returned to the user instead of sleeping indefinitely or retrying too early.
func (c *Client) request(ctx context.Context, method, address string) (*http.Response, error) {
	ctx, evidence := diagnostics.Start(ctx, "network", "loader-metadata")
	defer evidence.End()
	for attempt := range 3 {
		evidence.Network(diagnostics.Source(address), 0, attempt+1)
		req, err := http.NewRequestWithContext(ctx, method, address, nil)
		if err != nil {
			return nil, errors.New("加载器请求地址无效")
		}
		req.Header.Set("User-Agent", "mcl/0.1.0")
		req.Header.Set("Accept", "application/json, text/plain")
		req.Header.Set("Accept-Encoding", "identity")
		response, fetchErr := c.HTTP.Do(req)
		if ctx.Err() != nil {
			if response != nil {
				response.Body.Close()
			}
			return nil, ctx.Err()
		}
		delay := time.Duration(1<<attempt) * 300 * time.Millisecond
		var failure error
		if fetchErr != nil {
			if response != nil {
				response.Body.Close()
			}
			code := diagnostics.Code(fetchErr)
			if code == "unknown" {
				code = "network"
			}
			failure = diagnostics.Wrap(code, errors.New("无法连接加载器服务"))
			evidence.Fail(failure)
		} else {
			status := response.StatusCode
			evidence.Network(diagnostics.Source(address), status, attempt+1)
			if status != http.StatusTooManyRequests && status != http.StatusRequestTimeout && status < 500 {
				return response, nil
			}
			failure = diagnostics.Wrap(diagnostics.HTTPCode(status), fmt.Errorf("加载器服务繁忙，HTTP %d", status))
			evidence.Fail(failure)
			retry := response.Header.Get("Retry-After")
			response.Body.Close()
			if seconds, err := strconv.Atoi(retry); err == nil && seconds > 0 {
				if seconds > 10 {
					return nil, failure
				}
				delay = max(delay, time.Duration(seconds)*time.Second)
			} else if deadline, err := http.ParseTime(retry); err == nil {
				if time.Until(deadline) > 10*time.Second {
					return nil, failure
				}
				delay = max(delay, time.Until(deadline))
			}
		}
		if attempt == 2 {
			return nil, failure
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
	panic("unreachable loader retry loop")
}
func (c *Client) Releases(ctx context.Context, kind, game string) ([]Release, error) {
	if !meta.ValidID(game) {
		return nil, errors.New("游戏版本无效")
	}
	if UsesProcessors(kind) {
		return c.forgeReleases(ctx, kind, game)
	}
	base, err := c.base(kind)
	if err != nil {
		return nil, err
	}
	if !meta.ValidID(game) {
		return nil, errors.New("游戏版本无效")
	}
	data, err := c.bytes(ctx, base+"/versions/loader/"+url.PathEscape(game), 4<<20)
	if err != nil {
		return nil, err
	}
	var raw []struct {
		Loader Release `json:"loader"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, errors.New("加载器版本列表无效")
	}
	if len(raw) > 10000 {
		return nil, errors.New("加载器版本数量过多")
	}
	var releases []Release
	for _, entry := range raw {
		if meta.ValidID(entry.Loader.Version) {
			releases = append(releases, entry.Loader)
		}
	}
	return releases, nil
}

type rawProfile struct {
	ID           string           `json:"id"`
	InheritsFrom string           `json:"inheritsFrom"`
	MainClass    string           `json:"mainClass"`
	Arguments    meta.Arguments   `json:"arguments"`
	Java         meta.JavaVersion `json:"javaVersion"`
	Libraries    []struct {
		Name      string `json:"name"`
		URL       string `json:"url"`
		Downloads struct {
			Artifact    *meta.Download           `json:"artifact"`
			Classifiers map[string]meta.Download `json:"classifiers"`
		} `json:"downloads"`
		Natives map[string]string `json:"natives"`
		Rules   []meta.Rule       `json:"rules"`
	} `json:"libraries"`
}

func (c *Client) Resolve(ctx context.Context, request Request) (Profile, error) {
	if err := request.Validate(); err != nil {
		return Profile{}, err
	}
	base, err := c.base(request.Type)
	if err != nil {
		return Profile{}, err
	}
	data, err := c.bytes(ctx, base+"/versions/loader/"+url.PathEscape(request.GameVersion)+"/"+url.PathEscape(request.Version)+"/profile/json", 4<<20)
	if err != nil {
		return Profile{}, err
	}
	var raw rawProfile
	if err := json.Unmarshal(data, &raw); err != nil {
		return Profile{}, errors.New("加载器启动配置无效")
	}
	if raw.InheritsFrom != request.GameVersion || len(raw.Libraries) == 0 || len(raw.Libraries) > 128 {
		return Profile{}, errors.New("加载器配置继承版本或依赖数量无效")
	}
	p := Profile{Request: request, ID: raw.ID, MainClass: raw.MainClass, Arguments: raw.Arguments, JavaMajor: raw.Java.Major, Libraries: make([]meta.Library, len(raw.Libraries))}
	work, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	jobs := make(chan int)
	var workers sync.WaitGroup
	for range 4 {
		workers.Go(func() {
			for index := range jobs {
				if work.Err() != nil {
					return
				}
				lib := raw.Libraries[index]
				if len(lib.Natives) > 0 || len(lib.Downloads.Classifiers) > 0 {
					cancel(errors.New("加载器配置包含不支持的本地分类"))
					return
				}
				coordinate, err := meta.ParseCoordinate(lib.Name)
				if err != nil {
					cancel(err)
					return
				}
				expectedPath := coordinate.Path()
				artifact := lib.Downloads.Artifact
				if artifact == nil {
					address := strings.TrimRight(lib.URL, "/") + "/" + expectedPath
					if !AllowedLibraryURL(address) {
						cancel(errors.New("加载器库来源不受支持"))
						return
					}
					resolved, err := c.mavenMetadata(work, address, expectedPath)
					if err != nil {
						cancel(fmt.Errorf("解析库 %s: %w", lib.Name, err))
						return
					}
					artifact = &resolved
				} else {
					copy := *artifact
					artifact = &copy
					if artifact.Path == "" {
						artifact.Path = expectedPath
					}
				}
				p.Libraries[index] = meta.Library{Name: lib.Name, Rules: lib.Rules}
				p.Libraries[index].Downloads.Artifact = artifact
			}
		})
	}
dispatch:
	for index := range raw.Libraries {
		select {
		case jobs <- index:
		case <-work.Done():
			break dispatch
		}
	}
	close(jobs)
	workers.Wait()
	if err := context.Cause(work); err != nil {
		return Profile{}, err
	}
	if err := p.Validate(); err != nil {
		return Profile{}, err
	}
	return p, nil
}
func (c *Client) mavenMetadata(ctx context.Context, address, relative string) (meta.Download, error) {
	ctx, evidence := diagnostics.Start(ctx, "metadata", relative)
	defer evidence.End()
	data, err := c.bytes(ctx, address+".sha1", 1024)
	if err != nil {
		return meta.Download{}, err
	}
	fields := strings.Fields(string(data))
	if len(fields) == 0 || !meta.ValidSHA1(fields[0]) {
		return meta.Download{}, errors.New("Maven SHA-1 无效")
	}
	size, err := c.mavenSize(ctx, address)
	if err != nil {
		return meta.Download{}, err
	}
	return meta.Download{Path: relative, URL: address, SHA1: strings.ToLower(fields[0]), Size: size}, nil
}

// Keep size discovery separate: a legacy fixed installer may already pin SHA-1
// even when the repository does not publish a .sha1 sidecar for that artifact.
func (c *Client) mavenSize(ctx context.Context, address string) (int64, error) {
	response, err := c.request(ctx, http.MethodHead, address)
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return 0, diagnostics.Wrap(diagnostics.HTTPCode(response.StatusCode), fmt.Errorf("Maven 文件返回 HTTP %d", response.StatusCode))
	}
	size, err := strconv.ParseInt(response.Header.Get("Content-Length"), 10, 64)
	if err != nil || size <= 0 || size > 512<<20 {
		return 0, errors.New("Maven 未提供有效文件大小")
	}
	return size, ctx.Err()
}
func AllowedLibraryURL(address string) bool {
	u, err := url.Parse(address)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" || u.RawQuery != "" || u.Fragment != "" {
		return false
	}
	for _, host := range LibraryHosts() {
		if strings.EqualFold(u.Hostname(), host) {
			return true
		}
	}
	return false
}

func LibraryHosts() []string {
	return []string{"maven.fabricmc.net", "maven.quiltmc.org", "repo.maven.apache.org", "repo1.maven.org",
		"libraries.minecraft.net", "maven.minecraftforge.net", "maven.neoforged.net"}
}
