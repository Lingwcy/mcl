package ui

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"

	"mcl/internal/app"
	"mcl/internal/core/market"
)

type assetUpdateState struct {
	instanceID, name, loader                        string
	file                                            app.AssetFile
	origin                                          market.InstallFile
	versions                                        []market.Version
	plan                                            *market.UpdatePlan
	result                                          app.AssetUpdateResult
	progress                                        app.AssetUpdateProgress
	busy, applying, recovering, canceling, complete bool
	planning                                        bool
	supplying                                       bool
	message                                         string
	cancel                                          func()
	list                                            ui.ListState
}

func (s *Shell) assetUpdateBlocked(id string) bool {
	state := s.assets.update
	return (state != nil && state.instanceID == id && (state.busy || state.result.RecoveryRequired)) ||
		(s.assets.snapshot.InstanceID == id && s.assets.snapshot.PendingUpdate != nil)
}

func (s *Shell) viewAssetUpdate(state *assetUpdateState) {
	s.selectInstance(state.instanceID)
	s.assets.showUpdate = true
}

func (s *Shell) assetUpdateButton(c *ui.Context, id string, file app.AssetFile) {
	if file.Origin == nil || file.Directory {
		return
	}
	disabled := s.assets.changing || s.diagnosingAssets(id) || s.identifyingAssets(id) || s.assetUpdateBlocked(id) || s.assets.actions.UpdateVersions == nil
	if len(file.Origins) < 2 && file.Origin.Source == market.CurseForge && !s.market.actions.CurseForgeAvailable {
		disabled = true
	}
	button := ui.ButtonBase(c.Key("asset-update")).Label("选择资产版本").Tooltip("选择版本并确认更新计划").Size(24, 24).Radius(5).Center().
		Disabled(disabled).
		Children(func() { ui.Icon(c, iconDownload).Size(14, 14).TextColor(colors(s.theme).Icon) })
	if len(file.Origins) > 1 {
		button.Menu(func(menu *ui.Menu) {
			for _, origin := range file.Origins {
				if menu.Item(string(origin.Source)).Disabled(origin.Source == market.CurseForge && !s.market.actions.CurseForgeAvailable).Chosen() {
					s.openAssetUpdate(file, origin)
				}
			}
		})
	} else {
		button.OnClick(func() { s.openAssetUpdate(file, *file.Origin) })
	}
}

func (s *Shell) openAssetUpdate(file app.AssetFile, origin market.InstallFile) {
	if s.assets.actions.UpdateVersions == nil {
		return
	}
	state := s.newAssetUpdate(file, origin)
	if state == nil {
		return
	}
	a := &s.assets
	state.busy = true
	state.cancel = a.actions.UpdateVersions(state.instanceID, file, origin.Source, func(versions []market.Version, err error) {
		if a.update != state {
			return
		}
		state.busy, state.cancel, state.canceling = false, nil, false
		if err != nil {
			state.message = err.Error()
			return
		}
		state.versions = versions
	})
}

func (s *Shell) newAssetUpdate(file app.AssetFile, origin market.InstallFile) *assetUpdateState {
	value, ok := s.selectedInstance()
	a := &s.assets
	if !ok || value.ID != a.snapshot.InstanceID || a.changing || s.diagnosingAssets(value.ID) || s.identifyingAssets(value.ID) || s.assetUpdateBlocked(value.ID) {
		return nil
	}
	if a.update != nil && a.update.busy {
		s.viewAssetUpdate(a.update)
		return nil
	}
	state := &assetUpdateState{instanceID: value.ID, name: value.Name, file: file, origin: origin}
	if file.Kind == app.AssetMod {
		if required, ok := value.RequiredLoader(); ok {
			state.loader = required.Type
		} else if origin.Metadata != nil && len(origin.Metadata.Loaders) == 1 {
			state.loader = origin.Metadata.Loaders[0]
		}
	}
	a.update, a.showUpdate = state, true
	a.showDiagnostic = false
	return state
}

func (s *Shell) planAssetUpdate(state *assetUpdateState, version market.Version) {
	if s.assets.update != state || state.busy || s.assets.actions.PlanUpdate == nil {
		return
	}
	state.busy, state.message, state.canceling = true, "", false
	state.planning = true
	request := market.UpdateRequest{InstanceID: state.instanceID, Kind: market.Kind(state.file.Kind), World: state.file.World, Loader: state.loader,
		Selections: []market.UpdateSelection{{Path: state.file.Path, Source: state.origin.Source, ProjectID: state.origin.ProjectID, VersionID: version.ID}}}
	state.cancel = s.assets.actions.PlanUpdate(request, func(plan market.UpdatePlan, err error) {
		if s.assets.update != state {
			return
		}
		state.busy, state.cancel, state.canceling = false, nil, false
		state.planning = false
		if err != nil {
			state.message = err.Error()
			return
		}
		state.plan = &plan
	})
}

