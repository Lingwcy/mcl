package ui

import (
	"strings"

	"mcl/internal/app"
	"mcl/internal/core/loader"
	"mcl/internal/core/market"
	"mcl/internal/core/meta"

	"github.com/egoist/mygo/ui"
)

func (s *Shell) newInstanceView(c *ui.Context) {
	d, l, t := &s.creation, &s.library, s.theme
	ui.Column(c.Key("creation-workspace")).Grow(1).MinHeight(0).FillWidth().Padding(14, 24, 16).Gap(12).Children(func() {
		ui.Row(c).FillWidth().Height(28).Shrink(0).Gap(8).Children(func() {
			ui.Text(c, "新建实例").FontSize(15).FontWeight(600).Grow(1)
			ui.Row(c.Key("creation-modes")).Height(27).Padding(2).Gap(2).Radius(7).Background(t.Surface).Children(func() {
				for index, label := range []string{"新建", "导入", "下载"} {
					fill, color := ui.Transparent, t.TextMuted
					button := ui.ButtonBase(c.Key(index)).Label(label+"实例").Height(23).Padding(0, 10).Radius(5).Disabled(l.busy || d.importBusy)
					if d.mode == index {
						fill, color = colors(t).Selected, colors(t).AccentInk
					} else if button.Hovered() {
						fill = t.SurfaceHover
					}
					rowMotion(c, button, fill).OnClick(func() {
						s.gameDirectories.pickerOpen = false
						d.mode = index
						d.err = ""
						d.versionOpen, d.loaderOpen, d.iconOpen, d.kindOpen = false, false, false, false
					}).Children(func() { ui.Text(c, label).FontSize(11).TextColor(color) })
				}
			})
		})
		s.libraryStatus(c)
		if d.err != "" {
			ui.Text(c.Key("creation-error"), d.err).FontSize(11).TextColor(t.Danger).MaxLines(3)
		}
		ui.Column(c.Key(d.mode)).Grow(1).MinHeight(0).FillWidth().Gap(10).Transition(motionReveal).Children(func() {
			switch d.mode {
			case 1:
				s.creationImportView(c)
			case 2:
				s.creationDownloadView(c)
			default:
				s.creationForm(c)
			}
		})
	})
}

