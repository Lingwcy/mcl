package ui

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
	"mcl/internal/core/discovery"
)

func (s *Shell) existingGamesView(c *ui.Context) {
	d, t := &s.existing, s.theme
	ui.Column(c.Key("existing-games-workspace")).Fill().Padding(14, 18).Gap(10).Children(func() {
		ui.Row(c.Key("existing-header")).FillWidth().Height(28).Gap(8).Shrink(0).Children(func() {
			back := func() {
				if d.selectedID != "" {
					d.selectedID = ""
				} else {
					s.closeExistingGames()
				}
			}
			iconButton(c, "existing-back", "返回", iconBack, back)
			ui.Text(c, "发现已有游戏").FontSize(13).FontWeight(600).Grow(1)
			if d.busy {
				busyIndicator(c, 12)
				ui.Button(c.Key("cancel-existing"), "取消").Height(26).OnClick(s.cancelExistingGames)
			} else {
				iconButton(c, "refresh-existing", "重新扫描", iconRefresh, func() { s.loadExistingSources(true) }).Disabled(d.actions.List == nil)
				ui.Button(c.Key("add-existing-source"), "添加目录…").Height(26).FontSize(12).Disabled(d.actions.Add == nil).OnClick(s.addExistingSource)
			}
		})
		if len(d.sources) > 0 {
			source, selected := s.existingSource()
			ui.Row(c.Key("existing-source-choice")).FillWidth().Height(28).Gap(8).Children(func() {
				label := "选择来源"
				if selected {
					label = source.Name
				}
				ui.Button(c.Key("existing-source-menu"), label).Label("选择游戏来源").Grow(1).MinWidth(0).Height(27).Disabled(d.busy).Menu(func(menu *ui.Menu) {
					for _, row := range d.sources {
						if menu.Item(row.Name + " · " + row.Path).Checked(d.sourceID == row.ID).Chosen() {
							s.scanExistingGames(row.ID)
						}
					}
				})
				iconButton(c, "remove-existing-source", "移除来源（保留文件）", iconClose, s.removeExistingSource).Disabled(d.busy || !selected)
			})
		}
		if d.selectedID != "" {
			for _, row := range d.report.Candidates {
				if row.ID == d.selectedID {
					s.existingGameDetails(c, row)
					return
				}
			}
		}
		if len(d.sources) == 0 {
			return
		}
		ui.TextInput(c.Key("existing-search"), &d.query).Label("搜索已有游戏").Placeholder("搜索名称、版本或加载器").FillWidth().Height(28)
		if d.busy && d.operation == "scan" {
			loadingStatus(c, "正在扫描…")
			return
		}
		if !d.scanned {
			return
		}
		var rows []discovery.Candidate
		query := strings.ToLower(strings.TrimSpace(d.query))
		for _, row := range d.report.Candidates {
			if query == "" || strings.Contains(strings.ToLower(row.Folder+" "+row.DocumentID+" "+row.GameVersion+" "+existingLoaderLabel(row)), query) {
				rows = append(rows, row)
			}
		}
		if len(rows) == 0 {
			return
		}
		d.list.Key = func(index int) any { return rows[index].ID }
		d.list.Label = func(index int) string { return rows[index].Folder }
		ui.List(c.Key("existing-candidates"), &d.list, len(rows), func(index int) {
			row := rows[index]
			button := ui.ButtonBase(c.Key(row.ID)).Label(row.Folder+"，查看本地识别详情").FillWidth().Height(54).Padding(5, 9).Gap(9).Radius(7)
			fill := ui.Transparent
			if button.Hovered() {
				fill = t.SurfaceHover
			}
			rowMotion(c, button, fill).OnClick(func() { d.selectedID = row.ID }).Children(func() {
				ui.Icon(c, iconFolder).Size(17, 17).Shrink(0).TextColor(colors(t).Icon)
				ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
					ui.Text(c, row.Folder).FontSize(12).MaxLines(1).Ellipsis("…")
					label := row.GameVersion
					if label == "" {
						label = "基底未确认"
					}
					ui.Text(c, label+" · "+existingLoaderLabel(row)).FontSize(10).TextColor(colors(t).Metadata).MaxLines(1).Ellipsis("…")
				})
				if row.Status != "identified" {
					label := "需核对"
					color := colors(t).Metadata
					if row.Status == "unreadable" {
						label = "无法读取"
						color = t.Danger
					}
					ui.Text(c, label).FontSize(10).TextColor(color).Shrink(0)
				}
				ui.Icon(c, iconChevron).Size(12, 12).TextColor(colors(t).Icon)
			})
		}).FillWidth().Grow(1).MinHeight(0)
	})
}
func existingLoaderLabel(row discovery.Candidate) string {
	if len(row.Loaders) == 0 {
		if row.MainClass == "net.minecraft.client.main.Main" || row.MainClass == "net.minecraft.client.Minecraft" {
			return "原版"
		}
		return "未确认加载器"
	}
	var labels []string
	for _, hint := range row.Loaders {
		labels = append(labels, marketLoaderLabel(hint.Kind)+" "+hint.Version)
	}
	return strings.Join(labels, " / ")
}
func (s *Shell) existingGameDetails(c *ui.Context, row discovery.Candidate) {
	t := s.theme
	ui.Scroll(c.Key("existing-detail-scroll")).FillWidth().Grow(1).MinHeight(0).Children(func() {
		ui.Column(c.Key(row.ID)).FillWidth().Gap(10).Children(func() {
			ui.Row(c.Key("existing-import-header")).FillWidth().Height(28).Gap(8).Children(func() {
				ui.Text(c, row.Folder).FontSize(13).FontWeight(600).Grow(1).MinWidth(0)
				ui.Button(c.Key("copy-existing-game"), "复制为受管实例…").Height(26).Disabled(row.Status == "unreadable" || row.GameVersion == "" || row.UnknownComponents || s.directories.actions.InspectImport == nil).OnClick(func() { s.openDirectoryImport(row) })
			})
			environment := row.GameVersion
			loader := existingLoaderLabel(row)
			if environment != "" {
				environment += " · "
			}
			environment += loader
			ui.Text(c, environment).FontSize(11).TextColor(colors(t).Metadata).FillWidth()
			ui.Text(c, row.Path).FontSize(11).TextColor(t.TextMuted).FillWidth()
			for index, problem := range row.Problems {
				ui.Text(c.Key("problem-"+fmt.Sprint(index)), problem).FontSize(11).TextColor(t.Danger).FillWidth()
			}
		})
	})
}
