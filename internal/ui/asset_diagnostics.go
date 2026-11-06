package ui

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strings"

	"mcl/internal/app"
	"mcl/internal/core/download"
	"mcl/internal/core/market"

	"github.com/egoist/mygo/ui"
)

type assetDiagnosticState struct {
	instanceID, name, message        string
	active, canceling, failed, stale bool
	result                           app.AssetDiagnosticResult
	progress                         download.Progress
	cancel                           func()
	filter                           int
	list                             ui.ListState
}

func (s *Shell) diagnosingAssets(id string) bool {
	state := s.assets.diagnostic
	return state != nil && state.active && state.instanceID == id
}

func (s *Shell) viewAssetDiagnostic(state *assetDiagnosticState) {
	if state == nil {
		return
	}
	s.selectInstance(state.instanceID)
	s.assets.showDiagnostic = true
}

func (s *Shell) startAssetDiagnostic() {
	a := &s.assets
	value, ok := s.selectedInstance()
	if !ok || a.busy || a.changing || s.identifyingAssets(value.ID) || s.assetUpdateBlocked(value.ID) || a.actions.Diagnose == nil {
		return
	}
	if a.diagnostic != nil && a.diagnostic.active {
		s.viewAssetDiagnostic(a.diagnostic)
		return
	}
	state := &assetDiagnosticState{instanceID: value.ID, name: value.Name, active: true}
	generation := a.generation
	a.diagnostic = state
	s.viewAssetDiagnostic(state)
	state.cancel = a.actions.Diagnose(value.ID, func(p download.Progress) {
		if a.diagnostic == state && state.active {
			state.progress = p
		}
	}, func(result app.AssetDiagnosticResult, err error) {
		if a.diagnostic != state {
			return
		}
		state.active, state.cancel, state.canceling = false, nil, false
		state.stale = state.stale || generation != a.generation
		if err != nil {
			state.message, state.failed = err.Error(), true
			if errors.Is(err, context.Canceled) {
				state.message, state.failed = "已取消，未形成完整结果", false
			}
			return
		}
		state.result = result
	})
}

func (s *Shell) cancelAssetDiagnostic() {
	state := s.assets.diagnostic
	if state != nil && state.active && !state.canceling && state.cancel != nil {
		state.canceling = true
		state.cancel()
	}
}

func (s *Shell) assetDiagnosticButton(c *ui.Context) {
	a := &s.assets
	button := ui.ButtonBase(c.Key("diagnose-assets")).Label("检查资产关系").Tooltip("核验摘要、模组内部声明和平台记录；检查已配置的 Java，不运行模组、不联网、不修改文件").Size(28, 28).Radius(6).Center().
		Children(func() { ui.Icon(c, iconPuzzle).Size(17, 17).TextColor(colors(s.theme).Icon) })
	if state := a.diagnostic; state != nil && (state.active || state.instanceID == s.library.selectedID) {
		button.OnClick(func() { s.viewAssetDiagnostic(state) })
		return
	}
	button.Disabled(a.busy || a.changing || s.identifyingAssets(s.library.selectedID) || s.assetUpdateBlocked(s.library.selectedID) || a.actions.Diagnose == nil).OnClick(s.startAssetDiagnostic)
}

func (s *Shell) assetDiagnosticNotice(c *ui.Context, id string) {
	state := s.assets.diagnostic
	if state == nil || state.instanceID != id || (!state.active && !state.failed && (state.stale || state.result.Report.Errors == 0)) {
		return
	}
	ui.Row(c.Key("asset-diagnostic-notice")).FillWidth().Height(24).Gap(8).Children(func() {
		message := fmt.Sprintf("%d 项资产问题", state.result.Report.Errors)
		if state.active {
			message = "正在检查资产…"
		} else if state.failed {
			message = state.message
		}
		ui.Text(c, message).FontSize(11).TextColor(s.theme.TextMuted).Grow(1).MinWidth(0).MaxLines(1).Ellipsis("…")
		ui.Button(c.Key("view-diagnostic-notice"), "查看").Height(22).OnClick(func() { s.viewAssetDiagnostic(state) })
	})
}

