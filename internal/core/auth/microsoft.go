package auth

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"mcl/internal/core/launch"
)

const (
	xboxUser       = "https://user.auth.xboxlive.com/user/authenticate"
	xboxXSTS       = "https://xsts.auth.xboxlive.com/xsts/authorize"
	minecraftAPI   = "https://api.minecraftservices.com"
	minecraftLogin = minecraftAPI + "/authentication/login_with_xbox"
)

var (
	clientIDPattern = regexp.MustCompile(`(?i)^[a-f0-9]{8}(-[a-f0-9]{4}){3}-[a-f0-9]{12}$`)
)

type MicrosoftClient struct {
	clientID string
	http     *http.Client
}

func NewMicrosoftClient(clientID string) (*MicrosoftClient, error) {
	clientID = strings.ToLower(strings.TrimSpace(clientID))
	if !clientIDPattern.MatchString(clientID) {
		return nil, errors.New("请为 mcl 配置自有微软应用 Client ID")
	}
	return &MicrosoftClient{clientID: clientID, http: &http.Client{
		Timeout:       15 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}, nil
}

// Grant is transient. Only the public profile belongs in application storage.
type Grant struct {
	Profile Profile
	Session launch.Session
	refresh string
}

func (g Grant) RefreshToken() string { return g.refresh }
func (g Grant) String() string       { return "MicrosoftGrant{credentials:[REDACTED]}" }
func (g Grant) GoString() string     { return g.String() }

type microsoftToken struct {
	Access  string `json:"access_token"`
	Refresh string `json:"refresh_token"`
}

func (m *MicrosoftClient) Refresh(ctx context.Context, refresh string) (Grant, error) {
	token, err := m.token(ctx, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {refresh}})
	if err != nil {
		return Grant{}, err
	}
	if token.Refresh == "" {
		token.Refresh = refresh
	}
	return m.exchange(ctx, token)
}

func (m *MicrosoftClient) token(ctx context.Context, form url.Values) (microsoftToken, error) {
	form.Set("client_id", m.clientID)
	form.Set("scope", microsoftScope)
	var token microsoftToken
	status, err := m.request(ctx, "微软授权", http.MethodPost, microsoftOAuth+"token", "application/x-www-form-urlencoded", strings.NewReader(form.Encode()), "", &token)
	if err != nil {
		if status == http.StatusBadRequest || status == http.StatusUnauthorized {
			return microsoftToken{}, errors.New("微软拒绝授权或凭据已失效，请检查应用配置并重新登录")
		}
		return microsoftToken{}, err
	}
	if token.Access == "" || len(token.Access) > 16<<10 || len(token.Refresh) > 16<<10 {
		return microsoftToken{}, errors.New("微软授权返回的令牌无效")
	}
	return token, nil
}

type xboxToken struct {
	Token         string
	XErr          int64
	DisplayClaims struct {
		XUI []struct {
			Hash string `json:"uhs"`
			ID   string `json:"xid"`
		}
	}
}