func (s *Shell) creationForm(c *ui.Context) {
	d, l, t := &s.creation, &s.library, s.theme
	_, height := c.Size()
	topPadding := min(24, max(8, height-MinHeight+8))
	ui.Column(c.Key("creation-form")).Grow(1).MinHeight(0).FillWidth().Padding(topPadding, 0, 0).Children(func() {
		ui.Column(c.Key("creation-fields")).FillWidth().MaxWidth(480).AlignSelf(ui.Center).Shrink(0).Gap(20).Children(func() {
			ui.Row(c.Key("creation-identity")).FillWidth().Height(56).Shrink(0).Gap(14).Children(func() {
				button := ui.ButtonBase(c.Key("creation-icon")).Label("选择实例图标").Tooltip("更换图标").Size(48, 48).Shrink(0).Radius(11).Center().Disabled(l.busy || d.iconBusy)
				controlMotion(c, button, ui.Transparent, !l.busy && !d.iconBusy).OnClick(func() {
					s.gameDirectories.pickerOpen = false
					d.iconOpen = !d.iconOpen
					d.kindOpen, d.versionOpen, d.loaderOpen = false, false, false
				}).Children(func() { drawInstanceArtwork(c, s.creationArtworkStyle(), d.bitmap, 48) })
				creationPopover(c.Key("creation-icon-popover"), button, &d.iconOpen, 270, func() { s.creationIconPicker(c) })
				ui.Column(c).Grow(1).MinWidth(0).Gap(5).Children(func() {
					ui.Text(c, "名称").FontSize(10).TextColor(colors(t).Metadata)
					ui.TextInput(c.Key("new-instance-name"), &l.name).Label("新实例名称").Placeholder("实例名称").FillWidth().Height(28).FontSize(12).Disabled(l.busy)
				})
			})
			ui.Column(c.Key("creation-settings")).FillWidth().Shrink(0).Radius(10).Background(t.Surface).Padding(0, 10).Children(func() {
				creationSettingRow(c, "directory", "游戏目录", true, func() {
					s.gameDirectoryMenu(c, "creation-game-directory", s.creationGameDirectoryID(), l.busy, s.chooseCreationGameDirectory)
				})
				creationSettingRow(c, "runtime", "运行环境", true, func() {
					label := "Vanilla"
					if d.kind != "" {
						label = marketLoaderLabel(d.kind)
					}
					anchor := s.creationSelect(c, "creation-runtime", label, s.creationLoaderStyle(d.kind), func() {
						s.gameDirectories.pickerOpen = false
						d.kindOpen = !d.kindOpen
						d.versionOpen, d.loaderOpen, d.iconOpen = false, false, false
					}, l.busy)
					creationPopover(c.Key("creation-runtime-popover"), anchor, &d.kindOpen, 220, func() { s.creationRuntimePicker(c) })
				})
				creationSettingRow(c, "game", "游戏版本", d.kind != "", func() {
					label := l.selected
					if label == "" {
						label = "选择版本"
					}
					anchor := s.creationSelect(c, "creation-game-version", label, "", func() {
						s.gameDirectories.pickerOpen = false
						d.versionOpen = !d.versionOpen
						d.kindOpen, d.loaderOpen, d.iconOpen = false, false, false
					}, l.busy)
					creationPopover(c.Key("creation-game-popover"), anchor, &d.versionOpen, 350, func() { s.creationGamePicker(c) })
				})
				if d.kind != "" {
					creationSettingRow(c, "loader", "加载器版本", false, func() {
						if d.busy {
							busyIndicator(c, 12)
						}
						label := d.loaderVersion
						if label == "" {
							label = "暂无兼容版本"
						}
						if d.busy {
							label = "正在读取…"
						}
						anchor := s.creationSelect(c, "creation-loader-version", label, "", func() {
							s.gameDirectories.pickerOpen = false
							d.loaderOpen = !d.loaderOpen
							d.versionOpen, d.kindOpen, d.iconOpen = false, false, false
						}, l.busy || d.busy || len(d.releases) == 0)
						creationPopover(c.Key("creation-loader-popover"), anchor, &d.loaderOpen, 300, func() { s.creationLoaderPicker(c) })
						iconButton(c, "reload-creation-loader", "刷新加载器版本", iconRefresh, func() { delete(d.cache, d.kind+":"+l.selected); s.loadCreationReleases() })
					})
				}
			})
		})
		ui.Spacer(c)
	})
	ui.Row(c.Key("creation-footer")).FillWidth().MaxWidth(480).AlignSelf(ui.Center).Height(30).Shrink(0).Gap(10).Children(func() {
		ui.Checkbox(c.Key("creation-install-now"), &d.installNow, "创建后安装").FontSize(11).Disabled(l.busy || s.installation.active)
		ui.Spacer(c)
		if l.cancelCreate != nil {
			ui.Button(c.Key("cancel-instance-creation"), "取消").Height(27).Disabled(l.cancelingCreate).OnClick(func() {
				if l.cancelCreate != nil {
					l.cancelingCreate = true
					l.cancelCreate()
				}
			})
		}
		label := "创建实例"
		if d.installNow {
			label = "创建并安装"
		}
		ui.PrimaryButton(c.Key("create-instance"), label).Height(28).Width(94).FontSize(12).Disabled(!s.creationReady() || (d.installNow && s.installation.active)).OnClick(s.createInstance)
	})
}

// Build-scoped anchors are used immediately and never retained by a job.
func creationPopover(c *ui.Context, anchor ui.Element, open *bool, width float32, body func()) {
	ui.PopoverBase(c, anchor, open, func(panel ui.Element) {
		panel.Width(width).Margin(4, 0, 0, 0).Padding(4).Radius(10).Background(colors(c.Theme()).Raised).Border(1, c.Theme().Border).Shadow(0, 6, 20, 0, colors(c.Theme()).Shadow)
		body()
	})
}