func (s *Shell) supplyAssetUpdate(state *assetUpdateState) {
	if s.assets.update != state || state.busy || state.plan == nil || s.assets.actions.SupplyUpdate == nil {
		return
	}
	state.busy, state.supplying, state.canceling, state.message = true, true, false, ""
	state.cancel = s.assets.actions.SupplyUpdate(*state.plan, func(result app.LocalFileSupply, err error) {
		if s.assets.update != state {
			return
		}
		state.busy, state.supplying, state.canceling, state.cancel = false, false, false, nil
		if len(result.Matched) > 0 {
			plan := *state.plan
			plan.ReadyLocal = slices.Concat(plan.ReadyLocal, result.Matched)
			slices.Sort(plan.ReadyLocal)
			plan.ReadyLocal = slices.Compact(plan.ReadyLocal)
			state.plan = &plan
		}
		switch {
		case errors.Is(err, context.Canceled):
			state.message = "已取消"
		case err != nil:
			state.message = err.Error()
		case len(result.Warnings) > 0:
			state.message = fmt.Sprintf("匹配 %d 项，跳过 %d 项 · %s", len(result.Matched), result.Skipped, result.Warnings[0])
		case len(result.Matched) > 0:
			state.message = fmt.Sprintf("已核验 %d 项", len(result.Matched))
		}
	})
}

func (s *Shell) applyAssetUpdate(state *assetUpdateState) {
	if s.assets.update != state || state.busy || state.plan == nil || len(state.plan.Changes) == 0 ||
		state.plan.MissingLocal() > 0 || s.assets.actions.ApplyUpdate == nil {
		return
	}
	state.busy, state.applying, state.canceling, state.message = true, true, false, ""
	state.cancel = s.assets.actions.ApplyUpdate(*state.plan, func(progress app.AssetUpdateProgress) {
		if s.assets.update == state && state.busy {
			state.progress = progress
		}
	}, func(result app.AssetUpdateResult, err error) {
		if s.assets.update != state {
			return
		}
		var missing *app.MissingMarketFilesError
		if errors.As(err, &missing) {
			plan := *state.plan
			plan.ReadyLocal = missing.Ready
			state.plan = &plan
			state.busy, state.applying, state.canceling, state.cancel = false, false, false, nil
			state.message = err.Error()
			return
		}
		s.finishAssetUpdate(state, result, err)
	})
}

func (s *Shell) recoverAssetUpdate(id, transaction string) {
	a := &s.assets
	if a.actions.RecoverUpdate == nil || (a.update != nil && a.update.busy) {
		return
	}
	name := id
	for _, value := range s.library.snapshot.Instances {
		if value.ID == id {
			name = value.Name
			break
		}
	}
	state := &assetUpdateState{instanceID: id, name: name, busy: true, recovering: true, result: app.AssetUpdateResult{ID: transaction, RecoveryRequired: true}}
	a.update = state
	s.viewAssetUpdate(state)
	state.cancel = a.actions.RecoverUpdate(id, transaction, func(result app.AssetUpdateResult, err error) {
		if err != nil && result.ID == "" {
			result = state.result
		}
		s.finishAssetUpdate(state, result, err)
	})
}

func (s *Shell) finishAssetUpdate(state *assetUpdateState, result app.AssetUpdateResult, err error) {
	if s.assets.update != state {
		return
	}
	state.busy, state.cancel, state.result, state.complete = false, nil, result, true
	state.message = "更新已应用"
	if !result.Applied {
		state.message = "已恢复更新前的文件"
	}
	if err != nil {
		state.message = err.Error()
		if errors.Is(err, context.Canceled) && !result.Applied && !result.RecoveryRequired {
			state.message = "已取消，文件未变更或已恢复"
		}
	}
	state.canceling = false
	if state.instanceID == s.library.selectedID {
		s.refreshAssets()
	}
}

func (s *Shell) cancelAssetUpdate() {
	state := s.assets.update
	if state != nil && state.busy && !state.canceling && state.cancel != nil {
		state.canceling = true
		state.cancel()
	}
}

func assetUpdateStage(state *assetUpdateState) string {
	if state.supplying {
		return "核验本地更新文件…"
	}
	if state.recovering {
		return "恢复资产更新…"
	}
	if !state.applying {
		if state.planning {
			return "准备更新计划…"
		}
		return "读取兼容版本…"
	}
	switch state.progress.Stage {
	case "update-download":
		return "下载与校验"
	case "update-stage":
		return "准备新文件"
	case "update-local-check":
		return "检查模组声明"
	case "update-apply":
		return "应用更新"
	default:
		return "检查本地文件"
	}
}

