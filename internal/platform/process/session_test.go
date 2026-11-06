package process_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"mcl/internal/core/instance"
	"mcl/internal/core/java"
	"mcl/internal/core/launch"
	"mcl/internal/core/meta"
	"mcl/internal/platform/process"
)

const syntheticToken = "synthetic-process-token-1234567890"

func TestMain(m *testing.M) {
	if handled, code := process.GateMain(os.Args[1:]); handled {
		os.Exit(code)
	}
	mode := os.Getenv("MCL_TEST_PROCESS_MODE")
	if mode == "" {
		os.Exit(m.Run())
	}
	switch mode {
	case "small":
		fmt.Fprintln(os.Stdout, "stdout "+syntheticToken)
		fmt.Fprintln(os.Stderr, "stderr "+syntheticToken)
		os.Exit(0)
	case "marker":
		os.WriteFile(os.Getenv("MCL_TEST_EXECUTED_FILE"), []byte("executed"), 0o600)
		fmt.Fprintln(os.Stdout, "executed")
		os.Exit(0)
	case "output":
		fmt.Fprintln(os.Stdout, "args", os.Args)
		fmt.Fprintln(os.Stderr, "stderr ready")
		fmt.Fprintln(os.Stdout, strings.Repeat("x", 1<<20))
		fmt.Fprintln(os.Stderr, strings.Repeat("y", 1<<20))
		for i := range 12000 {
			fmt.Fprintln(os.Stdout, "line", i)
		}
		fmt.Fprint(os.Stdout, syntheticToken[:15])
		time.Sleep(time.Millisecond)
		fmt.Fprintln(os.Stdout, syntheticToken[15:])
		fmt.Fprintln(os.Stderr, "final stderr")
		os.Exit(7)
	case "tree":
		signal.Ignore(syscall.SIGTERM)
		child := exec.Command(os.Args[0])
		child.Env = append(os.Environ(), "MCL_TEST_PROCESS_MODE=descendant")
		child.Stdout, child.Stderr = os.Stdout, os.Stderr
		if err := child.Start(); err != nil {
			os.Exit(8)
		}
		go child.Wait()
		for {
			time.Sleep(time.Hour)
		}
	case "descendant":
		signal.Reset(syscall.SIGTERM)
		os.WriteFile(os.Getenv("MCL_TEST_CHILD_PID_FILE"), []byte(fmt.Sprint(os.Getpid())), 0o600)
		for {
			time.Sleep(time.Hour)
		}
	case "inherit":
		child := exec.Command(os.Args[0])
		child.Env = append(os.Environ(), "MCL_TEST_PROCESS_MODE=hold")
		child.Stdout, child.Stderr = os.Stdout, os.Stderr
		if err := child.Start(); err != nil {
			os.Exit(8)
		}
		os.Exit(0)
	case "detachable", "hold":
		if mode == "hold" {
			os.WriteFile(os.Getenv("MCL_TEST_CHILD_PID_FILE"), []byte(fmt.Sprint(os.Getpid())), 0o600)
		}
		fmt.Fprintln(os.Stdout, "ready")
		for {
			if _, err := os.Stat(os.Getenv("MCL_TEST_RELEASE")); err == nil {
				os.WriteFile(os.Getenv("MCL_TEST_RELEASE")+".finished", []byte("finished"), 0o600)
				os.Exit(0)
			}
			time.Sleep(5 * time.Millisecond)
		}
	}
	os.Exit(9)
}

