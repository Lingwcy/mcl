//go:build integration

package launch

import (
	"bytes"
	"context"
	"encoding/base64"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"mcl/internal/core/java"
)

// Explicit real-JVM protocol validation; the ordinary test suite stays offline
// and does not depend on a preinstalled JDK. No game or real account is used.
func TestJavaArgumentFileRoundTrip(t *testing.T) {
	executable := os.Getenv("MCL_INTEGRATION_JAVA")
	if executable == "" {
		t.Skip("set MCL_INTEGRATION_JAVA to a local stable Java 17+ executable")
	}
	t.Setenv("JAVA_TOOL_OPTIONS", "-javaagent:inherited-agent-must-not-run.jar")
	runtime, err := java.Probe(t.Context(), executable)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.Major < 17 {
		t.Fatal("test requires Java 17+")
	}
	dir := filepath.Join(t.TempDir(), "中文 空格 # 引号'")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(dir, "ArgumentProbe.java")
	code := `import java.nio.charset.StandardCharsets; import java.util.Base64;
class ArgumentProbe { public static void main(String[] args) {
for (String arg : args) System.out.println(Base64.getEncoder().encodeToString(arg.getBytes(StandardCharsets.UTF_8)));
} }`
	if err := os.WriteFile(source, []byte(code), 0o600); err != nil {
		t.Fatal(err)
	}
	want := []string{"", `C:\Program Files\Java\`, "中文 空格", `quote" and ' # hash`, "a\nb\tc\r\f", "@literal", "@@literal"}
	for range 32 {
		want = append(want, strings.Repeat("classpath 中文 segment ", 256))
	}
	args := append([]string{"-Dfile.encoding=UTF-8", source}, want...)
	data, err := encodeArgumentFile(args)
	if err != nil {
		t.Fatal(err)
	}
	argfile := filepath.Join(dir, "arguments.args")
	if err := os.WriteFile(argfile, data, 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, runtime.Executable, "@"+argfile)
	cmd.Env = java.Environment(os.Environ())
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("Java argument probe: %v: %s", err, stderr.String())
	}
	var got []string
	for _, line := range strings.Split(strings.TrimSuffix(stdout.String(), "\n"), "\n") {
		decoded, err := base64.StdEncoding.DecodeString(strings.TrimSuffix(line, "\r"))
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, string(decoded))
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatal("JVM changed argument boundaries, escaping or Unicode")
	}
	t.Logf("real %s %s: %d arguments, %d-byte argument file, exact Unicode/escaping round trip", runtime.Vendor, runtime.Version, len(want), len(data))
}
