package ui

import (
	"errors"
	"strconv"
	"strings"

	"mcl/internal/core/download"

	"github.com/egoist/mygo/ui"
)

type downloadSettingsState struct {
	cacheDirectory                 string
	customConcurrency, customSpeed bool
	automatic                      bool
	concurrency                    string
	speedLimited                   bool
	speed                          string
	source                         download.SourceMode
	proxyMode                      download.ProxyMode
	proxyHost, proxyPort           string
}

func newDownloadSettingsState(preferences download.Preferences) downloadSettingsState {
	concurrency, speed := preferences.Concurrency, preferences.SpeedLimitKiB
	if concurrency == 0 {
		concurrency = 6
	}
	if speed == 0 {
		speed = 1024
	}
	host, port := preferences.Proxy.Host, preferences.Proxy.Port
	if host == "" {
		host = "127.0.0.1"
	}
	if port == 0 {
		port = 7890
	}
	return downloadSettingsState{
		cacheDirectory:    preferences.CacheDirectory,
		customConcurrency: preferences.Concurrency != 0 && !downloadPresetContains(concurrencyChoices, strconv.Itoa(preferences.Concurrency)),
		customSpeed:       preferences.SpeedLimitKiB != 0 && !downloadPresetContains(speedChoices, strconv.Itoa(preferences.SpeedLimitKiB)),
		automatic:         preferences.Concurrency == 0, concurrency: strconv.Itoa(concurrency),
		speedLimited: preferences.SpeedLimitKiB != 0, speed: strconv.Itoa(speed), source: preferences.Source,
		proxyMode: preferences.Proxy.Mode, proxyHost: host, proxyPort: strconv.Itoa(port),
	}
}

func (d downloadSettingsState) preferences() (download.Preferences, error) {
	value := download.Preferences{CacheDirectory: d.cacheDirectory, Source: d.source, Proxy: download.Proxy{Mode: d.proxyMode}}
	if !d.automatic {
		concurrency, err := strconv.Atoi(strings.TrimSpace(d.concurrency))
		if err != nil || concurrency < 1 || concurrency > 128 {
			return value, errors.New("下载并发数应为 1–128 的整数")
		}
		value.Concurrency = concurrency
	}
	if d.speedLimited {
		speed, err := strconv.Atoi(strings.TrimSpace(d.speed))
		if err != nil || speed < 1 || speed > 1048576 {
			return value, errors.New("下载限速应为 1–1048576 KiB/s 的整数")
		}
		value.SpeedLimitKiB = speed
	}
	if d.proxyMode == download.ProxyHTTP || d.proxyMode == download.ProxySOCKS5 {
		port, err := strconv.Atoi(strings.TrimSpace(d.proxyPort))
		if err != nil || port < 1 || port > 65535 {
			return value, errors.New("代理端口应为 1–65535")
		}
		value.Proxy.Host, value.Proxy.Port = strings.TrimSpace(d.proxyHost), port
	}
	return value, value.Validate()
}

type downloadChoice struct{ value, label string }

var sourceChoices = []downloadChoice{
	{string(download.SourceOfficial), "仅官方"},
	{string(download.SourceAuto), "自动"},
	{string(download.SourceOfficialFirst), "官方优先"},
	{string(download.SourceMirrorFirst), "镜像优先"},
}
var proxyChoices = []downloadChoice{
	{string(download.ProxyEnvironment), "环境变量"},
	{string(download.ProxyDirect), "直连"},
	{string(download.ProxyHTTP), "HTTP"},
	{string(download.ProxySOCKS5), "SOCKS5"},
}

var concurrencyChoices = []downloadChoice{
	{"auto", "自动"}, {"2", "2"}, {"4", "4"}, {"8", "8"}, {"16", "16"}, {"32", "32"}, {"64", "64"}, {"custom", "自定义"},
}
var speedChoices = []downloadChoice{
	{"unlimited", "不限速"}, {"512", "512 KiB/s"}, {"1024", "1 MiB/s"}, {"2048", "2 MiB/s"}, {"5120", "5 MiB/s"}, {"10240", "10 MiB/s"}, {"custom", "自定义"},
}

func downloadPresetContains(choices []downloadChoice, value string) bool {
	for _, choice := range choices {
		if choice.value == value {
			return true
		}
	}
	return false
}

