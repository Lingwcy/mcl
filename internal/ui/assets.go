package ui

import (
	"context"
	"errors"
	"image"
	"slices"
	"strings"

	"mcl/internal/app"
	"mcl/internal/core/download"
	"mcl/internal/core/instance"
	"mcl/internal/core/market"

	"github.com/egoist/mygo/ui"
)

// AssetActions keeps directory I/O off the UI thread. List returns cancellation;
// callbacks, including failures, are delivered on the UI thread.
type AssetActions struct {
	Icon           func(string, app.AssetFile, func(image.Image, error)) func()
	List           func(string, func(app.AssetSnapshot, error)) func()
	OpenDirectory  func(string, func(error))
	Change         func(app.AssetMutation, func(app.AssetChange, error)) func()
	Restore        func(string, string, func(error)) func()
	Identify       func(string, market.Source, func(app.AssetIdentityProgress), func(app.AssetIdentityResult, error)) func()
	Associate      func(app.AssetAssociation, func(market.InstallFile, error)) func()
	UpdateVersions func(string, app.AssetFile, market.Source, func([]market.Version, error)) func()
	PlanUpdate     func(market.UpdateRequest, func(market.UpdatePlan, error)) func()
	SupplyUpdate   func(market.UpdatePlan, func(app.LocalFileSupply, error)) func()
	ApplyUpdate    func(market.UpdatePlan, func(app.AssetUpdateProgress), func(app.AssetUpdateResult, error)) func()
	RecoverUpdate  func(string, string, func(app.AssetUpdateResult, error)) func()
	Diagnose       func(string, func(download.Progress), func(app.AssetDiagnosticResult, error)) func()
}

type assetState struct {
	actions         AssetActions
	snapshot        app.AssetSnapshot
	instanceID      string
	generation      uint64
	cancel          func()
	busy, failed    bool
	message, search string
	kind            app.AssetKind
	sort            int
	list            ui.ListState
	changing        bool
	changeID        string
	cancelChange    func()
	undo            map[string]string
	identity        *assetIdentityState
	update          *assetUpdateState
	showUpdate      bool
	diagnostic      *assetDiagnosticState
	showDiagnostic  bool
	repair          *packRepairState
	showRepair      bool
	revision        uint64
	filtered        assetFilterCache
	artwork         *assetArtwork
}

type assetFilterCache struct {
	revision uint64
	query    string
	kind     app.AssetKind
	sort     int
	valid    bool
	rows     []app.AssetFile
}

func (s *Shell) ConnectAssets(actions AssetActions) {
	s.assets.actions = actions
	s.ensureAssets()
}

func (s *Shell) selectInstance(id string) {
	s.cancelInstanceSettingsIcon()
	s.assets.showUpdate = false
	s.assets.showDiagnostic = false
	s.assets.showRepair = false
	if id != s.library.selectedID {
		s.assets.search, s.assets.kind = "", ""
		s.assets.list = ui.ListState{}
	}
	s.library.selectedID, s.page = id, libraryPage
	s.ensureAssets()
}

func (s *Shell) ensureAssets() {
	if value, ok := s.selectedInstance(); ok && value.ID != s.assets.instanceID {
		s.refreshAssets()
	}
}

func (s *Shell) refreshAssets() {
	value, ok := s.selectedInstance()
	a := &s.assets
	if !ok || a.actions.List == nil {
		return
	}
	if state := a.diagnostic; state != nil && state.instanceID == value.ID {
		state.stale = true
	}
	if a.cancel != nil {
		a.cancel()
	}
	s.cancelAssetArtwork()
	a.generation++
	generation := a.generation
	a.instanceID, a.busy, a.failed, a.message = value.ID, true, false, ""
	a.snapshot = app.AssetSnapshot{InstanceID: value.ID}
	a.revision++
	a.cancel = a.actions.List(value.ID, func(snapshot app.AssetSnapshot, err error) {
		if generation != a.generation || value.ID != s.library.selectedID {
			return
		}
		a.busy, a.cancel = false, nil
		if err != nil {
			a.message, a.failed = err.Error(), true
			return
		}
		a.snapshot = snapshot
		a.revision++
		a.message = snapshot.Warning
		s.loadAssetArtwork(snapshot)
	})
}

