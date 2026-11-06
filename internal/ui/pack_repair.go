package ui

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"mcl/internal/app"
	"mcl/internal/core/download"
	"mcl/internal/core/instance"

	"github.com/egoist/mygo/ui"
)

const restoringPackFiles = "正在恢复文件"

type packRepairState struct {
	id, name     string
	plan         app.PackRepairPlan
	selected     map[string]bool
	list         ui.ListState
	job, message string
	failed       bool
	progress     download.Progress
	cancel       func()
}

func (s *Shell) repairingPack(id string) bool {
	state := s.assets.repair
	return state != nil && state.id == id && state.job == restoringPackFiles
}

func (s *Shell) openPackRepair(value instance.Instance) {
	state := s.assets.repair
	if state != nil && state.id == value.ID {
		s.viewPackRepair(state)
		return
	}
	if state != nil && state.job != "" {
		return
	}
	state = &packRepairState{id: value.ID, name: value.Name}
	s.assets.repair = state
	s.viewPackRepair(state)
	s.inspectPackRepair(state)
}

func (s *Shell) viewPackRepair(state *packRepairState) {
	s.selectInstance(state.id)
	s.assets.showRepair = true
}

func (s *Shell) inspectPackRepair(state *packRepairState) {
	if state.job != "" {
		return
	}
	state.job, state.progress = "正在检查文件", download.Progress{}
	state.cancel = s.market.actions.InspectPackRepair(state.id, func(plan app.PackRepairPlan, err error) {
		if !s.finishPackRepairJob(state, err) {
			return
		}
		state.plan, state.list = plan, ui.ListState{}
		state.selected = make(map[string]bool)
	})
}

func (s *Shell) supplyPackRepair(state *packRepairState) {
	if state.job != "" {
		return
	}
	state.job = "正在核验本地文件"
	state.cancel = s.market.actions.SupplyPackRepair(state.plan, func(result app.LocalFileSupply, err error) {
		if !s.finishPackRepairJob(state, err) {
			return
		}
		if len(result.Matched) == 0 && result.Skipped == 0 {
			return
		}
		state.message = fmt.Sprintf("已核验 %d 项", len(result.Matched))
		if result.Skipped > 0 {
			state.message += fmt.Sprintf(" · %d 个文件未匹配", result.Skipped)
			if len(result.Warnings) > 0 {
				state.message += " · " + strings.Join(result.Warnings, " · ")
			}
		}
		s.inspectPackRepair(state)
	})
}

func (s *Shell) finishPackRepairJob(state *packRepairState, err error) bool {
	if s.assets.repair != state {
		return false
	}
	state.job, state.cancel = "", nil
	state.failed = err != nil && !errors.Is(err, context.Canceled)
	if state.failed {
		state.message = err.Error()
	} else if err != nil {
		state.message = "已取消"
	}
	return err == nil
}

func (s *Shell) restorePack(state *packRepairState) {
	if s.packRepairBlocked(state) {
		return
	}
	var paths []string
	for _, entry := range state.plan.Entries {
		if state.selected[entry.Path] && !entry.NeedsLocal {
			paths = append(paths, entry.Path)
		}
	}
	if len(paths) == 0 || len(paths) > app.MaxPackRepairSelection {
		return
	}
	state.job, state.message, state.progress = restoringPackFiles, "", download.Progress{}
	state.cancel = s.market.actions.RestorePack(state.plan, paths, func(progress download.Progress) {
		if s.assets.repair == state {
			state.progress = progress
		}
	}, func(count int, err error) {
		if !s.finishPackRepairJob(state, err) {
			return
		}
		state.message = fmt.Sprintf("已恢复 %d 项", count)
		if s.library.selectedID == state.id {
			s.refreshAssets()
		}
		s.inspectPackRepair(state)
	})
}

func (s *Shell) packRepairBlocked(state *packRepairState) bool {
	i := &s.installation
	return state.job != "" || s.assetUpdateBlocked(state.id) || s.assets.changeID == state.id ||
		(i.active && i.instanceID == state.id) || s.diagnosingAssets(state.id) || s.identifyingAssets(state.id)
}

