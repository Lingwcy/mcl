package modpack

import (
	"archive/zip"
	"bytes"
	"context"
	"strings"
	"testing"

	"mcl/internal/core/archive"
	"mcl/internal/core/meta"
)

func testArchive(t *testing.T, files map[string]string) *archive.Zip {
	t.Helper()
	var data bytes.Buffer
	writer := zip.NewWriter(&data)
	for name, content := range files {
		file, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	result, err := archive.Open(bytes.NewReader(data.Bytes()), int64(data.Len()))
	if err != nil {
		t.Fatal(err)
	}
	return result
}

const vanillaMultiMC = `{"formatVersion":1,"components":[{"uid":"net.minecraft","version":"1.20.4"},{"uid":"net.fabricmc.fabric-loader","cachedVersion":"0.16.10"}]}`

func TestMultiMCAndPrismWrappedPayloads(t *testing.T) {
	for _, prefix := range []string{"", "pack/", "exports/pack/"} {
		for _, payload := range []string{".minecraft/", "minecraft/"} {
			z := testArchive(t, map[string]string{prefix + "mmc-pack.json": vanillaMultiMC, prefix + "instance.cfg": "[General]\nname=测试包\nPreLaunchCommand=do-not-execute\n", prefix + payload + "config/example.json": "{}", prefix + "launcher-private.txt": "not a game payload"})
			got, err := Read(t.Context(), z)
			if err != nil || got.Format != "multimc" || got.Name != "测试包" || got.Minecraft != "1.20.4" || len(got.Loaders) != 1 || got.Loaders[0].Version != "0.16.10" || len(got.Overrides) != 1 || got.Overrides[0].Target != "config/example.json" {
				t.Fatalf("bad MultiMC import: %+v %v", got, err)
			}
		}
	}
}
func TestMultiMCRejectsAmbiguityUnsupportedComponentsAndPatches(t *testing.T) {
	for _, files := range []map[string]string{
		{"a/mmc-pack.json": vanillaMultiMC, "a/instance.cfg": "name=A", "b/mmc-pack.json": vanillaMultiMC, "b/instance.cfg": "name=B"},
		{"mmc-pack.json": strings.Replace(vanillaMultiMC, "net.fabricmc.fabric-loader", "unknown.execution.component", 1), "instance.cfg": "name=A"},
		{"mmc-pack.json": vanillaMultiMC, "instance.cfg": "name=A", "patches/custom.json": "{}"},
		{"mmc-pack.json": vanillaMultiMC, "instance.cfg": "name=A", "manifest.json": "{}"},
		{"mmc-pack.json": vanillaMultiMC},
	} {
		if _, err := Read(t.Context(), testArchive(t, files)); err == nil {
			t.Fatal("ambiguous/unsupported pack was silently imported")
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := Read(ctx, testArchive(t, map[string]string{"mmc-pack.json": vanillaMultiMC, "instance.cfg": "name=A"})); err == nil {
		t.Fatal("canceled inspection succeeded")
	}
}
func TestCurseForgePrimaryLoaderAndMetadata(t *testing.T) {
	manifest := `{"manifestType":"minecraftModpack","manifestVersion":1,"name":"Pack","version":"v1","author":"Author","minecraft":{"version":"1.20.4","modLoaders":[{"id":"forge-1.20.4-49.0.1","primary":true},{"id":"fabric-0.16.10","primary":false}]},"files":[],"overrides":"overrides"}`
	got, err := Read(t.Context(), testArchive(t, map[string]string{"manifest.json": manifest, "overrides/config/test.json": "{}"}))
	if err != nil || got.Author != "Author" || got.Version != "v1" || len(got.Loaders) != 1 || got.Loaders[0].Type != "forge" || got.Loaders[0].Version != "49.0.1" {
		t.Fatalf("bad primary loader: %+v %v", got, err)
	}
}
func TestMultiMCLWJGLConstraintCannotSilentlyChangeRuntime(t *testing.T) {
	manifest := Manifest{RuntimeComponents: []RuntimeComponent{{UID: "org.lwjgl3", Version: "3.3.3"}}}
	version := meta.Version{}
	if manifest.CheckRuntime(version) == nil {
		t.Fatal("unmatched runtime constraint ignored")
	}
	version.Libraries = []meta.Library{{Name: "org.lwjgl:lwjgl:3.3.3"}}
	if err := manifest.CheckRuntime(version); err != nil {
		t.Fatal(err)
	}
}