func (s *Shell) changeAsset(id string, file app.AssetFile, action app.AssetAction) {
	a := &s.assets
	if id == "" || id != s.library.selectedID || id != a.snapshot.InstanceID || a.changing || s.diagnosingAssets(id) || s.assetUpdateBlocked(id) || s.identifyingAssets(id) || s.repairingPack(id) || a.actions.Change == nil {
		return
	}
	a.changing, a.changeID = true, id
	a.cancelChange = a.actions.Change(app.AssetMutation{InstanceID: id, File: file, Action: action}, func(change app.AssetChange, err error) {
		a.changing, a.changeID, a.cancelChange = false, "", nil
		if err == nil && change.BackupID != "" {
			if a.undo == nil {
				a.undo = make(map[string]string)
			}
			a.undo[id] = change.BackupID
		}
		if id != s.library.selectedID {
			return
		}
		if err != nil {
			s.assetChangeError(err)
			return
		}
		s.refreshAssets()
	})
}

func (s *Shell) restoreAsset() {
	a := &s.assets
	id := s.library.selectedID
	backup := a.undo[id]
	if a.changing || s.diagnosingAssets(id) || s.assetUpdateBlocked(id) || s.identifyingAssets(id) || s.repairingPack(id) || backup == "" || a.actions.Restore == nil {
		return
	}
	a.changing, a.changeID = true, id
	a.cancelChange = a.actions.Restore(id, backup, func(err error) {
		a.changing, a.changeID, a.cancelChange = false, "", nil
		if err == nil && a.undo[id] == backup {
			delete(a.undo, id)
		}
		if id != s.library.selectedID {
			return
		}
		if err != nil {
			s.assetChangeError(err)
			return
		}
		s.refreshAssets()
	})
}

func (s *Shell) assetChangeError(err error) {
	s.assets.message, s.assets.failed = err.Error(), true
	if errors.Is(err, context.Canceled) {
		s.assets.message, s.assets.failed = "已取消资产修改", false
	}
}

func (s *Shell) assetChangeStatus(c *ui.Context) {
	a := &s.assets
	if !a.changing {
		return
	}
	ui.Row(c.Key("asset-change-status")).FillWidth().Height(24).Gap(8).Children(func() {
		busyIndicator(c, 12)
		label := "正在检查并修改资产…"
		if a.cancelChange == nil {
			label = "正在取消资产修改…"
		}
		ui.Text(c, label).FontSize(11).TextColor(s.theme.TextMuted).Grow(1)
		ui.Button(c.Key("cancel-asset-change"), "取消").Height(22).Disabled(a.cancelChange == nil).OnClick(func() {
			if a.cancelChange != nil {
				a.cancelChange()
				a.cancelChange = nil
			}
		})
	})
}

func (s *Shell) openAssetProject(file app.AssetFile) {
	if file.Origin == nil {
		return
	}
	s.market.showInstall, s.market.showKey, s.market.showAssociation = false, false, false
	origin := *file.Origin
	s.market.source = origin.Source
	s.market.kind = file.Kind
	s.market.gameVersion = ""
	if value, ok := s.selectedInstance(); ok {
		s.market.targetID, s.market.gameVersion = value.ID, value.Version.ID
	}
	s.market.resetFilters()
	s.page, s.sidebarCollapsed = marketPage, false
	s.loadMarketCategories()
	s.searchMarket(true)
	s.openMarketProject(market.Project{Source: origin.Source, Kind: market.Kind(file.Kind), ID: origin.ProjectID, Title: assetTitle(file)})
}

func (s *Shell) openInstanceDirectory() {
	value, ok := s.selectedInstance()
	if !ok || s.assets.actions.OpenDirectory == nil {
		return
	}
	id := value.ID
	s.assets.actions.OpenDirectory(id, func(err error) {
		if id == s.library.selectedID && err != nil {
			s.assets.message, s.assets.failed = err.Error(), true
		}
	})
}

