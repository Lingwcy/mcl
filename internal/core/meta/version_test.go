package meta

import (
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"
)

func TestOfficialModernVersions(t *testing.T) {
	t.Parallel()
	for id, major := range map[string]int{"1.20.4": 17, "1.21.1": 21} {
		t.Run(id, func(t *testing.T) {
			version, err := ParseVersion(fixture(t, id+".json"))
			if err != nil {
				t.Fatal(err)
			}
			if err := version.ValidateVanilla(); err != nil {
				t.Fatal(err)
			}
			if version.Java.Major != major || version.MainClass != "net.minecraft.client.main.Main" {
				t.Fatalf("unexpected runtime requirements: %+v", version.Java)
			}
			platform := Platform{OS: "osx", Arch: "arm64"}
			jvm, err := SelectArguments(version.Arguments.JVM, platform)
			if err != nil || !slices.Contains(jvm, "-XstartOnFirstThread") || slices.Contains(jvm, "-Xss1M") {
				t.Fatalf("incorrect JVM selection: %v, %v", jvm, err)
			}
			game, err := SelectArguments(version.Arguments.Game, platform)
			if err != nil || slices.Contains(game, "--demo") || slices.Contains(game, "--width") {
				t.Fatalf("optional features leaked into defaults: %v, %v", game, err)
			}
			platform.Features = map[string]bool{"has_custom_resolution": true, "is_demo_user": true}
			game, err = SelectArguments(version.Arguments.Game, platform)
			if err != nil || !slices.Contains(game, "--demo") || !slices.Contains(game, "${resolution_width}") {
				t.Fatalf("conditional string/array selection failed: %v, %v", game, err)
			}
		})
	}
}

func TestRulesFailClosedAndRespectOrder(t *testing.T) {
	t.Parallel()
	var rules []Rule
	if err := json.Unmarshal([]byte(`[{"action":"allow"},{"action":"disallow","os":{"name":"osx"}},{"action":"allow","os":{"name":"osx","arch":"aarch64"},"features":{"custom":true}}]`), &rules); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		platform Platform
		want     bool
	}{
		{Platform{OS: "linux", Arch: "amd64"}, true},
		{Platform{OS: "osx", Arch: "arm64"}, false},
		{Platform{OS: "osx", Arch: "arm64", Features: map[string]bool{"custom": true}}, true},
	} {
		got, err := Allowed(rules, test.platform)
		if err != nil || got != test.want {
			t.Fatalf("rules(%+v)=%v, %v; want %v", test.platform, got, err, test.want)
		}
	}
	for _, document := range []string{
		`{"action":"unknown"}`, `{"action":"allow","future_condition":true}`,
		`{"action":"allow","os":{"future":true}}`, `{"action":"allow","os":{"version":"["}}`,
	} {
		if err := json.Unmarshal([]byte(document), new(Rule)); err == nil {
			t.Fatalf("unsupported condition was accepted: %s", document)
		}
	}
	if err := json.Unmarshal([]byte(`[{"action":"disallow","os":{"version":"^10\\."}}]`), &rules); err != nil {
		t.Fatal(err)
	}
	if _, err := Allowed(rules, Platform{OS: "windows"}); err == nil {
		t.Fatal("missing OS version cannot silently change rule meaning")
	}
	for _, document := range []string{`null`, `123`, `{}`, `{"value":null}`, `{"value":[]}`, `{"value":[1]}`, `{"value":"x","future":true}`} {
		if err := json.Unmarshal([]byte(document), new(Argument)); err == nil {
			t.Fatalf("invalid argument was accepted: %s", document)
		}
	}
}

func TestMavenAndPortablePaths(t *testing.T) {
	t.Parallel()
	for coordinate, want := range map[string]string{
		"org.lwjgl:lwjgl:3.3.3:natives-macos-arm64": "org/lwjgl/lwjgl/3.3.3/lwjgl-3.3.3-natives-macos-arm64.jar",
		"org.example:resources:1.0@zip":             "org/example/resources/1.0/resources-1.0.zip",
	} {
		parsed, err := ParseCoordinate(coordinate)
		if err != nil || parsed.Path() != want {
			t.Fatalf("Maven path %s: %s, %v", coordinate, parsed.Path(), err)
		}
	}
	for _, coordinate := range []string{"a:b", "a:b:c:d:e", "a..b:c:d", "a:../b:1", "a:b:1@../zip"} {
		if _, err := ParseCoordinate(coordinate); err == nil {
			t.Errorf("accepted unsafe coordinate %q", coordinate)
		}
	}
	for _, path := range []string{"../escape.jar", "/absolute.jar", "a//b", "a/./b", "a/../b", `C:\Windows\file`, "a/NUL.txt", "a/COM1", "a/b.", "a/b\x00"} {
		if ValidPath(path) {
			t.Errorf("accepted unsafe path %q", path)
		}
	}
}

func TestLegacyVersionIsExplicitlyUnsupported(t *testing.T) {
	t.Parallel()
	v, err := ParseVersion([]byte(`{"id":"legacy","type":"release","mainClass":"example.Main","minecraftArguments":"--username ${auth_player_name}"}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := v.ValidateVanilla(); err == nil || !strings.Contains(err.Error(), "旧版") {
		t.Fatalf("expected an explicit legacy-format error, got %v", err)
	}
}

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestModernOfficialSnapshotUsesTheSameValidatedFormat(t *testing.T) {
	version, err := ParseVersion(fixture(t, "1.20.4.json"))
	if err != nil {
		t.Fatal(err)
	}
	version.Type = "snapshot"
	if err := version.ValidateVanilla(); err != nil {
		t.Fatal("modern official snapshot rejected solely by type", err)
	}
	version.Type = "old_alpha"
	if err := version.ValidateVanilla(); err == nil {
		t.Fatal("legacy type entered the modern pipeline")
	}
}
