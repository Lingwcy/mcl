package ui

import (
	"fmt"
	"slices"
	"strings"

	"mcl/internal/app"
	"mcl/internal/core/market"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
)

const (
	marketProjectRowHeight = 52
	marketProjectIconSize  = 32
)

func (s *Shell) marketView(c *ui.Context) {
	m, t := &s.market, s.theme
	ui.Column(c.Key("market-content")).Grow(1).MinHeight(0).FillWidth().Padding(12, 20, 10).Gap(8).Children(func() {
		if m.showKey {
			s.marketKeyView(c)
			return
		}
		if m.install != nil && m.showInstall {
			s.marketInstallView(c)
			return
		}
		association := s.marketAssociation()
		if association != nil {
			s.assetAssociationBanner(c, association)
		}
		if m.selected != nil {
			s.marketDetailView(c)
			return
		}
		ui.Column(c.Key("market-filters")).FillWidth().Shrink(0).Padding(8, 10).Gap(6).Radius(10).Background(t.Surface).Children(func() {
			ui.Row(c).FillWidth().Gap(8).Children(func() {
				if association == nil {
					target := "不指定实例"
					for _, v := range s.library.snapshot.Instances {
						if v.ID == m.targetID {
							target = v.Name
						}
					}
					ui.Button(c.Key("market-target"), target).Label("选择目标实例").Width(160).Height(28).Menu(func(menu *ui.Menu) {
						if menu.Item("不指定实例").Checked(m.targetID == "").Chosen() {
							s.selectMarketTarget("")
						}
						for _, v := range s.library.snapshot.Instances {
							if menu.Item(v.Name + " · " + v.Version.ID).Checked(v.ID == m.targetID).Chosen() {
								s.selectMarketTarget(v.ID)
							}
						}
					})
				}
				ui.TextInput(c.Key("market-query"), &m.text).Label("搜索市场").Placeholder("搜索").Grow(1).MinWidth(0).Height(28).OnSubmit(func() { s.searchMarket(true) })
				iconButton(c, "market-search", "搜索", iconSearch, func() { s.searchMarket(true) })
			})
			ui.Row(c).FillWidth().Gap(8).Children(func() {
				game := m.gameVersion
				if game == "" {
					game = "全部版本"
				}
				ui.Button(c.Key("market-game-version"), game).Label("筛选游戏版本").Height(26).Width(126).Menu(func(menu *ui.Menu) {
					if menu.Item("全部版本").Checked(m.gameVersion == "").Chosen() {
						s.selectMarketGame("")
					}
					for _, v := range s.library.snapshot.Releases {
						if menu.Item(v.ID).Checked(m.gameVersion == v.ID).Chosen() {
							s.selectMarketGame(v.ID)
						}
					}
				})
				sortLabel := marketSortLabel(m.sort)
				ui.Button(c.Key("market-sort"), sortLabel).Label("市场排序").Height(26).Width(100).Menu(func(menu *ui.Menu) {
					for _, sort := range []string{"relevance", "downloads", "follows", "newest", "updated", "name"} {
						if m.source == market.Modrinth && sort == "name" {
							continue
						}
						if m.source == market.CurseForge && sort == "follows" {
							continue
						}
						if menu.Item(marketSortLabel(sort)).Checked(m.sort == sort).Chosen() {
							m.sort = sort
							s.searchMarket(true)
						}
					}
				})
				if m.filtersActive() {
					iconButton(c, "market-reset", "清除筛选", iconBack, func() { m.resetFilters(); s.searchMarket(true) })
				}
				ui.Spacer(c)
				if !m.busy && m.error == "" && (m.source != market.CurseForge || m.actions.CurseForgeAvailable) {
					ui.Textf(c, "%s 项", compactCount(int64(m.results.Total))).FontSize(11).TextColor(colors(t).Metadata)
				}
				if m.source == market.CurseForge && m.actions.SetKey != nil && m.actions.CurseForgeAvailable {
					iconButton(c, "market-cf-key", "配置 CurseForge API Key", iconSettings, s.openMarketKey)
				}
			})
		})
		if m.source == market.CurseForge && !m.actions.CurseForgeAvailable {
			ui.Column(c).Grow(1).FillWidth().Center().Gap(12).Children(func() {
				ui.Icon(c, iconInfo).Size(25, 25).TextColor(colors(t).Icon)
				ui.Text(c, "需要 CurseForge API Key").FontSize(12)
				ui.Button(c.Key("configure-cf"), "配置").Height(28).OnClick(s.openMarketKey)
			})
			return
		}
		if m.busy {
			loadingList(c, true)
			return
		}
		if m.error != "" {
			s.marketError(c)
			return
		}
		if len(m.results.Projects) == 0 {
			marketStatus(c, "无结果", iconSearch)
			return
		}
		projects := m.results.Projects
		m.list.Key = func(index int) any { return projects[index].ID }
		m.list.Label = func(index int) string { return projects[index].Title }
		ui.List(c.Key("market-results"), &m.list, len(projects), func(index int) {
			s.marketProjectRow(c, projects[index], index+1 < len(projects))
		}).Grow(1).MinHeight(0).FillWidth().Radius(10).Border(0.5, colors(t).ListDivider).Transition(motionReveal)
		s.marketPagination(c)
	})
}
func (s *Shell) marketProjectRow(c *ui.Context, p market.Project, separated bool) {
	t := s.theme
	button := ui.ButtonBase(c.Key("market-project-"+p.ID)).Label(p.Title).FillWidth().Height(marketProjectRowHeight).Padding(6, 10).Gap(8).Justify(ui.Start)
	if separated {
		const inset = 12
		line := colors(t).ListDivider
		button.DrawOver(func(p *ui.Painter, r ui.Rect) {
			pixel := 1 / p.Scale()
			p.Fill(ui.Rect{X: r.X + inset, Y: r.Y + r.H - pixel, W: max(r.W-2*inset, 0), H: pixel}, line, 0)
		})
	}
	fill := ui.Transparent
	if button.Hovered() {
		fill = t.Surface
	}
	rowMotion(c, button, fill).OnClick(func() { s.openMarketProject(p) }).Children(func() {
		s.marketProjectIcon(c, p, marketProjectIconSize)
		ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
			ui.Row(c).FillWidth().Gap(4).Children(func() {
				ui.Text(c, p.Title).FontSize(13).FontWeight(500).Grow(1).MinWidth(0).MaxLines(1).Ellipsis("…")
				for _, tag := range firstStrings(p.Loaders, 3) {
					s.marketLoaderTag(c, tag)
				}
			})
			ui.Text(c, p.Description).FontSize(11).TextColor(t.TextMuted).FillWidth().MaxLines(1).Ellipsis("…")
		})
		ui.Column(c).Width(62).Shrink(0).AlignItems(ui.End).Gap(3).Children(func() {
			ui.Row(c).Gap(4).Children(func() {
				ui.Icon(c, iconDownload).Size(11, 11).TextColor(colors(t).Icon)
				ui.Text(c, compactCount(p.Downloads)).FontSize(10).TextColor(colors(t).Metadata)
			})
			if len(p.Categories) > 0 {
				ui.Row(c).FillWidth().Gap(3).Children(func() {
					s.categorySymbol(p.Categories[0], p.Categories[0]).draw(c, 10, t.TextMuted)
					ui.Text(c, marketCategoryLabel(p.Categories[0])).FontSize(10).TextColor(colors(t).Metadata).MinWidth(0).Grow(1).MaxLines(1).Ellipsis("…")
				})
			}
		})
	})
}
func (s *Shell) marketProjectIcon(c *ui.Context, p market.Project, size float32) {
	t := s.theme
	ui.Box(c).Size(size, size).Shrink(0).Radius(8).Clip().Background(colors(t).Control).Center().Children(func() {
		if bitmap := s.market.icons[p.IconURL]; bitmap != nil {
			ui.Image(c.Key("project-image"), bitmap).Size(size, size).Transition(motionArtwork)
		} else {
			ui.Icon(c.Key("project-fallback"), category(app.AssetKind(p.Kind)).icon).Size(size*.55, size*.55).TextColor(colors(t).Icon)
		}
	})
}
func (s *Shell) marketPagination(c *ui.Context) {
	m, t := &s.market, s.theme
	pages := max(1, (m.results.Total+market.PageSize-1)/market.PageSize)
	if m.source == market.CurseForge {
		pages = min(500, pages)
	}
	ui.Row(c).FillWidth().Height(30).Shrink(0).Justify(ui.Center).Gap(5).Children(func() {
		ui.Button(c.Key("market-prev"), "‹").Label("上一页").Size(27, 27).Disabled(m.page == 0).OnClick(func() { m.page--; s.searchMarket(false) })
		indexes := []int{0, m.page - 1, m.page, m.page + 1, pages - 1}
		slices.Sort(indexes)
		indexes = slices.Compact(indexes)
		last := -1
		for _, index := range indexes {
			if index < 0 || index >= pages {
				continue
			}
			if last >= 0 && index-last > 1 {
				ui.Text(c, "…").FontSize(11).TextColor(t.TextMuted)
			}
			last = index
			fill := ui.Transparent
			if index == m.page {
				fill = colors(t).Selected
			}
			ui.Button(c.Key(fmt.Sprintf("market-page-%d", index)), fmt.Sprint(index+1)).Label(fmt.Sprintf("第 %d 页", index+1)).Size(27, 27).Background(fill).BorderWidth(0).OnClick(func() { m.page = index; s.searchMarket(false) })
		}
		ui.Button(c.Key("market-next"), "›").Label("下一页").Size(27, 27).Disabled(m.page+1 >= pages).OnClick(func() { m.page++; s.searchMarket(false) })
	})
}
func (s *Shell) marketError(c *ui.Context) {
	ui.Column(c).Grow(1).FillWidth().Center().Gap(12).Children(func() {
		ui.Text(c, s.market.error).FontSize(12).TextColor(s.theme.Danger).MaxLines(3)
		ui.Button(c.Key("market-retry"), "重试").Height(27).OnClick(func() {
			if s.market.selected != nil {
				s.openMarketProject(*s.market.selected)
			} else {
				s.searchMarket(false)
			}
		})
	})
}
func (s *Shell) marketKeyView(c *ui.Context) {
	m := &s.market
	ui.Row(c).FillWidth().Children(func() {
		iconButton(c, "cf-key-back", "返回", iconBack, func() { m.showKey = false; m.keyInput = "" })
		ui.Text(c, "CurseForge").FontSize(16).FontWeight(500)
	})
	ui.TextInput(c.Key("cf-key"), &m.keyInput).Label("CurseForge API Key").Placeholder("API Key").Password().Height(30).FillWidth()
	ui.Text(c, "仅在本次会话使用").FontSize(11).TextColor(s.theme.TextMuted)
	ui.Row(c).Gap(8).Children(func() {
		ui.Button(c.Key("cf-key-apply"), "应用").Height(28).Width(80).Disabled(strings.TrimSpace(m.keyInput) == "").OnClick(func() {
			s.applyMarketKey(strings.TrimSpace(m.keyInput))
		})
		if m.actions.CurseForgeAvailable {
			ui.Button(c.Key("cf-key-clear"), "清除会话 Key").Height(28).OnClick(func() { s.applyMarketKey("") })
		}
	})
	if m.keyError != "" {
		ui.Text(c, m.keyError).FontSize(11).TextColor(s.theme.Danger)
	}
}
func marketStatus(c *ui.Context, label string, icon *ui.SVG) {
	ui.Column(c.Key("market-status")).Grow(1).MinHeight(0).FillWidth().Center().Gap(10).Transition(motionReveal).Children(func() {
		color := colors(c.Theme()).IconMuted
		if icon == iconCheck {
			color = c.Theme().Success
		}
		mark := ui.Icon(c.Key("status-icon"), icon).Size(25, 25).TextColor(color).ContentTransition(ui.ContentTransition{Animation: &motionFade})
		if icon == iconCheck {
			scale := mark.Keyframes("success", successPop)
			mark.Scale(scale, scale)
		}
		ui.Text(c, label).FontSize(12).TextColor(c.Theme().TextMuted).ContentTransition(ui.ContentTransition{Animation: &motionFade})
	})
}
func marketSortLabel(sort string) string {
	return map[string]string{"relevance": "相关度", "downloads": "下载量", "follows": "关注数", "newest": "最新发布", "updated": "最近更新", "name": "名称", "popular": "热门"}[sort]
}
func compactCount(n int64) string {
	if n >= 100000000 {
		return fmt.Sprintf("%.1f亿", float64(n)/100000000)
	}
	if n >= 10000 {
		return fmt.Sprintf("%.1f万", float64(n)/10000)
	}
	if n >= 1000 {
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	}
	return fmt.Sprint(n)
}
func firstStrings(values []string, n int) []string { return values[:min(n, len(values))] }
func marketTag(c *ui.Context, value string, accent bool) {
	color, fill := colors(c.Theme()).Secondary, colors(c.Theme()).Tag
	if accent {
		color, fill = colors(c.Theme()).AccentInk, colors(c.Theme()).Selected
	}
	ui.Text(c, value).FontSize(10).TextColor(color).Padding(2, 5).Radius(4).Background(fill).MaxLines(1).Ellipsis("…")
}