func (s *Shell) assetUpdateNotice(c *ui.Context, id string) {
	state := s.assets.update
	pending := s.assets.snapshot.PendingUpdate
	if pending == nil && (state == nil || state.instanceID != id) {
		return
	}
	ui.Row(c.Key("asset-update-notice")).FillWidth().Height(26).Gap(8).Children(func() {
		message := "更新尚未完成"
		if state != nil && state.instanceID == id {
			message = "已选择更新版本"
			if state.busy {
				message = assetUpdateStage(state)
			} else if state.message != "" {
				message = state.message
			}
		}
		ui.Text(c, message).FontSize(11).TextColor(s.theme.TextMuted).Grow(1).MinWidth(0).MaxLines(1).Ellipsis("…")
		if pending != nil && (state == nil || state.instanceID != id || !state.busy) {
			ui.Button(c.Key("recover-assets"), "恢复").Height(24).Disabled(s.assets.actions.RecoverUpdate == nil || (state != nil && state.busy)).
				OnClick(func() { s.recoverAssetUpdate(id, pending.ID) })
		} else if state != nil && state.instanceID == id {
			ui.Button(c.Key("view-update"), "查看").Height(24).OnClick(func() { s.viewAssetUpdate(state) })
		}
	})
}

func (s *Shell) assetUpdateView(c *ui.Context) {
	state, t := s.assets.update, s.theme
	ui.Column(c.Key("asset-update-"+state.instanceID)).Grow(1).FillWidth().Padding(16, 24, 20).Gap(12).Children(func() {
		ui.Row(c).FillWidth().Height(28).Gap(8).Children(func() {
			toolbarAction(c, "update-back", "返回实例", iconBack, func() { s.assets.showUpdate = false })
			ui.Text(c, "版本更新").FontSize(14).FontWeight(600).Grow(1)
			if state.busy {
				label := "取消"
				if state.canceling {
					label = "正在取消…"
				}
				ui.Button(c.Key("cancel-update"), label).Height(25).Disabled(state.canceling).OnClick(s.cancelAssetUpdate)
			}
		})
		ui.Text(c, state.name).FontSize(11).TextColor(t.TextMuted)
		if !state.recovering {
			ui.Text(c, assetTitle(state.file)).FontSize(16).FontWeight(600).MaxLines(1).Ellipsis("…")
			ui.Text(c, string(state.origin.Source)+" · 当前 "+state.origin.VersionID).FontSize(11).TextColor(t.TextMuted)
		}
		if state.message != "" {
			ui.Text(c, state.message).FontSize(12).TextColor(t.TextMuted).MaxLines(4)
		}
		if state.busy {
			s.workStatus(c, s.taskPaused("update", state.instanceID), assetUpdateStage(state))
			p := state.progress.Progress
			if p.TotalBytes > 0 {
				smoothProgress(c, float64(p.Bytes)/float64(p.TotalBytes)).FillWidth().Height(4)
			}
			return
		}
		if state.complete {
			if state.result.RecoveryRequired {
				ui.Text(c, "恢复完成前，此实例不能启动或修改资产。").FontSize(11).TextColor(t.Danger)
				ui.Button(c.Key("retry-update-recovery"), "恢复").Height(28).Disabled(s.assets.actions.RecoverUpdate == nil).
					OnClick(func() { s.recoverAssetUpdate(state.instanceID, state.result.ID) })
			}
			ui.Spacer(c)
			return
		}
		if state.plan != nil {
			s.assetUpdatePlanView(c, state)
			return
		}
		if state.file.Kind == app.AssetMod {
			label := state.loader
			if label == "" {
				label = "选择加载器"
			}
			fixed := false
			if value, ok := s.selectedInstance(); ok {
				_, fixed = value.RequiredLoader()
			}
			ui.Button(c.Key("update-loader"), label).Height(25).Disabled(fixed).Menu(func(menu *ui.Menu) {
				for _, loader := range []string{"fabric", "quilt", "forge", "neoforge"} {
					if menu.Item(loader).Checked(state.loader == loader).Chosen() {
						state.loader = loader
					}
				}
			})
		}
		versions := make([]market.Version, 0, len(state.versions))
		for _, version := range state.versions {
			if state.file.Kind != app.AssetMod || state.loader == "" || slices.Contains(version.Loaders, state.loader) {
				versions = append(versions, version)
			}
		}
		if len(versions) == 0 {
			ui.Text(c, "没有兼容版本").FontSize(12).TextColor(t.TextMuted)
		}
		state.list.Key = func(index int) any { return versions[index].ID }
		state.list.Label = func(index int) string { return versions[index].Name }
		ui.List(c.Key("asset-update-versions"), &state.list, len(versions), func(index int) {
			version := versions[index]
			ui.Row(c.Key(version.ID)).FillWidth().Height(48).Gap(10).Children(func() {
				ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
					ui.Text(c, version.Name).FontSize(12).MaxLines(1).Ellipsis("…")
					ui.Text(c, version.Type+" · "+version.Published.Local().Format("2006-01-02")+" · "+strings.Join(version.Loaders, " / ")).FontSize(10).TextColor(colors(t).Metadata).MaxLines(1)
				})
				label := "选择"
				if version.ID == state.origin.VersionID {
					label = "当前版本"
				}
				ui.Button(c.Key("choose-update-version"), label).Height(25).Disabled(state.file.Kind == app.AssetMod && state.loader == "").OnClick(func() { s.planAssetUpdate(state, version) })
			})
		}).Grow(1).MinHeight(0).FillWidth()
	})
}

