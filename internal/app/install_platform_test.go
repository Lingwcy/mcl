package app

import (
	"context"
	"crypto/sha1"
	"encoding/json"
	"errors"
	"fmt"
	"mcl/internal/core/instance"
	"os"
	"strings"
	"testing"

	"mcl/internal/core/meta"
)

func TestOfficial1122ImportRuntimeSelectsConsistentMacAMD64(t *testing.T) {
	raw, err := os.ReadFile("../core/meta/testdata/1.12.2.json")
	if err != nil {
		t.Fatal(err)
	}
	document, err := os.ReadFile("../core/meta/testdata/1.12.2-source.json")
	if err != nil {
		t.Fatal(err)
	}
	var entry meta.ManifestEntry
	if err := json.Unmarshal(document, &entry); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprintf("%x", sha1.Sum(raw)) != entry.SHA1 {
		t.Fatal("fixture digest mismatch")
	}
	catalog := meta.NewClient(t.TempDir())
	catalog.VerifiedFetch = func(ctx context.Context, address, digest string, limit int64) ([]byte, error) {
		if address != entry.URL || digest != entry.SHA1 {
			return nil, errors.New("unexpected metadata request")
		}
		return raw, nil
	}
	version, err := catalog.Version(t.Context(), entry)
	if err != nil {
		t.Fatal(err)
	}
	checked := false
	service := &InstallationService{platform: meta.Platform{OS: "osx", Arch: "arm64"}, CheckExecutionPlatform: func(ctx context.Context, host, target meta.Platform) error {
		checked = true
		if host.Arch != "arm64" || target.OS != "osx" || target.Arch != "amd64" {
			t.Fatal(host, target)
		}
		return nil
	}}
	resolved, err := service.resolveLoader(t.Context(), instance.Instance{ID: strings.Repeat("a", 32)}, version, "", nil)
	if err != nil || !checked || service.resolvedPlatform(resolved).Arch != "amd64" {
		t.Fatal(resolved.platform, checked, err)
	}
	// The same selector feeds automatic pack runtime installation and launch.
	// No Java, transfer, pack import, user instance or game is executed here.
}

func TestTargetPlatformRequiresExplicitTranslationAdapter(t *testing.T) {
	var version meta.Version
	if err := json.Unmarshal([]byte(`{"id":"legacy","type":"release","mainClass":"example.Main","javaVersion":{"majorVersion":8},"arguments":{"game":["game-argument"],"jvm":["jvm-argument"]},"downloads":{"client":{"url":"https://piston-data.mojang.com/client.jar","sha1":"0000000000000000000000000000000000000000","size":1}},"assetIndex":{"id":"legacy","url":"https://piston-meta.mojang.com/index.json","sha1":"0000000000000000000000000000000000000000","size":1},"libraries":[{"name":"org.lwjgl.lwjgl:lwjgl-platform:2.9.1","natives":{"osx":"natives-osx"},"downloads":{"classifiers":{"natives-osx":{"url":"https://libraries.minecraft.net/native.jar","size":1,"sha1":"0000000000000000000000000000000000000000"}}}}]}`), &version); err != nil {
		t.Fatal(err)
	}
	service := &InstallationService{platform: meta.Platform{OS: "osx", Arch: "arm64", Version: "15.0"}}
	if _, err := service.targetPlatform(t.Context(), version); err == nil {
		t.Fatal("translation accepted without OS adapter")
	}
	missing := errors.New("synthetic missing Rosetta")
	service.CheckExecutionPlatform = func(ctx context.Context, host, target meta.Platform) error {
		if host.Arch != "arm64" || target.Arch != "amd64" {
			t.Fatal("mixed target")
		}
		return missing
	}
	if _, err := service.targetPlatform(t.Context(), version); !errors.Is(err, missing) {
		t.Fatal(err)
	}
	service.CheckExecutionPlatform = func(context.Context, meta.Platform, meta.Platform) error { return nil }
	target, err := service.targetPlatform(t.Context(), version)
	if err != nil || target.Arch != "amd64" {
		t.Fatal(target, err)
	}
}
