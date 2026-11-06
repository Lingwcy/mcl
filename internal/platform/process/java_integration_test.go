//go:build integration

package process_test

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha1"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mcl/internal/core/instance"
	"mcl/internal/core/java"
	"mcl/internal/core/launch"
	"mcl/internal/core/meta"
	"mcl/internal/platform/process"
	"mcl/internal/storage"
)

func TestRealJavaSession(t *testing.T) {
	executable := os.Getenv("MCL_INTEGRATION_JAVA")
	if executable == "" {
		t.Skip("set MCL_INTEGRATION_JAVA to a stable local Java 17+ JDK")
	}
	runtime, err := java.Probe(t.Context(), executable)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "中文 Java 空格")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(dir, "LauncherProbe.java")
	code := `import java.nio.file.*;
public class LauncherProbe { public static void main(String[] args) throws Exception {
System.out.println("READY"); for (String arg : args) System.out.println(arg);
System.err.println("STDERR:" + System.getProperty("file.encoding"));
String release = System.getProperty("mcl.detach.release");
if (release != null) {
while (!Files.exists(Path.of(release))) Thread.sleep(10);
System.out.println("write after launcher detaches");
Files.writeString(Path.of(release + ".finished"), "finished");
} } }`
	if err := os.WriteFile(source, []byte(code), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	compiler := exec.CommandContext(ctx, filepath.Join(filepath.Dir(runtime.Executable), "javac"), "-encoding", "UTF-8", "-d", dir, source)
	compiler.Env = java.Environment(os.Environ())
	if output, err := compiler.CombinedOutput(); err != nil {
		t.Fatalf("compile probe: %v: %s", err, output)
	}
	class, err := os.ReadFile(filepath.Join(dir, "LauncherProbe.class"))
	if err != nil {
		t.Fatal(err)
	}
	var archive bytes.Buffer
	z := zip.NewWriter(&archive)
	entry, err := z.Create("LauncherProbe.class")
	if err != nil {
		t.Fatal(err)
	}
	entry.Write(class)
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	metadata, err := os.ReadFile("../../core/meta/testdata/1.20.4.json")
	if err != nil {
		t.Fatal(err)
	}
	version, err := meta.ParseVersion(metadata)
	if err != nil {
		t.Fatal(err)
	}
	version.ID, version.MainClass, version.Libraries, version.Logging.Client = "process-probe", "LauncherProbe", nil, nil
	version.Downloads["client"] = meta.Download{URL: "https://example.invalid/test-probe.jar", Size: int64(archive.Len()), SHA1: fmt.Sprintf("%x", sha1.Sum(archive.Bytes()))}
	id := instance.NewID()
	native := filepath.ToSlash(filepath.Join("instances", id, "natives", "ready-"+strings.Repeat("A", 26)))
	for _, relative := range []string{native, filepath.Join("instances", id, "game"), "cache/versions/process-probe"} {
		if err := os.MkdirAll(filepath.Join(dir, relative), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "cache/versions/process-probe/process-probe.jar"), archive.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	session, err := launch.NewSession("Player", strings.Repeat("a", 32), syntheticToken, "test-client", "1234", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	options := launch.Options{Root: dir, InstanceID: id, Version: version, Platform: meta.CurrentPlatform(), Java: runtime, NativeDirectory: native, MaxMemoryMiB: 512}
	plan, err := launch.Build(options, session)
	if err != nil {
		t.Fatal(err)
	}
	game, err := process.Start(plan)
	if err != nil {
		t.Fatal(err)
	}
	stopCleanup(t, game)
	result := game.Wait()
	if result.Code != 0 || result.OutputIncomplete {
		t.Fatalf("real JVM exit %+v", result)
	}
	var text strings.Builder
	for _, line := range game.Output().Lines {
		text.WriteString(line.Text)
		text.WriteByte('\n')
	}
	if strings.Contains(text.String(), syntheticToken) || !strings.Contains(text.String(), "[REDACTED]") || !strings.Contains(text.String(), "STDERR:UTF-8") || !strings.Contains(text.String(), filepath.Join(dir, "instances", id, "game")) {
		t.Fatalf("real JVM output/arguments: %s", text.String())
	}
	// Exercise Java's behavior after the launcher stops reading its pipes:
	// it must still run and finish its own work after writing to stdout.
	release := filepath.Join(dir, "release")
	options.Version.Arguments.JVM = append(options.Version.Arguments.JVM, meta.Argument{Values: []string{"-Dmcl.detach.release=" + release}})
	plan, err = launch.Build(options, session)
	if err != nil {
		t.Fatal(err)
	}
	registry := storage.NewSessionStore(dir, process.Running)
	lease, err := registry.Reserve(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	var registered launch.ProcessIdentity
	game, err = process.StartTracked(plan, func(identity launch.ProcessIdentity) error {
		registered = identity
		return lease.Track(t.Context(), launch.Record{ID: instance.NewID(), InstanceID: id, VersionID: version.ID, VersionSHA1: strings.Repeat("a", 40), NativeDirectory: native, Process: identity, CreatedAt: time.Now().UTC()})
	})
	if err != nil {
		t.Fatal(err)
	}
	stopCleanup(t, game)
	waitFor(t, func() bool { return len(game.Output().Lines) > 0 })
	current, err := process.Inspect(game.PID())
	if err != nil || current != registered {
		t.Fatalf("identity changed across JVM exec: %+v %+v %v", registered, current, err)
	}
	game.Detach()
	if err := lease.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := storage.NewSessionStore(dir, process.Running)
	if _, err := reopened.Reserve(t.Context(), id); err == nil {
		t.Fatal("reopened registry allowed repair while Java is live")
	}
	select {
	case <-game.Done():
		t.Fatal("detach terminated Java")
	default:
	}
	if err := os.WriteFile(release, []byte("release"), 0o600); err != nil {
		t.Fatal(err)
	}
	result = game.Wait()
	if result.Code != 0 || !result.Detached || result.Stopped {
		t.Fatalf("detached Java result %+v", result)
	}
	if _, err := os.Stat(release + ".finished"); err != nil {
		t.Fatal("detached Java did not finish its own work")
	}
	reclaimed, err := reopened.Reserve(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	reclaimed.Close()
	t.Logf("real %s %s: Unicode/space classpath, JVM argfile, stdout/stderr, token redaction, tracked exec identity, detached/reopened registry protection and natural exit all passed", runtime.Vendor, runtime.Version)
}
