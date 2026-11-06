package process_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mcl/internal/core/instance"
	"mcl/internal/core/launch"
	"mcl/internal/fileio"
	"mcl/internal/platform/instancelock"
	"mcl/internal/platform/process"
	"mcl/internal/storage"
)

// Invoked as a separate test-binary launcher. Exiting here intentionally skips
// in-process defers: the parent test verifies what survives an owner crash.
func TestSessionOwnerHelper(t *testing.T) {
	action := os.Getenv("MCL_TEST_OWNER_ACTION")
	if action == "" {
		t.Skip("subprocess helper")
	}
	root := os.Getenv("MCL_TEST_OWNER_ROOT")
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	plan := launchPlan(t, executable, root)
	id := filepath.Base(plan.InstancePath())
	registry := storage.NewSessionStore(root, process.Running)
	lease, err := registry.Reserve(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	if err := fileio.Write(t.Context(), filepath.Join(root, "owner.ready"), []byte(id), 0o600); err != nil {
		t.Fatal(err)
	}
	if action == "hold-lock" {
		waitFor(t, func() bool { _, err := os.Stat(filepath.Join(root, "owner.release")); return err == nil })
		lease.Close()
		os.Exit(0)
	}
	t.Setenv("MCL_TEST_PROCESS_MODE", "detachable")
	t.Setenv("MCL_TEST_RELEASE", filepath.Join(root, "release"))
	game, err := process.StartTracked(plan, func(identity launch.ProcessIdentity) error {
		record := launch.Record{ID: instance.NewID(), InstanceID: id, VersionID: "1.20.4", VersionSHA1: strings.Repeat("a", 40), NativeDirectory: filepath.ToSlash(filepath.Join(plan.InstancePath(), "natives", "ready-"+strings.Repeat("A", 26))), Process: identity, CreatedAt: time.Now().UTC()}
		if err := lease.Track(t.Context(), record); err != nil {
			return err
		}
		if action == "before-handoff" {
			os.Exit(0)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return len(game.Output().Lines) > 0 })
	os.Exit(0)
}

func owner(t *testing.T, action string) (string, *exec.Cmd) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "owner data")
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	t.Cleanup(cancel)
	cmd := exec.CommandContext(ctx, executable, "-test.run=^TestSessionOwnerHelper$")
	cmd.Env = append(os.Environ(), "MCL_TEST_OWNER_ACTION="+action, "MCL_TEST_OWNER_ROOT="+root, "MCL_TEST_PROCESS_MODE=")
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if cmd.ProcessState == nil {
			cmd.Process.Kill()
			cmd.Wait()
		}
		// A failed assertion must not strand this test's detached target after
		// the owner has exited. Only identities in our isolated test registry
		// are eligible for cleanup; never signal an unverified/reused PID.
		registry := storage.NewSessionStore(root, process.Running)
		if records, err := registry.Active(context.Background()); err == nil {
			for _, record := range records {
				if alive, _ := process.Running(record.Process); alive {
					p, _ := os.FindProcess(record.Process.PID)
					p.Kill()
				}
			}
		}
	})
	waitFor(t, func() bool { _, err := os.Stat(filepath.Join(root, "owner.ready")); return err == nil })
	return root, cmd
}

func TestOSLockExcludesAnotherLauncher(t *testing.T) {
	root, owner := owner(t, "hold-lock")
	data, err := os.ReadFile(filepath.Join(root, "owner.ready"))
	if err != nil {
		t.Fatal(err)
	}
	registry := storage.NewSessionStore(root, process.Running)
	if _, err := registry.Reserve(t.Context(), string(data)); !errors.Is(err, instancelock.ErrBusy) {
		t.Fatalf("second launcher bypassed OS lock: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "owner.release"), []byte("release"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := owner.Wait(); err != nil {
		t.Fatal(err)
	}
	lease, err := registry.Reserve(t.Context(), string(data))
	if err != nil {
		t.Fatal(err)
	}
	lease.Close()
}

func TestOwnerCrashBeforeHandoffCannotLaunchJava(t *testing.T) {
	root, owner := owner(t, "before-handoff")
	if err := owner.Wait(); err != nil {
		t.Fatal(err)
	}
	id, _ := os.ReadFile(filepath.Join(root, "owner.ready"))
	registry := storage.NewSessionStore(root, process.Running)
	waitFor(t, func() bool { active, err := registry.Active(t.Context()); return err == nil && len(active) == 0 })
	lease, err := registry.Reserve(t.Context(), string(id))
	if err != nil {
		t.Fatal(err)
	}
	lease.Close()
	if _, err := os.Stat(filepath.Join(root, "release.finished")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("unregistered target ran")
	}
	if _, err := os.Stat(filepath.Join(root, "instances", string(id), "session.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("dead gated record was not reclaimed")
	}
}

func TestOwnerExitPreservesLiveSessionUntilItsNaturalExit(t *testing.T) {
	root, owner := owner(t, "after-handoff")
	if err := owner.Wait(); err != nil {
		t.Fatal(err)
	}
	registry := storage.NewSessionStore(root, process.Running)
	active, err := registry.Active(t.Context())
	if err != nil || len(active) != 1 {
		t.Fatalf("live restarted session missing: %+v %v", active, err)
	}
	record := active[0]
	t.Cleanup(func() {
		if alive, _ := process.Running(record.Process); alive {
			p, _ := os.FindProcess(record.Process.PID)
			p.Kill()
		}
	})
	if _, err := registry.Reserve(t.Context(), record.InstanceID); err == nil {
		t.Fatal("reopened launcher modified a live game")
	}
	if err := os.WriteFile(filepath.Join(root, "release"), []byte("release"), 0o600); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { alive, err := process.Running(record.Process); return err == nil && !alive })
	lease, err := registry.Reserve(t.Context(), record.InstanceID)
	if err != nil {
		t.Fatal(err)
	}
	lease.Close()
	if _, err := os.Stat(filepath.Join(root, "instances", record.InstanceID, "session.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("finished session record retained")
	}
}