func (s *Shell) assetDiagnosticView(c *ui.Context) {
	state, t := s.assets.diagnostic, s.theme
	r := state.result.Report
	ui.Column(c.Key("asset-diagnostic-"+state.instanceID)).Grow(1).MinHeight(0).FillWidth().Padding(16, 24, 20).Gap(12).Children(func() {
		ui.Row(c).FillWidth().Height(28).Gap(8).Children(func() {
			toolbarAction(c, "diagnostic-back", "返回实例", iconBack, func() { s.assets.showDiagnostic = false })
			ui.Text(c, "资产检查").FontSize(14).FontWeight(600).Grow(1)
			if state.active {
				label := "取消"
				if state.canceling {
					label = "正在取消…"
				}
				ui.Button(c.Key("cancel-diagnostic"), label).Height(25).Disabled(state.canceling).OnClick(s.cancelAssetDiagnostic)
			} else {
				iconButton(c, "refresh-diagnostic", "重新检查", iconRefresh, s.startAssetDiagnostic)
			}
		})
		ui.Text(c, state.name).FontSize(11).TextColor(t.TextMuted)
		if state.message != "" {
			color := t.TextMuted
			if state.failed {
				color = t.Danger
			}
			ui.Text(c, state.message).FontSize(12).TextColor(color).MaxLines(4)
		}
		if state.active {
			p := state.progress
			loadingStatus(c, fmt.Sprintf("核验文件 · %d / %d", p.Files, p.TotalFiles))
			if p.TotalBytes > 0 {
				smoothProgress(c, float64(p.Bytes)/float64(p.TotalBytes)).FillWidth().Height(4)
			}
			return
		}
		if state.result.ScannedAt.IsZero() {
			ui.Spacer(c)
			return
		}
		if state.stale {
			ui.Text(c, "文件列表已刷新，请重新检查").FontSize(11).TextColor(t.Warning)
		}
		if r.Truncated {
			ui.Text(c, "读取或报告达到上限，结果不完整").FontSize(11).TextColor(t.Warning)
		}
		if len(state.result.Warnings) > 0 {
			ui.Text(c, strings.Join(state.result.Warnings, "\n")).FontSize(11).TextColor(t.Warning).MaxLines(4)
		}
		ui.Row(c).FillWidth().Gap(6).Children(func() {
			assetTab(c, "diagnostic-all", "全部", state.filter == 0, func() { state.filter = 0 })
			assetTab(c, "diagnostic-errors", "问题", state.filter == 1, func() { state.filter = 1 })
			assetTab(c, "diagnostic-warnings", "提示", state.filter == 2, func() { state.filter = 2 })
			ui.Spacer(c)
			if r.Unknown > 0 || r.Unchecked > 0 || r.Unconfirmed > 0 {
				ui.Button(c.Key("diagnostic-identify"), "识别来源").Height(25).Disabled(s.assets.actions.Identify == nil || s.assetUpdateBlocked(state.instanceID)).Menu(func(menu *ui.Menu) {
					for _, source := range []market.Source{market.Modrinth, market.CurseForge} {
						if menu.Item(string(source)).Disabled(source == market.CurseForge && !s.market.actions.CurseForgeAvailable).Chosen() {
							s.assets.showDiagnostic = false
							s.startAssetIdentity(source)
						}
					}
				})
			}
		})
		rows := make([]market.AssetIssue, 0, len(r.Issues))
		for _, issue := range r.Issues {
			if state.filter == 0 || state.filter == 1 && issue.Error || state.filter == 2 && !issue.Error {
				rows = append(rows, issue)
			}
		}
		if len(rows) == 0 {
			label := "没有此类项目"
			if len(r.Issues) == 0 {
				label = "未发现已知问题"
			}
			ui.Column(c).Grow(1).FillWidth().Center().Gap(8).Children(func() {
				ui.Icon(c, iconCheck).Size(24, 24).TextColor(colors(t).Icon)
				ui.Text(c, label).FontSize(12).TextColor(t.TextMuted)
			})
			return
		}
		state.list.Key = func(index int) any { return rows[index].ID }
		state.list.Label = func(index int) string { return rows[index].Summary() }
		ui.List(c.Key("asset-diagnostic-rows"), &state.list, len(rows), func(index int) { s.assetDiagnosticRow(c, state, rows[index]) }).Grow(1).MinHeight(0).FillWidth()
	})
}

func (s *Shell) assetDiagnosticRow(c *ui.Context, state *assetDiagnosticState, issue market.AssetIssue) {
	t := s.theme
	ui.Row(c.Key(issue.ID)).FillWidth().Height(62).Gap(8).Children(func() {
		color := t.TextMuted
		if issue.Error {
			color = t.Danger
		}
		ui.Icon(c, iconInfo).Size(16, 16).TextColor(color)
		ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
			ui.Text(c, issue.Summary()).FontSize(12).MaxLines(1).Ellipsis("…")
			ui.Text(c, issue.Detail).FontSize(10).TextColor(t.TextMuted).MaxLines(2).Ellipsis("…")
		})
		blocked := state.stale || s.assets.changing || s.identifyingAssets(state.instanceID) || s.assetUpdateBlocked(state.instanceID)
		hasKey := issue.Origin == nil || issue.Origin.Source != market.CurseForge || s.market.actions.CurseForgeAvailable
		switch issue.Code {
		case market.IssueLocalDeclaration:
			if file, ok := s.diagnosticAsset(issue.PeerPath); issue.CanEnable && ok {
				ui.Button(c.Key("enable-local-dependency"), "启用").Height(24).Disabled(blocked || s.assets.actions.Change == nil).OnClick(func() { s.changeAsset(state.instanceID, file, app.AssetEnable) })
			} else if issue.LocalID != "" {
				ui.Button(c.Key("search-local-dependency"), "搜索").Height(24).Disabled(s.market.actions.Search == nil).OnClick(func() { s.searchLocalDependency(issue.LocalID) })
			}
		case market.IssueDisabledDependency:
			if file, ok := s.diagnosticAsset(issue.PeerPath); ok {
				ui.Button(c.Key("enable-diagnostic-dependency"), "启用").Height(24).Disabled(blocked || s.assets.actions.Change == nil).OnClick(func() { s.changeAsset(state.instanceID, file, app.AssetEnable) })
			}
		case market.IssueMissingDependency, market.IssueDependencyVersion:
			ui.Button(c.Key("repair-diagnostic-dependency"), "修复").Height(24).Disabled(blocked || !hasKey || s.assets.actions.PlanUpdate == nil).OnClick(func() { s.repairDiagnosticDependency(issue) })
		case market.IssueMissingFile:
			ui.Button(c.Key("diagnostic-file-version"), "查看版本").Height(24).Disabled(blocked || !hasKey || s.market.actions.VersionDetail == nil).OnClick(func() { s.openDiagnosticVersion(issue) })
		case market.IssueGameVersion, market.IssueLoader:
			if file, ok := s.diagnosticAsset(issue.Path); ok && issue.Origin != nil {
				ui.Button(c.Key("diagnostic-select-version"), "选择版本").Height(24).Disabled(blocked || !hasKey || s.assets.actions.UpdateVersions == nil).OnClick(func() { s.openAssetUpdate(file, *issue.Origin) })
			}
		}
		name := issue.Path
		if _, ok := s.diagnosticAsset(name); !ok {
			name = issue.PeerPath
		}
		if name != "" {
			ui.ButtonBase(c.Key("locate-diagnostic-asset")).Label("定位文件").Tooltip("在实例列表中定位 "+name).Size(24, 24).Radius(5).Center().OnClick(func() { s.locateDiagnosticAsset(name) }).Children(func() { ui.Icon(c, iconSearch).Size(14, 14).TextColor(colors(t).Icon) })
		}
	})
}