func launchPlan(t *testing.T, selected ...string) launch.Plan {
	t.Helper()
	data, err := os.ReadFile("../../core/meta/testdata/1.20.4.json")
	if err != nil {
		t.Fatal(err)
	}
	v, err := meta.ParseVersion(data)
	if err != nil {
		t.Fatal(err)
	}
	var root string
	if len(selected) > 1 {
		root = selected[1]
	} else {
		root = filepath.Join(t.TempDir(), "中文 游戏 空格")
	}
	id := instance.NewID()
	native := filepath.Join("instances", id, "natives", "ready-"+strings.Repeat("A", 26))
	for _, dir := range []string{native, filepath.Join("instances", id, "game")} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if len(selected) > 0 {
		executable = selected[0]
	}
	runtime := java.Runtime{Executable: executable, Version: "17.0.1", Major: 17, Arch: meta.CurrentPlatform().Arch}
	session, err := launch.NewSession("Player", strings.Repeat("a", 32), syntheticToken, "test-client", "12345", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	plan, err := launch.Build(launch.Options{Root: root, InstanceID: id, Version: v, Platform: meta.CurrentPlatform(), Java: runtime, NativeDirectory: filepath.ToSlash(native), MaxMemoryMiB: 1024}, session)
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func TestStartFailureCleansRuntimeFile(t *testing.T) {
	p := launchPlan(t, filepath.Join(t.TempDir(), "missing-java"))
	if _, err := process.Start(p); err == nil {
		t.Fatal("missing executable launched")
	}
	entries, err := os.ReadDir(filepath.Join(p.DataRoot(), p.InstancePath(), "runtime"))
	if err != nil || len(entries) != 0 {
		t.Fatalf("failed launch retained JVM arguments: %v %v", entries, err)
	}
}

func waitFor(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatal("condition timed out")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func stopCleanup(t *testing.T, s *process.Session) {
	t.Helper()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		s.Stop(ctx)
	})
}

func TestProcessOutputExitAndArgumentCleanup(t *testing.T) {
	t.Setenv("MCL_TEST_PROCESS_MODE", "output")
	p := launchPlan(t)
	s, err := process.Start(p)
	if err != nil {
		t.Fatal(err)
	}
	stopCleanup(t, s)
	done := make(chan process.Exit, 1)
	go func() { done <- s.Wait() }()
	select {
	case result := <-done:
		if result.Code != 7 || result.Error == "" || result.Stopped || result.OutputIncomplete {
			t.Fatalf("exit %+v", result)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("long output blocked the child")
	}
	output := s.Output()
	if output.Dropped == 0 || len(output.Lines) > launch.MaxLogLines {
		t.Fatal("output not bounded")
	}
	encoded, _ := json.Marshal(output)
	if strings.Contains(string(encoded), syntheticToken) || !strings.Contains(string(encoded), "[REDACTED]") {
		t.Fatalf("unsafe/missing log tail: %s", encoded)
	}
	entries, err := os.ReadDir(filepath.Join(p.DataRoot(), p.InstancePath(), "runtime"))
	if err != nil || len(entries) != 0 {
		t.Fatalf("argument file retained: %v %v", entries, err)
	}
	if again := s.Wait(); again.Code != 7 {
		t.Fatal("Wait is not repeatable")
	}
}

func TestBothStreamsAreDrainedAndRedacted(t *testing.T) {
	t.Setenv("MCL_TEST_PROCESS_MODE", "small")
	s, err := process.Start(launchPlan(t))
	if err != nil {
		t.Fatal(err)
	}
	stopCleanup(t, s)
	if result := s.Wait(); result.Code != 0 || result.OutputIncomplete {
		t.Fatalf("exit %+v", result)
	}
	lines := s.Output().Lines
	if len(lines) != 2 {
		t.Fatalf("missing stream output: %+v", lines)
	}
	seen := make(map[string]bool)
	for _, line := range lines {
		if line.Text != line.Stream+" [REDACTED]" {
			t.Fatalf("unsafe line %+v", line)
		}
		seen[line.Stream] = true
	}
	if !seen["stdout"] || !seen["stderr"] {
		t.Fatal("stream identity lost")
	}
}

func TestDetachDoesNotTerminateSession(t *testing.T) {
	t.Setenv("MCL_TEST_PROCESS_MODE", "detachable")
	release := filepath.Join(t.TempDir(), "release")
	t.Setenv("MCL_TEST_RELEASE", release)
	p := launchPlan(t)
	s, err := process.Start(p)
	if err != nil {
		t.Fatal(err)
	}
	stopCleanup(t, s)
	waitFor(t, func() bool { return len(s.Output().Lines) > 0 })
	entries, err := os.ReadDir(filepath.Join(p.DataRoot(), p.InstancePath(), "runtime"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("runtime file: %v %v", entries, err)
	}
	argfile := filepath.Join(p.DataRoot(), p.InstancePath(), "runtime", entries[0].Name())
	data, err := os.ReadFile(argfile)
	if err != nil || strings.Contains(string(data), syntheticToken) {
		t.Fatal("credential in JVM argument file")
	}
	if info, _ := os.Stat(argfile); info.Mode().Perm()&0o077 != 0 {
		t.Fatal("argument file is not private")
	}
	s.Detach()
	select {
	case <-s.Done():
		t.Fatal("detach killed the game")
	default:
	}
	if err := os.WriteFile(release, []byte("release"), 0o600); err != nil {
		t.Fatal(err)
	}
	result := s.Wait()
	if result.Code != 0 || !result.Detached || result.Stopped {
		t.Fatalf("detach result %+v", result)
	}
}

func TestInheritedPipeCannotHangWait(t *testing.T) {
	t.Setenv("MCL_TEST_PROCESS_MODE", "inherit")
	release := filepath.Join(t.TempDir(), "release")
	t.Setenv("MCL_TEST_RELEASE", release)
	t.Setenv("MCL_TEST_CHILD_PID_FILE", filepath.Join(t.TempDir(), "child.pid"))
	s, err := process.Start(launchPlan(t))
	if err != nil {
		t.Fatal(err)
	}
	stopCleanup(t, s)
	defer func() {
		os.WriteFile(release, []byte("release"), 0o600)
		waitFor(t, func() bool { _, err := os.Stat(release + ".finished"); return err == nil })
	}()
	done := make(chan process.Exit, 1)
	go func() { done <- s.Wait() }()
	select {
	case result := <-done:
		if result.Code != 0 || !result.OutputIncomplete {
			t.Fatalf("inherited pipe result %+v", result)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("inherited output pipe blocked forever")
	}
}

func TestInvalidPlanAndDirectoryEscapeNeverStart(t *testing.T) {
	if _, err := process.Start(launch.Plan{}); err == nil {
		t.Fatal("zero launch plan accepted")
	}
	p := launchPlan(t)
	if err := os.Remove(p.Directory()); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), p.Directory()); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := process.Start(p); err == nil {
		t.Fatal("foreign game directory accepted")
	}
}