func (s *Shell) assetUpdatePlanView(c *ui.Context, state *assetUpdateState) {
	plan, t := state.plan, s.theme
	missing := plan.MissingLocal()
	services := c.Services()
	ui.Row(c).FillWidth().Gap(8).Children(func() {
		ui.Textf(c, "%d 项更改", len(plan.Changes)).FontSize(12).Grow(1)
		if len(plan.LocalFiles()) > 0 && s.assets.actions.SupplyUpdate != nil {
			ui.Button(c.Key("update-supply"), "从本地补齐").Height(25).OnClick(func() { s.supplyAssetUpdate(state) })
		}
		ui.Button(c.Key("update-reselect"), "重选").Height(25).OnClick(func() {
			if len(state.versions) == 0 {
				s.openAssetUpdate(state.file, state.origin)
			} else {
				state.plan, state.message = nil, ""
			}
		})
		ui.Button(c.Key("apply-asset-update"), "应用").Height(25).
			Disabled(missing > 0 || len(plan.Changes) == 0 || s.assets.actions.ApplyUpdate == nil).OnClick(func() { s.applyAssetUpdate(state) })
	})
	if missing > 0 {
		ui.Textf(c, "%d 项待补齐", missing).FontSize(11).TextColor(t.Warning)
	}
	ui.Scroll(c.Key("asset-update-changes")).Grow(1).MinHeight(0).FillWidth().Gap(10).Children(func() {
		for _, warning := range plan.Warnings {
			ui.Text(c, warning).FontSize(11).TextColor(t.Danger).MaxLines(3)
		}
		for index, change := range plan.Changes {
			ui.Column(c.Key(fmt.Sprintf("change-%d", index))).FillWidth().Gap(4).Padding(8, 0).Children(func() {
				name := change.After.File.Filename
				if change.After.Metadata != nil && change.After.Metadata.Title != "" {
					name = change.After.Metadata.Title
				}
				ui.Text(c, name).FontSize(12).MaxLines(1).Ellipsis("…")
				before := "新增依赖"
				if change.Before != nil {
					before = change.Before.File.VersionID
				}
				status := "启用"
				if !change.Enabled {
					status = "停用"
				}
				ui.Text(c, before+" → "+change.After.VersionID+" · "+status+" · "+formatBytes(change.After.File.Size)).FontSize(10).TextColor(colors(t).Metadata).MaxLines(1)
				if !change.ReusesContent() && change.After.File.URL == "" && !slices.Contains(plan.ReadyLocal, change.After.Path) {
					if page := plan.LocalPages[change.After.Path]; market.SafeURL(page) {
						ui.Button(c.Key("update-file-page"), "项目页").Height(22).OnClick(func() { services.OpenURL(page) })
					}
				}
			})
		}
	})
}

func (s *Shell) assetUpdateTaskCard(c *ui.Context) {
	state := s.assets.update
	if state == nil {
		return
	}
	section(c, state.name, func() {
		ui.Row(c).FillWidth().Gap(8).Children(func() {
			label := "版本更新待确认"
			if state.busy {
				label = assetUpdateStage(state)
			} else if state.message != "" {
				label = state.message
			}
			if s.taskPaused("update", state.instanceID) {
				label = "已暂停"
			}
			ui.Text(c, label).FontSize(12).Grow(1).MinWidth(0).MaxLines(3)
			ui.Button(c.Key("view-asset-update-task"), "查看").Height(25).OnClick(func() { s.viewAssetUpdate(state) })
			if state.busy {
				ui.Button(c.Key("cancel-asset-update-task"), "取消").Height(25).Disabled(state.canceling).OnClick(s.cancelAssetUpdate)
			}
		})
	})
}
