package ui

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"

	"mcl/internal/core/tasks"
)

func (s *Shell) activityView(c *ui.Context) {
	if s.downloadJobs.actions.Refresh == nil {
		s.legacyActivityView(c)
		return
	}
	s.downloadTasksView(c)
}

func (s *Shell) downloadTasksView(c *ui.Context) {
	d, t := &s.downloadJobs, s.theme
	ui.Column(c.Key("download-tasks-workspace")).Fill().Padding(18, 24, 24).Gap(8).Children(func() {
		ui.Row(c.Key("download-tasks-header")).FillWidth().Height(28).Gap(8).Children(func() {
			settingsGroupLabel(c, s.tr("下载与任务")).Grow(1)
			if d.loading {
				busyIndicator(c, 12)
			}
			iconButton(c, "refresh-download-tasks", s.tr("刷新任务"), iconRefresh, s.refreshDownloadTasks).Disabled(d.loading)
			ui.ButtonBase(c.Key("download-task-tools")).Label(s.tr("任务维护")).Tooltip(s.tr("任务维护")).Size(26, 26).Radius(5).Center().Disabled(d.commands["clear"]).Menu(func(menu *ui.Menu) {
				if menu.Item(s.tr("清理已结束记录…")).Chosen() {
					s.clearDownloadTasks(false)
				}
				if menu.Item(s.tr("清理下载分片…")).Chosen() {
					s.clearDownloadTasks(true)
				}
			}).Children(func() { ui.Icon(c, iconSliders).Size(14, 14).TextColor(colors(t).Icon) })
		})
		hasDownloads := false
		for _, group := range d.groups {
			if group.Intent.Kind != "preparation" {
				hasDownloads = true
				break
			}
		}
		if !hasDownloads {
			ui.Column(c).FillWidth().Grow(1).Center().Children(func() { ui.Text(c, s.tr("暂无下载任务")).FontSize(12).TextColor(t.TextMuted) })
			return
		}
		ui.Scroll(c.Key("download-task-groups")).FillWidth().Grow(1).MinHeight(0).Gap(6).Children(func() {
			for _, group := range d.groups {
				if group.Intent.Kind != "preparation" {
					s.downloadTaskRow(c, group)
				}
			}
		})
	})
}