const (
	marketDetailTabWidth float32 = 72
	marketDetailTabGap   float32 = 4
	marketDetailTabStep  float32 = marketDetailTabWidth + marketDetailTabGap
	marketDetailTabH     float32 = 28
)

func (s *Shell) marketDetailView(c *ui.Context) {
	if s.market.version != nil {
		s.marketVersionView(c)
		return
	}
	m, t := &s.market, s.theme
	p := *m.selected
	if m.detail != nil {
		p = m.detail.Project
	}
	s.configureMarketMarkdown(c, &m.markdown, m.mediaCache)
	ui.Row(c).FillWidth().Height(28).Shrink(0).Gap(8).Children(func() {
		iconButton(c, "market-detail-back", "返回搜索", iconBack, s.backMarketResults)
		ui.Spacer(c)
		if state := s.marketAssociation(); state != nil {
			ui.Button(c.Key("association-project"), "核对项目").Height(26).
				Disabled(state.active || state.origin != nil || m.detail == nil).
				OnClick(func() { s.startAssetAssociation(state, p.Source, p.ID, "") })
		}
	})
	if m.detailBusy {
		loadingStatus(c, "正在读取…")
		return
	}
	if m.error != "" {
		s.marketError(c)
		return
	}
	if m.detail == nil {
		return
	}
	p = m.detail.Project
	ui.Row(c).FillWidth().Shrink(0).AlignItems(ui.Center).Padding(2, 0, 4, 0).Gap(12).Children(func() {
		s.marketProjectIcon(c, p, 48)
		ui.Column(c).Grow(1).MinWidth(0).Gap(4).Children(func() {
			ui.Text(c, p.Title).FontSize(19).FontWeight(600).MaxLines(1).Ellipsis("…")
			ui.Text(c, p.Author).FontSize(12).TextColor(colors(t).Metadata).MaxLines(1).Ellipsis("…")
		})
		downloads := compactCount(p.Downloads)
		ui.Row(c).Shrink(0).AlignItems(ui.Center).Gap(5).
			Tooltip("下载量").Label("下载量 " + downloads).Children(func() {
			ui.Icon(c, iconDownload).Size(12, 12).TextColor(colors(t).Metadata)
			ui.Text(c, downloads).FontSize(12).TextColor(colors(t).Metadata).SingleLine()
		})
	})
	s.marketDetailTabs(c, p.PageURL)
	if m.tab == 0 {
		ui.Scroll(c.Key("market-description")).Grow(1).MinHeight(0).FillWidth().Gap(12).Padding(4, 0).Transition(motionReveal).Children(func() {
			ui.Text(c, p.Description).FontSize(12).FillWidth()
			if p.Body != "" {
				markdown.View(c, strings.TrimSpace(p.Body), &m.markdown).FillWidth()
			}
			if len(p.Loaders) > 0 {
				ui.Row(c).Wrap().Gap(6).Children(func() {
					for _, loader := range p.Loaders {
						s.marketLoaderTag(c, loader)
					}
				})
			}
		})
		return
	}
	versions := m.detail.Versions
	if len(versions) == 0 {
		marketStatus(c, "没有匹配版本", iconArchive)
		return
	}
	m.versionsList.Key = func(index int) any { return versions[index].ID }
	m.versionsList.Label = func(index int) string { return versions[index].Name }
	ui.List(c.Key("market-versions"), &m.versionsList, len(versions), func(index int) { v := versions[index]; s.marketVersionRow(c, p, v) }).Grow(1).MinHeight(0).FillWidth().Transition(motionReveal)
}

