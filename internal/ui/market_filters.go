package ui

import (
	"slices"
	"strings"

	"mcl/internal/app"
	"mcl/internal/core/market"

	"github.com/egoist/mygo/ui"
)

func (m *marketState) filterKey(group string) string {
	return string(m.source) + ":" + string(m.kind) + ":" + group
}

func (s *Shell) marketFilterGroup(c *ui.Context, id, title string, selected int, defaultOpen bool, body func()) {
	m, t := &s.market, s.theme
	key := m.filterKey(id)
	open, known := m.filterExpanded[key]
	if !known {
		open = defaultOpen
	}
	ui.Column(c.Key("filter-section-" + key)).FillWidth().Shrink(0).Gap(1).Children(func() {
		parts := ui.CollapsibleBase(c.Key("filter-group-"+key), &open)
		button := parts.Trigger.Label(title).Width(secondarySidebarWidth(c)-24).Height(26).Margin(3, 12, 0).Padding(0, 4).Gap(6).Radius(5).Justify(ui.Start)
		fill := ui.Transparent
		if button.Hovered() {
			fill = t.SurfaceHover
		}
		rowMotion(c, button, fill).OnClick(func() {
			if m.filterExpanded == nil {
				m.filterExpanded = make(map[string]bool)
			}
			// Collapsible updates the bound value before this deferred action.
			m.filterExpanded[key] = open
		}).Children(func() {
			groupIcon := iconFilter
			switch id {
			case "loaders":
				groupIcon = iconComponent
			case "categories":
				groupIcon = iconTags
			case "environments":
				groupIcon = iconMonitor
			}
			ui.Icon(c, groupIcon).Size(13, 13).TextColor(colors(t).Icon)
			ui.Text(c, title).FontSize(11).FontWeight(500).TextColor(t.TextMuted)
			if selected > 0 {
				ui.Textf(c, "%d", selected).FontSize(10).TextColor(colors(t).AccentInk)
			}
			ui.Spacer(c)
			ui.Icon(c, iconChevron).Size(11, 11).TextColor(colors(t).Icon).Rotate(parts.Progress() * 90)
		})
		parts.Panel(body).FillWidth().Gap(1)
	})
}

func marketFilterChoice(c *ui.Context, key, label string, symbol marketSymbol, selected, disabled bool, change func(bool)) {
	checked := selected
	ui.Checkbox(c.Key(key), &checked, "").Label(label).Width(secondarySidebarWidth(c)-30).Height(24).Margin(0, 15).Gap(6).Disabled(disabled).OnClick(func() { change(checked) }).Children(func() {
		color := c.Theme().TextMuted
		if selected {
			color = colors(c.Theme()).AccentInk
		}
		symbol.draw(c, 16, color)
		ui.Text(c, label).FontSize(12).Grow(1).MinWidth(0).MaxLines(1).Ellipsis("…")
	})
}

func selectedFilter(values []string, value string, selected bool) []string {
	result := slices.Clone(values)
	if selected {
		if !slices.Contains(result, value) {
			result = append(result, value)
		}
		return result
	}
	return slices.DeleteFunc(result, func(item string) bool { return item == value })
}

func (s *Shell) marketFilterMore(c *ui.Context, group string, total, initial int) {
	m := &s.market
	key := m.filterKey(group)
	expanded := m.filterMore[key]
	if total > initial {
		label := "显示更多"
		if expanded {
			label = "收起"
		}
		ui.ButtonBase(c.Key("filter-more-"+key)).Label(label).Width(secondarySidebarWidth(c)-32).Height(22).Margin(1, 16, 2).Padding(0, 4).Justify(ui.Start).OnClick(func() {
			if m.filterMore == nil {
				m.filterMore = make(map[string]bool)
			}
			m.filterMore[key] = !expanded
		}).Children(func() {
			icon := iconChevronDown
			if expanded {
				icon = iconChevron
			}
			ui.Icon(c, icon).Size(12, 12).TextColor(colors(s.theme).Icon)
			ui.Text(c, label).FontSize(11).TextColor(s.theme.TextMuted)
		})
	}
}

func (s *Shell) marketLoaderFilters(c *ui.Context) {
	m := &s.market
	loaders := []string{"fabric", "forge", "neoforge", "quilt"}
	if m.source == market.Modrinth {
		loaders = append(loaders, "liteloader", "rift", "babric", "legacy-fabric", "ornithe")
	} else {
		loaders = append(loaders, "liteloader")
	}
	if m.kind == app.AssetShader {
		if m.source == market.CurseForge {
			return
		}
		loaders = []string{"iris", "optifine", "canvas", "vanilla"}
	} else if m.kind != app.AssetMod && m.kind != app.AssetModpack {
		return
	}
	s.marketFilterGroup(c, "loaders", "加载器", len(m.loaders), true, func() {
		more := m.filterMore[m.filterKey("loaders")]
		needsVersion := m.source == market.CurseForge && m.gameVersion == ""
		for index, loader := range loaders {
			selected := slices.Contains(m.loaders, loader)
			if !more && index >= 3 && !selected {
				continue
			}
			marketFilterChoice(c, "filter-loader-"+loader, marketLoaderLabel(loader), s.loaderSymbol(loader), selected, needsVersion && !selected, func(checked bool) {
				m.loaders = selectedFilter(m.loaders, loader, checked)
				s.searchMarket(true)
			})
		}
		s.marketFilterMore(c, "loaders", len(loaders), 3)
	})
}

