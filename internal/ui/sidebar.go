package ui

import (
	"fmt"

	"mcl/internal/app"
	"mcl/internal/branding"
	"mcl/internal/core/market"

	"github.com/egoist/mygo/ui"
)

func (s *Shell) titlebar(c *ui.Context) {
	t := s.theme
	ui.Row(c.Key("titlebar")).FillWidth().Height(titlebarHeight).Shrink(0).Padding(0, 12).Gap(8).DragWindow().Children(func() {
		ui.Row(c.Key("product-brand")).Height(28).Children(func() {
			logo := ui.Image(c.Key("product-logo"), brandLogo).Label(branding.FullName).Size(26, 26).Fit(ui.Contain)
			if scale := logo.Keyframes("entrance", appLogoEntrance); scale != 1 {
				logo.Scale(scale, scale)
			}
		})
		if s.page != launchPage {
			iconButton(c, "toggle-sidebar", "收起或展开侧栏", iconSidebar, func() { s.sidebarCollapsed = !s.sidebarCollapsed })
			if s.page != libraryPage {
				iconButton(c, "nav-library", "游戏库", iconBack, s.openHome)
			}
		}
		ui.Spacer(c)
		iconButton(c, "nav-activity", "下载与任务", iconDownload, func() { s.page = activityPage })
		icon := iconMoon
		if t.Dark {
			icon = iconSun
		}
		iconButton(c, "toggle-theme", "切换明暗主题", icon, s.toggleAppearance)
		if w := s.window; w != nil {
			ui.Box(c).Width(1).Height(16).Shrink(0).Background(colors(t).Divider)
			ui.Row(c.Key("window-controls")).Shrink(0).Gap(4).Children(func() {
				iconButton(c, "window-minimize", "最小化窗口", iconMinus, w.Minimize)
				label, icon := "最大化窗口", iconSquare
				if w.IsMaximized() {
					label, icon = "还原窗口", iconRestoreWindow
				}
				iconButton(c, "window-maximize", label, icon, w.ToggleMaximize).MaximizeButton()
				iconButton(c, "window-close", "关闭窗口", iconClose, w.Close)
			})
		}
	})
}

func (s *Shell) openHome() {
	s.cancelInstanceSettingsIcon()
	s.page, s.sidebarCollapsed = libraryPage, false
	s.ensureAssets()
}

func (s *Shell) openMarket(kind app.AssetKind) {
	s.market.importDirectoryID = nil
	if kind == "" {
		kind = app.AssetMod
	}
	changed := s.market.kind != kind
	s.market.kind, s.page, s.sidebarCollapsed = kind, marketPage, false
	s.market.showKey, s.market.showInstall, s.market.showAssociation, s.market.keyInput = false, false, false, ""
	if changed {
		s.market.resetFilters()
		s.loadMarketCategories()
	} else {
		s.backMarketResults()
	}
	if changed || !s.market.loaded {
		if !changed {
			s.loadMarketCategories()
		}
		s.searchMarket(true)
	}
}

func (s *Shell) navigationRail(c *ui.Context) {
	settings := s.page.inSettings()
	ui.Column(c.Key("navigation-rail")).Label("主导航").Width(railWidth).Shrink(0).FillHeight().Padding(8, 8, 6).Gap(10).Children(func() {
		s.railButton(c, "rail-home", "Home", iconHome, s.page != marketPage && !settings, s.openHome)
		s.railButton(c, "rail-market", "市场", iconMarket, s.page == marketPage, func() { s.openMarket(s.market.kind) })
		ui.Spacer(c)
		s.accountRailButton(c)
		s.railButton(c, "nav-settings", "设置", iconSettings, settings && s.page != accountsPage, func() { s.page, s.sidebarCollapsed = gameSettingsPage, false })
	})
}

func (s *Shell) railButton(c *ui.Context, key, label string, icon *ui.SVG, selected bool, action func()) {
	t := s.theme
	fill, color := ui.Transparent, colors(t).Icon
	button := ui.ButtonBase(c.Key(key)).Label(label).Tooltip(label).Size(36, 36).Shrink(0).Radius(10).Center()
	if selected {
		fill, color = colors(t).Selected, t.Text
		if t.Dark {
			color = colors(t).AccentInk
		}
	}
	controlMotion(c, button, fill, true).OnClick(action).Children(func() { ui.Icon(c, icon).Size(19, 19).TextColor(color) })
}

