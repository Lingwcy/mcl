package ui

import (
	"testing"

	"github.com/egoist/mygo/ui"
	"mcl/internal/app"
	"mcl/internal/core/discovery"
)

func TestExistingDiscoveryRejectsObsoleteListsAndScans(t *testing.T) {
	shell := NewShell(app.DefaultSettings(), t.TempDir(), nil, nil)
	var lists []func([]app.ExistingSource, error)
	var scans []func(discovery.Report, error)
	stops := 0
	shell.ConnectExistingGames(ExistingGamesActions{
		List: func(done func([]app.ExistingSource, error)) func() {
			lists = append(lists, done)
			return func() { stops++ }
		},
		Scan: func(id string, done func(discovery.Report, error)) func() {
			scans = append(scans, done)
			return func() { stops++ }
		},
	})
	shell.openExistingGames()
	shell.cancelExistingGames()
	shell.loadExistingSources(false)
	lists[1]([]app.ExistingSource{{ID: "current", Name: "current", Path: "/current"}}, nil)
	lists[0]([]app.ExistingSource{{ID: "obsolete", Name: "obsolete", Path: "/obsolete"}}, nil)
	if len(shell.existing.sources) != 1 || shell.existing.sources[0].ID != "current" {
		t.Fatal("old list replaced current source state")
	}
	shell.scanExistingGames("current")
	shell.scanExistingGames("replacement")
	scans[0](discovery.Report{Root: "old"}, nil)
	if shell.existing.scanned || shell.existing.report.Root != "" {
		t.Fatal("late scan replaced selected source")
	}
	scans[1](discovery.Report{Root: "new"}, nil)
	if !shell.existing.scanned || shell.existing.report.Root != "new" || stops < 2 {
		t.Fatal("new scan failed or cancellation was lost")
	}
	shell.scanExistingGames("closing")
	shell.closeExistingGames()
	scans[2](discovery.Report{Root: "after-leaving"}, nil)
	if shell.existing.report.Root == "after-leaving" {
		t.Fatal("navigation accepted a hidden result")
	}
}

func TestExistingDiscoveryViewSurvivesDetailsAndNavigation(t *testing.T) {
	shell := NewShell(app.DefaultSettings(), t.TempDir(), nil, nil)
	shell.page = existingGamesPage
	shell.existing.sources = []app.ExistingSource{{ID: "source", Name: "Source", Path: "/explicit/source"}}
	shell.existing.sourceID = "source"
	shell.existing.scanned = true
	shell.existing.report = discovery.Report{Candidates: []discovery.Candidate{{ID: "candidate", Folder: "Local game", DocumentID: "local", GameVersion: "1.20.4", MainClass: "net.minecraft.client.main.Main", Status: "identified", Path: "/explicit/source/versions/local"}}}
	tester := ui.NewTester(shell.View, DefaultWidth, DefaultHeight)
	tester.Frame()
	shell.existing.selectedID = "candidate"
	tester.Frame()
	tester.Frame()
	shell.existing.selectedID = ""
	tester.Frame()
	tester.SetSize(MinWidth, MinHeight)
	tester.SetScale(2)
	tester.SetDark(true)
	tester.Frame()
	shell.page = libraryPage
	tester.Frame()
	shell.page = existingGamesPage
	tester.Frame()
	if !tester.HasText("发现已有游戏") {
		t.Fatal("read-only discovery page disappeared")
	}
}

func TestExistingDiscoveryHasNoInstructionEmptyStateOrReadOnlyBadge(t *testing.T) {
	shell := NewShell(app.DefaultSettings(), t.TempDir(), nil, nil)
	shell.page = existingGamesPage
	tester := ui.NewTester(shell.View, DefaultWidth, DefaultHeight)
	tester.Frame()
	tester.Frame()
	for _, label := range []string{"选择只读来源", "只读发现 · 不修改原目录，不代表已导入或可运行", "选择目录后扫描 versions 中的版本声明", "未发现匹配的版本", "本次未能完成有效版本发现"} {
		if tester.HasText(label) {
			t.Fatal("removed discovery filler returned", label)
		}
	}
	if !tester.HasText("发现已有游戏") {
		t.Fatal("functional page header removed")
	}
	shell.existing.sources = []app.ExistingSource{{ID: "source", Name: "Swift Craft Launcher", Path: "/explicit/source"}}
	shell.existing.sourceID = "source"
	shell.existing.scanned = true
	tester.Frame()
	tester.Frame()
	if tester.HasText("未发现匹配的版本") {
		t.Fatal("zero-result empty state returned")
	}
	tester.SetSize(MinWidth, MinHeight)
	tester.SetDark(true)
	tester.Frame()
}