func (s *Shell) marketCategoryFilters(c *ui.Context) {
	m, t := &s.market, s.theme
	limit := 32
	if m.source == market.CurseForge {
		limit = 10
	}
	s.marketFilterGroup(c, "categories", "类别", len(m.categoryIDs), m.source == market.CurseForge, func() {
		if m.categoriesBusy {
			ui.Text(c, "读取中…").FontSize(11).TextColor(t.TextMuted).Margin(3, 16)
			return
		}
		if m.categoryError != "" {
			ui.Row(c).FillWidth().Padding(0, 14).Gap(8).Children(func() {
				ui.Text(c, "类别不可用").FontSize(11).TextColor(t.TextMuted)
				iconButton(c, "retry-categories", "重试类别", iconRefresh, s.loadMarketCategories)
			})
			return
		}
		atLimit := len(m.categoryIDs) >= limit
		groups := make(map[string][]market.Category)
		for _, category := range m.categories {
			groups[category.Header] = append(groups[category.Header], category)
		}
		headers := make([]string, 0, len(groups))
		for header := range groups {
			headers = append(headers, header)
		}
		slices.Sort(headers)
		for _, header := range headers {
			entries := groups[header]
			key := "categories:" + header
			more := m.filterMore[m.filterKey(key)]
			if header != "" && len(headers) > 1 {
				ui.Text(c, marketHeaderLabel(header)).FontSize(10).TextColor(colors(t).Metadata).Margin(5, 17, 2)
			}
			for index, category := range entries {
				selected := slices.Contains(m.categoryIDs, category.ID)
				if !more && index >= 6 && !selected {
					continue
				}
				marketFilterChoice(c, "filter-category-"+category.ID, marketCategoryLabel(category.Name), s.categorySymbol(category.ID, category.Name), selected, !selected && atLimit, func(checked bool) {
					m.categoryIDs = selectedFilter(m.categoryIDs, category.ID, checked)
					s.searchMarket(true)
				})
			}
			s.marketFilterMore(c, key, len(entries), 6)
		}
		if len(m.categories) == 0 {
			ui.Text(c, "无可用类别").FontSize(11).TextColor(t.TextMuted).Margin(3, 16)
		}
	})
}

func (s *Shell) marketEnvironmentFilters(c *ui.Context) {
	m := &s.market
	if m.source != market.Modrinth {
		return
	}
	selected := len(m.environments)
	if m.openSource {
		selected++
	}
	s.marketFilterGroup(c, "environments", "环境", selected, true, func() {
		for _, environment := range []struct{ id, label string }{{"client", "客户端"}, {"server", "服务端"}} {
			icon := iconMonitor
			if environment.id == "server" {
				icon = iconServer
			}
			marketFilterChoice(c, "filter-environment-"+environment.id, environment.label, marketSymbol{svg: icon}, slices.Contains(m.environments, environment.id), false, func(checked bool) {
				m.environments = selectedFilter(m.environments, environment.id, checked)
				s.searchMarket(true)
			})
		}
		marketFilterChoice(c, "filter-open-source", "开源", marketSymbol{svg: iconCode}, m.openSource, false, func(checked bool) { m.openSource = checked; s.searchMarket(true) })
	})
}

func marketLoaderLabel(value string) string {
	switch value {
	case "fabric":
		return "Fabric"
	case "forge":
		return "Forge"
	case "neoforge":
		return "NeoForge"
	case "quilt":
		return "Quilt"
	case "liteloader":
		return "LiteLoader"
	case "rift":
		return "Rift"
	case "babric":
		return "Babric"
	case "legacy-fabric":
		return "Legacy Fabric"
	case "ornithe":
		return "Ornithe"
	case "iris":
		return "Iris"
	case "optifine":
		return "OptiFine"
	case "canvas":
		return "Canvas"
	case "vanilla":
		return "原版"
	default:
		return value
	}
}

func marketHeaderLabel(value string) string {
	switch value {
	case "categories":
		return "类别"
	case "features":
		return "特性"
	case "resolutions":
		return "分辨率"
	case "performance impact":
		return "性能开销"
	default:
		return value
	}
}

func marketCategoryLabel(value string) string {
	switch value {
	case "adventure":
		return "冒险"
	case "technology":
		return "科技"
	case "optimization":
		return "优化"
	case "library":
		return "库"
	case "utility":
		return "实用"
	case "worldgen":
		return "世界生成"
	case "decoration":
		return "装饰"
	case "equipment":
		return "装备"
	case "magic":
		return "魔法"
	case "management":
		return "管理"
	case "mobs":
		return "生物"
	case "storage":
		return "存储"
	case "transportation":
		return "交通"
	case "food":
		return "食物"
	case "game-mechanics":
		return "游戏机制"
	case "social":
		return "社交"
	case "realistic":
		return "写实"
	case "fantasy":
		return "奇幻"
	default:
		return strings.ReplaceAll(value, "-", " ")
	}
}
