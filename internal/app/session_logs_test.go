package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func diagnosticRoot(t *testing.T) *os.Root {
	t.Helper()
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { root.Close() })
	for _, dir := range []string{"logs", "crash-reports"} {
		if err := root.Mkdir(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	return root
}
func TestSessionFilesRequireChangeAndExactPID(t *testing.T) {
	root := diagnosticRoot(t)
	if err := root.WriteFile("logs/latest.log", []byte("old unrelated warning"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := root.WriteFile("crash-reports/crash-2026-11-10_00.00.00-client.txt", []byte("old unrelated crash"), 0o600); err != nil {
		t.Fatal(err)
	}
	source, err := newSessionLogs(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	logs, err := source.Capture(t.Context(), 123, nil)
	if err != nil || len(logs) != 0 {
		t.Fatal("unchanged prior logs attributed to new session", err)
	}
	secret := "synthetic-current-session-secret"
	if err := root.WriteFile("logs/latest.log", []byte("NoClassDefFoundError: fixed.Class token="+secret), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := root.WriteFile("hs_err_pid123.log", []byte("SIGSEGV\nCommand Line: Main --accessToken "+secret), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := root.WriteFile("hs_err_pid456.log", []byte("FOREIGN_PROCESS"), 0o600); err != nil {
		t.Fatal(err)
	}
	logs, err = source.Capture(t.Context(), 123, func(v string) string { return strings.ReplaceAll(v, secret, "[REDACTED]") })
	if err != nil || len(logs) != 2 {
		t.Fatal("expected exact changed sources", len(logs), err)
	}
	for _, log := range logs {
		text := strings.Join(log.Lines, "\n")
		if strings.Contains(text, secret) || strings.Contains(text, "FOREIGN_PROCESS") || strings.Contains(text, "old unrelated") {
			t.Fatal("unrelated source or secret admitted")
		}
		if log.Kind == "jvm-error" && log.Association != "pid-and-session" {
			t.Fatal("PID association lost")
		}
	}
	source.Close()
	if _, err := source.Capture(t.Context(), 123, nil); err == nil {
		t.Fatal("closed source read from a later directory")
	}
}

func TestSessionFileTailRejectsChangesAndDropsPartialLeadingLine(t *testing.T) {
	root := diagnosticRoot(t)
	source, err := newSessionLogs(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	data := strings.Repeat("partial-sensitive-command", diagnosticTailBytes/8) + "\nNoClassDefFoundError: fixed.Class\n"
	if err := root.WriteFile("logs/latest.log", []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	logs, err := source.Capture(t.Context(), 123, nil)
	if err != nil || len(logs) != 1 || !logs[0].Truncated {
		t.Fatal("bounded tail absent", err)
	}
	text := strings.Join(logs[0].Lines, "\n")
	if strings.Contains(text, "partial-sensitive-command") || !strings.Contains(text, "fixed.Class") {
		t.Fatal("partial leading command/credential exported")
	}
	before, err := sessionLogStat(root, "logs/latest.log")
	if err != nil {
		t.Fatal(err)
	}
	if err := root.WriteFile("logs/latest.log", []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := readSessionLogTail(t.Context(), root, "logs/latest.log", before); err == nil {
		t.Fatal("replaced log accepted with old identity")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := source.Capture(ctx, 123, nil); !errors.Is(err, context.Canceled) {
		t.Fatal("file capture lost cancellation", err)
	}
}

func TestSessionFilesDoNotFollowLogLinks(t *testing.T) {
	root := diagnosticRoot(t)
	source, err := newSessionLogs(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "private.log")
	if err := os.WriteFile(outside, []byte("FOREIGN_SECRET"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := root.Symlink(outside, "logs/latest.log"); err != nil {
		t.Skip("symlink unavailable", err)
	}
	if logs, err := source.Capture(t.Context(), 123, nil); err == nil || logs != nil {
		t.Fatal("followed foreign log symlink")
	}
}

func TestOldModifiedTimesAreNotSessionEvidence(t *testing.T) {
	root := diagnosticRoot(t)
	source, err := newSessionLogs(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	if err := root.WriteFile("logs/latest.log", []byte("stale copied log"), 0o600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-24 * time.Hour)
	if err := os.Chtimes(filepath.Join(root.Name(), "logs/latest.log"), old, old); err != nil {
		t.Fatal(err)
	}
	logs, err := source.Capture(t.Context(), 123, nil)
	if err != nil || len(logs) != 0 {
		t.Fatal("old copied log used as current evidence", err)
	}
}
