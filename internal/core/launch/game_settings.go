package launch

import (
	"errors"
	"maps"
	"net"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"mcl/internal/core/meta"

	"golang.org/x/net/idna"
)

// GameSettings is a value-only launch preference, independent of UI and storage.
// Zero dimensions retain the default resolution for older configuration files.
type GameSettings struct {
	Priority       ProcessPriority `json:"process_priority,omitempty"`
	ShowLogs       bool            `json:"show_logs,omitempty"`
	Width          int             `json:"width,omitempty"`
	Height         int             `json:"height,omitempty"`
	Fullscreen     bool            `json:"fullscreen,omitempty"`
	CustomInfo     string          `json:"custom_info,omitempty"`
	AutoJoinServer bool            `json:"auto_join_server,omitempty"`
	ServerAddress  string          `json:"server_address,omitempty"`
}

func (g GameSettings) Resolution() (int, int) {
	if g.Width == 0 && g.Height == 0 {
		return 854, 480
	}
	return g.Width, g.Height
}

func (g GameSettings) Validate() error {
	if err := g.Priority.Validate(); err != nil {
		return err
	}
	width, height := g.Resolution()
	if width < 320 || width > 16384 || height < 200 || height > 16384 {
		return errors.New("游戏分辨率宽度应为 320–16384，高度应为 200–16384")
	}
	if !safeText(g.CustomInfo) || utf8.RuneCountInString(g.CustomInfo) > 120 {
		return errors.New("自定义信息最多 120 个字符，且不能包含控制字符")
	}
	if strings.HasPrefix(strings.TrimSpace(g.CustomInfo), "-") {
		return errors.New("自定义信息不能以连字符开头")
	}
	if g.AutoJoinServer || g.ServerAddress != "" {
		if _, _, err := g.Server(); err != nil {
			return err
		}
	}
	return nil
}

func supportsQuickPlay(arguments []meta.Argument) bool {
	for _, argument := range arguments {
		if !slices.Contains(argument.Values, "--quickPlayMultiplayer") {
			continue
		}
		for _, rule := range argument.Rules {
			if rule.Features["is_quick_play_multiplayer"] {
				return true
			}
		}
	}
	return false
}

func gameArguments(templates []string, values map[string]string, settings GameSettings, quickPlay bool) ([]string, error) {
	width, height := settings.Resolution()
	gameValues := maps.Clone(values)
	gameValues["resolution_width"], gameValues["resolution_height"] = strconv.Itoa(width), strconv.Itoa(height)
	if info := strings.TrimSpace(settings.CustomInfo); info != "" {
		// The JVM/loader version name and physical jar paths remain untouched.
		gameValues["version_name"], gameValues["version_type"] = info, info
	}
	var host string
	var port int
	if settings.AutoJoinServer {
		var err error
		host, port, err = settings.Server()
		if err != nil {
			return nil, err
		}
		gameValues["quickPlayMultiplayer"] = net.JoinHostPort(host, strconv.Itoa(port))
	}
	var result []string
	for _, template := range templates {
		arg, err := substitute(template, gameValues)
		if err != nil {
			return nil, err
		}
		result = append(result, arg)
	}
	// Inspect raw templates, not substituted identity values which might happen
	// to spell an option. Never parse user names or tokens as launcher flags.
	if settings.Width != 0 && settings.Height != 0 {
		if !slices.Contains(templates, "--width") {
			result = append(result, "--width", strconv.Itoa(width))
		}
		if !slices.Contains(templates, "--height") {
			result = append(result, "--height", strconv.Itoa(height))
		}
	}
	if settings.Fullscreen && !slices.Contains(templates, "--fullscreen") {
		result = append(result, "--fullscreen")
	}
	if settings.AutoJoinServer {
		if quickPlay {
			if !slices.Contains(templates, "--quickPlayMultiplayer") {
				return nil, errors.New("当前平台的元数据未提供可用的多人 Quick Play 参数")
			}
		} else {
			if slices.Contains(templates, "--server") || slices.Contains(templates, "--port") {
				return nil, errors.New("游戏元数据已有服务器参数，不能重复添加自动加入设置")
			}
			result = append(result, "--server", host, "--port", strconv.Itoa(port))
		}
	}
	return result, nil
}

// Server parses only a Minecraft host and optional port, never a URL or command.
// IPv6 may be written bare or as [address]:port. IDNs are normalized to ASCII.
func (g GameSettings) Server() (host string, port int, err error) {
	value := strings.TrimSpace(g.ServerAddress)
	invalid := errors.New("请输入有效的服务器地址和可选端口，例如 mc.example.com:25565 或 [::1]:25565")
	if value == "" || len(value) > 512 || !safeText(g.ServerAddress) || strings.ContainsAny(value, "/\\@?# 	") {
		return "", 0, invalid
	}
	host, port = value, 25565
	if strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]") {
		host = value[1 : len(value)-1]
		if net.ParseIP(host) == nil {
			return "", 0, invalid
		}
	} else if strings.Count(value, ":") == 1 || strings.HasPrefix(value, "[") {
		bracketed := strings.HasPrefix(value, "[")
		var rawPort string
		host, rawPort, err = net.SplitHostPort(value)
		if err != nil {
			return "", 0, invalid
		}
		port, err = strconv.Atoi(rawPort)
		if err != nil || port < 1 || port > 65535 {
			return "", 0, invalid
		}
		// Square brackets are the IPv6 form only; a bracketed hostname would
		// reach the game as a literal address the client cannot resolve.
		if bracketed && net.ParseIP(host) == nil {
			return "", 0, invalid
		}
	} else if strings.Contains(value, ":") && net.ParseIP(value) == nil {
		return "", 0, invalid
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.String(), port, nil
	}
	host, err = idna.Lookup.ToASCII(host)
	if err != nil || len(host) > 253 || host == "" {
		return "", 0, invalid
	}
	for _, label := range strings.Split(strings.TrimSuffix(host, "."), ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return "", 0, invalid
		}
		for _, char := range label {
			if char != '-' && char != '_' && !(char >= 'a' && char <= 'z') && !(char >= 'A' && char <= 'Z') && !(char >= '0' && char <= '9') {
				return "", 0, invalid
			}
		}
	}
	return host, port, nil
}