func (s *Shell) diagnosticAsset(name string) (app.AssetFile, bool) {
	for _, file := range s.assets.snapshot.Files {
		if file.Path == name {
			return file, true
		}
	}
	return app.AssetFile{}, false
}

func (s *Shell) searchLocalDependency(id string) {
	m := &s.market
	m.source, m.kind, m.text, m.targetID = market.Modrinth, app.AssetMod, id, s.library.selectedID
	if value, ok := s.selectedInstance(); ok {
		m.gameVersion = value.Version.ID
	}
	m.showKey, m.showInstall = false, false
	m.resetFilters()
	s.page, s.sidebarCollapsed = marketPage, false
	s.loadMarketCategories()
	s.searchMarket(true)
}

func (s *Shell) locateDiagnosticAsset(name string) {
	s.assets.showDiagnostic = false
	s.assets.search, s.assets.kind = path.Base(name), ""
}

func (s *Shell) repairDiagnosticDependency(issue market.AssetIssue) {
	file, ok := s.diagnosticAsset(issue.Path)
	if !ok || issue.Origin == nil {
		return
	}
	if file.Kind == app.AssetMod {
		value, selected := s.selectedInstance()
		_, configured := value.RequiredLoader()
		if !selected || !configured && (issue.Origin.Metadata == nil || len(issue.Origin.Metadata.Loaders) != 1) {
			s.openAssetUpdate(file, *issue.Origin)
			return
		}
	}
	state := s.newAssetUpdate(file, *issue.Origin)
	if state != nil {
		s.planAssetUpdate(state, market.Version{ID: issue.Origin.VersionID})
	}
}

func (s *Shell) openDiagnosticVersion(issue market.AssetIssue) {
	if issue.Origin == nil {
		return
	}
	file := app.AssetFile{Path: issue.Path, Name: path.Base(issue.Path), Origin: issue.Origin}
	switch path.Dir(issue.Path) {
	case "mods":
		file.Kind = app.AssetMod
	case "shaderpacks":
		file.Kind = app.AssetShader
	case "resourcepacks":
		file.Kind = app.AssetResourcePack
	default:
		file.Kind = app.AssetDataPack
	}
	s.openAssetProject(file)
	if project := s.market.selected; project != nil {
		s.openMarketVersion(*project, market.Version{Source: issue.Origin.Source, ID: issue.Origin.VersionID, ProjectID: issue.Origin.ProjectID})
	}
}

func (s *Shell) assetDiagnosticTaskCard(c *ui.Context) {
	state := s.assets.diagnostic
	if state == nil {
		return
	}
	section(c, state.name, func() {
		ui.Row(c).FillWidth().Gap(8).Children(func() {
			message := fmt.Sprintf("资产检查 · %d 项问题", state.result.Report.Errors)
			if state.active {
				message = "正在检查资产…"
			} else if state.message != "" {
				message = state.message
			} else if state.stale {
				message = "资产检查结果需刷新"
			}
			ui.Text(c, message).FontSize(12).Grow(1).MinWidth(0).MaxLines(2)
			ui.Button(c.Key("view-diagnostic-task"), "查看").Height(25).OnClick(func() { s.viewAssetDiagnostic(state) })
			if state.active {
				ui.Button(c.Key("cancel-diagnostic-task"), "取消").Height(25).Disabled(state.canceling).OnClick(s.cancelAssetDiagnostic)
			}
		})
	})
}
