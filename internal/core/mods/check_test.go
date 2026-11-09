package mods

import (
	"archive/zip"
	"bytes"
	"errors"
	"slices"
	"strings"
	"testing"
)

func descriptorFile(path, loader string, mod Mod) File {
	return File{Path: path, Enabled: true, Archive: Archive{Variants: []Variant{{Loader: loader, Mods: []Mod{mod}}}}}
}

func TestForeignDescriptorIsUnknownAndDoesNotBlockPublication(t *testing.T) {
	for _, pair := range [][2]string{{"forge", "fabric"}, {"neoforge", "fabric"}, {"fabric", "forge"}, {"quilt", "forge"}} {
		t.Run(pair[0]+"-with-"+pair[1], func(t *testing.T) {
			runtime := Runtime{GameVersion: "1.20.1", Loader: pair[0], LoaderVersion: "47.3.0"}
			foreign := descriptorFile("mods/compat.jar", pair[1], Mod{ID: "compat", Version: "1.0.0", Requires: []Requirement{{ID: "not-a-runtime-dependency", Syntax: Fabric}}})
			report, err := Check(t.Context(), []File{foreign}, runtime, false)
			if err != nil {
				t.Fatal(err)
			}
			if report.Modules != 0 || len(report.Issues) != 1 || report.Issues[0].Code != "loader" || report.Issues[0].Error || report.Issues[0].Path != foreign.Path {
				t.Fatalf("foreign descriptor treated as loaded or rejected: %+v", report)
			}
			if err := GuardTransition(t.Context(), nil, []File{foreign}, runtime, []string{foreign.Path}); err != nil {
				t.Fatal("unknown descriptor blocked publication", err)
			}
			foreign.Enabled = false
			report, err = Check(t.Context(), []File{foreign}, runtime, false)
			if err != nil || len(report.Issues) != 0 {
				t.Fatal("disabled foreign file affected runtime", report, err)
			}
		})
	}
}
func TestForeignDescriptorCannotEstablishProvidersOrProveMissingDependency(t *testing.T) {
	for _, id := range []string{"foreign", "not-found"} {
		t.Run(id, func(t *testing.T) {
			parent := descriptorFile("mods/parent.jar", "forge", Mod{ID: "parent", Version: "1.0.0", Requires: []Requirement{{ID: id, Syntax: Maven}}})
			foreign := descriptorFile("mods/foreign.jar", "fabric", Mod{ID: "foreign", Version: "1.0.0"})
			report, err := Check(t.Context(), []File{parent, foreign}, Runtime{Loader: "forge"}, false)
			if err != nil {
				t.Fatal(err)
			}
			index := slices.IndexFunc(report.Issues, func(issue Issue) bool { return issue.Code == "dependency" && issue.ModID == "parent" })
			if report.Modules != 1 || index < 0 || report.Issues[index].Error || report.Issues[index].Title != "声明关系无法确认" {
				t.Fatal("foreign providers assumed loaded or complete", report)
			}
		})
	}
}
func TestKnownViolationsStillBlockWithForeignMetadata(t *testing.T) {
	base := descriptorFile("mods/base.jar", "forge", Mod{ID: "base", Version: "1.0.0"})
	parent := descriptorFile("mods/parent.jar", "forge", Mod{ID: "parent", Version: "1.0.0", Requires: []Requirement{{ID: "base", Syntax: Maven}}})
	foreign := descriptorFile("mods/compat.jar", "fabric", Mod{ID: "compat", Version: "1.0.0"})
	runtime := Runtime{Loader: "forge"}
	before := []File{parent, base, foreign}
	for _, disabled := range []bool{false, true} {
		after := []File{parent, foreign}
		if disabled {
			base.Enabled = false
			after = append(after, base)
		}
		if err := GuardTransition(t.Context(), before, after, runtime, []string{base.Path}); !errors.Is(err, ErrUnsafeChange) {
			t.Fatal("satisfied dependency lost protection", disabled, err)
		}
	}
	duplicate := descriptorFile("mods/duplicate.jar", "forge", Mod{ID: "parent", Version: "1.0.0"})
	if err := GuardTransition(t.Context(), nil, []File{parent, duplicate, foreign}, runtime, nil); !errors.Is(err, ErrUnsafeChange) || !strings.Contains(err.Error(), duplicate.Path) {
		t.Fatal("duplicate not blocked with file path", err)
	}
	conflict := descriptorFile("mods/conflict.jar", "forge", Mod{ID: "conflict", Version: "1.0.0", Requires: []Requirement{{ID: "parent", Syntax: Maven, Incompatible: true}}})
	if err := GuardTransition(t.Context(), nil, []File{parent, conflict, foreign}, runtime, nil); !errors.Is(err, ErrUnsafeChange) {
		t.Fatal("known incompatible provider not blocked", err)
	}
}
func TestJarWithBothDescriptorsUsesConfiguredRuntime(t *testing.T) {
	var body bytes.Buffer
	writer := zip.NewWriter(&body)
	for _, entry := range []struct{ name, text string }{
		{"fabric.mod.json", "{\"schemaVersion\":1,\"id\":\"fabric-side\",\"version\":\"1.0.0\",\"depends\":{\"fabric-only\":\"*\"}}"},
		{"META-INF/mods.toml", "modLoader=\"javafml\"\nloaderVersion=\"[47,)\"\n[[mods]]\nmodId=\"forge-side\"\nversion=\"1.0.0\"\n"},
	} {
		member, err := writer.Create(entry.name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := member.Write([]byte(entry.text)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	metadata, err := Read(t.Context(), bytes.NewReader(body.Bytes()), int64(body.Len()))
	if err != nil || len(metadata.Variants) != 2 || len(metadata.Problems) != 0 {
		t.Fatal(metadata, err)
	}
	file := File{Path: "mods/multiloader.jar", Enabled: true, Archive: metadata}
	runtime := Runtime{Loader: "forge", LoaderVersion: "47.3.0"}
	report, err := Check(t.Context(), []File{file}, runtime, false)
	if err != nil || report.Modules != 1 {
		t.Fatal(report, err)
	}
	for _, issue := range report.Issues {
		if issue.ModID == "fabric-side" || issue.Code == "loader" {
			t.Fatal("foreign variant evaluated for Forge", report)
		}
	}
	if err := GuardTransition(t.Context(), nil, []File{file}, runtime, nil); err != nil {
		t.Fatal(err)
	}
}
func TestQuiltFabricFallbackRetainsDependencyProtection(t *testing.T) {
	file := descriptorFile("mods/fabric-compatible.jar", "fabric", Mod{ID: "fabric-compatible", Version: "1.0.0", Requires: []Requirement{{ID: "missing", Syntax: Fabric}}})
	report, err := Check(t.Context(), []File{file}, Runtime{Loader: "quilt"}, false)
	if err != nil || report.Modules != 1 || len(report.Issues) != 1 || !report.Issues[0].Error {
		t.Fatal(report, err)
	}
	if err := GuardTransition(t.Context(), nil, []File{file}, Runtime{Loader: "quilt"}, nil); !errors.Is(err, ErrUnsafeChange) {
		t.Fatal("supported fallback lost dependency check", err)
	}
}
