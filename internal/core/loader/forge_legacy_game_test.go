package loader

import (
	"context"
	"crypto/sha1"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"mcl/internal/core/download"
	"mcl/internal/core/java"
	"mcl/internal/core/meta"
)

// Synthetic complete spec1 fixture. Headers/classes are not executable and the
// runner below only writes test ZIPs. No real Java or network is used by design.
func processedLegacyFixture(t *testing.T) (Request, meta.Version, map[string]any, map[string]any, map[string][]byte, []byte) {
	t.Helper()
	request, base, legacy, files := legacyFixture(t)
	id := "1.12.2-forge-14.23.5.2860"
	version := legacy["versionInfo"].(map[string]any)
	version["id"] = id
	coreName := "net.minecraftforge:forge:1.12.2-14.23.5.2860:client"
	core, _ := meta.ParseCoordinate(coreName)
	patched := legacyTestZip(t, map[string][]byte{"net/minecraftforge/fml/common/launcher/FMLTweaker.class": syntheticClass(52, 0)})
	hash := fmt.Sprintf("%x", sha1.Sum(patched))
	libraries := version["libraries"].([]any)
	libraries[0] = map[string]any{"name": coreName, "downloads": map[string]any{"artifact": map[string]any{"path": core.Path(), "url": "", "sha1": hash, "size": len(patched)}}}
	version["libraries"] = libraries
	toolName := "net.minecraftforge:binarypatcher:1.0.12"
	tool, _ := meta.ParseCoordinate(toolName)
	toolURL := "https://maven.minecraftforge.net/" + tool.Path()
	crlf := string([]byte{13, 10})
	toolBytes := legacyTestZip(t, map[string][]byte{"META-INF/MANIFEST.MF": []byte("Manifest-Version: 1.0" + crlf + "Main-Class: fixture.Tool" + crlf + crlf), "fixture/Tool.class": syntheticClass(61, 0)})
	files[toolURL] = toolBytes
	clientBytes := legacyTestZip(t, map[string][]byte{"fixture/Game.class": syntheticClass(52, 0)})
	clientURL := "https://piston-data.mojang.com/synthetic/client.jar"
	base.Downloads["client"] = meta.Download{URL: clientURL, SHA1: fmt.Sprintf("%x", sha1.Sum(clientBytes)), Size: int64(len(clientBytes))}
	files[clientURL] = clientBytes
	spec := map[string]any{
		"spec": 1, "minecraft": base.ID, "version": id, "json": "/version.json",
		"data":       map[string]any{"PATCHED": map[string]any{"client": "[" + coreName + "]"}, "PATCHED_SHA": map[string]any{"client": "'" + hash + "'"}},
		"libraries":  []any{map[string]any{"name": toolName, "downloads": map[string]any{"artifact": map[string]any{"path": tool.Path(), "url": toolURL, "sha1": fmt.Sprintf("%x", sha1.Sum(toolBytes)), "size": len(toolBytes)}}}},
		"processors": []any{map[string]any{"jar": toolName, "classpath": []string{}, "args": []string{"--output", "{PATCHED}"}, "outputs": map[string]string{"{PATCHED}": "{PATCHED_SHA}"}, "sides": []string{"client"}}},
	}
	return request, base, spec, version, files, patched
}

func processedInstaller(t *testing.T, request Request, spec, version map[string]any, files map[string][]byte, runner ProcessorRunner, resolve ProcessorJavaResolver) (*ForgeInstaller, string) {
	t.Helper()
	specBytes, _ := json.Marshal(spec)
	versionBytes, _ := json.Marshal(version)
	bundle := legacyTestZip(t, map[string][]byte{"install_profile.json": specBytes, "version.json": versionBytes})
	coordinate, origin, _, err := forgeInstaller(request)
	if err != nil {
		t.Fatal(err)
	}
	files[origin+"/"+coordinate.Path()] = bundle
	transport := legacyTestTransport{files}
	catalog := NewClient()
	catalog.HTTP = &http.Client{Transport: transport}
	root := t.TempDir()
	client := download.NewClient(root)
	client.HTTP = &http.Client{Transport: transport}
	return NewForgeInstallerWithJava(catalog, client, runner, resolve), root
}

