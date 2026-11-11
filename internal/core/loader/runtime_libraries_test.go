package loader

import (
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"

	"mcl/internal/core/install"
	"mcl/internal/core/meta"
)

// Original official metadata, with a synthetic generated-file size only. No
// class/patch payload is provided or executed; this proves plan relationships.
func bootstrapRuntimeFixture(t *testing.T) (Profile, meta.Version) {
	t.Helper()
	raw, err := os.ReadFile("../meta/testdata/1.20.4.json")
	if err != nil {
		t.Fatal(err)
	}
	base, err := meta.ParseVersion(raw)
	if err != nil {
		t.Fatal(err)
	}
	var sample officialSample
	for _, item := range officialMetadataSamples(t) {
		if item.Game == "1.20.4" {
			sample = item
		}
	}
	version, err := meta.ParseVersion(officialProjection(t, sample.Version, sample.VersionProjectionSHA256))
	if err != nil {
		t.Fatal(err)
	}
	var spec forgeSpec
	if err := json.Unmarshal(officialProjection(t, sample.Profile, sample.ProfileProjectionSHA256), &spec); err != nil {
		t.Fatal(err)
	}
	values, _, err := forgeData(spec)
	if err != nil {
		t.Fatal(err)
	}
	request := Request{Type: "forge", GameVersion: sample.Game, Version: sample.Build, BaseSHA1: strings.Repeat("a", 40)}
	known := make(map[string]meta.Download)
	for _, lib := range version.Libraries {
		file := *lib.Downloads.Artifact
		file.URL = ""
		known["libraries/"+file.Path] = file
	}
	client := values["PATCHED"].text
	known[client] = meta.Download{Path: client, SHA1: values["PATCHED_SHA"].text, Size: 128} // Synthetic size, not a real measurement.
	profile, err := completedForgeProfile(request, version, known, "ready-"+strings.Repeat("A", 26), sample.SHA1, false, ForgeFormatProcessors)
	if err != nil {
		t.Fatal(err)
	}
	return profile, base
}
func TestForgeBootstrapRuntimeLookupIsLocalButNotClasspath(t *testing.T) {
	profile, base := bootstrapRuntimeFixture(t)
	effective, err := profile.Merge(base)
	if err != nil {
		t.Fatal(err)
	}
	if len(effective.RuntimeLookups) != 1 || effective.LibraryDirectory != profile.LibraryDirectory() {
		t.Fatal(effective.RuntimeLookups)
	}
	file := effective.RuntimeLookups[0]
	if !strings.HasSuffix(file.Path, "forge-1.20.4-49.0.3-client.jar") || file.URL != "" {
		t.Fatal(file)
	}
	plan, err := install.Build(effective, meta.CurrentPlatform())
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.RuntimeLookups) != 1 || slices.Contains(plan.Classpath, plan.RuntimeLookups[0].Path) {
		t.Fatal("lookup became classpath", plan.RuntimeLookups)
	}
	for _, remote := range plan.Files {
		if remote.Path == plan.RuntimeLookups[0].Path {
			t.Fatal("generated lookup became remote")
		}
	}
	effective.RuntimeLookups[0].SHA1 = strings.Repeat("b", 40)
	second, err := profile.Merge(base)
	if err != nil || second.RuntimeLookups[0].SHA1 == effective.RuntimeLookups[0].SHA1 {
		t.Fatal("shared lookup identity", err)
	}
}
func TestForgeBootstrapRejectsMissingWrongBuildAndUnmanagedDirectory(t *testing.T) {
	for _, name := range []string{"missing-client", "wrong-client", "wrong-fmlloader", "literal-directory"} {
		t.Run(name, func(t *testing.T) {
			profile, _ := bootstrapRuntimeFixture(t)
			switch name {
			case "missing-client":
				profile.Outputs = nil
			case "wrong-client":
				profile.Outputs[0].Path = strings.ReplaceAll(profile.Outputs[0].Path, "49.0.3", "49.0.4")
			case "wrong-fmlloader":
				profile.Libraries = slices.DeleteFunc(profile.Libraries, func(lib meta.Library) bool { return strings.HasPrefix(lib.Name, "net.minecraftforge:fmlloader:") })
			case "literal-directory":
				profile.Arguments.JVM = append(profile.Arguments.JVM, meta.Argument{Values: []string{"-DlibraryDirectory=/other/libraries"}})
			}
			if err := profile.Validate(); err == nil {
				t.Fatal("accepted incompatible runtime contract")
			}
		})
	}
}
