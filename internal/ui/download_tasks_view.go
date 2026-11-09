package ui

import (
	"fmt"

	"github.com/egoist/mygo/ui"

	"mcl/internal/core/tasks"
)

func (s *Shell) activityView(c *ui.Context) {
	if s.downloadJobs.actions.Refresh == nil || s.downloadJobs.showLegacy {
		s.legacyActivityView(c)
		return
	}
	s.downloadTasksView(c)
}
func (s *Shell) downloadTasksView(c *ui.Context) {
	d, t := &s.downloadJobs, s.theme
	ui.Column(c.Key("download-tasks-workspace")).Fill().Padding(18, 24, 24).Gap(10).Children(func() {
		ui.Row(c.Key("download-tasks-header")).FillWidth().Height(28).Gap(8).Children(func() {
			if d.selected != "" {
				iconButton(c, "download-task-back", "返回任务列表", iconBack, s.closeDownloadTask)
			}
			settingsGroupLabel(c, "下载与任务").Grow(1)
			if d.loading {
				busyIndicator(c, 12)
			}
			if d.selected == "" {
				ui.Button(c.Key("other-tasks"), "会话与其他任务").Height(26).OnClick(func() { d.showLegacy = true })
				iconButton(c, "refresh-download-tasks", "刷新任务", iconRefresh, s.refreshDownloadTasks).Disabled(d.loading)
				ui.ButtonBase(c.Key("download-task-tools")).Label("任务维护").Tooltip("任务维护").Size(26, 26).Radius(5).Center().Disabled(d.commands["clear"]).Menu(func(menu *ui.Menu) {
					if menu.Item("清理已结束记录…").Chosen() {
						s.clearDownloadTasks(false)
					}
					if menu.Item("清理下载分片…").Chosen() {
						s.clearDownloadTasks(true)
					}
				}).Children(func() { ui.Icon(c, iconSliders).Size(14, 14).TextColor(colors(t).Icon) })
			} else {
				iconButton(c, "refresh-task-files", "重新读取文件状态", iconRefresh, func() { s.openDownloadTask(d.selected) }).Disabled(d.loading)
			}
		})
		if d.selected != "" {
			s.downloadTaskFiles(c)
			return
		}
		d.list.Key = func(index int) any { return d.groups[index].ID }
		d.list.Label = func(index int) string { return d.groups[index].Intent.Name }
		ui.List(c.Key("download-task-groups"), &d.list, len(d.groups), func(index int) { group := d.groups[index]; s.downloadTaskRow(c, group) }).FillWidth().Grow(1).MinHeight(0)
	})
}
func (s *Shell) downloadTaskRow(c *ui.Context, group tasks.Group) {
	d, t := &s.downloadJobs, s.theme
	ui.Column(c.Key(group.ID)).FillWidth().Padding(10, 5).Gap(5).Children(func() {
		ui.Row(c).FillWidth().Gap(8).Children(func() {
			if (group.State == "running" || group.State == "pausing" || group.State == "canceling") && !group.Remote {
				busyIndicator(c, 12)
			} else {
				ui.Icon(c, iconDownload).Size(14, 14).TextColor(colors(t).Icon)
			}
			ui.ButtonBase(c.Key("open-task-files")).Label(group.Intent.Name).Grow(1).MinWidth(0).Height(26).OnClick(func() { s.openDownloadTask(group.ID) }).Children(func() { ui.Text(c, group.Intent.Name).FontSize(12).MaxLines(1).Ellipsis("…") })
			ui.Text(c, downloadTaskLabel(group)).FontSize(11).TextColor(t.TextMuted).Shrink(0)
			if !group.Remote {
				switch group.State {
				case "running":
					if group.Owned {
						ui.Button(c.Key("pause-task"), "暂停").Height(24).Disabled(d.commands[group.ID] || group.Stage == "发布").OnClick(func() { s.downloadTaskCommand(group.ID, "pause") })
					}
				case "paused", "interrupted", "failed":
					label := "继续"
					if group.State == "failed" {
						label = "重试"
					}
					if group.Intent.Kind == "preparation" && !group.Owned {
						label = "继续准备"
					}
					ui.Button(c.Key("resume-task"), label).Height(24).Disabled(d.commands[group.ID]).OnClick(func() { s.downloadTaskCommand(group.ID, "resume") })
				}
				if !group.Terminal() {
					ui.Button(c.Key("cancel-task"), "取消").Height(24).Disabled(d.commands[group.ID] || group.State == "canceling").OnClick(func() { s.downloadTaskCommand(group.ID, "cancel") })
				} else {
					iconButton(c, "remove-task-history", "移除任务记录", iconClose, func() { s.downloadTaskCommand(group.ID, "remove") }).Disabled(d.commands[group.ID])
				}
			}
		})
		if group.FileCount > 0 {
			ui.Text(c, fmt.Sprintf("%s / %s · %d / %d 文件", formatBytes(group.Received), formatBytes(group.TotalBytes), group.DoneFiles, group.FileCount)).FontSize(10).TextColor(colors(t).Metadata)
			if group.TotalBytes > 0 && group.State != "completed" {
				smoothProgress(c, float64(group.Received)/float64(group.TotalBytes))
			}
		}
		if group.Reason != "" && group.State != "canceled" {
			ui.Row(c).FillWidth().Gap(8).Children(func() {
				ui.Text(c, group.Reason).FontSize(11).TextColor(t.TextMuted).Grow(1).MinWidth(0)
				if group.Intent.InstanceID != "" {
					ui.Button(c.Key("review-task-instance"), "查看实例").Height(24).OnClick(func() { s.selectInstance(group.Intent.InstanceID) })
				}
			})
		}
		gameSettingsDivider(c)
	})
}
func (s *Shell) downloadTaskFiles(c *ui.Context) {
	d := &s.downloadJobs
	group := d.detail
	if group.ID == "" {
		return
	}
	ui.Text(c, group.Intent.Name).FontSize(12).FillWidth()
	if group.Reason != "" {
		ui.Text(c, group.Reason).FontSize(11).TextColor(s.theme.TextMuted).FillWidth()
	}
	d.files.Key = func(index int) any { return group.Files[index].Key }
	d.files.Label = func(index int) string { return group.Files[index].Path }
	ui.List(c.Key("download-task-files"), &d.files, len(group.Files), func(index int) {
		file := group.Files[index]
		label := file.State
		switch label {
		case "pending":
			label = "等待"
		case "checking":
			label = "检查缓存"
		case "downloading":
			label = "下载"
		case "verifying":
			label = "校验"
		case "publishing":
			label = "发布"
		case "done":
			label = "已校验"
		case "paused":
			label = "已暂停"
		case "failed":
			label = "未完成"
		case "canceled":
			label = "已取消"
		case "interrupted":
			label = "可继续"
		}
		ui.Row(c.Key(file.Key)).FillWidth().Height(40).Gap(8).Padding(0, 5).Children(func() {
			ui.Text(c, file.Path).FontSize(11).Grow(1).MinWidth(0).MaxLines(1).Ellipsis("…")
			ui.Text(c, label).FontSize(10).TextColor(colors(s.theme).Metadata)
			ui.Text(c, formatBytes(file.Received)+" / "+formatBytes(file.Total)).FontSize(10).TextColor(colors(s.theme).Metadata).Shrink(0)
		})
	}).FillWidth().Grow(1).MinHeight(0)
}