func creationSettingRow(c *ui.Context, key, label string, divider bool, body func()) {
	row := ui.Row(c.Key("setting-" + key)).FillWidth().Height(40).Shrink(0).Gap(8)
	if divider {
		row.BorderWidth(0, 0, 1, 0).BorderColor(colors(c.Theme()).Divider)
	}
	row.Children(func() { ui.Text(c, label).Width(82).FontSize(12).Shrink(0); body() })
}

func (s *Shell) creationSelect(c *ui.Context, key, label, style string, action func(), disabled bool) ui.Element {
	accessible := "选择游戏版本"
	if key == "creation-loader-version" {
		accessible = "选择加载器版本"
	} else if key == "creation-runtime" {
		accessible = "选择运行环境"
	}
	button := ui.ButtonBase(c.Key(key)).Label(accessible).Grow(1).MinWidth(0).Height(28).Padding(0, 5).Gap(7).Radius(5).Justify(ui.End).Disabled(disabled)
	fill := ui.Transparent
	if !disabled && button.Hovered() {
		fill = s.theme.SurfaceHover
	}
	return rowMotion(c, button, fill).OnClick(action).Children(func() {
		ui.Spacer(c)
		if style != "" {
			drawInstanceArtwork(c, style, nil, 18)
		}
		ui.Text(c, label).FontSize(12).TextColor(s.theme.TextMuted).MinWidth(0).MaxLines(1).Ellipsis("…")
		ui.Icon(c, iconChevronDown).Size(10, 10).TextColor(colors(s.theme).Icon)
	})
}

func (s *Shell) creationRuntimePicker(c *ui.Context) {
	ui.Column(c).Width(200).Gap(2).Children(func() {
		for _, kind := range []string{"", "fabric", "forge", "neoforge", "quilt"} {
			label := "Vanilla"
			if kind != "" {
				label = marketLoaderLabel(kind)
			}
			button := ui.ButtonBase(c.Key(kind)).Label(label).FillWidth().Height(32).Padding(0, 8).Gap(9).Radius(6).Disabled(s.library.busy)
			fill := ui.Transparent
			if s.creation.kind == kind {
				fill = colors(s.theme).Selected
				if button.Hovered() || button.Pressed() {
					fill = colors(s.theme).SelectedHover
				}
			} else if button.Hovered() {
				fill = s.theme.Surface
			}
			rowMotion(c, button, fill).OnClick(func() { s.selectCreationLoader(kind); s.creation.kindOpen = false }).Children(func() {
				drawInstanceArtwork(c, s.creationLoaderStyle(kind), nil, 22)
				ui.Text(c, label).FontSize(12).Grow(1)
				if s.creation.kind == kind {
					ui.Icon(c, iconCheck).Size(13, 13).TextColor(colors(s.theme).AccentInk)
				}
			})
		}
	})
}

func (s *Shell) creationGamePicker(c *ui.Context) {
	l, t := &s.library, s.theme
	_, height := c.Size()
	ui.Column(c.Key("creation-game-picker")).Width(330).Padding(6).Gap(10).Children(func() {
		ui.Row(c).FillWidth().Gap(4).Children(func() {
			ui.TextInput(c.Key("version-search"), &l.versionSearch).Label("搜索版本").Placeholder("搜索版本").Grow(1).MinWidth(0).Height(26).AutoFocus().OnSubmit(func() { s.pickVersion(strings.TrimSpace(l.versionSearch)) })
			iconButton(c, "refresh-library", "刷新版本", iconRefresh, func() { s.refreshLibrary(true) })
		})
		ui.Scroll(c.Key("creation-version-groups")).FillWidth().Height(min(240, height*.36)).Gap(12).Children(func() {
			query := strings.ToLower(strings.TrimSpace(l.versionSearch))
			var groups []string
			rows := make(map[string][]meta.ManifestEntry)
			for _, entry := range l.snapshot.Releases {
				if query != "" && !strings.Contains(strings.ToLower(entry.ID), query) {
					continue
				}
				parts := strings.Split(entry.ID, ".")
				group := entry.ID
				if len(parts) >= 2 {
					group = strings.Join(parts[:2], ".")
				}
				if _, ok := rows[group]; !ok {
					groups = append(groups, group)
				}
				rows[group] = append(rows[group], entry)
			}
			if len(groups) == 0 {
				ui.Text(c, "没有匹配的版本").FontSize(11).TextColor(t.TextMuted)
			}
			for _, group := range groups {
				ui.Column(c.Key(group)).FillWidth().Gap(6).Children(func() {
					ui.Text(c, group).FontSize(10).FontWeight(600).TextColor(t.TextMuted)
					ui.Row(c).FillWidth().Wrap().Gap(4).Children(func() {
						for _, entry := range rows[group] {
							fill, color := t.Surface, t.Text
							if entry.ID == l.selected {
								fill, color = colors(t).Selected, colors(t).AccentInk
							}
							button := ui.ButtonBase(c.Key(entry.ID)).Label("选择版本 "+entry.ID).Height(25).Padding(0, 8).Radius(5).Disabled(l.busy)
							rowMotion(c, button, fill).OnClick(func() { s.pickVersion(entry.ID) }).Children(func() { ui.Text(c, entry.ID).FontSize(11).TextColor(color) })
						}
					})
				})
			}
		})
	})
}

