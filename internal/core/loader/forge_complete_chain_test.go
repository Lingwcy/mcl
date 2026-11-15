package loader

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"mcl/internal/core/java"
	"mcl/internal/core/meta"
)

func TestInheritedOfficialBaseCanPreparePinnedProcessorGeneration(t *testing.T) {
	request, base, spec, version, files, patched := processedLegacyFixture(t)
	base.Sources = []meta.ManifestEntry{chainSource("1.12.1", strings.Repeat("b", 40)), chainSource(base.ID, request.BaseSHA1)}
	base.ClientVersion = "1.12.1"
	var err error
	request, err = request.WithBase(base)
	if err != nil {
		t.Fatal(err)
	}
	request, err = request.WithPlatform(meta.CurrentPlatform())
	if err != nil {
		t.Fatal(err)
	}
	runner := func(ctx context.Context, executable, work string, args []string) error {
		index := slices.Index(args, "--output")
		if index < 0 {
			return errors.New("missing output")
		}
		return os.WriteFile(args[index+1], patched, 0o600)
	}
	resolver := func(ctx context.Context, minimum int, target meta.Platform, preferred []string, report func(java.Progress)) (java.Runtime, error) {
		return java.Runtime{Executable: filepath.Join(t.TempDir(), "synthetic-java"), Major: 17, Version: "17.0.16", Arch: target.Arch}, nil
	}
	installer, root := processedInstaller(t, request, spec, version, files, runner, resolver)
	profile, err := installer.Prepare(t.Context(), request, base, "synthetic-game-java", meta.CurrentPlatform(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if profile.Request.BaseChainSHA256 == "" {
		t.Fatal("profile lost complete inherited identity")
	}
	if _, err := os.Stat(filepath.Join(root, "cache", "versions", "1.12.1", "1.12.1.jar")); err != nil {
		t.Fatal("processor downloaded client under the wrong origin", err)
	}
	changed := base
	changed.Sources = append([]meta.ManifestEntry(nil), base.Sources...)
	changed.Sources[0] = chainSource("1.12.1", strings.Repeat("d", 40))
	if _, err := profile.Merge(changed); err == nil {
		t.Fatal("prepared generation merged into a different parent")
	}
}

func TestPrivateProcessorOutputReuseAndConflicts(t *testing.T) {
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	fixed := stageFixed(t, root, "libraries/output.jar", []byte("fixed output"))
	known := map[string]meta.Download{"libraries/output.jar": fixed}
	p := preparedProcessor{outputs: map[string]string{"libraries/output.jar": fixed.SHA1}}
	reused, err := prepareProcessorOutputs(t.Context(), root, known, p)
	if err != nil || !reused {
		t.Fatal("verified same output was not reused", err)
	}
	if err := root.WriteFile("libraries/output.jar", []byte("external change"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := prepareProcessorOutputs(t.Context(), root, known, p); err == nil {
		t.Fatal("unknown output mutation was overwritten")
	}
	p = preparedProcessor{outputs: map[string]string{"libraries/unknown.jar": ""}}
	stageFixed(t, root, "libraries/unknown.jar", []byte("not in known sources"))
	if _, err := prepareProcessorOutputs(t.Context(), root, known, p); err == nil {
		t.Fatal("unknown pre-existing file was removed")
	}
}

func TestCleanJarsplitterSidecarCache(t *testing.T) {
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	slim := stageFixed(t, root, "libraries/net/minecraft/client/1.20.1/client-1.20.1-slim.jar", []byte("slim"))
	sidecar := "libraries/net/minecraft/client/1.20.1/client-1.20.1-slim.jar.cache"
	if err := root.WriteFile(sidecar, []byte("Input: abc\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	known := map[string]meta.Download{"libraries/net/minecraft/client/1.20.1/client-1.20.1-slim.jar": slim}
	p := preparedProcessor{
		coordinate: "net.minecraftforge:jarsplitter:1.1.4",
		outputs:    map[string]string{"libraries/net/minecraft/client/1.20.1/client-1.20.1-slim.jar": slim.SHA1},
	}
	if err := cleanProcessorSidecars(root, known, p); err != nil {
		t.Fatal(err)
	}
	if _, err := root.Lstat(sidecar); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected sidecar .cache removed, got %v", err)
	}
	// Known/unrelated files and non-jarsplitter outputs must never be removed.
	known[sidecar] = stageFixed(t, root, sidecar, []byte("fixed"))
	if err := cleanProcessorSidecars(root, known, p); err != nil {
		t.Fatal(err)
	}
	if _, err := root.Lstat(sidecar); err != nil {
		t.Fatalf("expected known file preserved, got %v", err)
	}
}