func secondarySidebarWidth(c *ui.Context) float32 {
	width, _ := c.Size()
	// Give the space lost below the compact breakpoint to the main panel first.
	const compactBreakpoint = MinWidth + sidebarWidth - minSidebarWidth
	return max(minSidebarWidth, sidebarWidth-max(0, compactBreakpoint-width))
}

func (s *Shell) sidebar(c *ui.Context) ui.Element {
	t := s.theme
	group := "instances"
	if s.page == marketPage {
		group = "market"
	} else if s.page.inSettings() {
		group = "settings"
	}
	panel := ui.Column(c.Key("secondary-sidebar")).Label("副侧栏").Width(secondarySidebarWidth(c)).Shrink(0).FillHeight().
		Background(sidebarColor(t)).BorderWidth(0, 1, 0, 0).BorderColor(colors(t).Divider)
	return panel.Children(func() {
		ui.Column(c.Key(group)).Grow(1).MinHeight(0).FillWidth().Transition(motionReveal).Children(func() {
			switch group {
			case "market":
				s.marketSidebar(c)
			case "settings":
				s.settingsSidebar(c)
			default:
				s.instanceSidebar(c)
			}
		})
	})
}

func (s *Shell) instanceSidebar(c *ui.Context) {
	l, t := &s.library, s.theme
	ui.Row(c).FillWidth().Height(44).Shrink(0).Padding(0, 12, 0, 16).Gap(6).Children(func() {
		ui.Text(c, "实例").FontSize(12).FontWeight(600).Grow(1)
		iconButton(c, "open-instance-search", "搜索实例", iconSearch, s.openInstanceSearch)
		iconButton(c, "nav-new-instance", "新建实例", iconPlus, s.openNewInstance)
	})
	rows := l.snapshot.Instances
	l.sidebarList.Key = func(index int) any { return rows[index].ID }
	l.sidebarList.Label = func(index int) string { return rows[index].Name }
	ui.Column(c).Grow(1).MinHeight(0).FillWidth().Children(func() {
		if len(rows) == 0 {
			ui.Text(c, "暂无实例").FontSize(11).TextColor(t.TextMuted).Margin(8, 16)
			return
		}
		ui.List(c.Key("sidebar-instances"), &l.sidebarList, len(rows), func(index int) {
			value := rows[index]
			selected := value.ID == l.selectedID
			fill := ui.Transparent
			versionColor := colors(t).Metadata
			button := ui.ButtonBase(c.Key("select-"+value.ID)).Label(value.Name).Width(secondarySidebarWidth(c)-16).Height(38).Margin(0, 8, 2).Padding(0, 9).Gap(9).Radius(8).Justify(ui.Start)
			if selected {
				fill = colors(t).Selected
				if !t.Dark {
					versionColor = t.TextMuted
				}
			}
			rowMotion(c, button, fill).OnClick(func() { s.selectInstance(value.ID) }).Children(func() {
				s.instanceAvatar(c, value, 24)
				ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
					ui.Text(c, value.Name).FontSize(12).FontWeight(500).MaxLines(1).Ellipsis("…")
					if value.Name != value.Version.ID {
						ui.Text(c, value.Version.ID).FontSize(10).TextColor(versionColor)
					}
				})
			})
		}).Grow(1).MinHeight(0).FillWidth()
	})
}

