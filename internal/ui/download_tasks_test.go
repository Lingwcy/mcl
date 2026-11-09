package ui

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"mcl/internal/app"
	"mcl/internal/core/tasks"
)

func TestDownloadTaskDetailsRejectObsoleteCallbacks(t *testing.T) {
	shell := NewShell(app.DefaultSettings(), t.TempDir(), nil, nil)
	var callbacks []func(tasks.Group, error)
	stopped := 0
	shell.ConnectDownloadTasks(DownloadTaskActions{Details: func(id string, done func(tasks.Group, error)) func() {
		callbacks = append(callbacks, done)
		return func() { stopped++ }
	}})
	shell.openDownloadTask("old")
	shell.openDownloadTask("new")
	callbacks[0](tasks.Group{ID: "old"}, nil)
	if shell.downloadJobs.detail.ID != "" {
		t.Fatal("late details replaced selected job")
	}
	callbacks[1](tasks.Group{ID: "new"}, nil)
	if shell.downloadJobs.detail.ID != "new" || stopped < 1 {
		t.Fatal("current details missing")
	}
	shell.closeDownloadTask()
	callbacks[1](tasks.Group{ID: "after-exit"}, nil)
	if shell.downloadJobs.detail.ID != "" {
		t.Fatal("hidden details accepted late callback")
	}
}
func TestDownloadTaskCommandsWaitForPauseAcknowledgement(t *testing.T) {
	shell := NewShell(app.DefaultSettings(), t.TempDir(), nil, nil)
	var complete func(error)
	calls := 0
	shell.ConnectDownloadTasks(DownloadTaskActions{Control: func(id, command string, done func(error)) { calls++; complete = done }})
	shell.downloadTaskCommand("job", "pause")
	shell.downloadTaskCommand("job", "pause")
	if calls != 1 || !shell.downloadJobs.commands["job"] {
		t.Fatal("duplicate pause or command completed early")
	}
	complete(nil)
	if shell.downloadJobs.commands["job"] {
		t.Fatal("acknowledged pause still blocked")
	}
}
func TestDownloadTasksSurviveExtraFramesFilteringDetailsAndNavigation(t *testing.T) {
	shell := NewShell(app.DefaultSettings(), t.TempDir(), nil, nil)
	shell.ConnectDownloadTasks(DownloadTaskActions{Refresh: func(done func(error)) { done(nil) }})
	shell.page = activityPage
	shell.SetDownloadTasks([]tasks.Group{{ID: "one", Intent: tasks.Intent{Name: "Install", Kind: "game"}, State: "running", Owned: true, Stage: "下载", Received: 30, TotalBytes: 100, FileCount: 2}, {ID: "two", Intent: tasks.Intent{Name: "Paused", Kind: "preparation"}, State: "paused"}})
	tester := ui.NewTester(shell.View, DefaultWidth, DefaultHeight)
	tester.Frame()
	tester.Frame()
	shell.downloadJobs.selected = "one"
	shell.downloadJobs.detail = tasks.Group{ID: "one", Intent: tasks.Intent{Name: "Install"}, Files: []tasks.File{{Key: "file", Path: "cache/a.jar", State: "downloading", Received: 30, Total: 100}}}
	tester.Frame()
	tester.Frame()
	tester.SetSize(MinWidth, MinHeight)
	tester.SetScale(2)
	tester.SetDark(true)
	tester.Frame()
	shell.page = libraryPage
	tester.Frame()
	if shell.downloadJobs.selected != "" {
		t.Fatal("leaving task page kept stale detail request")
	}
	shell.page = activityPage
	tester.Frame()
	if !tester.HasText("下载与任务") {
		t.Fatal("unified task page disappeared")
	}
}