func (s *Shell) creationLoaderPicker(c *ui.Context) {
	d, t := &s.creation, s.theme
	_, height := c.Size()
	ui.Column(c.Key("creation-loader-picker")).Width(280).Padding(6).Gap(8).Children(func() {
		ui.TextInput(c.Key("loader-version-search"), &d.loaderQuery).Label("搜索加载器版本").Placeholder("搜索版本").FillWidth().Height(26).AutoFocus()
		var rows []loader.Release
		query := strings.ToLower(strings.TrimSpace(d.loaderQuery))
		for _, row := range d.releases {
			if strings.Contains(strings.ToLower(row.Version), query) {
				rows = append(rows, row)
			}
		}
		d.loaderList.Key = func(index int) any { return rows[index].Version }
		d.loaderList.Label = func(index int) string { return rows[index].Version }
		ui.List(c.Key("creation-loader-releases"), &d.loaderList, len(rows), func(index int) {
			row := rows[index]
			fill := ui.Transparent
			if row.Version == d.loaderVersion {
				fill = colors(t).Selected
			}
			button := ui.ButtonBase(c.Key(row.Version)).Label("加载器 "+row.Version).FillWidth().Height(29).Padding(0, 6).Radius(5).Gap(8)
			rowMotion(c, button, fill).OnClick(func() { d.loaderVersion = row.Version; d.loaderOpen = false }).Children(func() {
				ui.Text(c, row.Version).FontSize(12).Grow(1)
				if row.Recommended {
					ui.Text(c, "推荐").FontSize(10).TextColor(colors(t).AccentInk)
				} else if !row.Stable {
					ui.Text(c, "预发布").FontSize(10).TextColor(colors(t).Metadata)
				}
			})
		}).FillWidth().Height(min(215, height*.36))
	})
}

func (s *Shell) creationIconPicker(c *ui.Context) {
	d := &s.creation
	ui.Column(c).Width(250).Padding(7).Gap(10).Children(func() {
		ui.Row(c).FillWidth().Gap(8).Children(func() {
			ui.Text(c, "实例图标").FontSize(11).TextColor(s.theme.TextMuted).Grow(1)
			ui.Button(c, "自动").Height(23).FontSize(11).OnClick(func() { d.iconStyle = ""; d.icon, d.bitmap = nil, nil; d.iconOpen = false })
		})
		ui.Row(c).FillWidth().Wrap().Gap(6).Children(func() {
			for _, style := range []string{"grass", "fabric", "forge", "neoforge", "quilt", "cat", "leaf", "spark", "craft"} {
				button := ui.ButtonBase(c.Key(style)).Label("图标 "+style).Tooltip(instancePresetLabel(style)).Size(38, 38).Radius(9).Center().Border(1, ui.Transparent).Disabled(s.library.busy)
				if s.creationArtworkStyle() == style && len(d.icon) == 0 {
					button.BorderColor(s.theme.Accent)
				}
				controlMotion(c, button, ui.Transparent, !s.library.busy).OnClick(func() { d.iconStyle = style; d.icon, d.bitmap = nil, nil; d.iconOpen = false }).Children(func() { drawInstanceArtwork(c, style, nil, 30) })
			}
		})
		ui.Button(c.Key("creation-custom-icon"), "选择图片…").Height(26).FontSize(11).FillWidth().Disabled(d.iconBusy).OnClick(s.pickCreationIcon)
	})
}