func (m *MicrosoftClient) exchange(ctx context.Context, token microsoftToken) (Grant, error) {
	var xbox xboxToken
	_, err := m.post(ctx, "Xbox 登录", xboxUser, map[string]any{
		"Properties":   map[string]string{"AuthMethod": "RPS", "SiteName": "user.auth.xboxlive.com", "RpsTicket": "d=" + token.Access},
		"RelyingParty": "http://auth.xboxlive.com", "TokenType": "JWT",
	}, &xbox)
	if err != nil {
		return Grant{}, err
	}
	if xbox.Token == "" || len(xbox.DisplayClaims.XUI) != 1 || xbox.DisplayClaims.XUI[0].Hash == "" {
		return Grant{}, errors.New("Xbox 登录返回的身份无效")
	}

	var xsts xboxToken
	status, err := m.post(ctx, "Xbox XSTS", xboxXSTS, map[string]any{
		"Properties":   map[string]any{"SandboxId": "RETAIL", "UserTokens": []string{xbox.Token}},
		"RelyingParty": "rp://api.minecraftservices.com/", "TokenType": "JWT",
	}, &xsts)
	if err != nil {
		if status == http.StatusUnauthorized {
			switch xsts.XErr {
			case 2148916233:
				return Grant{}, errors.New("此微软账户尚未建立 Xbox 档案")
			case 2148916238:
				return Grant{}, errors.New("此账户需要家长在 Xbox 家庭设置中授权")
			default:
				return Grant{}, errors.New("Xbox 拒绝此账户访问 Minecraft，请检查 Xbox 账户及地区设置")
			}
		}
		return Grant{}, err
	}
	if xsts.Token == "" || len(xsts.DisplayClaims.XUI) != 1 || xsts.DisplayClaims.XUI[0].Hash != xbox.DisplayClaims.XUI[0].Hash {
		return Grant{}, errors.New("Xbox XSTS 返回的身份不一致")
	}

	var mc struct {
		Access   string `json:"access_token"`
		Lifetime int64  `json:"expires_in"`
	}
	issuedAt := time.Now()
	_, err = m.post(ctx, "Minecraft 登录", minecraftLogin, map[string]string{
		"identityToken": "XBL3.0 x=" + xsts.DisplayClaims.XUI[0].Hash + ";" + xsts.Token,
	}, &mc)
	if err != nil {
		return Grant{}, err
	}
	if mc.Access == "" || len(mc.Access) > 16<<10 || mc.Lifetime <= 30 || mc.Lifetime > 7*24*60*60 {
		return Grant{}, errors.New("Minecraft 返回的会话无效")
	}
	var entitlements struct {
		Items []struct{ Name string }
	}
	if _, err := m.request(ctx, "Minecraft 拥有权", http.MethodGet, minecraftAPI+"/entitlements/mcstore", "", nil, mc.Access, &entitlements); err != nil {
		return Grant{}, err
	}
	if !slices.ContainsFunc(entitlements.Items, func(item struct{ Name string }) bool {
		return item.Name == "game_minecraft" || item.Name == "product_minecraft"
	}) {
		return Grant{}, errors.New("此账户没有可用的 Minecraft Java 版授权")
	}

	var player struct {
		ID, Name string
		Skins    []struct{ State, URL string }
	}
	status, err = m.request(ctx, "Minecraft 玩家档案", http.MethodGet, minecraftAPI+"/minecraft/profile", "", nil, mc.Access, &player)
	if err != nil {
		if status == http.StatusNotFound {
			return Grant{}, errors.New("此账户还没有 Minecraft Java 版玩家档案，请先创建玩家名称")
		}
		return Grant{}, err
	}
	profile := Profile{Kind: Microsoft, Name: player.Name, UUID: strings.ToLower(strings.ReplaceAll(player.ID, "-", ""))}
	if err := profile.Validate(); err != nil {
		return Grant{}, err
	}
	xuid := xsts.DisplayClaims.XUI[0].ID
	if xuid == "" {
		xuid = xbox.DisplayClaims.XUI[0].ID
	}
	if xuid == "" {
		xuid = tokenXUID(mc.Access)
	}
	session, err := launch.NewSession(profile.Name, profile.UUID, mc.Access, m.clientID, xuid, issuedAt.Add(time.Duration(mc.Lifetime)*time.Second))
	if err != nil {
		return Grant{}, err
	}
	for _, skin := range player.Skins {
		if skin.State == "ACTIVE" {
			profile.Head, _ = downloadSkinHead(ctx, m.http, skin.URL)
			break
		}
	}
	if err := ctx.Err(); err != nil {
		return Grant{}, err
	}
	return Grant{Profile: profile, Session: session, refresh: token.Refresh}, nil
}

// Only a game argument uses this optional claim. Authorization is established
// through the Minecraft profile and entitlement endpoints, never by decoding it.
func tokenXUID(access string) string {
	parts := strings.Split(access, ".")
	if len(parts) != 3 {
		return ""
	}
	data, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return ""
	}
	var claims struct {
		XUID string `json:"xuid"`
	}
	if json.Unmarshal(data, &claims) != nil || len(claims.XUID) > 20 || strings.Trim(claims.XUID, "0123456789") != "" {
		return ""
	}
	return claims.XUID
}

func (m *MicrosoftClient) post(ctx context.Context, stage, address string, body any, output any) (int, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return 0, fmt.Errorf("%s请求格式无效", stage)
	}
	return m.request(ctx, stage, http.MethodPost, address, "application/json", bytes.NewReader(data), "", output)
}

// Only local messages and status codes enter errors. Remote bodies and
// transport errors may contain credentials.
func (m *MicrosoftClient) request(ctx context.Context, stage, method, address, contentType string, body io.Reader, bearer string, output any) (int, error) {
	req, err := http.NewRequestWithContext(ctx, method, address, body)
	if err != nil {
		return 0, fmt.Errorf("%s请求无效", stage)
	}
	req.Header.Set("Accept", "application/json")
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	response, err := m.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return 0, ctx.Err()
		}
		return 0, fmt.Errorf("%s网络请求失败", stage)
	}
	defer response.Body.Close()
	const maxResponse = 1 << 20
	data, err := io.ReadAll(io.LimitReader(response.Body, maxResponse+1))
	status := response.StatusCode
	if err != nil || len(data) > maxResponse {
		return status, fmt.Errorf("%s响应无法读取", stage)
	}
	if status < 200 || status >= 300 {
		if xsts, ok := output.(*xboxToken); ok && status == http.StatusUnauthorized {
			_ = json.Unmarshal(data, xsts)
		}
		if address == minecraftLogin && status == http.StatusForbidden {
			var refusal struct {
				ErrorMessage string `json:"errorMessage"`
			}
			if json.Unmarshal(data, &refusal) == nil && strings.Contains(strings.ToLower(refusal.ErrorMessage), "invalid app registration") {
				return status, errors.New("Minecraft 拒绝了应用注册（HTTP 403）。请提交 mcl 的 Client ID 申请 Java 版服务 API 审核")
			}
		}
		return status, fmt.Errorf("%s失败（HTTP %d）", stage, status)
	}
	if err := json.Unmarshal(data, output); err != nil {
		return status, fmt.Errorf("%s响应格式无效", stage)
	}
	return status, nil
}
