package ui

import (
	"strconv"
	"strings"
	"unicode/utf8"

	"mcl/internal/core/market"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
)

type marketVersionState struct {
	project         market.Project
	version         market.Version
	changelog       string
	truncated, busy bool
	err             string
	cancel          func()
	markdown        markdown.Options
	mediaCache      *marketMediaCache
}

func (s *Shell) closeMarketVersion() {
	if state := s.market.version; state != nil && state.cancel != nil {
		state.cancel()
	}
	s.market.version = nil
}

func (s *Shell) openMarketVersion(project market.Project, version market.Version) {
	s.closeMarketVersion()
	state := &marketVersionState{project: project, version: version, mediaCache: newMarketMediaCache()}
	state.changelog, state.truncated = marketDisplayText(version.Changelog)
	state.version.Changelog = ""
	s.market.version = state
	action := s.market.actions.VersionDetail
	if action == nil {
		return
	}
	state.busy = true
	state.cancel = action(project.Source, project.ID, version.ID, func(result market.Version, err error) {
		if s.market.version != state {
			return
		}
		state.busy, state.cancel = false, nil
		if err != nil {
			state.err = err.Error()
			return
		}
		state.changelog, state.truncated = marketDisplayText(result.Changelog)
		result.Changelog = ""
		state.version = result
	})
}

func (s *Shell) marketVersionView(c *ui.Context) {
	state, t := s.market.version, s.theme
	p, v := state.project, state.version
	services := c.Services()
	s.configureMarketMarkdown(c, &state.markdown, state.mediaCache)
	_, installErr := v.PrimaryFileMetadata()
	ui.Row(c).FillWidth().Height(28).Shrink(0).Gap(8).Children(func() {
		iconButton(c, "version-back", "返回版本列表", iconBack, func() { s.closeMarketVersion(); s.market.tab = 1 })
		ui.Text(c, p.Title).FontSize(12).TextColor(t.TextMuted).Grow(1).MinWidth(0).MaxLines(1).Ellipsis("…")
		label, disabled, action := s.marketVersionAction(p, v, installErr != nil)
		ui.PrimaryButton(c.Key("version-install"), label).Height(26).Disabled(disabled).OnClick(action)
	})
	ui.Scroll(c.Key("market-version-detail")).Grow(1).MinHeight(0).FillWidth().Gap(14).Children(func() {
		ui.Column(c).FillWidth().Gap(6).Children(func() {
			ui.Text(c, v.Name).FontSize(18).FontWeight(500).FillWidth().MaxLines(2)
			ui.Text(c, marketReleaseType(v.Type)+" · "+v.Published.Format("2006-01-02")).FontSize(11).TextColor(colors(t).Metadata)
		})
		ui.Row(c).FillWidth().Wrap().Gap(5).Children(func() {
			for _, game := range firstStrings(v.GameVersions, 24) {
				marketTag(c, game, false)
			}
		})
		if len(v.Loaders) > 0 || v.Environment != "" {
			ui.Row(c).FillWidth().Wrap().Gap(5).Children(func() {
				for _, loader := range firstStrings(v.Loaders, 12) {
					s.marketLoaderTag(c, loader)
				}
				if v.Environment != "" {
					ui.Box(c).Children(func() { marketTag(c, marketEnvironmentLabel(v.Environment), false) })
				}
			})
		}
		if installErr != nil && s.marketAssociation() == nil {
			ui.Text(c, installErr.Error()).FontSize(11).TextColor(t.Warning).MaxLines(3)
		}
		ui.Column(c).FillWidth().Gap(4).Children(func() {
			for index, file := range v.Files {
				if index >= 64 {
					break
				}
				ui.ButtonBase(c.Key("version-file-"+strconv.Itoa(index))).Label("复制文件摘要 "+file.Filename).FillWidth().Height(34).Gap(8).Padding(0, 8).Radius(6).Background(t.Surface).Disabled(file.SHA1 == "").OnClick(func() { services.WriteClipboard(file.SHA1) }).Children(func() {
					ui.Icon(c, iconFile).Size(14, 14).TextColor(colors(t).Icon)
					ui.Text(c, file.Filename).FontSize(11).Grow(1).MinWidth(0).MaxLines(1).Ellipsis("…")
					ui.Text(c, formatBytes(file.Size)).FontSize(10).TextColor(colors(t).Metadata)
				})
			}
		})
		ui.Divider(c)
		if state.busy {
			ui.Row(c.Key("changelog-loading")).Gap(7).Children(func() {
				busyIndicator(c, 12)
				ui.Text(c, "读取更新日志…").FontSize(11).TextColor(t.TextMuted)
			})
		}
		if state.err != "" {
			ui.Row(c).FillWidth().Gap(8).Children(func() {
				ui.Text(c, state.err).FontSize(11).TextColor(t.Danger).Grow(1).MinWidth(0).MaxLines(3)
				iconButton(c, "retry-version-detail", "重试更新日志", iconRefresh, func() { s.openMarketVersion(p, v) })
			})
		}
		if state.changelog != "" {
			markdown.View(c, state.changelog, &state.markdown).FillWidth()
		} else if !state.busy && state.err == "" {
			ui.Text(c, "暂无更新日志").FontSize(11).TextColor(t.TextMuted)
		}
		if market.SafeURL(p.PageURL) {
			label := "项目主页"
			if state.truncated {
				label = "查看完整说明"
			}
			ui.Button(c.Key("version-website"), label).Height(26).Width(116).OnClick(func() { services.OpenURL(p.PageURL) })
		}
	})
}

func marketDisplayText(value string) (string, bool) {
	const limit = 256 << 10
	value = strings.TrimSpace(value)
	if len(value) <= limit {
		return value, false
	}
	end := limit
	for end > 0 && !utf8.RuneStart(value[end]) {
		end--
	}
	return value[:end], true
}

func marketReleaseType(value string) string {
	switch value {
	case "release":
		return "正式版"
	case "beta":
		return "Beta"
	case "alpha":
		return "Alpha"
	default:
		return value
	}
}

func marketEnvironmentLabel(value string) string {
	switch value {
	case "client_only":
		return "仅客户端"
	case "server_only":
		return "服务端 / 单人游戏"
	case "dedicated_server_only":
		return "仅独立服务端"
	case "singleplayer_only":
		return "仅单人游戏"
	case "client_and_server":
		return "客户端与服务端"
	case "client_only_server_optional":
		return "客户端 / 服务端可选"
	case "server_only_client_optional":
		return "服务端 / 客户端可选"
	case "client_or_server_prefers_both":
		return "任一端 / 推荐双端"
	case "client_or_server":
		return "客户端或服务端"
	default:
		return "环境未标注"
	}
}
