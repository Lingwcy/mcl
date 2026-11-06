//go:build darwin || linux

package process_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"mcl/internal/platform/process"
)

func TestStopEscalatesAndTerminatesDescendants(t *testing.T) {
	t.Setenv("MCL_TEST_PROCESS_MODE", "tree")
	pidFile := filepath.Join(t.TempDir(), "child.pid")
	t.Setenv("MCL_TEST_CHILD_PID_FILE", pidFile)
	s, err := process.Start(launchPlan(t))
	if err != nil {
		t.Fatal(err)
	}
	stopCleanup(t, s)
	waitFor(t, func() bool { _, err := os.Stat(pidFile); return err == nil })
	data, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	if err := s.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	result := s.Wait()
	if !result.Stopped || result.Code == 0 {
		t.Fatalf("stop result %+v", result)
	}
	waitFor(t, func() bool { return errors.Is(syscall.Kill(pid, 0), syscall.ESRCH) })
}
