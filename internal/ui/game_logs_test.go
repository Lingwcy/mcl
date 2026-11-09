package ui

import "testing"

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