func downloadSettingMenu(c *ui.Context, key, label, selected string, choices []downloadChoice, blocked bool, choose func(string)) {
	name := "未知"
	for _, choice := range choices {
		if choice.value == selected {
			name = choice.label
			break
		}
	}
	t := c.Theme()
	width := float32(144)
	if selected == "custom" {
		width = 94
	}
	ui.ButtonBase(c.Key(key)).Label(label).Width(width).MinWidth(0).Height(26).Radius(6).Padding(0, 9).Gap(8).Background(t.Surface).Disabled(blocked).
		Menu(func(menu *ui.Menu) {
			for _, choice := range choices {
				if menu.Item(choice.label).Checked(choice.value == selected).Chosen() {
					choose(choice.value)
				}
			}
		}).Children(func() {
		ui.Text(c, name).FontSize(12).Grow(1).MinWidth(0).MaxLines(1).Ellipsis("…")
		ui.Icon(c, iconChevronDown).Size(12, 12).Shrink(0).TextColor(colors(t).Metadata)
	})
}

func (s *Shell) downloadSettingsView(c *ui.Context) {
	t, draft := s.theme, &s.downloadSettings
	blocked := s.loadErr != nil || s.save == nil
	ui.Column(c.Key("download-settings")).Grow(1).MinHeight(0).FillWidth().Children(func() {
		ui.Scroll(c.Key("download-settings-scroll")).Grow(1).MinHeight(0).FillWidth().Padding(18, 24, 24).Gap(12).Children(func() {
			ui.Column(c.Key("download-network-settings")).FillWidth().Radius(10).Border(0.5, colors(t).ListDivider).Clip().Children(func() {
				gameSettingRow(c, "source", "下载源", func() {
					ui.Spacer(c)
					downloadSettingMenu(c, "download-source", "下载源策略", string(draft.source), sourceChoices, blocked, func(value string) { draft.source = download.SourceMode(value); s.saveSettings() })
				})
				gameSettingsDivider(c)
				gameSettingRow(c, "concurrency", "并发下载", func() {
					ui.Spacer(c)
					selected := "auto"
					if !draft.automatic {
						selected = draft.concurrency
						if draft.customConcurrency {
							selected = "custom"
							ui.TextInput(c.Key("download-concurrency"), &draft.concurrency).Label("自定义下载并发数").Placeholder("1–128").Width(72).Height(26).Disabled(blocked).OnChange(s.saveSettings).OnSubmit(s.saveSettings)
						}
					}
					downloadSettingMenu(c, "download-concurrency-mode", "并发下载", selected, concurrencyChoices, blocked, func(value string) {
						draft.automatic = value == "auto"
						draft.customConcurrency = value == "custom"
						if value != "auto" && value != "custom" {
							draft.concurrency = value
						}
						s.saveSettings()
					})
				})
				gameSettingsDivider(c)
				gameSettingRow(c, "speed-limit", "速度限制", func() {
					ui.Spacer(c)
					selected := "unlimited"
					if draft.speedLimited {
						selected = draft.speed
						if draft.customSpeed {
							selected = "custom"
							ui.TextInput(c.Key("download-speed-value"), &draft.speed).Label("自定义下载速率上限，KiB/s").Width(72).Height(26).Disabled(blocked).OnChange(s.saveSettings).OnSubmit(s.saveSettings)
							ui.Text(c, "KiB/s").FontSize(11).TextColor(colors(t).Metadata)
						}
					}
					downloadSettingMenu(c, "download-speed-mode", "下载速率上限", selected, speedChoices, blocked, func(value string) {
						draft.speedLimited = value != "unlimited"
						draft.customSpeed = value == "custom"
						if value != "unlimited" && value != "custom" {
							draft.speed = value
						}
						s.saveSettings()
					})
				})
				gameSettingsDivider(c)
				gameSettingRow(c, "proxy-mode", "下载代理", func() {
					ui.Spacer(c)
					downloadSettingMenu(c, "download-proxy-mode", "下载代理类型", string(draft.proxyMode), proxyChoices, blocked, func(value string) { draft.proxyMode = download.ProxyMode(value); s.saveSettings() })
				})
				if draft.proxyMode == download.ProxyHTTP || draft.proxyMode == download.ProxySOCKS5 {
					gameSettingsDivider(c)
					gameSettingRow(c, "proxy-address", "代理地址", func() {
						ui.Spacer(c)
						ui.TextInput(c.Key("download-proxy-host"), &draft.proxyHost).Label("代理主机").Placeholder("主机").Width(156).MinWidth(0).Height(26).Disabled(blocked).OnChange(s.saveSettings).OnSubmit(s.saveSettings)
						ui.Text(c, ":").FontSize(12).TextColor(colors(t).Metadata)
						ui.TextInput(c.Key("download-proxy-port"), &draft.proxyPort).Label("代理端口").Placeholder("端口").Width(66).Height(26).Disabled(blocked).OnChange(s.saveSettings).OnSubmit(s.saveSettings)
					})
				}
			})
			s.downloadToolsView(c, blocked)
		})
	})
}
