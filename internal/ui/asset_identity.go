package ui

import (
	"fmt"

	"mcl/internal/app"
	"mcl/internal/core/market"

	"github.com/egoist/mygo/ui"
)

type assetIdentityState struct {
	instanceID, name          string
	source                    market.Source
	active, canceling, failed bool
	progress                  app.AssetIdentityProgress
	result                    app.AssetIdentityResult
	message                   string
	cancel                    func()
}

func (s *Shell) identifyingAssets(id string) bool {
	state := s.assets.identity
	return state != nil && state.active && state.instanceID == id
}

func (s *Shell) startAssetIdentity(source market.Source) {
	a := &s.assets
	value, ok := s.selectedInstance()
	if !ok || a.busy || a.changing || s.diagnosingAssets(value.ID) || s.assetUpdateBlocked(value.ID) || a.actions.Identify == nil {
		return
	}
	if a.identity != nil && a.identity.active {
		s.page = activityPage
		return
	}
	state := &assetIdentityState{instanceID: value.ID, name: value.Name, source: source, active: true}
	a.identity = state
	state.cancel = a.actions.Identify(value.ID, source, func(progress app.AssetIdentityProgress) {
		if a.identity == state && state.active {
			state.progress = progress
		}
	}, func(result app.AssetIdentityResult, err error) {
		if a.identity != state {
			return
		}
		state.active, state.cancel, state.result = false, nil, result
		state.message = "没有可识别文件"
		if result.Matched > 0 {
			state.message = fmt.Sprintf("已识别 %d 项", result.Matched)
		}
		if result.Unmatched > 0 {
			if result.Matched > 0 {
				state.message += " · "
			} else {
				state.message = ""
			}
			state.message += fmt.Sprintf("未匹配 %d 项", result.Unmatched)
		}
		if result.Skipped > 0 {
			state.message += fmt.Sprintf(" · 跳过 %d 项", result.Skipped)
		}
		if result.Truncated {
			state.message += " · 列表已达到读取上限"
		}
		if err != nil {
			state.failed = true
			state.message = fmt.Sprintf("%s · 已保存 %d 项", err.Error(), result.Matched)
			if state.canceling {
				state.failed = false
				state.message = fmt.Sprintf("已取消 · 已保存 %d 项", result.Matched)
			}
		}
		state.canceling = false
		if value.ID == s.library.selectedID {
			s.refreshAssets()
		}
	})
}

func (s *Shell) cancelAssetIdentity() {
	state := s.assets.identity
	if state != nil && state.active && !state.canceling && state.cancel != nil {
		state.canceling = true
		state.cancel()
	}
}

func (s *Shell) assetIdentityButton(c *ui.Context) {
	a, t := &s.assets, s.theme
	button := ui.ButtonBase(c.Key("identify-assets")).Label("识别资产").Tooltip("按文件摘要识别来源；只发送摘要或指纹，不上传文件").Size(28, 28).Radius(6).Center().Children(func() { ui.Icon(c, iconScan).Size(17, 17).TextColor(colors(t).Icon) })
	if a.identity != nil && a.identity.active {
		button.OnClick(func() { s.page = activityPage })
		return
	}
	button.Disabled(a.busy || a.changing || s.diagnosingAssets(s.library.selectedID) || s.assetUpdateBlocked(s.library.selectedID) || a.actions.Identify == nil || len(a.snapshot.Files) == 0).Menu(func(menu *ui.Menu) {
		if menu.Item("Modrinth").Chosen() {
			s.startAssetIdentity(market.Modrinth)
		}
		label := "CurseForge"
		if !s.market.actions.CurseForgeAvailable {
			label += " · 需要 API Key"
		}
		if menu.Item(label).Disabled(!s.market.actions.CurseForgeAvailable).Chosen() {
			s.startAssetIdentity(market.CurseForge)
		}
	})
}

func assetIdentityStage(value string) string {
	switch value {
	case "identify-lookup":
		return "匹配文件来源"
	case "identify-projects":
		return "读取项目信息"
	case "identify-save":
		return "校验并保存来源"
	default:
		return "计算文件摘要"
	}
}

func (s *Shell) assetIdentityStatus(c *ui.Context, id string) {
	state, t := s.assets.identity, s.theme
	if state == nil || state.instanceID != id {
		return
	}
	ui.Row(c.Key("asset-identify-status")).FillWidth().Height(24).Shrink(0).Gap(8).Children(func() {
		color := t.TextMuted
		message := state.message
		if state.active {
			message = assetIdentityStage(state.progress.Stage)
			busyIndicator(c, 12)
		}
		if state.failed {
			color = t.Danger
		}
		ui.Text(c, message).FontSize(11).TextColor(color).Grow(1).MinWidth(0).MaxLines(1).Ellipsis("…")
		if state.active {
			ui.Button(c.Key("identify-task"), "查看任务").Height(22).OnClick(func() { s.page = activityPage })
		} else {
			ui.Button(c.Key("identify-dismiss"), "收起").Height(22).OnClick(func() {
				if s.assets.identity == state {
					s.assets.identity = nil
				}
			})
		}
	})
}

func (s *Shell) assetIdentityTaskCard(c *ui.Context) {
	state, t := s.assets.identity, s.theme
	if state == nil {
		return
	}
	section(c, state.name, func() {
		ui.Row(c).FillWidth().Gap(8).Children(func() {
			label := state.message
			if state.active {
				label = assetIdentityStage(state.progress.Stage)
				busyIndicator(c, 13)
			}
			color := t.TextMuted
			if state.failed {
				color = t.Danger
			}
			ui.Text(c, label).FontSize(12).TextColor(color).Grow(1).MinWidth(0).MaxLines(3)
			ui.Button(c.Key("identity-view-instance"), "查看实例").Height(25).OnClick(func() { s.selectInstance(state.instanceID) })
			if state.active {
				label := "取消"
				if state.canceling {
					label = "正在取消…"
				}
				ui.Button(c.Key("identity-cancel"), label).Height(25).Disabled(state.canceling).OnClick(s.cancelAssetIdentity)
			}
		})
		if state.active {
			progress := state.progress.Progress
			ui.Textf(c, "%s · %d / %d", state.source, progress.Files, progress.TotalFiles).FontSize(11).TextColor(t.TextMuted)
			if progress.TotalBytes > 0 {
				smoothProgress(c, float64(progress.Bytes)/float64(progress.TotalBytes)).FillWidth().Height(4)
			} else if progress.TotalFiles > 0 {
				smoothProgress(c, float64(progress.Files)/float64(progress.TotalFiles)).FillWidth().Height(4)
			}
		}
	})
}