func (s *Shell) instanceView(c *ui.Context) {
	value, ok := s.selectedInstance()
	if !ok {
		s.welcomeView(c)
		return
	}
	if s.assets.showRepair && s.assets.repair != nil && s.assets.repair.id == value.ID {
		s.packRepairView(c)
		return
	}
	if s.assets.showUpdate && s.assets.update != nil && s.assets.update.instanceID == value.ID {
		s.assetUpdateView(c)
		return
	}
	if s.assets.showDiagnostic && s.assets.diagnostic != nil && s.assets.diagnostic.instanceID == value.ID {
		s.assetDiagnosticView(c)
		return
	}
	a, t := &s.assets, s.theme
	rows := s.filteredAssets()
	ui.Column(c.Key("instance-overview-"+value.ID)).Grow(1).MinHeight(0).FillWidth().Padding(16, 24, 20).Gap(14).Children(func() {
		s.instanceSummary(c, value)
		s.assetUpdateNotice(c, value.ID)
		s.assetDiagnosticNotice(c, value.ID)
		if a.changeID == value.ID {
			s.assetChangeStatus(c)
		}
		s.assetIdentityStatus(c, value.ID)
		ui.Row(c).FillWidth().Height(30).Shrink(0).Gap(3).Children(func() {
			assetTab(c, "assets-all", "全部", a.kind == "", func() { a.kind = "" })
			for _, item := range assetCategories {
				if item.kind == app.AssetModpack {
					continue
				}
				assetTab(c, "assets-tab-"+string(item.kind), item.name, a.kind == item.kind, func() { a.kind = item.kind })
			}
			ui.Spacer(c)
			ui.Row(c).Width(142).MinWidth(72).Shrink(1).Height(26).Padding(0, 7).Gap(5).Radius(6).Background(t.Surface).Children(func() {
				ui.Icon(c, iconSearch).Size(13, 13).TextColor(colors(t).Icon)
				ui.TextInputBase(c.Key("asset-search"), &a.search).Label("搜索实例资产").Placeholder("搜索").Grow(1).MinWidth(0).FillHeight().FontSize(11)
			})
			sortLabels := []string{"名称", "修改日期", "文件大小"}
			sortButton := ui.ButtonBase(c.Key("asset-sort")).Label("资产排序").Tooltip("排序："+sortLabels[a.sort]).Size(26, 26).Radius(5).Center()
			if sortButton.Hovered() {
				sortButton.Background(t.SurfaceHover)
			}
			sortButton.Children(func() { ui.Icon(c, iconSort).Size(15, 15).TextColor(colors(t).Icon) }).Menu(func(m *ui.Menu) {
				for index, label := range sortLabels {
					if m.Item(label).Checked(a.sort == index).Chosen() {
						a.sort = index
					}
				}
			})
			iconButton(c, "refresh-assets", "刷新资产", iconRefresh, s.refreshAssets)
		})
		ui.Column(c.Key("asset-content-" + string(a.kind))).Grow(1).MinHeight(0).FillWidth().Transition(motionReveal).Children(func() {
			if a.busy {
				loadingList(c, false)
			} else if len(rows) == 0 {
				s.assetEmptyState(c)
			} else {
				a.list.Key = func(index int) any { return rows[index].Path }
				a.list.Label = func(index int) string { return assetTitle(rows[index]) }
				ui.List(c.Key("instance-assets"), &a.list, len(rows), func(index int) { s.assetRow(c, rows[index]) }).Grow(1).MinHeight(0).FillWidth().Transition(motionReveal)
			}
		})
	})
}

