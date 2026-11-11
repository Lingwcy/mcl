package install

import (
	"slices"
	"strings"
	"testing"

	"mcl/internal/core/meta"
)

func TestLocalClasspathAndLookupProjectToDistinctRoles(t *testing.T) {
	version := fixture(t, "1.20.4")
	directory := "cache/loader-runtimes/" + strings.Repeat("a", 64) + "/ready-" + strings.Repeat("A", 26) + "/libraries"
	library := selectedFixtureLibrary(t, "example:runtime:1")
	library.LocalRoot = directory
	library.Downloads.Artifact.URL = ""
	version.LibraryDirectory = directory
	version.Libraries = []meta.Library{library}
	coordinate, _ := meta.ParseCoordinate("example:generated:1:client")
	version.RuntimeLookups = []meta.Download{{Path: coordinate.Path(), SHA1: strings.Repeat("b", 40), Size: 20}}
	plan, err := Build(version, meta.Platform{OS: "osx", Arch: "amd64"})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Libraries) != 2 || plan.Libraries[0].Role != LibraryClasspath || plan.Libraries[1].Role != LibraryRuntimeLookup || !plan.Libraries[0].Local || !plan.Libraries[1].Local {
		t.Fatal(plan.Libraries)
	}
	if len(plan.LocalLibraries) != 1 || len(plan.RuntimeLookups) != 1 || !slices.Contains(plan.Classpath, plan.LocalLibraries[0].Path) || slices.Contains(plan.Classpath, plan.RuntimeLookups[0].Path) {
		t.Fatal(plan.Classpath)
	}
	for _, remote := range plan.Files {
		for _, local := range plan.Libraries {
			if remote.Path == local.File.Path {
				t.Fatal("local artifact became remote download")
			}
		}
	}
}
func TestSelectedLibraryAliasesAreNotDedupedAsTheSameFile(t *testing.T) {
	version := fixture(t, "1.20.4")
	first := selectedFixtureLibrary(t, "example:alias:1")
	second := selectedFixtureLibrary(t, "example:Alias:1")
	version.Libraries = []meta.Library{first, second}
	if _, err := Build(version, meta.Platform{OS: "osx", Arch: "amd64"}); err == nil {
		t.Fatal("case aliases conflated")
	}
	// Distinct declarations cannot claim one physical target just by sharing SHA.
	second = selectedFixtureLibrary(t, "other:identity:1")
	second.Downloads.Artifact.Path = first.Downloads.Artifact.Path
	version.Libraries = []meta.Library{first, second}
	if _, err := Build(version, meta.Platform{OS: "osx", Arch: "amd64"}); err == nil {
		t.Fatal("coordinate alias conflated")
	}
}
