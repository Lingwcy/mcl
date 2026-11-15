package loader

import (
	"context"
	"crypto/sha1"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"slices"
	"strings"
	"testing"

	"mcl/internal/core/download"
	"mcl/internal/core/meta"
)

func stageFixed(t *testing.T, root *os.Root, name string, data []byte) meta.Download {
	t.Helper()
	if err := root.MkdirAll(path.Dir(name), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := root.WriteFile(name, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return meta.Download{Path: name, SHA1: fmt.Sprintf("%x", sha1.Sum(data)), Size: int64(len(data))}
}
func TestNativeMcpAndExtractRunWithoutJava(t *testing.T) {
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	config := []byte(`{"data":{"mappings":"maps.txt"}}`)
	archiveBytes := legacyTestZip(t, map[string][]byte{"config.json": config, "maps.txt": []byte("fixed-mapping")})
	known := map[string]meta.Download{"libraries/config.zip": stageFixed(t, root, "libraries/config.zip", archiveBytes)}
	values := map[string]forgeValue{"INPUT": {"libraries/config.zip", true}, "OUTPUT": {"libraries/maps.txt", true}}
	spec := forgeProcessor{Jar: "net.minecraftforge:installertools:1.0", Args: []string{"--task", "MCP_DATA", "--input", "{INPUT}", "--key", "mappings", "--output", "{OUTPUT}"}}
	processor, err := prepareForgeProcessor(spec, values, root.Name())
	if err != nil {
		t.Fatal(err)
	}
	installer := NewForgeInstaller(nil, nil, func(context.Context, string, string, []string) error {
		t.Fatal("native task executed Java")
		return nil
	})
	if err := installer.runProcessors(t.Context(), root, root, root.Name(), "", meta.CurrentPlatform(), meta.Version{}, []preparedProcessor{processor}, known, func(download.Progress) {}, nil); err != nil {
		t.Fatal(err)
	}
	result, err := root.ReadFile("libraries/maps.txt")
	if err != nil || string(result) != "fixed-mapping" {
		t.Fatal("MCP extraction lost fixed data")
	}
	if _, ok := known["libraries/maps.txt"]; !ok {
		t.Fatal("native output not pinned for later processors")
	}
	values["OUTPUT"] = forgeValue{"libraries/second.txt", true}
	spec.Args = []string{"--task", "EXTRACT_FILES", "--archive", "{INPUT}", "--from", "maps.txt", "--to", "{OUTPUT}"}
	processor, err = prepareForgeProcessor(spec, values, root.Name())
	if err != nil {
		t.Fatal(err)
	}
	if err := installer.runProcessors(t.Context(), root, root, root.Name(), "", meta.CurrentPlatform(), meta.Version{}, []preparedProcessor{processor}, known, func(download.Progress) {}, nil); err != nil {
		t.Fatal(err)
	}
	processor.nativeValues[5] = forgeValue{"../outside", false}
	if err := validateNativeTask(&processor); err == nil {
		t.Fatal("unsafe ZIP entry accepted")
	}
}

func TestOfflineSanitizedMappingsMatchFixedOutput(t *testing.T) {
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	raw := []byte(strings.Join([]string{"# public mapping comment", "com.Test -> a:", "    java.lang.String a -> b", "    int z -> a", "    1:2:void <init>() -> <init>", "    int foo(java.lang.String[]) -> b", ""}, string(byte(10))))
	expected := []byte(strings.Join([]string{"com/Test a", string(byte(9)) + "z a", string(byte(9)) + "a b", string(byte(9)) + "<init> ()V <init>", string(byte(9)) + "foo ([Ljava/lang/String;)I b", ""}, string(byte(10))))
	base := meta.Version{ID: "mapping-fixture", Downloads: map[string]meta.Download{}}
	fixed := meta.Download{URL: "https://piston-data.mojang.com/fixed.txt", SHA1: fmt.Sprintf("%x", sha1.Sum(raw)), Size: int64(len(raw))}
	base.Downloads["client_mappings"] = fixed
	cacheFile := forgeMappings(base)
	stageFixed(t, root, cacheFile.Path, raw)
	processor := preparedProcessor{copyMappings: "libraries/clean.tsrg", sanitizeMappings: true, mappingFormat: "TSRG", outputs: map[string]string{"libraries/clean.tsrg": fmt.Sprintf("%x", sha1.Sum(expected))}}
	if err := root.MkdirAll("libraries", 0o700); err != nil {
		t.Fatal(err)
	}
	if err := prepareForgeMappings(t.Context(), root, root, base, processor); err != nil {
		t.Fatal(err)
	}
	actual, err := root.ReadFile("libraries/clean.tsrg")
	if err != nil || string(actual) != string(expected) {
		t.Fatal("canonical mapping conversion changed")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := parseProguard(ctx, raw); !errors.Is(err, context.Canceled) {
		t.Fatal("mapping cancellation lost", err)
	}
	processor.copyMappings = "libraries/wrong.tsrg"
	processor.outputs = map[string]string{processor.copyMappings: strings.Repeat("a", 40)}
	if err := prepareForgeMappings(t.Context(), root, root, base, processor); err == nil {
		t.Fatal("incorrect output digest accepted")
	}
}

func TestProcessorVariablesAreBoundedAndTyped(t *testing.T) {
	var spec forgeSpec
	if err := json.Unmarshal([]byte(`{"minecraft":"1.20.4","data":{"V":{"client":"'1.2.3'"},"PATCH":{"client":"[net.minecraftforge:forge:{V}:client]"},"OTHER":{"client":"{LIBRARY_DIR}/other-{V}.jar"}}}`), &spec); err != nil {
		t.Fatal(err)
	}
	values, _, err := forgeData(spec)
	if err != nil {
		t.Fatal(err)
	}
	if !values["PATCH"].file || !strings.Contains(values["OTHER"].text, "other-1.2.3.jar") {
		t.Fatal("typed variable composition lost")
	}
	spec.Data["V"] = struct{ Client string }{Client: "{PATCH}"}
	if _, _, err := forgeData(spec); err == nil {
		t.Fatal("cyclic processor variables accepted")
	}
	if _, err := forgeArg("{ROOT}/../outside", values); err == nil {
		t.Fatal("private path escape accepted")
	}
}

func TestOfflineSanitizedMappingsSupportSrgUtilsHashOverloadOrder(t *testing.T) {
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	raw := []byte(strings.Join([]string{
		"com.mojang.blaze3d.platform.DebugMemoryUntracker -> ezz:",
		"    void untrack(long) -> a",
		"    void untrack(org.lwjgl.system.Pointer) -> a",
		"",
	}, "\n"))
	classes, err := parseProguard(t.Context(), raw)
	if err != nil {
		t.Fatal(err)
	}
	wantTSRG, err := writeTSRG(t.Context(), classes, "TSRG", tsrgSrgUtilsHashOrder)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(wantTSRG), "\tuntrack (Lorg/lwjgl/system/Pointer;)V a\n\tuntrack (J)V a\n") {
		t.Fatalf("unexpected srgutils overload bucket order:\n%s", string(wantTSRG))
	}
	base := meta.Version{ID: "mapping-overload", Downloads: map[string]meta.Download{}}
	fixed := meta.Download{URL: "https://piston-data.mojang.com/fixed.txt", SHA1: fmt.Sprintf("%x", sha1.Sum(raw)), Size: int64(len(raw))}
	base.Downloads["client_mappings"] = fixed
	stageFixed(t, root, forgeMappings(base).Path, raw)
	if err := root.MkdirAll("libraries", 0o700); err != nil {
		t.Fatal(err)
	}
	processor := preparedProcessor{copyMappings: "libraries/clean.tsrg", sanitizeMappings: true, mappingFormat: "TSRG", outputs: map[string]string{"libraries/clean.tsrg": fmt.Sprintf("%x", sha1.Sum(wantTSRG))}}
	if err := prepareForgeMappings(t.Context(), root, root, base, processor); err != nil {
		t.Fatal(err)
	}
}

func TestForgeSpec0ProcessorsSpecialSourceNullPathAndProcessMinecraftJar(t *testing.T) {
	spec0, _ := json.Marshal(map[string]any{
		"spec":       0,
		"minecraft":  "1.16.5",
		"version":    "1.16.5-forge-36.2.42",
		"path":       "net.minecraftforge:forge:1.16.5-36.2.42",
		"processors": []any{map[string]any{"jar": "net.md-5:SpecialSource:1.8.5"}},
	})
	format, err := installerFormat(spec0)
	if err != nil || format != ForgeFormatProcessors {
		t.Fatalf("spec0 with processors format=%q err=%v", format, err)
	}

	request := Request{Type: "forge", GameVersion: "1.18.2", Version: "40.2.21", BaseSHA1: strings.Repeat("a", 40)}
	spec1Null, _ := json.Marshal(map[string]any{
		"spec": 1, "minecraft": "1.18.2", "version": "1.18.2-forge-40.2.21", "path": nil, "json": "/version.json",
		"data": map[string]any{}, "libraries": []any{}, "processors": []any{},
	})
	verJSON, _ := json.Marshal(map[string]any{
		"id": "1.18.2-forge-40.2.21", "inheritsFrom": "1.18.2", "mainClass": "cpw.mods.bootstraplauncher.BootstrapLauncher",
		"arguments": map[string]any{"game": []any{}}, "libraries": []any{map[string]any{"name": "net.minecraftforge:forge:1.18.2-40.2.21"}},
	})
	z := officialMetadataArchive(t, map[string][]byte{"install_profile.json": spec1Null, "version.json": verJSON})
	if _, _, err := readForgeSpec(t.Context(), z, request); err != nil {
		t.Fatalf("spec1 path:null rejected: %v", err)
	}

	values := map[string]forgeValue{
		"ROOT":          {".", true},
		"MINECRAFT_JAR": {"work/client.jar", true},
		"PATCHED":       {"libraries/net/neoforged/minecraft-client-patched/26.1.0.15-beta/minecraft-client-patched-26.1.0.15-beta.jar", true},
		"BINPATCH":      {"work/client.lzma", true},
	}
	if !knownForgeProcessor(meta.Coordinate{Group: "net.md-5", Artifact: "SpecialSource", Version: "1.8.5", Extension: "jar"}) {
		t.Fatal("SpecialSource processor rejected")
	}
	proc, err := prepareForgeProcessor(forgeProcessor{
		Jar:  "net.neoforged.installertools:installertools:4.0.12:fatjar",
		Args: []string{"--task", "PROCESS_MINECRAFT_JAR", "--no-mod-manifest", "--input", "{MINECRAFT_JAR}", "--output", "{PATCHED}", "--extract-libraries-to", "{ROOT}/libraries/", "--apply-patches", "{BINPATCH}"},
	}, values, t.TempDir())
	if err != nil {
		t.Fatalf("PROCESS_MINECRAFT_JAR rejected: %v", err)
	}
	if _, ok := proc.outputs[values["PATCHED"].text]; !ok || slices.Contains(proc.inputs, "libraries") {
		t.Fatalf("PROCESS_MINECRAFT_JAR inputs/outputs wrong: %+v", proc)
	}
	if _, err := prepareForgeProcessor(forgeProcessor{
		Jar:  "net.neoforged.installertools:installertools:4.0.12:fatjar",
		Args: []string{"--task", "PROCESS_MINECRAFT_JAR", "--input", "{MINECRAFT_JAR}", "--output", "{PATCHED}", "--extract-libraries-to", "{ROOT}/other/", "--apply-patches", "{BINPATCH}"},
	}, values, t.TempDir()); err == nil {
		t.Fatal("unsafe extract-libraries-to path accepted")
	}
}
