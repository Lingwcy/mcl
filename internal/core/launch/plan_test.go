package launch

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"mcl/internal/core/instance"
	"mcl/internal/core/java"
	"mcl/internal/core/meta"
)

func options(t *testing.T, id string) Options {
	t.Helper()
	data, err := os.ReadFile("../meta/testdata/" + id + ".json")
	if err != nil {
		t.Fatal(err)
	}
	v, err := meta.ParseVersion(data)
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "中文 空格")
	instanceID := instance.NewID()
	return Options{Root: root, InstanceID: instanceID, Version: v, Platform: meta.Platform{OS: "osx", Arch: "arm64"}, Java: java.Runtime{Executable: filepath.Join(root, "Java Home/bin/java"), Version: "21.0.8", Major: 21, Arch: "arm64"}, NativeDirectory: "instances/" + instanceID + "/natives/ready-" + strings.Repeat("A", 26), MaxMemoryMiB: 4096}
}

func session(t *testing.T) Session {
	t.Helper()
	s, err := NewSession("Player", strings.Repeat("a", 32), "synthetic-session-secret-1234567890", "project-client-id", "1234567890", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestOfficialCommandsPathsRulesAndPrivacy(t *testing.T) {
	for _, id := range []string{"1.20.4", "1.21.1"} {
		t.Run(id, func(t *testing.T) {
			o := options(t, id)
			s := session(t)
			p, err := Build(o, s)
			if err != nil {
				t.Fatal(err)
			}
			preview := p.Preview()
			if preview.Directory != filepath.Join(o.Root, "instances", o.InstanceID, "game") || preview.Executable != o.Java.Executable || !slices.Contains(preview.Arguments, "-XstartOnFirstThread") {
				t.Fatalf("paths/rules %+v", preview)
			}
			if !slices.Contains(preview.Arguments, "-Xmx4096m") || slices.Contains(preview.Arguments, "--demo") || slices.Contains(preview.Arguments, "--width") {
				t.Fatal("unexpected features or memory")
			}
			cp := preview.Arguments[slices.Index(preview.Arguments, "-cp")+1]
			if !strings.Contains(cp, "natives-macos-arm64.jar") || strings.Contains(cp, "natives-macos.jar") || strings.Contains(cp, "natives-windows") || !strings.HasSuffix(cp, filepath.Join(id, id+".jar")) {
				t.Fatalf("wrong classpath %s", cp)
			}
			for _, formatted := range []string{fmt.Sprint(p), fmt.Sprintf("%#v", p), fmt.Sprintf("%+v", p)} {
				if strings.Contains(formatted, s.token) {
					t.Fatal("formatted plan leaked token")
				}
			}
			encoded, _ := json.Marshal(p)
			if strings.Contains(string(encoded), s.token) {
				t.Fatal("JSON plan leaked token")
			}
			encoded, _ = json.Marshal(s)
			if strings.Contains(string(encoded), s.token) {
				t.Fatal("JSON session leaked token")
			}
			if strings.Contains(fmt.Sprintf("%#v", s), s.token) {
				t.Fatal("formatted session leaked token")
			}
			data, err := p.JVMArgumentFile()
			if err != nil || strings.Contains(string(data), s.token) {
				t.Fatalf("JVM file contains session: %v", err)
			}
			args := p.ProcessArguments("/path with spaces/runtime.args")
			if args[0] != "@/path with spaces/runtime.args" || args[1] != o.Version.MainClass || !slices.Contains(args, s.token) {
				t.Fatal("exec argument boundaries changed")
			}
			draft, err := PreviewOptions(o)
			if err != nil || !slices.Contains(draft.Arguments, "<登录后提供>") {
				t.Fatal("unauthenticated preview must retain unresolved identity")
			}
		})
	}
}

func TestPlatformClasspathAndUnsupportedInputs(t *testing.T) {
	o := options(t, "1.21.1")
	o.Platform = meta.Platform{OS: "windows", Arch: "amd64", Version: "10.0"}
	o.Java.Arch = "amd64"
	p, err := Build(o, session(t))
	if err != nil {
		t.Fatal(err)
	}
	args := p.Preview().Arguments
	cp := args[slices.Index(args, "-cp")+1]
	if !strings.Contains(cp, ";") || !strings.Contains(cp, "natives-windows.jar") || slices.Contains(args, "-XstartOnFirstThread") {
		t.Fatal("Windows command selection")
	}
	for _, tc := range []struct {
		name   string
		change func(*Options)
	}{
		{"Java too old", func(o *Options) { o.Java.Major = 17 }},
		{"foreign native directory", func(o *Options) { o.NativeDirectory = "instances/other/natives/ready-" + strings.Repeat("A", 26) }},
		{"bad main", func(o *Options) { o.Version.MainClass = "-jar arbitrary.jar" }},
		{"classpath separator in path", func(o *Options) { o.Root += ";bad" }},
		{"unknown placeholder", func(o *Options) {
			o.Version.Arguments.Game = append(o.Version.Arguments.Game, meta.Argument{Values: []string{"${unknown}"}})
		}},
		{"agent", func(o *Options) {
			o.Version.Arguments.JVM = append(o.Version.Arguments.JVM, meta.Argument{Values: []string{"-javaagent:bad.jar"}})
		}},
		{"session in JVM", func(o *Options) {
			o.Version.Arguments.JVM = append(o.Version.Arguments.JVM, meta.Argument{Values: []string{"-Dsecret=${auth_access_token}"}})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fresh := options(t, "1.21.1")
			fresh.Platform = o.Platform
			fresh.Java.Arch = "amd64"
			tc.change(&fresh)
			if _, err := Build(fresh, session(t)); err == nil {
				t.Fatal("invalid launch accepted")
			}
		})
	}
	s := session(t)
	s.ExpiresAt = time.Now()
	if _, err := Build(o, s); err == nil {
		t.Fatal("expired session accepted")
	}
	if _, err := NewSession("name", strings.Repeat("a", 32), "secret\nvalue", "client", "123", time.Now().Add(time.Hour)); err == nil {
		t.Fatal("control character accepted")
	}
}
