package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"mcl/internal/core/instance"
	"mcl/internal/core/launch"
)

type diagnosticGame struct {
	done   chan struct{}
	output launch.OutputSnapshot
}

func (g *diagnosticGame) PID() int                      { return 123 }
func (g *diagnosticGame) Done() <-chan struct{}         { return g.done }
func (g *diagnosticGame) Wait() launch.Exit             { <-g.done; return launch.Exit{Code: 1} }
func (g *diagnosticGame) Stop(context.Context) error    { return nil }
func (g *diagnosticGame) Detach()                       {}
func (g *diagnosticGame) Output() launch.OutputSnapshot { return g.output }
func (g *diagnosticGame) Changed() <-chan struct{}      { return nil }

func TestCompletedSessionDiagnosticsUseOnlyRedactedCapturedFiles(t *testing.T) {
	root := diagnosticRoot(t)
	directory := root.Name()
	source, err := newSessionLogs(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	secret := "synthetic-credential-not-to-retain"
	if err := root.WriteFile("logs/latest.log", []byte("java.lang.NoClassDefFoundError: fixed.Class "+secret), 0o600); err != nil {
		t.Fatal(err)
	}
	service := NewLaunchService(nil, nil, nil, nil, nil)
	game := &diagnosticGame{done: make(chan struct{}), output: launch.OutputSnapshot{Lines: []launch.Line{{Text: "Sound engine started"}}}}
	record := launch.Record{ID: instance.NewID(), InstanceID: instance.NewID(), VersionID: "1.12.2", Process: launch.ProcessIdentity{PID: 123, OS: "linux", Stamp: "123"}}
	var releases atomic.Int32
	service.manageSession(record, game, func() { releases.Add(1) }, func(v string) string { return strings.ReplaceAll(v, secret, "[REDACTED]") }, source, "ready")
	close(game.done)
	deadline := time.Now().Add(3 * time.Second)
	for {
		rows := service.Sessions()
		if len(rows) == 1 && !rows[0].Running && rows[0].DiagnosticFileState == "captured" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("session finalization did not complete")
		}
		time.Sleep(time.Millisecond)
	}
	if releases.Load() != 1 {
		t.Fatal("instance ownership was not released exactly once")
	}
	service.gamesMu.Lock()
	entry := service.games[record.ID]
	retainedSecret := entry.redact != nil
	service.gamesMu.Unlock()
	if retainedSecret {
		t.Fatal("completed history retained credential redactor")
	}
	if err := os.WriteFile(filepath.Join(directory, "logs", "latest.log"), []byte("NEXT_SESSION_DATA"), 0o600); err != nil {
		t.Fatal(err)
	}
	report, err := service.Diagnose(t.Context(), record.ID)
	if err != nil {
		t.Fatal(err)
	}
	text := report.Text()
	if strings.Contains(text, secret) || strings.Contains(text, "NEXT_SESSION_DATA") || !strings.Contains(text, "fixed.Class") {
		t.Fatal("wrong session source or raw credential in completed diagnosis")
	}
	rows := service.Sessions()
	rows[0].DiagnosticFiles[0].Lines[0] = "mutated"
	if service.Sessions()[0].DiagnosticFiles[0].Lines[0] == "mutated" {
		t.Fatal("snapshot aliases retained diagnostic files")
	}
}

func TestProbeErrorsAreNotReportedAsNoWindowOrVisible(t *testing.T) {
	service := NewLaunchService(nil, nil, nil, nil, func(launch.ProcessIdentity) (bool, error) { return false, os.ErrPermission })
	game := &diagnosticGame{done: make(chan struct{})}
	record := launch.Record{ID: instance.NewID(), InstanceID: instance.NewID(), Process: launch.ProcessIdentity{PID: 123, OS: "linux", Stamp: "123"}}
	service.manage(record, game, func() {}, func(s string) string { return s })
	defer close(game.done)
	deadline := time.Now().Add(2 * time.Second)
	for {
		rows := service.Sessions()
		if len(rows) == 1 && !rows[0].WindowCheckedAt.IsZero() {
			if rows[0].Window != GameWindowUnavailable || rows[0].WindowProbeCode != "permission" {
				t.Fatal("probe failure became false absence/visibility")
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("window probe was not recorded")
		}
		time.Sleep(time.Millisecond)
	}
}
