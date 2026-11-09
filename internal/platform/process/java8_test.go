package process_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mcl/internal/core/launch"
	"mcl/internal/platform/process"
)

func TestJava8TrackedHandoffAndSupportCleanup(t *testing.T) {
	for _, failRegistration := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "registration-failure"}[failRegistration], func(t *testing.T) {
			marker := filepath.Join(t.TempDir(), "executed")
			t.Setenv("MCL_TEST_PROCESS_MODE", "marker")
			t.Setenv("MCL_TEST_EXECUTED_FILE", marker)
			p := launchPlanWithMajor(t, 8)
			registered := false
			game, err := process.StartTracked(p, func(launch.ProcessIdentity) error {
				registered = true
				runtime := filepath.Join(p.DataRoot(), p.InstancePath(), "runtime")
				entries, err := os.ReadDir(runtime)
				if err != nil || len(entries) != 1 || !strings.HasSuffix(entries[0].Name(), ".jar") {
					t.Fatalf("Java 8 support file: %v %v", entries, err)
				}
				body, err := os.ReadFile(filepath.Join(runtime, entries[0].Name()))
				if err != nil || strings.Contains(string(body), syntheticToken) {
					t.Fatal("support privacy")
				}
				if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("target ran before registration")
				}
				if failRegistration {
					return errors.New("synthetic registration failure")
				}
				return nil
			})
			if !registered {
				t.Fatal("no identity registration")
			}
			if failRegistration {
				if err == nil {
					t.Fatal("failed registration launched")
				}
				if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("failed registration executed target")
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				stopCleanup(t, game)
				if exit := game.Wait(); exit.Code != 0 {
					t.Fatalf("helper exit %+v", exit)
				}
				if _, err := os.Stat(marker); err != nil {
					t.Fatal("registered target did not execute")
				}
			}
			entries, err := os.ReadDir(filepath.Join(p.DataRoot(), p.InstancePath(), "runtime"))
			if err != nil || len(entries) != 0 {
				t.Fatalf("support not reclaimed: %v %v", entries, err)
			}
		})
	}
}

func TestJava8MissingExecutableCleansSupport(t *testing.T) {
	p := launchPlanWithMajor(t, 8, filepath.Join(t.TempDir(), "missing-java"))
	if _, err := process.Start(p); err == nil {
		t.Fatal("missing executable launched")
	}
	entries, err := os.ReadDir(filepath.Join(p.DataRoot(), p.InstancePath(), "runtime"))
	if err != nil || len(entries) != 0 {
		t.Fatalf("failed launch retained support: %v %v", entries, err)
	}
}
