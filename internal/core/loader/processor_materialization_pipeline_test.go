package loader

import (
	"context"
	"crypto/sha1"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"mcl/internal/core/java"
	"mcl/internal/core/meta"
)

func TestPrepareResolvesWholeArchivePlanBeforeAnyJava(t *testing.T) {
	request, base, spec, version, files, _ := processedLegacyFixture(t)
	request, err := request.WithPlatform(meta.CurrentPlatform())
	if err != nil {
		t.Fatal(err)
	}
	c, _ := meta.ParseCoordinate("example:mcp:1@zip")
	payload := legacyTestZip(t, map[string][]byte{"config.json": []byte("{\"data\":{\"patches\":\"patches/\"}}"), "patches/real.patch": []byte("real member")})
	url := "https://maven.minecraftforge.net/" + c.Path()
	files[url] = payload
	spec["libraries"] = append(spec["libraries"].([]any), map[string]any{"name": c.String(), "downloads": map[string]any{"artifact": map[string]any{"path": c.Path(), "url": url, "sha1": fmt.Sprintf("%x", sha1.Sum(payload)), "size": len(payload)}}})
	original := spec["processors"].([]any)[0]
	spec["processors"] = []any{
		original, // A Java step could run first if concrete preflight were too late.
		map[string]any{"jar": "net.minecraftforge:installertools:1.4.0", "args": []string{"--task", "MCP_DATA", "--input", "[" + c.String() + "]", "--key", "patches", "--output", "{LIBRARY_DIR}/patches"}},
		map[string]any{"jar": "net.minecraftforge:binarypatcher:1.0.12", "args": []string{"--clean", "{LIBRARY_DIR}/patches/never-existed.patch", "--output", "{PATCHED}"}, "outputs": map[string]string{"{PATCHED}": "{PATCHED_SHA}"}},
	}
	selected, runs := 0, 0
	installer, root := processedInstaller(t, request, spec, version, files, func(context.Context, string, string, []string) error { runs++; return nil }, func(context.Context, int, meta.Platform, []string, func(java.Progress)) (java.Runtime, error) {
		selected++
		return java.Runtime{}, nil
	})
	_, err = installer.Prepare(t.Context(), request, base, "synthetic-game-java", meta.CurrentPlatform(), nil)
	var detail *ProcessorError
	if !errors.As(err, &detail) || detail.Position != 3 || detail.Stage != "输入计划" || selected != 0 || runs != 0 {
		t.Fatal("late/ambiguous failure or Java executed before whole concrete plan", err, selected, runs)
	}
	entries, err := os.ReadDir(filepath.Join(root, "cache", "loader-runtimes", request.Key()))
	if err != nil || len(entries) != 0 {
		t.Fatal("failed materialization retained a generation", err)
	}
}

func TestPrepareAndCachedRuntimeRejectUnrecordedLibraries(t *testing.T) {
	for _, unrecordedAtRun := range []bool{false, true} {
		t.Run(fmt.Sprint(unrecordedAtRun), func(t *testing.T) {
			request, base, spec, version, files, patched := processedLegacyFixture(t)
			request, err := request.WithPlatform(meta.CurrentPlatform())
			if err != nil {
				t.Fatal(err)
			}
			runner := func(_ context.Context, _ string, _ string, args []string) error {
				target := args[slices.Index(args, "--output")+1]
				if err := os.WriteFile(target, patched, 0o600); err != nil {
					return err
				}
				if unrecordedAtRun {
					return os.WriteFile(filepath.Join(filepath.Dir(target), "unrecorded.jar"), []byte("not a recorded output"), 0o600)
				}
				return nil
			}
			resolver := func(_ context.Context, _ int, target meta.Platform, _ []string, _ func(java.Progress)) (java.Runtime, error) {
				return java.Runtime{Executable: "/synthetic-tool-java", Major: 17, Version: "17.0.16", Arch: target.Arch}, nil
			}
			installer, rootPath := processedInstaller(t, request, spec, version, files, runner, resolver)
			profile, err := installer.Prepare(t.Context(), request, base, "synthetic-game-java", meta.CurrentPlatform(), nil)
			if unrecordedAtRun {
				if err == nil {
					t.Fatal("processor side effect published as a valid generation")
				}
				entries, readErr := os.ReadDir(filepath.Join(rootPath, "cache", "loader-runtimes", request.Key()))
				if readErr != nil || len(entries) != 0 {
					t.Fatal("unrecorded file retained or published", readErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			root, err := os.OpenRoot(rootPath)
			if err != nil {
				t.Fatal(err)
			}
			defer root.Close()
			if err := profile.VerifyRuntime(t.Context(), root); err != nil {
				t.Fatal(err)
			}
			foreign := filepath.ToSlash(filepath.Join(profile.LibraryDirectory(), "unrecorded.jar"))
			if err := root.WriteFile(foreign, []byte("foreign"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := profile.VerifyRuntime(t.Context(), root); err == nil {
				t.Fatal("cache accepted an unrecorded runtime file")
			}
			if err := root.Remove(foreign); err != nil {
				t.Fatal(err)
			}
			if err := profile.VerifyRuntime(t.Context(), root); err != nil {
				t.Fatal("unrelated cache state falsely changed identity", err)
			}
			missing := filepath.ToSlash(filepath.Join(profile.LibraryDirectory(), profile.Libraries[0].Downloads.Artifact.Path))
			if err := root.Remove(missing); err != nil {
				t.Fatal(err)
			}
			if err := profile.VerifyRuntime(t.Context(), root); err == nil {
				t.Fatal("directory enumeration missed a deleted fixed runtime")
			}
		})
	}
}
