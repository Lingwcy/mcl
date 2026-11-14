package market

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const maxResponse = 12 << 20

var ErrAPIKey = errors.New("CurseForge 需要自有 API Key，请在市场配置页填写")

// Client settings are immutable after construction. No remote response bodies,
// request headers or API keys are included in errors or diagnostic formatting.
type Client struct {
	HTTP                       *http.Client
	ModrinthURL, CurseForgeURL string
	apiKey                     string
}

func NewClient(curseForgeKey string) *Client {
	return &Client{HTTP: &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 4 || req.URL.Scheme != "https" || req.URL.Host != via[0].URL.Host {
			return errors.New("拒绝跨站市场重定向")
		}
		return nil
	}}, ModrinthURL: "https://api.modrinth.com/v2", CurseForgeURL: "https://api.curseforge.com/v1", apiKey: strings.TrimSpace(curseForgeKey)}
}
func (c *Client) CurseForgeAvailable() bool { return c.apiKey != "" }
func (c *Client) String() string            { return "market.Client" }
func (c *Client) GoString() string          { return c.String() }
func (c *Client) get(ctx context.Context, source Source, endpoint string, values url.Values, out any) error {
	return c.request(ctx, source, http.MethodGet, endpoint, values, nil, out)
}

// post is used for read-only batch lookup endpoints. The body is rebuilt for
// each finite retry; credentials and payloads never enter diagnostics.
func (c *Client) post(ctx context.Context, source Source, endpoint string, payload any, out any) error {
	body, err := json.Marshal(payload)
	if err != nil || len(body) > 1<<20 {
		return errors.New("市场查询内容无效或过大")
	}
	return c.request(ctx, source, http.MethodPost, endpoint, nil, body, out)
}

func (c *Client) request(ctx context.Context, source Source, method, endpoint string, values url.Values, body []byte, out any) error {
	base := c.ModrinthURL
	if source == CurseForge {
		if c.apiKey == "" {
			return ErrAPIKey
		}
		base = c.CurseForgeURL
	} else if source != Modrinth {
		return errors.New("未知市场来源")
	}
	u, err := url.Parse(base + endpoint)
	if err != nil {
		return errors.New("市场地址无效")
	}
	u.RawQuery = values.Encode()
	for attempt := 0; attempt < 3; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, u.String(), bytes.NewReader(body))
		if err != nil {
			return errors.New("无法创建市场请求")
		}
		req.Header.Set("User-Agent", "mcl/0.1.0 (native Minecraft launcher)")
		req.Header.Set("Accept", "application/json")
		if method == http.MethodPost {
			req.Header.Set("Content-Type", "application/json")
		}
		if source == CurseForge {
			req.Header.Set("x-api-key", c.apiKey)
		}
		response, err := c.HTTP.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return errors.New("无法连接市场，请检查网络后重试")
		}
		if response.StatusCode == http.StatusOK {
			data, readErr := io.ReadAll(io.LimitReader(response.Body, maxResponse+1))
			response.Body.Close()
			if readErr != nil {
				return errors.New("读取市场响应失败")
			}
			if len(data) > maxResponse {
				return errors.New("市场响应过大")
			}
			if err := json.Unmarshal(data, out); err != nil {
				return errors.New("市场返回无效数据")
			}
			return ctx.Err()
		}
		status := response.StatusCode
		delay := time.Duration(attempt+1) * time.Second
		if status == 429 {
			if seconds, err := time.ParseDuration(response.Header.Get("Retry-After") + "s"); err == nil {
				delay = seconds
			}
		}
		response.Body.Close()
		if (status != 429 && status < 500) || attempt == 2 || delay > 15*time.Second {
			return fmt.Errorf("市场服务返回 HTTP %d", status)
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return errors.New("市场请求失败")
}
func (c *Client) Search(ctx context.Context, q Query) (Results, error) {
	if err := q.Validate(); err != nil {
		return Results{}, err
	}
	if q.Source == Modrinth {
		return c.searchModrinth(ctx, q)
	}
	return c.searchCurseForge(ctx, q)
}
func (c *Client) Categories(ctx context.Context, source Source, kind Kind) ([]Category, error) {
	if source == Modrinth {
		return c.categoriesModrinth(ctx, kind)
	}
	if source == CurseForge {
		return c.categoriesCurseForge(ctx, kind)
	}
	return nil, errors.New("未知市场来源")
}
func (c *Client) Versions(ctx context.Context, source Source, projectID, gameVersion, loader string) ([]Version, error) {
	if !ValidID(projectID) {
		return nil, errors.New("项目 ID 无效")
	}
	if source == Modrinth {
		return c.versionsModrinth(ctx, projectID, gameVersion, loader)
	}
	if source == CurseForge {
		return c.versionsCurseForge(ctx, projectID, gameVersion, loader)
	}
	return nil, errors.New("未知市场来源")
}
func (c *Client) Project(ctx context.Context, source Source, id string, kind Kind) (Project, error) {
	if !ValidID(id) {
		return Project{}, errors.New("项目 ID 无效")
	}
	if source == Modrinth {
		var raw mrProject
		err := c.get(ctx, source, "/project/"+id, nil, &raw)
		if err != nil {
			return Project{}, err
		}
		return raw.project(kind), nil
	}
	if source == CurseForge {
		var raw struct {
			Data cfProject `json:"data"`
		}
		if err := c.get(ctx, source, "/mods/"+id, nil, &raw); err != nil {
			return Project{}, err
		}
		p := raw.Data.project(kind)
		var desc struct {
			Data string `json:"data"`
		}
		if err := c.get(ctx, source, "/mods/"+id+"/description", nil, &desc); err != nil {
			return p, err
		}
		p.Body = DescriptionMarkdown(desc.Data)
		return p, nil
	}
	return Project{}, errors.New("未知市场来源")
}

