package ui

import (
	"github.com/egoist/mygo/ui"
	"mcl/internal/app"
	"mcl/internal/core/discovery"
	"mcl/internal/core/instance"
	"mcl/internal/core/meta"
	"mcl/internal/core/transfer"
	"testing"
)

func TestDirectoryPreviewRejectsLateResultsAfterNavigation(t *testing.T) {
	shell := NewShell(app.DefaultSettings(), t.TempDir(), nil, nil)
	var callbacks []func(app.DirectoryPlan, error)
	stops := 0
	shell.ConnectDirectories(DirectoryActions{InspectImport: func(_ app.DirectoryImportRequest, _ func(transfer.Progress), done func(app.DirectoryPlan, error)) func() {
		callbacks = append(callbacks, done)
		return func() { stops++ }
	}})
	shell.existing.sourceID = instance.NewID()
	shell.openDirectoryImport(discovery.Candidate{ID: "candidate", Folder: "copy", GamePaths: []string{"/source"}})
	shell.previewDirectory()
	shell.closeDirectoryView()
	callbacks[0](app.DirectoryPlan{Kind: "import", Token: "obsolete"}, nil)
	if shell.directories.plan != nil || shell.directories.busy || stops != 1 {
		t.Fatal("navigation accepted obsolete preview")
	}
	shell.openDirectoryImport(discovery.Candidate{ID: "other", Folder: "other", GamePaths: []string{"/other"}})
	shell.previewDirectory()
	callbacks[1](app.DirectoryPlan{Kind: "import", Token: "current"}, nil)
	if shell.directories.plan == nil || shell.directories.plan.Token != "current" {
		t.Fatal("current preview missing")
	}
}
func TestDirectoryStopWaitsForWorkerAndSurvivesNavigation(t *testing.T) {
	shell := NewShell(app.DefaultSettings(), t.TempDir(), nil, nil)
	var done func(instance.Instance, error)
	var progress func(transfer.Progress)
	stops := 0
	shell.ConnectDirectories(DirectoryActions{Run: func(_ app.DirectoryPlan, report func(transfer.Progress), callback func(instance.Instance, error)) func() {
		done = callback
		progress = report
		return func() { stops++ }
	}})
	shell.page = directoryPage
	shell.directories.mode = "move"
	shell.directories.returnPage = activityPage
	plan := app.DirectoryPlan{Kind: "move", Token: "job", After: instance.Instance{ID: "stable"}}
	shell.runDirectoryPlan(plan)
	shell.stopDirectoryTask()
	shell.stopDirectoryTask()
	if !shell.directories.running || !shell.directories.busy || !shell.directories.canceling || stops != 1 {
		t.Fatal("stop released job before worker completion")
	}
	shell.closeDirectoryView()
	progress(transfer.Progress{Stage: "copy", Files: 1})
	if shell.page != activityPage || shell.directories.progress.Files != 1 {
		t.Fatal("hidden job lost its status or navigated")
	}
	done(plan.After, nil)
	if shell.page != activityPage || shell.directories.running || shell.directories.busy {
		t.Fatal("completion stole navigation or retained busy state")
	}
}
func TestDirectoryWorkbenchUsesStableViewsAcrossPlansAndTasks(t *testing.T) {
	shell := NewShell(app.DefaultSettings(), t.TempDir(), nil, nil)
	shell.existing.sourceID = "source"
	shell.openDirectoryImport(discovery.Candidate{ID: "candidate", Folder: "Game", GamePaths: []string{"/shared", "/isolated"}})
	tester := ui.NewTester(shell.View, DefaultWidth, DefaultHeight)
	tester.Frame()
	tester.Frame()
	shell.directories.plan = &app.DirectoryPlan{Kind: "import", After: instance.Instance{Version: meta.ManifestEntry{ID: "1.20.4"}}, Tree: transfer.Tree{Files: []transfer.File{{Path: "mods/local.jar", Size: 12}}, Skipped: []string{"config/accounts.json"}}}
	tester.Frame()
	tester.Frame()
	shell.directories.plan = nil
	tester.Frame()
	shell.directories.mode = "tasks"
	shell.directories.operations = []app.DirectoryOperation{{InstanceID: "id", Token: "token", Name: "Game", Kind: "move", Phase: "copying"}, {InstanceID: "id", Token: "backup", Name: "Game", Kind: "move", Phase: "complete", Backup: true}}
	tester.Frame()
	tester.SetSize(MinWidth, MinHeight)
	tester.SetDark(true)
	tester.Frame()
	shell.page = activityPage
	tester.Frame()
	shell.page = directoryPage
	tester.Frame()
	if !tester.HasText("目录任务与旧副本") {
		t.Fatal("directory task view missing")
	}
}