func (s *Shell) marketSidebar(c *ui.Context) {
	const entryHeight = 28
	m := &s.market
	ui.Scroll(c.Key("market-sidebar-scroll")).Grow(1).MinHeight(0).FillWidth().Padding(8, 0, 10).Gap(1).Children(func() {
		s.secondaryEntry(c, "source-modrinth", "Modrinth", iconModrinth, entryHeight, m.source == market.Modrinth, func() { s.switchMarketSource(market.Modrinth) })
		s.secondaryEntry(c, "source-curseforge", "CurseForge", iconCurseForge, entryHeight, m.source == market.CurseForge, func() { s.switchMarketSource(market.CurseForge) })
		if m.source == market.CurseForge {
			ui.Row(c).FillWidth().Padding(0, 12).Justify(ui.End).Children(func() { iconButton(c, "cf-config", "配置 API Key", iconSettings, s.openMarketKey) })
		}
		ui.Divider(c).Margin(4, 14)
		for _, item := range assetCategories {
			s.secondaryEntry(c, "market-"+string(item.kind), item.name, item.icon, entryHeight, m.kind == item.kind, func() { s.openMarket(item.kind) })
		}
		if m.source == market.CurseForge {
			s.secondaryEntry(c, "market-world", "地图", iconMap, entryHeight, m.kind == app.AssetKind(market.World), func() { s.openMarket(app.AssetKind(market.World)) })
		}
		ui.Divider(c).Margin(4, 14)
		s.marketLoaderFilters(c)
		s.marketCategoryFilters(c)
		s.marketEnvironmentFilters(c)
	})
}

func (s *Shell) settingsSidebar(c *ui.Context) {
	ui.Scroll(c.Key("settings-links")).Label("设置导航").Grow(1).MinHeight(0).FillWidth().Padding(12, 10, 8).Gap(2).Children(func() {
		s.settingsEntry(c, "settings-game", "游戏设置", iconSettingsGame, gameSettingsPage)
		s.settingsEntry(c, "settings-download", "下载设置", iconSettingsDownload, downloadSettingsPage)
		s.settingsEntry(c, "settings-runtime", "Java 管理", iconSettingsRuntime, settingsPage)
		s.settingsEntry(c, "settings-accounts", "账户", iconSettingsAccount, accountsPage)
		s.settingsEntry(c, "settings-tasks", "下载与任务", iconSettingsDownload, settingsActivityPage)
		s.settingsEntry(c, "settings-about", "关于与更新", iconInfo, aboutPage)
	})
}

func (s *Shell) settingsEntry(c *ui.Context, key, label string, icon *ui.SVG, destination page) {
	t := s.theme
	p := colors(t)
	button := ui.ButtonBase(c.Key(key)).Label(label).FillWidth().Height(30).Shrink(0).Padding(0, 8).Gap(8).Radius(6).Justify(ui.Start)
	fill, iconColor, textColor := ui.Transparent, p.Metadata, p.Metadata
	if button.Hovered() {
		fill, iconColor, textColor = p.Control, p.Secondary, t.Text
	}
	if s.page == destination {
		iconColor, textColor = p.SettingsNavAccent, t.Text
	}
	button.Background(fill).OnClick(func() {
		if destination == settingsPage {
			s.closeJavaDownloads()
		}
		s.page = destination
	}).Children(func() {
		ui.Icon(c, icon).Size(14, 14).Shrink(0).TextColor(iconColor)
		ui.Text(c, label).FontSize(12).TextColor(textColor).MinWidth(0).MaxLines(1).Ellipsis("…")
	})
}

func (s *Shell) secondaryEntry(c *ui.Context, key, label string, icon *ui.SVG, height float32, selected bool, action func()) {
	t := s.theme
	p := colors(t)
	fill, color := ui.Transparent, t.Text
	button := ui.ButtonBase(c.Key(key)).Label(label).Width(secondarySidebarWidth(c)-16).Height(height).Margin(0, 8, 3).Padding(0, 10).Gap(10).Radius(8).Justify(ui.Start)
	if selected {
		fill = p.Selected
		if t.Dark {
			color = p.AccentInk
		}
	}
	rowMotion(c, button, fill).OnClick(action).Children(func() {
		ui.Icon(c, icon).Size(16, 16).TextColor(color)
		ui.Text(c, label).FontSize(12).TextColor(color)
	})
}

func formatBytes(size int64) string {
	if size < 1024 {
		return fmt.Sprintf("%d B", size)
	}
	if size < 1<<20 {
		return fmt.Sprintf("%.1f KB", float64(size)/1024)
	}
	if size < 1<<30 {
		return fmt.Sprintf("%.1f MB", float64(size)/(1<<20))
	}
	return fmt.Sprintf("%.1f GB", float64(size)/(1<<30))
}
