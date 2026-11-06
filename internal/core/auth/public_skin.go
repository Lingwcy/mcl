package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

var (
	publicUUID     = regexp.MustCompile(`^[a-f0-9]{32}$`)
	texturePath    = regexp.MustCompile(`^/texture/[a-f0-9]{32,64}$`)
	publicSkinHTTP = &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
)

var errNoPublicSkin = errors.New("未找到同名 Minecraft 玩家的公开皮肤")

// PublicPlayerHead looks up public artwork only. It never returns an online
// identity or requires a Microsoft application or user session.
func PublicPlayerHead(ctx context.Context, name string) ([]byte, error) {
	if !playerName.MatchString(name) {
		return nil, errNoPublicSkin
	}
	ctx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()

	var player struct{ ID, Name string }
	if err := readPublicProfile(ctx, "https://api.mojang.com/users/profiles/minecraft/"+name, &player); err != nil {
		return nil, err
	}
	if !publicUUID.MatchString(player.ID) || !strings.EqualFold(player.Name, name) {
		return nil, errNoPublicSkin
	}
	var session struct {
		ID         string
		Properties []struct{ Name, Value string }
	}
	if err := readPublicProfile(ctx, "https://sessionserver.mojang.com/session/minecraft/profile/"+player.ID, &session); err != nil {
		return nil, err
	}
	if session.ID != player.ID {
		return nil, errNoPublicSkin
	}
	for _, property := range session.Properties {
		if property.Name != "textures" {
			continue
		}
		data, err := base64.StdEncoding.DecodeString(property.Value)
		if err != nil {
			return nil, errNoPublicSkin
		}
		var texture struct {
			ProfileID string
			Textures  struct{ Skin struct{ URL string } }
		}
		if json.Unmarshal(data, &texture) != nil || texture.ProfileID != player.ID || texture.Textures.Skin.URL == "" {
			return nil, errNoPublicSkin
		}
		return downloadSkinHead(ctx, publicSkinHTTP, texture.Textures.Skin.URL)
	}
	return nil, errNoPublicSkin
}

func readPublicProfile(ctx context.Context, address string, result any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return errors.New("无法发起公开玩家查询")
	}
	resp, err := publicSkinHTTP.Do(req)
	if err != nil {
		return errors.New("无法连接 Minecraft 公开档案服务")
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusNotFound, http.StatusNoContent:
		return errNoPublicSkin
	case http.StatusOK:
	default:
		return fmt.Errorf("Minecraft 公开档案查询失败（HTTP %d）", resp.StatusCode)
	}
	const limit = 32 << 10
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil || len(data) > limit || json.Unmarshal(data, result) != nil {
		return errors.New("Minecraft 公开档案响应无效")
	}
	return nil
}

func downloadSkinHead(ctx context.Context, client *http.Client, address string) ([]byte, error) {
	target, err := url.Parse(address)
	if err != nil || (target.Scheme != "https" && target.Scheme != "http") || target.Host != "textures.minecraft.net" || target.User != nil || !texturePath.MatchString(target.EscapedPath()) || target.RawQuery != "" || target.Fragment != "" {
		return nil, errors.New("Minecraft 皮肤地址无效")
	}
	target.Scheme = "https"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return nil, errors.New("无法发起 Minecraft 皮肤请求")
	}
	response, err := client.Do(req)
	if err != nil {
		return nil, errors.New("无法连接 Minecraft 纹理服务")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Minecraft 皮肤下载失败（HTTP %d）", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, MaxSkinBytes+1))
	if err != nil {
		return nil, errors.New("无法读取 Minecraft 皮肤")
	}
	return SkinHead(data)
}
