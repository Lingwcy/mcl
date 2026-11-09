package launch

import (
	"context"
	"crypto/sha1"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"mcl/internal/core/install"
	"mcl/internal/core/meta"
)

// The synthetic historical document is served through the injected verified
// fetcher, not a network request. Nothing here probes or starts a real JVM.
func legacyOptions(t *testing.T, template string) Options {
	t.Helper()
	o := options(t, "1.20.4")
	var raw map[string]any
	encoded, err := json.Marshal(o.Version)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(encoded, &raw); err != nil {
		t.Fatal(err)
	}
	raw["id"] = "1.12.2"
	raw["libraries"] = []any{}
	raw["jar"] = ""
	raw["patches"] = []any{}
	delete(raw, "arguments")
	delete(raw, "javaVersion")
	raw["minecraftArguments"] = template
	encoded, err = json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	sum := fmt.Sprintf("%x", sha1.Sum(encoded))
	entry := meta.ManifestEntry{ID: "1.12.2", Type: "release", SHA1: sum, URL: "https://piston-meta.mojang.com/v1/packages/" + sum + "/1.12.2.json"}
	client := meta.NewClient(t.TempDir())
	client.VerifiedFetch = func(ctx context.Context, address, digest string, limit int64) ([]byte, error) {
		if address != entry.URL || digest != entry.SHA1 {
			return nil, errors.New("unexpected document")
		}
		return slices.Clone(encoded), ctx.Err()
	}
	o.Version, err = client.Version(t.Context(), entry)
	if err != nil {
		t.Fatal(err)
	}
	o.Platform = meta.CurrentPlatform()
	o.Java.Arch, o.Java.Major, o.Java.Version = o.Platform.Arch, 8, "1.8.0_442"
	return o
}

func TestLegacyParametersReachPrivateJava8Transport(t *testing.T) {
	template := "--username ${auth_player_name} --gameDir ${game_directory} --session ${auth_session} --userProperties ${user_properties} --version ${version_name} --literal 'two words'"
	o := legacyOptions(t, template)
	login := session(t)
	login.Name = "中文 Name 'quoted'"
	plan, err := Build(o, login)
	if err != nil {
		t.Fatal(err)
	}
	args := plan.ProcessArguments(filepath.Join(o.Root, "runtime", "support.jar"))
	if !slices.Contains(args, login.Name) || !slices.Contains(args, "token:"+login.token+":"+login.UUID) || !slices.Contains(args, "two words") || !slices.Contains(args, "{}") {
		t.Fatal("legacy token boundaries changed")
	}
	if !slices.Contains(args, filepath.Join(o.Root, "instances", o.InstanceID, "game")) {
		t.Fatal("legacy game directory not substituted")
	}
	ext, data, err := plan.SupportFile()
	if err != nil || ext != ".jar" || strings.Contains(string(data), login.token) {
		t.Fatalf("legacy support privacy: %v", err)
	}
	preview, _ := json.Marshal(plan.Preview())
	if strings.Contains(string(preview), login.token) {
		t.Fatal("auth_session leaked through preview")
	}
	offline, err := NewOfflineSession(login.Name, login.UUID)
	if err != nil {
		t.Fatal(err)
	}
	plan, err = Build(o, offline)
	if err != nil {
		t.Fatal(err)
	}
	args = plan.ProcessArguments("support.jar")
	if index := slices.Index(args, "--session"); index < 0 || args[index+1] != "0" {
		t.Fatal("offline legacy session not normalized")
	}
}

func TestLegacyUnsupportedAssetsAndPlaceholdersFailClearly(t *testing.T) {
	for _, placeholder := range []string{"game_assets", "unknown_legacy_key"} {
		o := legacyOptions(t, "--username ${auth_player_name} --assets ${"+placeholder+"}")
		if _, err := Build(o, session(t)); err == nil || placeholder == "game_assets" && !strings.Contains(err.Error(), "历史") {
			t.Fatalf("unsupported placeholder did not fail: %v", err)
		}
	}
}

