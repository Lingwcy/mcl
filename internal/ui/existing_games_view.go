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
		source, selected := s.existingSource()
		ui.Row(c.Key("existing-source-choice")).FillWidth().Height(28).Gap(8).Children(func() {
			label := "选择只读来源"
			if selected {
				label = source.Name
			}
			ui.Button(c.Key("existing-source-menu"), label).Grow(1).MinWidth(0).Height(27).Disabled(d.busy || len(d.sources) == 0).Menu(func(menu *ui.Menu) {
				for _, row := range d.sources {
					if menu.Item(row.Name + " · " + row.Path).Checked(d.sourceID == row.ID).Chosen() {
						s.scanExistingGames(row.ID)
					}
				}
			})
			iconButton(c, "remove-existing-source", "移除来源登记（保留文件）", iconClose, s.removeExistingSource).Disabled(d.busy || !selected)
		})
		if selected {
			ui.Text(c, source.Path).FontSize(10).TextColor(colors(t).Metadata).FillWidth()
		}
		ui.Text(c, "只读发现 · 不修改原目录，不代表已导入或可运行").FontSize(11).TextColor(t.TextMuted).FillWidth()
		if d.selectedID != "" {
			for _, row := range d.report.Candidates {
				if row.ID == d.selectedID {
					s.existingGameDetails(c, row)
					return
				}
			}
		}
		ui.TextInput(c.Key("existing-search"), &d.query).Label("搜索已有游戏").Placeholder("搜索名称、版本或加载器").FillWidth().Height(28)
		if d.busy && d.operation == "scan" {
			loadingStatus(c, "正在只读扫描…")
			return
		}
		if len(d.report.Problems) > 0 {
			for index, message := range d.report.Problems {
				if index >= 4 {
					ui.Text(c, fmt.Sprintf("另有%d项来源问题", len(d.report.Problems)-index)).FontSize(11).TextColor(t.Danger)
					break
				}
				ui.Text(c, message).FontSize(11).TextColor(t.Danger).FillWidth()
			}
		}
		if d.report.Truncated {
			ui.Text(c, "扫描已达到数量或读取边界，以下不是完整目录清单").FontSize(11).TextColor(t.Danger).FillWidth()
		}
		if !d.scanned {
			ui.Text(c, "选择目录后扫描 versions 中的版本声明").FontSize(12).TextColor(t.TextMuted).Margin(10, 0)
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
			label := "未发现匹配的版本"
			if query == "" && len(d.report.Problems) > 0 {
				label = "本次未能完成有效版本发现"
			}
			ui.Text(c, label).FontSize(12).TextColor(t.TextMuted).Margin(10, 0)
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
				label := "格式已识别"
				color := colors(t).Metadata
				if row.Status == "unreadable" {
					label = "文档不可读"
					color = t.Danger
				} else if row.Status != "identified" {
					label = "需核对"
				}
				ui.Text(c, label).FontSize(10).TextColor(color).Shrink(0)
				ui.Icon(c, iconChevron).Size(12, 12).TextColor(colors(t).Icon)
			})
		}).FillWidth().Grow(1).MinHeight(0)
	})
}
func existingLoaderLabel(row discovery.Candidate) string {
	if len(row.Loaders) == 0 {
		if row.MainClass == "net.minecraft.client.main.Main" || row.MainClass == "net.minecraft.client.Minecraft" {
			return "原版主类声明"
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
			for _, field := range []struct{ name, value string }{
				{"文档ID", row.DocumentID}, {"游戏基底（本地声明）", row.GameVersion}, {"加载器线索", existingLoaderLabel(row)},
				{"版本目录", row.Path}, {"客户端JAR路径（仅检查存在）", row.ClientPath}, {"客户端主类", row.MainClass},
			} {
				if field.value != "" {
					ui.Column(c.Key(field.name)).FillWidth().Gap(3).Children(func() {
						ui.Text(c, field.name).FontSize(10).TextColor(colors(t).Metadata)
						ui.Text(c, field.value).FontSize(12).FillWidth()
					})
				}
			}
			if len(row.GamePaths) > 0 {
				ui.Text(c, "游戏数据候选位置（未自动选定）").FontSize(10).TextColor(colors(t).Metadata)
				for index, directory := range row.GamePaths {
					ui.Text(c.Key(index), directory).FontSize(11).FillWidth()
				}
			}
			for index, problem := range row.Problems {
				ui.Text(c.Key("problem-"+fmt.Sprint(index)), problem).FontSize(11).TextColor(t.Danger).FillWidth()
			}
			ui.Text(c, "未校验文件摘要或游戏兼容性；不会读取其他启动器的账户、执行脚本或改写配置。").FontSize(11).TextColor(t.TextMuted).FillWidth()
		})
	})
}