// The instance overview does not mount these actions while their entry point is undecided.
func (s *Shell) instanceToolbar(c *ui.Context) {
	t := s.theme
	ui.Row(c).FillWidth().Height(28).Shrink(0).Gap(6).Children(func() {
		toolbarAction(c, "instance-market", "添加资产", iconPlus, func() { s.openMarket(s.assets.kind) })
		s.assetIdentityButton(c)
		s.assetDiagnosticButton(c)
		if value, ok := s.selectedInstance(); ok && value.Pack != nil && s.market.actions.InspectPackRepair != nil && s.market.actions.RestorePack != nil {
			var action func()
			if state := s.assets.repair; state == nil || state.id == value.ID || state.job == "" {
				action = func() { s.openPackRepair(value) }
			}
			toolbarAction(c, "instance-pack-repair", "检查整合包文件", iconArchive, action)
		}
		if s.assets.undo[s.library.selectedID] != "" {
			ui.ButtonBase(c.Key("undo-asset-removal")).Label("撤销移除").Tooltip("恢复上次移除的资产，已有同名文件会保留").Size(28, 28).Radius(6).Center().Disabled(s.assets.changing || s.diagnosingAssets(s.library.selectedID) || s.assetUpdateBlocked(s.library.selectedID)).OnClick(s.restoreAsset).Children(func() { ui.Icon(c, iconUndo).Size(16, 16).TextColor(colors(t).Icon) })
		}
		ui.Spacer(c)
		toolbarAction(c, "instance-directory", "打开实例目录", iconFolder, s.openInstanceDirectory)
		toolbarAction(c, "instance-logs", "查看日志", iconLog, s.showInstanceLogs)
	})
}

func (s *Shell) instanceSummary(c *ui.Context, value instance.Instance) {
	t := s.theme
	ui.Row(c).FillWidth().Height(62).Shrink(0).Gap(12).Children(func() {
		s.instanceAvatar(c, value, 40)
		ui.Column(c).Grow(1).MinWidth(0).Gap(4).Children(func() {
			ui.Row(c).FillWidth().Height(26).Gap(6).Children(func() {
				ui.Text(c, value.Name).FontSize(20).FontWeight(600).MinWidth(0).Shrink(1).MaxLines(1).Ellipsis("…")
				button := ui.ButtonBase(c.Key("instance-preferences")).Label("实例设置").Tooltip("实例设置").Size(24, 24).Shrink(0).Radius(5).Center()
				rowMotion(c, button, ui.Transparent).OnClick(s.openInstanceSettings).Children(func() {
					ui.Icon(c, iconSettings).Size(13, 13).TextColor(colors(t).Icon)
				})
			})
			subtitle := instanceType(value)
			if value.Name != value.Version.ID {
				subtitle = value.Version.ID + " · " + instanceType(value)
			}
			ui.Text(c, subtitle).FillWidth().MinWidth(0).FontSize(11).TextColor(t.TextMuted).MaxLines(1).Ellipsis("…")
		})
		s.instanceLaunchButton(c)
	})
}

func (s *Shell) instanceLaunchButton(c *ui.Context) {
	t := s.theme
	label, detail, action, disabled := s.launchButton()
	icon := iconLog
	switch label {
	case "检查启动条件", "启动游戏", "选择账户", "查看启动任务", "等待游戏窗口":
		icon = iconPlay
	case "安装游戏文件":
		icon = iconDownload
	case "查看安装任务":
		icon = iconDownload
		if s.installation.checking {
			icon = iconCheck
		}
	case "游戏正在运行":
		label += " · 查看日志"
	case "游戏仍在运行":
		label += " · 查看任务"
	case "正在恢复整合包文件":
		icon = iconArchive
	case "暂不支持此加载器":
		label += " · 查看实例设置"
		icon = iconSettings
	}
	fill, ink := t.Surface, t.Text
	if !disabled && (label == "检查启动条件" || label == "启动游戏" || label == "选择账户" || label == "安装游戏文件") {
		fill, ink = t.Accent, t.AccentText
	}
	button := ui.ButtonBase(c.Key("launch-game")).Label(label).Tooltip(label+"\n"+detail).
		Size(64, 44).Shrink(0).Radius(11).Center().Disabled(disabled)
	rowMotion(c, button, fill).OnClick(action).Children(func() {
		ui.Icon(c, icon).Size(22, 22).TextColor(ink).ContentTransition(ui.ContentTransition{Animation: &motionFade})
	})
}

