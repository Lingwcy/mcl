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
	"slices"
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

// Explicit integration only: never discover/install Java or contact a server.
func TestRealJava8ManifestTransport(t *testing.T) {
	executable := os.Getenv("MCL_INTEGRATION_JAVA8")
	if executable == "" {
		t.Skip("set MCL_INTEGRATION_JAVA8 to a stable native Java 8 JDK")
	}
	runtime, err := java.Probe(t.Context(), executable)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.Major != 8 {
		t.Fatal("this protocol fixture requires exactly Java 8")
	}
	root := filepath.Join(t.TempDir(), "中文 Java 空格 #%")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "Java8Probe.java")
	code := `import java.nio.file.*; public class Java8Probe { public static void main(String[] args) throws Exception { System.out.println("JAVA8_READY"); for (String arg : args) System.out.println(arg); while (!Files.exists(Paths.get(System.getProperty("mcl.probe.release")))) Thread.sleep(10); } }`
	if err := os.WriteFile(source, []byte(code), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	compiler := exec.CommandContext(ctx, filepath.Join(filepath.Dir(runtime.Executable), "javac"+filepath.Ext(runtime.Executable)), "-encoding", "UTF-8", "-d", root, source)
	compiler.Env = java.Environment(os.Environ())
	if output, err := compiler.CombinedOutput(); err != nil {
		t.Fatalf("compile Java 8 fixture: %v: %s", err, output)
	}
	class, err := os.ReadFile(filepath.Join(root, "Java8Probe.class"))
	if err != nil {
		t.Fatal(err)
	}
	var bundle bytes.Buffer
	archive := zip.NewWriter(&bundle)
	entry, err := archive.Create("Java8Probe.class")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write(class); err != nil {
		t.Fatal(err)
	}
	if err := archive.Close(); err != nil {
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
	version.ID, version.MainClass, version.Java.Major = "java8-probe", "Java8Probe", 8
	version.Libraries, version.Logging.Client = nil, nil
	identity := meta.Download{URL: "https://example.invalid/java8-probe.jar", SHA1: fmt.Sprintf("%x", sha1.Sum(bundle.Bytes())), Size: int64(bundle.Len())}
	version.Downloads["client"] = identity
	publish := func(relative string) {
		name := filepath.Join(root, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(name), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(name, bundle.Bytes(), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	publish("cache/versions/java8-probe/java8-probe.jar")
	// Distinct fixture paths make the uncompressed classpath exceed Windows'
	// command limit. The generated files are private synthetic test artifacts.
	for i := range 500 {
		library := meta.Library{Name: fmt.Sprintf("test:probe-%d:1.0", i)}
		artifact := identity
		artifact.Path = fmt.Sprintf("test/probe-%d/1.0/probe-%d.jar", i, i)
		library.Downloads.Artifact = &artifact
		version.Libraries = append(version.Libraries, library)
		publish("cache/libraries/" + artifact.Path)
	}
	id := instance.NewID()
	native := "instances/" + id + "/natives/ready-" + strings.Repeat("A", 26)
	for _, relative := range []string{native, "instances/" + id + "/game"} {
		if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(relative)), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	release := filepath.Join(root, "release")
	version.Arguments.JVM = append(version.Arguments.JVM, meta.Argument{Values: []string{"-Dmcl.probe.release=" + release}})
	login, err := launch.NewSession("Player", strings.Repeat("a", 32), syntheticToken, "test-client", "1234", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	plan, err := launch.Build(launch.Options{Root: root, InstanceID: id, Version: version, Platform: meta.CurrentPlatform(), Java: runtime, NativeDirectory: native, MaxMemoryMiB: 512}, login)
	if err != nil {
		t.Fatal(err)
	}
	args := plan.Preview().Arguments
	if index := slices.Index(args, "-cp"); index < 0 || len(args[index+1]) <= 32767 {
		t.Fatal("fixture classpath too short")
	}
	registry := storage.NewSessionStore(root, process.Running)
	lease, err := registry.Reserve(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	var registered launch.ProcessIdentity
	game, err := process.StartTracked(plan, func(identity launch.ProcessIdentity) error {
		registered = identity
		return lease.Track(t.Context(), launch.Record{ID: instance.NewID(), InstanceID: id, VersionID: version.ID, VersionSHA1: strings.Repeat("a", 40), NativeDirectory: native, Process: identity, CreatedAt: time.Now().UTC()})
	})
	if err != nil {
		t.Fatal(err)
	}
	stopCleanup(t, game)
	waitFor(t, func() bool {
		for _, line := range game.Output().Lines {
			if strings.Contains(line.Text, "JAVA8_READY") {
				return true
			}
		}
		return false
	})
	current, err := process.Inspect(game.PID())
	if err != nil || current != registered {
		t.Fatal("identity changed across Java 8 exec")
	}
	if err := lease.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Reserve(t.Context(), id); err == nil {
		t.Fatal("live Java 8 did not protect its instance")
	}
	if err := os.WriteFile(release, []byte("release"), 0o600); err != nil {
		t.Fatal(err)
	}
	if result := game.Wait(); result.Code != 0 {
		t.Fatalf("Java 8 exit %+v", result)
	}
	var output strings.Builder
	for _, line := range game.Output().Lines {
		output.WriteString(line.Text)
	}
	if strings.Contains(output.String(), syntheticToken) || !strings.Contains(output.String(), "[REDACTED]") {
		t.Fatal("Java 8 output privacy/redaction")
	}
	reopened, err := registry.Reserve(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	reopened.Close()
}
