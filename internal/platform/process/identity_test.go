package process_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mcl/internal/core/launch"
	"mcl/internal/platform/process"
)

func TestKernelIdentityAndStalePID(t *testing.T) {
	identity, err := process.Inspect(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	if err := identity.Validate(); err != nil {
		t.Fatal(err)
	}
	if alive, err := process.Running(identity); err != nil || !alive {
		t.Fatalf("self identity %v %v", alive, err)
	}
	identity.Stamp += "-stale"
	if alive, err := process.Running(identity); err != nil || alive {
		t.Fatalf("PID reuse not rejected: %v %v", alive, err)
	}
	if _, err := process.Inspect(-1); err == nil {
		t.Fatal("invalid PID accepted")
	}
}

func TestTrackedHandoffExecutesOnlyAfterRegistration(t *testing.T) {
	t.Setenv("MCL_TEST_PROCESS_MODE", "marker")
	marker := filepath.Join(t.TempDir(), "executed")
	t.Setenv("MCL_TEST_EXECUTED_FILE", marker)
	var observed launch.ProcessIdentity
	s, err := process.StartTracked(launchPlan(t), func(identity launch.ProcessIdentity) error {
		observed = identity
		if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("target executed before registration")
		}
		time.Sleep(15 * time.Millisecond)
		if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("gated process executed during registration")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	stopCleanup(t, s)
	if result := s.Wait(); result.Code != 0 {
		t.Fatalf("handoff exit %+v", result)
	}
	if observed.PID != s.PID() {
		t.Fatal("identity changed PID")
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatal("registered target never ran")
	}
	if alive, err := process.Running(observed); err != nil || alive {
		t.Fatalf("exited target still live: %v %v", alive, err)
	}
}

func TestRegistrationFailureNeverRunsTarget(t *testing.T) {
	t.Setenv("MCL_TEST_PROCESS_MODE", "marker")
	marker := filepath.Join(t.TempDir(), "executed")
	t.Setenv("MCL_TEST_EXECUTED_FILE", marker)
	var observed launch.ProcessIdentity
	_, err := process.StartTracked(launchPlan(t), func(identity launch.ProcessIdentity) error {
		observed = identity
		return errors.New("cannot save identity " + syntheticToken)
	})
	if err == nil || strings.Contains(err.Error(), syntheticToken) {
		t.Fatalf("unsafe registration failure: %v", err)
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("target executed after registration failure")
	}
	if alive, err := process.Running(observed); err != nil || alive {
		t.Fatalf("failed gate not reaped: %v %v", alive, err)
	}
}