func (s *Shell) packRepairView(c *ui.Context) {
	state, t := s.assets.repair, s.theme
	busy := state.job != ""
	needLocal := false
	for _, entry := range state.plan.Entries {
		needLocal = needLocal || entry.NeedsLocal
	}
	ui.Column(c.Key("pack-repair-"+state.id)).Grow(1).MinHeight(0).FillWidth().Padding(16, 24, 20).Gap(10).Children(func() {
		ui.Row(c).FillWidth().Height(30).Gap(6).Children(func() {
			iconButton(c, "pack-repair-back", "返回实例", iconBack, func() { s.assets.showRepair = false })
			ui.Text(c, "整合包文件").FontSize(16).FontWeight(600).Grow(1)
			var refresh func()
			if !busy {
				refresh = func() {
					state.message = ""
					s.inspectPackRepair(state)
				}
			}
			iconButton(c, "pack-repair-check", "重新检查", iconRefresh, refresh)
		})
		s.packRepairStatus(c, state, "pack-repair")
		if state.plan.Legacy {
			ui.Text(c, "旧版 CurseForge 导入记录只支持恢复覆盖文件").FontSize(11).TextColor(t.Warning)
		}
		if state.plan.Receipt == "" {
			return
		}
		if len(state.plan.Entries) == 0 && !busy {
			marketStatus(c, "未发现可恢复的缺失文件", iconCheck)
			return
		}
		ui.Row(c).FillWidth().Height(27).Gap(8).Children(func() {
			ui.Textf(c, "%d 项缺失", len(state.plan.Entries)).FontSize(11).TextColor(t.TextMuted).Grow(1)
			if needLocal && s.market.actions.SupplyPackRepair != nil {
				ui.Button(c.Key("pack-repair-supply"), "从本地补齐").Height(25).Disabled(busy).OnClick(func() { s.supplyPackRepair(state) })
			}
			ui.Button(c.Key("pack-repair-select"), "选择一批").Height(25).Disabled(busy).OnClick(func() {
				clear(state.selected)
				for _, entry := range state.plan.Entries {
					if !entry.NeedsLocal && len(state.selected) < app.MaxPackRepairSelection {
						state.selected[entry.Path] = true
					}
				}
			})
			ui.PrimaryButton(c.Key("pack-repair-restore"), "恢复所选").Height(26).
				Disabled(len(state.selected) == 0 || s.packRepairBlocked(state)).
				OnClick(func() { s.restorePack(state) })
		})
		state.list.Key = func(index int) any { return state.plan.Entries[index].Path }
		ui.List(c.Key("pack-repair-files"), &state.list, len(state.plan.Entries), func(index int) {
			entry := state.plan.Entries[index]
			checked := state.selected[entry.Path]
			ui.Row(c).Height(34).FillWidth().Gap(8).Children(func() {
				ui.Checkbox(c.Key("pack-repair-select-file"), &checked, "").Label("恢复 " + entry.Path).
					Disabled(busy || entry.NeedsLocal || (!checked && len(state.selected) >= app.MaxPackRepairSelection)).
					OnClick(func() {
						if checked {
							state.selected[entry.Path] = true
						} else {
							delete(state.selected, entry.Path)
						}
					})
				ui.Text(c, entry.Path).FontSize(12).Grow(1).MinWidth(0).MaxLines(1).Ellipsis("…")
				if entry.NeedsLocal {
					ui.Text(c, "待补齐").FontSize(11).TextColor(t.Warning)
				} else if entry.Override {
					ui.Text(c, "覆盖文件").FontSize(11).TextColor(t.TextMuted)
				}
			})
		}).Grow(1).MinHeight(0).FillWidth()
	})
}

func (s *Shell) packRepairStatus(c *ui.Context, state *packRepairState, key string) {
	message, color := state.message, s.theme.TextMuted
	if state.failed {
		color = s.theme.Danger
	}
	if state.job != "" {
		message, color = state.job, s.theme.TextMuted
	}
	if message == "" {
		return
	}
	ui.Row(c).FillWidth().Gap(8).Children(func() {
		if state.job != "" {
			busyIndicator(c, 13)
		}
		ui.Text(c.Key(key+"-message"), message).FontSize(12).TextColor(color).Grow(1).MinWidth(0).MaxLines(3).ContentTransition(ui.ContentTransition{Animation: &motionFade})
		if state.cancel != nil {
			ui.Button(c.Key(key+"-cancel"), "取消").Height(25).OnClick(func() {
				if state.cancel != nil {
					state.cancel()
					state.cancel = nil
				}
			})
		}
	})
	if state.job != "" && state.progress.TotalBytes > 0 {
		smoothProgress(c, float64(state.progress.Bytes)/float64(state.progress.TotalBytes)).FillWidth().Height(4)
	}
}

func (s *Shell) packRepairTaskCard(c *ui.Context) {
	state := s.assets.repair
	if state == nil || (state.job == "" && state.message == "") {
		return
	}
	section(c, state.name+" · 整合包文件", func() {
		s.packRepairStatus(c, state, "pack-repair-task")
		ui.Button(c.Key("pack-repair-task-view"), "查看").Height(25).OnClick(func() { s.viewPackRepair(state) })
	})
}
