package ui

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
	"mcl/internal/core/instance"
	"mcl/internal/core/transfer"
)

func (s *Shell) directoryView(c *ui.Context) {
	d, t := &s.directories, s.theme
	ui.Column(c.Key("directory-workbench")).Fill().Padding(18, 24, 20).Gap(10).Children(func() {
		ui.Row(c.Key("directory-header")).FillWidth().Height(28).Gap(8).Children(func() {
			iconButton(c, "directory-back", "返回", iconBack, s.closeDirectoryView)
			title := "复制已有游戏"
			if d.mode == "move" {
				title = "迁移游戏目录"
			}
			if d.mode == "tasks" {
				title = "目录任务与旧副本"
			}
			ui.Text(c, title).FontSize(12).Grow(1)
			if d.mode != "tasks" && !d.running {
				ui.Button(c.Key("directory-tasks"), "目录任务").Height(26).OnClick(s.openDirectoryTasks)
			}
			if !d.busy && d.mode == "tasks" {
				iconButton(c, "directory-refresh", "重新读取目录任务", iconRefresh, s.loadDirectoryOperations)
			}
		})
		if d.busy {
			ui.Row(c.Key("directory-progress")).FillWidth().Height(30).Gap(8).Children(func() {
				busyIndicator(c, 12)
				label := d.progress.Stage
				if label == "" {
					label = "正在处理…"
				}
				if d.canceling {
					label = "正在停止…"
				}
				ui.Text(c, label).FontSize(12).Grow(1)
				if d.operation != "confirm" {
					ui.Button(c.Key("directory-cancel"), "停止").Height(24).Disabled(d.canceling).OnClick(s.stopDirectoryTask)
				}
			})
			if d.progress.TotalFiles > 0 {
				ui.Text(c, fmt.Sprintf("%d / %d 个文件 · %.1f / %.1f MiB", d.progress.Files, d.progress.TotalFiles, float64(d.progress.Bytes)/(1<<20), float64(d.progress.TotalBytes)/(1<<20))).FontSize(10).TextColor(colors(t).Metadata)
			} else if d.progress.Files > 0 {
				ui.Text(c, fmt.Sprintf("已检查 %d 个文件 · %.1f MiB", d.progress.Files, float64(d.progress.Bytes)/(1<<20))).FontSize(10).TextColor(colors(t).Metadata)
			}
		}
		if d.mode == "tasks" {
			s.directoryTasksView(c)
			return
		}
		if d.mode == "import" {
			gameSettingRow(c, "directory-import-name", "名称", func() {
				ui.TextInput(c.Key("directory-import-name-input"), &d.request.Name).Label("新实例名称").Grow(1).MinWidth(0).Height(28).Disabled(d.busy).OnChange(func() { d.plan = nil })
			})
			gameSettingRow(c, "directory-source-data", "源资料", func() { s.directorySourcePicker(c) })
			gameSettingRow(c, "directory-copy-groups", "复制内容", func() { s.directoryGroupPicker(c) })
		} else {
			ui.Text(c, d.request.Name).FontSize(12).FillWidth()
		}
		gameSettingRow(c, "directory-target", "目标位置", func() {
			s.gameDirectoryMenu(c, "directory-transfer-target", d.to, d.busy, func(id string) { d.to = id; d.plan = nil })
		})
		ui.Row(c.Key("directory-plan-actions")).FillWidth().Height(28).Gap(8).Children(func() {
			ui.Button(c.Key("directory-preview"), "检查并预览").Height(28).Disabled(d.busy || d.mode == "import" && (d.request.DataPath == "" || instance.ValidateName(strings.TrimSpace(d.request.Name)) != nil)).OnClick(s.previewDirectory)
			ui.Spacer(c)
			label := "确认复制…"
			if d.mode == "move" {
				label = "确认迁移…"
			}
			ui.PrimaryButton(c.Key("directory-confirm"), label).Height(28).Disabled(d.busy || d.plan == nil).OnClick(s.confirmDirectoryPlan)
		})
		if d.plan == nil {
			label := "先关闭原游戏，明确选择共享或隔离资料目录。原资料只读，运行库由 mcl 重新安装；不继承账户、JVM 参数或启动器脚本。"
			if d.mode == "move" {
				label = "仅迁移游戏、存档及实例专属备份。核验后提交位置，旧副本保留；不移动中央记录或共享 Java/运行缓存。"
			}
			ui.Text(c, label).FontSize(11).TextColor(t.TextMuted).FillWidth()
			return
		}
		plan := d.plan
		environment := plan.After.Version.ID
		if required, ok := plan.After.RequiredLoader(); ok {
			environment += " · " + marketLoaderLabel(required.Type) + " " + required.Version
		}
		ui.Text(c, environment).FontSize(11).FillWidth()
		ui.Text(c, fmt.Sprintf("将复制 %d 个文件 · %.2f GiB", len(plan.Tree.Files), float64(plan.Tree.Bytes)/(1<<30))).FontSize(11).TextColor(colors(t).Metadata)
		if len(plan.Tree.Skipped) > 0 {
			ui.Text(c, fmt.Sprintf("已排除 %d 项账户、脚本、锁文件等内容（最多列出512项）", len(plan.Tree.Skipped))).FontSize(11).TextColor(t.TextMuted)
		}
		s.directoryFileList(c, plan.Tree)
	})
}
func (s *Shell) directorySourcePicker(c *ui.Context) {
	d := &s.directories
	button := ui.ButtonBase(c.Key("directory-source-choice")).Label("明确选择来源游戏资料目录").Grow(1).MinWidth(0).Height(30).Padding(0, 5).Gap(6).Radius(5).Disabled(d.busy).OnClick(func() { d.pickerOpen = !d.pickerOpen })
	button.Children(func() {
		label := d.request.DataPath
		if label == "" {
			label = "选择共享或隔离目录"
		}
		ui.Text(c, label).FontSize(11).TextColor(s.theme.TextMuted).Grow(1).MinWidth(0).MaxLines(2)
		ui.Icon(c, iconChevronDown).Size(10, 10)
	})
	width, height := c.Size()
	creationPopover(c.Key("directory-source-panel"), button, &d.pickerOpen, min(440, width-32), func() {
		ui.Scroll(c.Key("directory-source-scroll")).FillWidth().Height(min(float32(max(1, len(d.candidate.GamePaths)))*65, min(240, height*.5))).Children(func() {
			ui.RadioGroup(c.Key("directory-source-radio"), func() {
				for _, source := range d.candidate.GamePaths {
					ui.RadioBase(c.Key(source), &d.request.DataPath, source).Label(source).FillWidth().MinHeight(56).Padding(8, 10).Radius(6).Disabled(d.busy).OnClick(func() { d.pickerOpen = false; d.plan = nil }).OnChange(func() { d.plan = nil }).Children(func() { ui.Text(c, source).FontSize(11).FillWidth() })
				}
			})
		})
	})
}
func (s *Shell) directoryGroupPicker(c *ui.Context) {
	d := &s.directories
	count := 0
	for _, enabled := range d.groups {
		if enabled {
			count++
		}
	}
	button := ui.ButtonBase(c.Key("directory-groups-choice")).Label("选择复制的游戏资料分类").Grow(1).Height(28).Padding(0, 5).Gap(7).Radius(5).Disabled(d.busy).OnClick(func() { d.groupsOpen = !d.groupsOpen })
	button.Children(func() {
		ui.Text(c, fmt.Sprintf("%d 项分类", count)).FontSize(12).Grow(1)
		ui.Icon(c, iconChevronDown).Size(10, 10)
	})
	creationPopover(c.Key("directory-group-panel"), button, &d.groupsOpen, 290, func() {
		ui.Scroll(c.Key("directory-groups-scroll")).FillWidth().Height(240).Padding(7).Children(func() {
			for _, group := range transfer.ImportGroups {
				checked := d.groups[group]
				ui.Checkbox(c.Key(group), &checked, directoryGroupLabel(group)).Label(directoryGroupLabel(group)).FillWidth().Height(26).FontSize(12).Disabled(d.busy).OnClick(func() { d.groups[group] = checked; d.plan = nil })
			}
		})
	})
}
func directoryGroupLabel(group string) string {
	switch group {
	case "saves":
		return "存档"
	case "mods":
		return "模组"
	case "config":
		return "模组配置（按需选择）"
	case "resourcepacks":
		return "资源包"
	case "shaderpacks":
		return "光影"
	case "screenshots":
		return "截图"
	case "texturepacks":
		return "旧纹理包"
	case "options.txt":
		return "游戏选项"
	case "optionsof.txt":
		return "OptiFine 选项（不含补丁）"
	case "servers.dat":
		return "服务器列表"
	}
	return group
}
func (s *Shell) directoryFileList(c *ui.Context, tree transfer.Tree) {
	d := &s.directories
	count := len(tree.Files) + len(tree.Skipped)
	d.files.Key = func(index int) any {
		if index < len(tree.Files) {
			return tree.Files[index].Path
		}
		return "excluded:" + tree.Skipped[index-len(tree.Files)]
	}
	d.files.Label = func(index int) string {
		if index < len(tree.Files) {
			return tree.Files[index].Path
		}
		return "不复制 " + tree.Skipped[index-len(tree.Files)]
	}
	ui.List(c.Key("directory-planned-files"), &d.files, count, func(index int) {
		label := d.files.Label(index)
		ui.Row(c.Key(label)).FillWidth().Height(26).Padding(0, 5).Children(func() {
			ui.Text(c, label).FontSize(11).TextColor(s.theme.TextMuted).Grow(1).MinWidth(0).MaxLines(1).Ellipsis("…")
		})
	}).FillWidth().Grow(1).MinHeight(0)
}
func (s *Shell) directoryTasksView(c *ui.Context) {
	d := &s.directories
	if len(d.operations) == 0 && !d.busy {
		ui.Text(c, "没有未完成的目录操作或待清理旧副本").FontSize(12).TextColor(s.theme.TextMuted)
		return
	}
	d.tasks.Key = func(index int) any {
		return d.operations[index].InstanceID + ":" + d.operations[index].Token + fmt.Sprint(d.operations[index].Backup)
	}
	d.tasks.Label = func(index int) string { return d.operations[index].Name }
	ui.List(c.Key("directory-operation-list"), &d.tasks, len(d.operations), func(index int) {
		operation := d.operations[index]
		label := "等待恢复"
		if operation.Backup {
			label = "旧副本保留"
		}
		if operation.Phase == "aborting" {
			label = "等待继续撤销"
		}
		sourceLabel := s.gameDirectoryName(operation.From)
		if operation.Kind == "import" {
			sourceLabel = "原目录"
		}
		if operation.Changed {
			label = "旧副本包含后续变化"
		}
		ui.Row(c.Key(d.tasks.Key(index))).FillWidth().MinHeight(60).Padding(7, 5).Gap(8).Children(func() {
			ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
				ui.Text(c, operation.Name).FontSize(12).MaxLines(1).Ellipsis("…")
				ui.Text(c, label+" · "+sourceLabel+" → "+s.gameDirectoryName(operation.To)).FontSize(10).TextColor(colors(s.theme).Metadata).FillWidth()
			})
			if operation.Backup {
				ui.Button(c.Key("directory-open-backup"), "核对旧副本").Height(26).Disabled(d.busy || d.actions.Open == nil).OnClick(func() { s.openDirectoryBackup(operation) })
				ui.Button(c.Key("directory-clean-backup"), "清理…").Height(26).Disabled(d.busy).OnClick(func() { s.directoryMaintenance(operation, true) })
			} else {
				if operation.Phase != "aborting" {
					ui.Button(c.Key("directory-resume"), "继续").Height(26).Disabled(d.busy).OnClick(func() { s.resumeDirectory(operation) })
				}
				if operation.Phase == "copying" || operation.Phase == "publishing" || operation.Phase == "aborting" {
					ui.Button(c.Key("directory-abort"), "撤销…").Height(26).Disabled(d.busy).OnClick(func() { s.directoryMaintenance(operation, false) })
				}
			}
		})
	}).FillWidth().Grow(1).MinHeight(0)
}