// ProjectInfo omits the separately fetched CurseForge HTML description when
// only the authoritative class and identity are needed for an install plan.
func (c *Client) ProjectInfo(ctx context.Context, source Source, id string, kind Kind) (Project, error) {
	if !ValidID(id) {
		return Project{}, errors.New("项目 ID 无效")
	}
	if source == CurseForge {
		var raw struct {
			Data cfProject `json:"data"`
		}
		if err := c.get(ctx, source, "/mods/"+id, nil, &raw); err != nil {
			return Project{}, err
		}
		p := raw.Data.project(kind)
		if p.ID != id {
			return Project{}, errors.New("项目身份不匹配")
		}
		return p, nil
	}
	return c.Project(ctx, source, id, kind)
}
func (c *Client) Version(ctx context.Context, source Source, projectID, versionID string) (Version, error) {
	if !ValidID(versionID) || !ValidID(projectID) {
		return Version{}, errors.New("版本 ID 无效")
	}
	if source == Modrinth {
		var raw mrVersion
		err := c.get(ctx, source, "/version/"+versionID, nil, &raw)
		return raw.version(), err
	}
	if source == CurseForge {
		var raw struct {
			Data cfFile `json:"data"`
		}
		err := c.get(ctx, source, "/mods/"+projectID+"/files/"+versionID, nil, &raw)
		return raw.Data.version(), err
	}
	return Version{}, errors.New("未知市场来源")
}

func (c *Client) VersionDetail(ctx context.Context, source Source, projectID, versionID string) (Version, error) {
	version, err := c.Version(ctx, source, projectID, versionID)
	if err != nil {
		return Version{}, err
	}
	if version.ID != versionID || version.ProjectID != projectID || version.Source != source {
		return Version{}, errors.New("版本详情身份不匹配")
	}
	if source == CurseForge {
		var raw struct {
			Data string `json:"data"`
		}
		if err := c.get(ctx, source, "/mods/"+projectID+"/files/"+versionID+"/changelog", nil, &raw); err != nil {
			return Version{}, err
		}
		version.Changelog = DescriptionMarkdown(raw.Data)
	}
	return version, nil
}