func (s *Shell) marketDetailTabs(c *ui.Context, pageURL string) {
	m, t := &s.market, s.theme
	services := c.Services()
	if m.tab != m.tabSynced {
		m.tabDrag = ui.Point{X: float32(m.tab) * marketDetailTabStep}
		m.tabSynced = m.tab
	}
	tabs := []struct {
		label string
		icon  *ui.SVG
	}{
		{label: "简介", icon: iconBook},
		{label: "版本", icon: iconLayers},
	}
	trackW := marketDetailTabWidth*2 + marketDetailTabGap
	ui.Row(c).FillWidth().Shrink(0).AlignItems(ui.Center).Gap(8).Children(func() {
		seg := ui.SegmentedBase(c.Key("market-detail-tabs"), &m.tab, len(tabs))
		seg.Track.Label("详情视图").Size(trackW, marketDetailTabH).AlignItems(ui.Center).Children(func() {
			pill := ui.Box(c).Absolute().Left(0).Top(0).Size(marketDetailTabWidth, marketDetailTabH)
			var segEls [2]ui.Element
			var labelIcons [2]ui.Element
			var labelTexts [2]ui.Element
			for i, item := range tabs {
				el := seg.Segment(i).Absolute().Left(float32(i)*marketDetailTabStep).Top(0).
					Size(marketDetailTabWidth, marketDetailTabH).Radius(7).Center().Gap(5).FocusRing(false)
				el.DrawOver(func(p *ui.Painter, r ui.Rect) {
					if el.FocusVisible() {
						p.FocusRing(r, [4]float32{7, 7, 7, 7})
					}
				})
				if i != m.tab && el.Hovered() {
					el.Background(colors(t).Hover)
				}
				el.Transition(ui.ElementTransition{Animation: &motionColor, Colors: true}).Children(func() {
					labelIcons[i] = ui.Icon(c, item.icon).Size(12, 12)
					labelTexts[i] = ui.Text(c, item.label).FontSize(12).SingleLine()
				})
				segEls[i] = el
			}
			handle := ui.Box(c.Key("market-tab-drag-handle")).Absolute().Left(0).Top(0).
				Size(marketDetailTabWidth, marketDetailTabH).Background(ui.Transparent).
				Draggable(&m.tabDrag, ui.DraggableOptions{
					Axis:      ui.Horizontal,
					Bounds:    &ui.Rect{W: marketDetailTabStep},
					Snap:      ui.Nearest(ui.Point{}, ui.Point{X: marketDetailTabStep}),
					Animation: motionTab,
				})
			dragPos := handle.DragOffset()
			dragging := handle.Dragging()
			if _, _, ended := handle.DragEnded(); ended {
				next := 0
				if m.tabDrag.X >= marketDetailTabStep/2 {
					next = 1
				}
				if next != m.tab {
					m.tab = next
				}
				m.tabSynced = m.tab
			}
			cursor := ui.CursorGrab
			if dragging {
				cursor = ui.CursorGrabbing
			}
			handle.Cursor(cursor)

			pill.Translate(dragPos.X, 0).Children(func() {
				lift := ui.Pose{Scale: 1}
				pillFill := colors(t).Selected
				if dragging {
					lift = ui.Pose{Scale: 1.03}
					pillFill = colors(t).SelectedHover
				} else if handle.Pressed() {
					lift = ui.Pose{Scale: 0.96}
					pillFill = colors(t).SelectedHover
				} else if handle.Hovered() {
					pillFill = colors(t).SelectedHover
				}
				ui.Box(c).Fill().Radius(7).
					Background(pillFill).
					Transition(ui.ElementTransition{Animation: &motionColor, Colors: true}).
					AnimatePose(lift, motionPress)
			})

			activeVisual := m.tab
			if dragging {
				if dragPos.X >= marketDetailTabStep/2 {
					activeVisual = 1
				} else {
					activeVisual = 0
				}
			}
			for i := range tabs {
				ink := colors(t).Metadata
				weight := 400
				if i == activeVisual {
					ink = t.Text
					weight = 600
				} else if segEls[i].Hovered() {
					ink = t.Text
				}
				labelIcons[i].TextColor(ink).Transition(ui.ElementTransition{Animation: &motionColor, Colors: true})
				labelTexts[i].TextColor(ink).FontWeight(weight).Transition(ui.ElementTransition{Animation: &motionColor, Colors: true})
			}
		})

		if market.SafeURL(pageURL) {
			label := "项目主页"
			if m.descriptionTruncated {
				label = "完整说明"
			}
			homeBtn := ui.ButtonBase(c.Key("market-website")).Label(label).Tooltip("在浏览器中打开项目主页").
				Height(marketDetailTabH).Padding(0, 10).Radius(7).Center().Gap(5).FocusRing(false)
			fill := ui.Transparent
			ink := colors(t).Metadata
			scale := float32(1)
			if homeBtn.Pressed() {
				fill = colors(t).Pressed
				ink = t.Text
				scale = 0.96
			} else if homeBtn.Hovered() {
				fill = colors(t).Hover
				ink = t.Text
			}
			homeBtn.Background(fill).
				Transition(ui.ElementTransition{Animation: &motionColor, Colors: true}).
				AnimatePose(ui.Pose{Scale: scale}, motionPress).
				OnClick(func() { services.OpenURL(pageURL) }).
				Children(func() {
					ui.Icon(c, iconCompass).Size(12, 12).TextColor(ink)
					ui.Text(c, label).FontSize(12).TextColor(ink).SingleLine()
				})
		}
	})
}
func (s *Shell) marketVersionRow(c *ui.Context, p market.Project, v market.Version) {
	t := s.theme
	ui.Row(c.Key(v.ID)).FillWidth().Height(48).Padding(4, 2).Gap(8).BorderWidth(0, 0, 1, 0).BorderColor(colors(t).Divider).Children(func() {
		ui.ButtonBase(c.Key("version-details")).Label("查看版本 " + v.Name).Grow(1).MinWidth(0).FillHeight().Justify(ui.Start).OnClick(func() { s.openMarketVersion(p, v) }).Children(func() {
			ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
				ui.Text(c, v.Name).FontSize(12).FontWeight(500).MaxLines(1).Ellipsis("…")
				ui.Text(c, strings.Join(firstStrings(v.GameVersions, 4), ", ")+" · "+strings.Join(v.Loaders, " / ")+" · "+v.Type).FontSize(10).TextColor(colors(t).Metadata).MaxLines(1).Ellipsis("…")
			})
		})
		ui.Text(c, v.Published.Format("2006-01-02")).FontSize(10).TextColor(colors(t).Metadata)
		label, disabled, action := s.marketVersionAction(p, v, false)
		ui.Button(c.Key("market-install-"+v.ID), label).Height(26).Disabled(disabled).OnClick(action)
	})
}