func TestProcessedLegacyTemplateUsesTool17AndKeepsGame8(t *testing.T) {
	request, base, spec, version, files, patched := processedLegacyFixture(t)
	selected, runs := 0, 0
	var gameHint, toolPath string
	runner := func(ctx context.Context, executable, work string, args []string) error {
		runs++
		if executable != toolPath || executable == gameHint {
			t.Fatal("game Java was used as processor Java")
		}
		index := slices.Index(args, "--output")
		if index < 0 || index+1 >= len(args) {
			return errors.New("missing fixed output")
		}
		return os.WriteFile(args[index+1], patched, 0o600)
	}
	resolver := func(ctx context.Context, minimum int, target meta.Platform, preferred []string, report func(java.Progress)) (java.Runtime, error) {
		selected++
		if minimum != 17 || len(preferred) != 1 || preferred[0] != gameHint {
			t.Fatal("processor minimum or game hint was lost")
		}
		return java.Runtime{Executable: toolPath, Major: 17, Version: "17.0.16", Arch: target.Arch}, nil
	}
	installer, rootPath := processedInstaller(t, request, spec, version, files, runner, resolver)
	toolPath = filepath.Join(rootPath, "tool-java-17")
	gameHint = filepath.Join(rootPath, "game-java-8") // No such executable is run/probed.
	profile, err := installer.Prepare(t.Context(), request, base, gameHint, meta.CurrentPlatform(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if selected != 1 || runs != 1 || !profile.LegacyGame || profile.Legacy || profile.JavaMajor != 8 {
		t.Fatal("tool/game separation failed")
	}
	effective, err := profile.Merge(base)
	if err != nil {
		t.Fatal(err)
	}
	if effective.Java.Major != 8 || effective.MainClass != legacyForgeMain {
		t.Fatal("tool requirement contaminated game metadata")
	}
	args, _ := meta.SelectArguments(effective.Arguments.Game, meta.CurrentPlatform())
	count := 0
	for _, arg := range args {
		if arg == "--username" {
			count++
		}
	}
	if count != 1 || !slices.Contains(args, legacyForgeTweaker) {
		t.Fatal("complete legacy template was appended instead of replaced")
	}
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err := profile.VerifyRuntime(t.Context(), root); err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(profile)
	var restored Profile
	if err := json.Unmarshal(encoded, &restored); err != nil || restored.Validate() != nil || restored.Fingerprint() != profile.Fingerprint() {
		t.Fatal("processed legacy cache identity did not round trip")
	}
}

func TestProcessorFailureCannotPublishRuntime(t *testing.T) {
	for _, failSelection := range []bool{false, true} {
		request, base, spec, version, files, _ := processedLegacyFixture(t)
		sentinel := errors.New("synthetic processor failure")
		runner := func(context.Context, string, string, []string) error { return sentinel }
		resolver := func(ctx context.Context, minimum int, target meta.Platform, preferred []string, report func(java.Progress)) (java.Runtime, error) {
			if failSelection {
				return java.Runtime{}, sentinel
			}
			return java.Runtime{Executable: "/synthetic/tool", Major: 17, Version: "17.0.16", Arch: target.Arch}, nil
		}
		installer, root := processedInstaller(t, request, spec, version, files, runner, resolver)
		_, err := installer.Prepare(t.Context(), request, base, "not-game-java", meta.CurrentPlatform(), nil)
		if !errors.Is(err, sentinel) {
			t.Fatalf("tool failure lost: %v", err)
		}
		entries, err := os.ReadDir(filepath.Join(root, "cache", "loader-runtimes", request.Key()))
		if err != nil || len(entries) != 0 {
			t.Fatalf("failed task published/retained a runtime: %v %v", entries, err)
		}
	}
}

func TestProcessedLegacyFormatRejectsUnknownSidesAndEntry(t *testing.T) {
	for _, change := range []func(map[string]any, map[string]any){
		func(spec, version map[string]any) { version["mainClass"] = "custom.Injector" },
		func(spec, version map[string]any) { version["minecraftArguments"] = "--tweakClass custom.Tweaker" },
		func(spec, version map[string]any) {
			spec["processors"].([]any)[0].(map[string]any)["sides"] = []string{"unknown"}
		},
	} {
		request, _, spec, version, _, _ := processedLegacyFixture(t)
		change(spec, version)
		_, _, id, _ := forgeInstaller(request)
		data, _ := json.Marshal(version)
		z := legacyArchive(t, spec, map[string][]byte{"version.json": data})
		if _, _, err := readForgeSpec(t.Context(), z, request, id); err == nil {
			t.Fatal("unsupported processed legacy format accepted")
		}
	}
}

func TestProcessorLegacyFlagCannotImpersonateOldInstaller(t *testing.T) {
	request, _, spec, version, _, _ := processedLegacyFixture(t)
	raw, _ := json.Marshal(version)
	_, _, id, _ := forgeInstaller(request)
	prepared, parsed, err := readForgeSpec(t.Context(), legacyArchive(t, spec, map[string][]byte{"version.json": raw}), request, id)
	if err != nil || !prepared.LegacyGame || parsed.Java.Major != 8 {
		t.Fatal("processed legacy format was not normalized", err)
	}
	if !processedLegacyCore(request, "net.minecraftforge:forge:1.12.2-14.23.5.2860:client") || processedLegacyCore(request, "net.minecraftforge:forge:1.12.2-unrelated:client") {
		t.Fatal("generated core identity not tied to request")
	}
	if strings.Contains(parsed.LegacyArguments, "tweakClass") {
		t.Fatal("un-normalized legacy text escaped")
	}
}
