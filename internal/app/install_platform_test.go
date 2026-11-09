package app

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"mcl/internal/core/meta"
)

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