func instanceType(value instance.Instance) string {
	if value.Pack != nil {
		return "整合包"
	}
	if required, ok := value.RequiredLoader(); ok {
		return marketLoaderLabel(required.Type) + " " + required.Version
	}
	return "原版"
}

func (s *Shell) filteredAssets() []app.AssetFile {
	a := &s.assets
	query := strings.ToLower(strings.TrimSpace(a.search))
	if cached := &a.filtered; cached.valid && cached.revision == a.revision && cached.query == query && cached.kind == a.kind && cached.sort == a.sort {
		return cached.rows
	}
	var rows []app.AssetFile
	for _, file := range a.snapshot.Files {
		if a.kind != "" && file.Kind != a.kind {
			continue
		}
		if query != "" {
			text := file.Name + " " + file.Path + " " + assetTitle(file)
			if file.Origin != nil && file.Origin.Metadata != nil {
				text += " " + file.Origin.Metadata.Description
			}
			if !strings.Contains(strings.ToLower(text), query) {
				continue
			}
		}
		rows = append(rows, file)
	}
	slices.SortFunc(rows, func(aFile, bFile app.AssetFile) int {
		if a.sort == 1 {
			if order := bFile.ModifiedAt.Compare(aFile.ModifiedAt); order != 0 {
				return order
			}
		}
		if a.sort == 2 && aFile.Size != bFile.Size {
			if aFile.Size > bFile.Size {
				return -1
			}
			return 1
		}
		if order := strings.Compare(strings.ToLower(assetTitle(aFile)), strings.ToLower(assetTitle(bFile))); order != 0 {
			return order
		}
		return strings.Compare(strings.ToLower(aFile.Path), strings.ToLower(bFile.Path))
	})
	a.filtered = assetFilterCache{revision: a.revision, query: query, kind: a.kind, sort: a.sort, valid: true, rows: rows}
	return rows
}

func (s *Shell) assetRow(c *ui.Context, file app.AssetFile) {
	t := s.theme
	instanceID := s.assets.snapshot.InstanceID
	ui.Row(c.Key(file.Path)).FillWidth().Height(48).Padding(8, 0).Gap(10).BorderWidth(0, 0, 1, 0).BorderColor(colors(t).Divider).Children(func() {
		s.assetArtworkIcon(c, file)
		ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
			ui.Text(c, assetTitle(file)).FontSize(12).MaxLines(1).Ellipsis("…")
			if assetTitle(file) != file.Name {
				ui.Text(c, file.Name).FontSize(10).TextColor(colors(t).Metadata).MaxLines(1).Ellipsis("…")
			}
		})
		size := formatBytes(file.Size)
		if file.Directory {
			size = "文件夹"
		}
		ui.Text(c, size).FontSize(11).TextColor(colors(t).Metadata)
		if file.Origin != nil {
			button := ui.ButtonBase(c.Key("asset-project")).Label("查看来源项目").Tooltip("查看来源项目 · "+string(file.Origin.Source)).Size(24, 24).Radius(5).Center().Children(func() { ui.Icon(c, iconInfo).Size(14, 14).TextColor(colors(t).Icon) })
			if len(file.Origins) > 1 {
				button.Menu(func(menu *ui.Menu) {
					for _, origin := range file.Origins {
						if menu.Item(string(origin.Source)).Chosen() {
							copy := file
							copy.Origin = &origin
							s.openAssetProject(copy)
						}
					}
				})
			} else {
				button.OnClick(func() { s.openAssetProject(file) })
			}
		}
		if !file.Directory {
			if file.Origin == nil && s.assets.actions.Associate != nil {
				toolbarAction(c, "asset-associate", "关联市场来源", iconScan, func() { s.openAssetAssociation(instanceID, file) })
			}
			s.assetUpdateButton(c, instanceID, file)
			disabled := s.assets.changing || s.diagnosingAssets(instanceID) || s.assetUpdateBlocked(instanceID) || s.identifyingAssets(instanceID) ||
				s.associatingAsset(instanceID) || s.installation.active || s.assets.actions.Change == nil
			enabled := file.Enabled
			ui.Switch(c.Key("asset-enabled"), &enabled).Label("启用 "+file.Name).Size(30, 18).Disabled(disabled).OnClick(func() {
				action := app.AssetDisable
				if enabled {
					action = app.AssetEnable
				}
				s.changeAsset(instanceID, file, action)
			})
			ui.ButtonBase(c.Key("asset-remove")).Label("移除 "+file.Name).Tooltip("移除，可撤销；文件保留在实例的 removed-assets 目录").Size(24, 24).Radius(5).Center().Disabled(disabled).OnClick(func() { s.changeAsset(instanceID, file, app.AssetRemove) }).Children(func() { ui.Icon(c, iconTrash).Size(14, 14).TextColor(colors(t).Icon) })
		}
	})
}

