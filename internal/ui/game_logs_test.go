package ui

import (
	"github.com/egoist/mygo/ui"
	"mcl/internal/app"
	"mcl/internal/core/launch"
	"testing"
)

func TestDiagnosticActionRejectsCanceledAndOtherSessionResults(t *testing.T) {
	shell := NewShell(app.DefaultSettings(), t.TempDir(), nil, nil)
	var completions []func(launch.RuntimeDiagnosis, error)
	cancels := 0
	shell.ConnectGames(GameActions{Diagnose: func(id string, done func(launch.RuntimeDiagnosis, error)) func() {
		if id != "game-1" {
			t.Fatal("wrong session requested")
		}
		completions = append(completions, done)
		return func() { cancels++ }
	}})
	shell.SetGames([]app.GameSnapshot{{ID: "game-1", VersionID: "1.12.2", Running: true, Window: app.GameWindowPending}})
	shell.showLogs("game-1")
	tester := ui.NewTester(shell.View, 1080, 720)
	click(t, tester, "诊断")
	tester.Frame()
	click(t, tester, "取消诊断")
	report := launch.DiagnoseRuntime(launch.OutputSnapshot{}, launch.Exit{}, true, "pending", nil)
	completions[0](report, nil)
	tester.Frame()
	if shell.games.showDiagnosis || shell.games.diagnosing || cancels != 1 {
		t.Fatal("canceled result changed view")
	}
	click(t, tester, "诊断")
	completions[1](report, nil)
	tester.Frame()
	if !shell.games.showDiagnosis || shell.games.diagnosing {
		t.Fatal("current result not shown")
	}
	click(t, tester, "返回日志")
	tester.Frame()
	tester.SetDark(true)
	tester.SetScale(2)
	tester.Frame()
	click(t, tester, "诊断")
	shell.showLogs("other-session")
	completions[2](report, nil)
	tester.Frame()
	if shell.games.showDiagnosis || shell.games.selected != "other-session" {
		t.Fatal("obsolete session stole diagnostic view")
	}
}

func TestAutomaticGameLogsRespectLaunchAndNavigation(t *testing.T) {
	for _, tc := range []struct {
		name              string
		page              page
		enabled, canceled bool
		id, current       string
		want              bool
	}{
		{"current success", launchPage, true, false, "session-1", "session-1", true},
		{"disabled", launchPage, false, false, "session-1", "session-1", false},
		{"stop requested", launchPage, true, true, "session-1", "session-1", false},
		{"navigated home", libraryPage, true, false, "session-1", "session-1", false},
		{"editing settings", gameSettingsPage, true, false, "session-1", "session-1", false},
		{"manual task log", logsPage, true, false, "session-1", "session-1", false},
		{"other launch", launchPage, true, false, "session-1", "session-2", false},
		{"no process", launchPage, true, false, "", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			shell := &Shell{page: tc.page}
			shell.installation.launchGameID = tc.current
			shell.maybeOpenGameLogs(tc.enabled, tc.canceled, tc.id)
			if tc.want {
				if shell.page != logsPage || shell.games.selected != tc.id {
					t.Fatal("did not select the exact launched session")
				}
			} else if shell.page != tc.page || shell.games.selected != "" {
				t.Fatal("automatic logging stole navigation or selected a guessed session")
			}
		})
	}
}
