package ui

import (
	"strings"

	"mcl/internal/core/instance"

	"github.com/egoist/mygo/ui"
)

type instanceSearchState struct {
	open   bool
	query  string
	active int
	list   ui.ListState
}

func (s *Shell) openInstanceSearch() {
	s.library.search = instanceSearchState{open: true}
}

func (s *Shell) instanceSearchResults() []instance.Instance {
	query := strings.ToLower(strings.TrimSpace(s.library.search.query))
	rows := s.library.snapshot.Instances
	if query == "" {
		return rows
	}
	var matches []instance.Instance
	for _, value := range rows {
		if strings.Contains(strings.ToLower(value.Name+" "+value.Version.ID+" "+instanceType(value)), query) {
			matches = append(matches, value)
		}
	}
	return matches
}

func (s *Shell) chooseInstanceSearchResult(id string) {
	l := &s.library
	if !l.search.open {
		return
	}
	for index, value := range l.snapshot.Instances {
		if value.ID == id {
			l.search.open = false
			l.sidebarList.ScrollIntoView(index)
			s.selectInstance(id)
			return
		}
	}
}

func (s *Shell) instanceSearchDialog(c *ui.Context) {
	l, t := &s.library, s.theme
	search := &l.search
	p := colors(t)
	ui.DialogBase(c.Key("instance-search-dialog"), &search.open, func(backdrop, panel ui.Element) {
		backdrop.Background(p.Shadow)
		panel.Label("搜索实例").Width(520).Padding(10).Radius(15).
			Background(p.Raised).Border(1, p.Border).Shadow(0, 12, 36, 0, p.Shadow).Transition(motionReveal)
		ui.Column(c).FillWidth().Gap(6).Children(func() {
			ui.Row(c).FillWidth().Height(38).Padding(0, 9).Gap(9).Children(func() {
				ui.Icon(c, iconSearch).Size(16, 16).TextColor(p.Icon)
				ui.TextInputBase(c.Key("instance-search-input"), &search.query).Label("实例搜索词").Placeholder("搜索实例…").
					Grow(1).MinWidth(0).FillHeight().FontSize(15).AutoFocus().OnChange(func() {
					search.active = 0
					search.list.ScrollTo(0, ui.Start)
				}).OnSubmit(func() {
					rows := s.instanceSearchResults()
					if len(rows) > 0 {
						s.chooseInstanceSearchResult(rows[min(search.active, len(rows)-1)].ID)
					}
				})
			})
			rows := s.instanceSearchResults()
			if len(rows) == 0 {
				message := "暂无实例"
				if strings.TrimSpace(search.query) != "" {
					message = "没有匹配的实例"
				}
				ui.Text(c, message).FontSize(12).TextColor(p.Secondary).Padding(12, 9)
			} else {
				s.instanceSearchList(c, panel, rows)
			}
			ui.Divider(c)
			button := ui.ButtonBase(c.Key("search-refresh-instances")).Label("刷新实例").FillWidth().Height(30).
				Padding(0, 9).Radius(7).Gap(9).Justify(ui.Start).Disabled(l.busy || l.actions.Load == nil)
			rowMotion(c, button, ui.Transparent).OnClick(func() { s.refreshLibrary(false) }).Children(func() {
				ui.Icon(c, iconRefresh).Size(14, 14).TextColor(p.Icon)
				ui.Text(c, "刷新实例").FontSize(12).TextColor(p.Secondary)
			})
		})
	})
}

func (s *Shell) instanceSearchList(c *ui.Context, panel ui.Element, rows []instance.Instance) {
	l, p := &s.library, colors(s.theme)
	search := &l.search
	active := min(search.active, len(rows)-1)
	if panel.OverlayShortcut(0, ui.KeyDown) {
		active = min(active+1, len(rows)-1)
		search.list.ScrollIntoView(active)
	} else if panel.OverlayShortcut(0, ui.KeyUp) {
		active = max(active-1, 0)
		search.list.ScrollIntoView(active)
	}
	search.active = active
	search.list.Key = func(index int) any { return rows[index].ID }
	search.list.Label = func(index int) string { return rows[index].Name }
	const rowHeight = 34
	ui.List(c.Key("instance-search-results"), &search.list, len(rows), func(index int) {
		value := rows[index]
		fill := ui.Transparent
		if index == active {
			fill = p.Control
		}
		row := ui.ButtonBase(c.Key(value.ID)).Label(value.Name).FillWidth().Height(rowHeight).
			Padding(0, 9).Radius(7).Gap(9).Justify(ui.Start)
		rowMotion(c, row, fill).OnClick(func() { s.chooseInstanceSearchResult(value.ID) }).Children(func() {
			s.instanceAvatar(c, value, 20)
			ui.Text(c, value.Name).Grow(1).MinWidth(0).FontSize(13).MaxLines(1).Ellipsis("…")
			if value.Name != value.Version.ID {
				ui.Text(c, value.Version.ID).FontSize(10).TextColor(p.Secondary).Padding(2, 6).Radius(5).Background(p.Tag)
			}
			if value.ID == l.selectedID {
				ui.Icon(c, iconCheck).Label("当前实例").Size(13, 13).TextColor(p.Icon)
			}
		})
	}).FillWidth().Height(float32(min(len(rows), 8) * rowHeight))
}
