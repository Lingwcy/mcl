package ui

import (
	"context"
	"errors"
	"slices"

	"github.com/egoist/mygo/ui"

	"mcl/internal/core/tasks"
)

type DownloadTaskActions struct {
	Refresh func(func(error))
	Control func(string, string, func(error))
	Resume  func(string, func(error))
	Remove  func(string, func(error))
	Details func(string, func(tasks.Group, error)) func()
	Clear   func(bool, func(error))
}
type downloadTaskState struct {
	actions                     DownloadTaskActions
	groups                      []tasks.Group
	selected                    string
	detail                      tasks.Group
	list, files                 ui.ListState
	commands                    map[string]bool
	showLegacy, loading, closed bool
	generation                  uint64
	cancelDetail                func()
}

func (s *Shell) ConnectDownloadTasks(actions DownloadTaskActions) {
	s.downloadJobs.actions = actions
	s.downloadJobs.commands = map[string]bool{}
}
func (s *Shell) SetDownloadTasks(groups []tasks.Group) {
	d := &s.downloadJobs
	if d.closed {
		return
	}
	previous := map[string]string{}
	for _, group := range d.groups {
		previous[group.ID] = group.State
	}
	d.groups = slices.Clone(groups)
	refresh, javaChanged, assetsChanged := false, false, false
	for _, group := range groups {
		old, found := previous[group.ID]
		if found && old != "completed" && group.State == "completed" {
			refresh = true
			javaChanged = javaChanged || group.Intent.Kind == "java"
			assetsChanged = assetsChanged || group.Intent.InstanceID == s.library.selectedID
		}
	}
	if refresh {
		s.refreshLibrary(false)
		s.refreshSessions()
	}
	if javaChanged {
		s.refreshJava()
	}
	if assetsChanged {
		s.refreshAssets()
	}
}
func (s *Shell) downloadTaskCommand(id, command string) {
	d := &s.downloadJobs
	if d.closed || d.commands[id] {
		return
	}
	d.commands[id] = true
	done := func(err error) {
		if d.closed {
			return
		}
		delete(d.commands, id)
		if err != nil && !errors.Is(err, context.Canceled) {
			s.Notify(ToastError, err.Error())
		}
	}
	switch command {
	case "resume":
		if d.actions.Resume != nil {
			d.actions.Resume(id, done)
		} else {
			done(nil)
		}
	case "remove":
		if d.actions.Remove != nil {
			d.actions.Remove(id, done)
		} else {
			done(nil)
		}
	default:
		if d.actions.Control != nil {
			d.actions.Control(id, command, done)
		} else {
			done(nil)
		}
	}
}
func (s *Shell) refreshDownloadTasks() {
	d := &s.downloadJobs
	if d.loading || d.actions.Refresh == nil {
		return
	}
	d.loading = true
	d.actions.Refresh(func(err error) {
		if d.closed {
			return
		}
		d.loading = false
		if err != nil {
			s.Notify(ToastError, err.Error())
		}
	})
}
func (s *Shell) clearDownloadTasks(parts bool) {
	d := &s.downloadJobs
	if d.actions.Clear == nil || d.commands["clear"] {
		return
	}
	d.commands["clear"] = true
	d.actions.Clear(parts, func(err error) {
		if d.closed {
			return
		}
		delete(d.commands, "clear")
		if err != nil && !errors.Is(err, context.Canceled) {
			s.Notify(ToastError, err.Error())
			return
		}
		if err == nil {
			s.refreshDownloadTasks()
		}
	})
}
func (s *Shell) openDownloadTask(id string) {
	d := &s.downloadJobs
	if d.actions.Details == nil {
		return
	}
	s.closeDownloadTask()
	d.selected = id
	d.loading = true
	generation := d.generation
	d.cancelDetail = d.actions.Details(id, func(group tasks.Group, err error) {
		if d.closed || d.generation != generation || d.selected != id {
			return
		}
		d.loading = false
		d.cancelDetail = nil
		if err != nil {
			s.Notify(ToastError, err.Error())
			return
		}
		d.detail = group
	})
}
func (s *Shell) closeDownloadTask() {
	d := &s.downloadJobs
	d.generation++
	if d.cancelDetail != nil {
		d.cancelDetail()
	}
	d.cancelDetail = nil
	d.selected = ""
	d.loading = false
	d.detail = tasks.Group{}
}
func (s *Shell) downloadGroupState(id string) string {
	for _, group := range s.downloadJobs.groups {
		if group.Intent.InstanceID == id && (group.Owned || group.State == "paused") {
			return group.State
		}
	}
	return ""
}
func downloadTaskLabel(group tasks.Group) string {
	if group.Remote {
		return "其他启动器占用"
	}
	switch group.State {
	case "running":
		return group.Stage
	case "queued":
		return "等待中"
	case "pausing":
		return "正在暂停…"
	case "paused":
		return "已暂停"
	case "canceling":
		return "正在取消…"
	case "interrupted":
		return "可继续"
	case "failed":
		return "未完成"
	case "completed":
		return "已完成"
	case "canceled":
		return "已取消"
	}
	return group.State
}

func (s *Shell) javaDownloadPaused(id string) bool {
	for _, group := range s.downloadJobs.groups {
		if group.Intent.Kind == "java" && group.Intent.Fingerprint == id && group.State == "paused" {
			return true
		}
	}
	return false
}

func (s *Shell) taskPaused(kind, id string) bool {
	for _, group := range s.downloadJobs.groups {
		if group.Owned && group.State == "paused" && group.Intent.Kind == kind && (id == "" || group.Intent.InstanceID == id) {
			return true
		}
	}
	return false
}
func (s *Shell) marketTaskPaused(i *marketInstallState) bool {
	if i == nil {
		return false
	}
	id := i.request.InstanceID
	if i.archivePlan != nil {
		id = i.archivePlan.Instance.ID
	}
	return s.taskPaused("market", id) || s.taskPaused("pack", id) || s.taskPaused("archive", "") || s.taskPaused("game", id)
}
func (s *Shell) workStatus(c *ui.Context, paused bool, label string) {
	if paused {
		ui.Text(c, "已暂停").FontSize(12).TextColor(s.theme.TextMuted)
	} else {
		loadingStatus(c, label)
	}
}