func (s *Shell) downloadTaskRow(c *ui.Context, group tasks.Group) {
	d, t := &s.downloadJobs, s.theme
	expanded := d.selected == group.ID
	ui.Column(c.Key(group.ID)).FillWidth().Shrink(0).Padding(6, 10).Gap(5).Radius(7).Border(0.5, colors(t).ListDivider).Background(t.Surface).Clip().Transition(motionTaskCard).Children(func() {
		ui.Row(c).FillWidth().Height(26).Gap(6).Children(func() {
			ui.Text(c, s.downloadTaskName(group)).FontSize(12).Grow(1).MinWidth(0).MaxLines(1).Ellipsis("…")
			if group.FileCount > 0 {
				ui.Text(c, fmt.Sprintf("%d / %d", group.DoneFiles, group.FileCount)).FontSize(11).TextColor(colors(t).Metadata).Shrink(0)
			}
			stateColor := t.TextMuted
			if group.State == "failed" || group.State == "canceled" {
				stateColor = t.Danger
			}
			ui.Text(c, s.tr(downloadTaskLabel(group))).FontSize(11).TextColor(stateColor).Shrink(0)
			if !group.Remote {
				switch group.State {
				case "running":
					if group.Owned {
						sizedIconButton(c, "pause-task", s.tr("暂停"), iconPause, 24, 14, 5, func() { s.downloadTaskCommand(group.ID, "pause") }).Disabled(d.commands[group.ID] || group.Stage == "发布")
					}
				case "paused", "interrupted", "failed":
					label := "继续"
					if group.State == "failed" {
						label = "重试"
					}
					if group.Intent.Kind == "preparation" && !group.Owned {
						label = "继续准备"
					}
					sizedIconButton(c, "resume-task", s.tr(label), iconPlay, 24, 14, 5, func() { s.downloadTaskCommand(group.ID, "resume") }).Disabled(d.commands[group.ID])
				}
				if !group.Terminal() {
					sizedIconButton(c, "cancel-task", s.tr("取消任务"), iconClose, 24, 14, 5, func() { s.downloadTaskCommand(group.ID, "cancel") }).Disabled(d.commands[group.ID] || group.State == "canceling")
				} else {
					sizedIconButton(c, "remove-task-history", s.tr("移除任务记录"), iconClose, 24, 14, 5, func() { s.downloadTaskCommand(group.ID, "remove") }).Disabled(d.commands[group.ID])
				}
			}
			icon, label := iconChevron, "展开文件"
			if expanded {
				icon, label = iconChevronDown, "收起文件"
			}
			sizedIconButton(c, "toggle-task-files", s.tr(label), icon, 24, 14, 5, func() {
				if expanded {
					s.closeDownloadTask()
				} else {
					s.openDownloadTask(group.ID)
				}
			})
		})
		if group.TotalBytes > 0 && (group.State == "running" || group.State == "pausing" || group.State == "canceling") {
			smoothProgress(c, float64(group.Received)/float64(group.TotalBytes)).Label(s.tr("下载进度"))
		}
		if group.Reason != "" && group.State != "canceled" {
			ui.Row(c).FillWidth().Gap(8).Children(func() {
				ui.Text(c, group.Reason).FontSize(11).TextColor(t.TextMuted).Grow(1).MinWidth(0)
				if group.Intent.InstanceID != "" {
					ui.Button(c.Key("review-task-instance"), s.tr("查看实例")).Height(24).OnClick(func() { s.selectInstance(group.Intent.InstanceID) })
				}
			})
		}
		if expanded {
			ui.Column(c.Key("task-expanded-files")).FillWidth().Shrink(0).Gap(4).Transition(motionTaskExpand).Children(func() {
				gameSettingsDivider(c)
				if d.loading {
					ui.Text(c, s.tr("正在读取…")).FontSize(11).TextColor(t.TextMuted)
				} else if d.detail.ID == group.ID {
					files := d.detail.Files
					if len(files) == 0 {
						ui.Text(c, s.tr("暂无文件")).FontSize(10).TextColor(colors(t).Metadata)
					} else {
						d.inlineFiles.Key = func(index int) any { return files[index].Key }
						d.inlineFiles.Label = func(index int) string { return files[index].Path }
						ui.List(c.Key("inline-task-files"), &d.inlineFiles, len(files), func(index int) {
							file := files[index]
							ui.Row(c.Key(file.Key)).FillWidth().Height(30).Gap(8).Padding(0, 2).Children(func() {
								ui.Text(c, file.Path).FontSize(11).Grow(1).MinWidth(0).MaxLines(1).Ellipsis("…")
								ui.Text(c, formatBytes(file.Received)+" / "+formatBytes(file.Total)).FontSize(10).TextColor(colors(t).Metadata).Shrink(0)
								ui.Text(c, s.tr(downloadFileLabel(file.State))).FontSize(10).TextColor(colors(t).Metadata).Shrink(0)
							})
						}).FillWidth().Height(min(float32(len(files))*30, 150))
					}
				}
			})
		}
	})
}

// Old records did not distinguish installation from repair. Preserve the
// record and avoid guessing the historical operation from today's receipt.
func (s *Shell) downloadTaskName(group tasks.Group) string {
	if group.Intent.Kind == "game" {
		if suffix, ok := strings.CutPrefix(group.Intent.Name, "安装 / 修复 · "); ok {
			return s.tr("客户端任务") + " · " + suffix
		}
	}
	return group.Intent.Name
}

func downloadFileLabel(label string) string {
	switch label {
	case "pending":
		return "等待"
	case "checking":
		return "检查缓存"
	case "downloading":
		return "下载"
	case "verifying":
		return "校验"
	case "publishing":
		return "发布"
	case "done":
		return "已校验"
	case "paused":
		return "已暂停"
	case "failed":
		return "未完成"
	case "canceled":
		return "已取消"
	case "skipped":
		return "已跳过"
	case "interrupted":
		return "可继续"
	}
	return label
}