func (s *Shell) creationImportView(c *ui.Context) {
	d := &s.creation
	if i := s.market.install; i != nil && i.localArchive != nil {
		s.archiveImportView(c)
		return
	}
	if d.importBusy {
		loadingStatus(c, "正在读取整合包…")
		ui.Button(c.Key("cancel-local-pack-read"), "取消").Height(28).OnClick(func() {
			if d.cancelImport != nil {
				d.cancelImport()
			}
		})
		return
	}
	ui.Row(c.Key("local-pack-location")).FillWidth().MaxWidth(480).AlignSelf(ui.Center).Height(40).Shrink(0).Gap(8).Children(func() {
		ui.Text(c, "游戏目录").FontSize(12).Width(84)
		s.gameDirectoryMenu(c, "local-pack-directory", s.creationGameDirectoryID(), false, s.chooseCreationGameDirectory)
	})
	ui.Column(c.Key("pack-import-methods")).FillWidth().MaxWidth(480).AlignSelf(ui.Center).Radius(10).Border(.5, colors(s.theme).ListDivider).Clip().Children(func() {
		for index, method := range []struct {
			key, label string
			icon       *ui.SVG
			action     func()
		}{{"local", "从本地文件导入", iconFolder, s.pickCreationArchive}, {"download", "下载整合包", iconDownload, func() { s.openCreationPackDownloads(market.Modrinth) }}} {
			if index > 0 {
				gameSettingsDivider(c)
			}
			button := ui.ButtonBase(c.Key(method.key)).Label(method.label).FillWidth().Height(48).Padding(0, 14).Gap(10)
			fill := ui.Transparent
			if button.Hovered() {
				fill = s.theme.SurfaceHover
			}
			rowMotion(c, button, fill).OnClick(method.action).Children(func() {
				ui.Icon(c, method.icon).Size(16, 16).TextColor(colors(s.theme).Icon)
				ui.Text(c, method.label).FontSize(12).Grow(1)
				ui.Icon(c, iconChevron).Size(12, 12).TextColor(colors(s.theme).Metadata)
			})
		}
	})
	ui.Text(c, "CurseForge · Modrinth · MultiMC / Prism").FontSize(11).TextColor(colors(s.theme).Metadata).MaxWidth(480).AlignSelf(ui.Center)
}
func (s *Shell) openCreationPackDownloads(source market.Source) {
	s.market.source = source
	s.market.loaded = false
	s.market.resetFilters()
	s.market.sort = "downloads"
	s.market.targetID, s.market.gameVersion = "", ""
	directory := s.creationGameDirectoryID()
	s.openMarket(app.AssetModpack)
	s.market.importDirectoryID = &directory
}

func (s *Shell) creationDownloadView(c *ui.Context) {
	ui.Column(c).Grow(1).FillWidth().Center().Gap(14).Children(func() {
		ui.Icon(c, iconDownload).Size(28, 28).TextColor(colors(s.theme).Icon)
		ui.Text(c, "发现整合包").FontSize(16).FontWeight(600)
		for _, source := range []struct {
			label string
			icon  *ui.SVG
			cf    bool
		}{{"Modrinth", iconModrinth, false}, {"CurseForge", iconCurseForge, true}} {
			button := ui.ButtonBase(c.Key(source.label)).Label("从 "+source.label+" 下载整合包").Width(240).Height(42).Padding(0, 12).Gap(10).Radius(9).Border(1, s.theme.Border)
			rowMotion(c, button, s.theme.Surface).OnClick(func() {
				selected := market.Modrinth
				if source.cf {
					selected = market.CurseForge
				}
				s.openCreationPackDownloads(selected)
			}).Children(func() {
				ui.Icon(c, source.icon).Size(18, 18).TextColor(colors(s.theme).Icon)
				ui.Text(c, source.label).FontSize(12).Grow(1)
				ui.Icon(c, iconChevron).Size(12, 12).TextColor(colors(s.theme).Icon)
			})
		}
	})
}