func assetTitle(file app.AssetFile) string {
	if file.Origin != nil && file.Origin.Metadata != nil && file.Origin.Metadata.Title != "" {
		return file.Origin.Metadata.Title
	}
	return file.Name
}

func (s *Shell) assetEmptyState(c *ui.Context) {
	a, t := &s.assets, s.theme
	title := "暂无资产"
	if strings.TrimSpace(a.search) != "" {
		title = "无结果"
	}
	if a.busy {
		title = "正在读取…"
	}
	if a.failed {
		title = "读取失败"
	}
	ui.Column(c.Key("asset-empty-state")).Grow(1).MinHeight(0).FillWidth().Center().Gap(8).Children(func() {
		ui.Icon(c, iconArchive).Size(25, 25).TextColor(colors(t).IconMuted)
		ui.Text(c, title).FontSize(12).TextColor(t.TextMuted)
	})
}

func (s *Shell) showInstanceLogs() {
	id := s.library.selectedID
	for _, game := range s.games.rows {
		if game.InstanceID == id {
			s.showLogs(game.ID)
			return
		}
	}
	if s.installation.instanceID == id {
		s.showLogs("")
		return
	}
	s.page = activityPage
}

func (s *Shell) welcomeView(c *ui.Context) {
	t := s.theme
	ui.Column(c.Key("welcome")).Grow(1).FillWidth().Center().Gap(12).Padding(24).Children(func() {
		ui.Icon(c, iconCube).Size(30, 30).TextColor(colors(t).IconMuted)
		ui.Button(c.Key("welcome-create"), "新建实例").Height(28).OnClick(s.openNewInstance)
	})
}

func toolbarAction(c *ui.Context, key, label string, icon *ui.SVG, action func()) {
	t := c.Theme()
	button := ui.ButtonBase(c.Key(key)).Label(label).Tooltip(label).Size(28, 28).Shrink(0).Radius(6).Center().Disabled(action == nil)
	fill := ui.Transparent
	if action != nil && button.Hovered() {
		fill = t.SurfaceHover
	}
	controlMotion(c, button, fill, action != nil).OnClick(action).Children(func() { ui.Icon(c, icon).Size(17, 17).TextColor(colors(t).Icon) })
}

func assetTab(c *ui.Context, key, label string, selected bool, action func()) {
	t := c.Theme()
	fill, color := ui.Transparent, t.TextMuted
	button := ui.ButtonBase(c.Key(key)).Label(label).Height(25).Padding(0, 8).Radius(5)
	if selected {
		fill, color = colors(t).Selected, colors(t).AccentInk
		if button.Hovered() || button.Pressed() {
			fill = colors(t).SelectedHover
		}
	} else if button.Hovered() {
		fill = t.Surface
	}
	rowMotion(c, button, fill).OnClick(action).Children(func() { ui.Text(c, label).FontSize(11).TextColor(color) })
}