func TestInheritedClientJarIdentityReachesInstallAndLaunch(t *testing.T) {
	o := options(t, "1.20.4")
	raw, err := os.ReadFile("../meta/testdata/1.20.4.json")
	if err != nil {
		t.Fatal(err)
	}
	child, err := json.Marshal(map[string]any{"id": "inherited-probe", "type": "release", "inheritsFrom": "1.20.4", "mainClass": "example.InheritedMain", "arguments": map[string]any{"game": []string{"--child", "yes"}}})
	if err != nil {
		t.Fatal(err)
	}
	bodies := map[string][]byte{"1.20.4": raw, "inherited-probe": child}
	entries := make(map[string]meta.ManifestEntry)
	manifest := meta.Manifest{}
	for id, body := range bodies {
		sum := fmt.Sprintf("%x", sha1.Sum(body))
		entry := meta.ManifestEntry{ID: id, Type: "release", SHA1: sum, URL: "https://piston-meta.mojang.com/v1/packages/" + sum + "/" + id + ".json"}
		entries[id] = entry
		manifest.Versions = append(manifest.Versions, entry)
	}
	client := meta.NewClient(t.TempDir())
	client.VerifiedFetch = func(ctx context.Context, address, digest string, limit int64) ([]byte, error) {
		for id, entry := range entries {
			if address == entry.URL && digest == entry.SHA1 {
				return slices.Clone(bodies[id]), ctx.Err()
			}
		}
		return nil, errors.New("unexpected document")
	}
	encoded, _ := json.Marshal(manifest)
	if err := os.WriteFile(filepath.Join(client.CacheDir, "manifest.json"), encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	o.Version, err = client.Version(t.Context(), entries["inherited-probe"])
	if err != nil {
		t.Fatal(err)
	}
	installation, err := install.Build(o.Version, o.Platform)
	if err != nil {
		t.Fatal(err)
	}
	if installation.ClientPath != "cache/versions/1.20.4/1.20.4.jar" {
		t.Fatal("client jar lost parent identity")
	}
	plan, err := Build(o, session(t))
	if err != nil {
		t.Fatal(err)
	}
	args := plan.Preview().Arguments
	cp := args[slices.Index(args, "-cp")+1]
	if !strings.HasSuffix(cp, filepath.Join("1.20.4", "1.20.4.jar")) || !slices.Contains(args, "example.InheritedMain") || !slices.Contains(args, "--child") {
		t.Fatal("inherited launch identity/arguments mismatch")
	}
}

func TestLegacyLog4jConfigurationRetainsVerifiedPath(t *testing.T) {
	o := legacyOptions(t, "--username ${auth_player_name}")
	if o.Version.Logging.Client == nil {
		t.Fatal("fixture has no logging")
	}
	logging := *o.Version.Logging.Client
	logging.Argument = "-Dlog4j.configuration=${path}"
	o.Version.Logging.Client = &logging
	plan, err := Build(o, session(t))
	if err != nil {
		t.Fatal(err)
	}
	ext, data, err := plan.SupportFile()
	if err != nil || ext != ".jar" {
		t.Fatal("wrong Java8 support transport", err)
	}
	_ = data
	args := plan.ProcessArguments("support.jar")
	expected := "-Dlog4j.configuration=" + filepath.Join(o.Root, "cache", "assets", "log_configs", logging.File.ID)
	if !slices.Contains(args, expected) {
		t.Fatal("legacy logging did not use verified file path")
	}
}

func TestHistoricalGameAssetsUsesCurrentInstanceGeneration(t *testing.T) {
	o := legacyOptions(t, "--username ${auth_player_name} --assets ${game_assets}")
	o.LegacyAssetsDirectory = "instances/" + o.InstanceID + "/legacy-assets/ready-" + strings.Repeat("A", 26)
	plan, err := Build(o, session(t))
	if err != nil {
		t.Fatal(err)
	}
	args := plan.ProcessArguments("support.jar")
	expected := filepath.Join(o.Root, filepath.FromSlash(o.LegacyAssetsDirectory), "files")
	if !slices.Contains(args, expected) {
		t.Fatal("game_assets did not use the prepared instance generation")
	}
	o.LegacyAssetsDirectory = "instances/foreign/legacy-assets/ready-" + strings.Repeat("A", 26)
	if _, err := Build(o, session(t)); err == nil {
		t.Fatal("foreign asset generation accepted")
	}
}
