package install

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"mcl/internal/core/diagnostics"
	"mcl/internal/core/meta"
)

func selectedFixtureLibrary(t *testing.T, name string) meta.Library {
	t.Helper()
	c, err := meta.ParseCoordinate(name)
	if err != nil {
		t.Fatal(err)
	}
	result := meta.Library{Name: name}
	result.Downloads.Artifact = &meta.Download{Path: c.Path(), URL: "https://libraries.minecraft.net/" + c.Path(), SHA1: strings.Repeat("a", 40), Size: 10}
	return result
}
func osRule(t *testing.T, name string) []meta.Rule {
	t.Helper()
	raw := []byte("[{\"action\":\"allow\",\"os\":{\"name\":\"" + name + "\"}}]")
	var rules []meta.Rule
	if err := json.Unmarshal(raw, &rules); err != nil {
		t.Fatal(err)
	}
	return rules
}
func TestOfficialLegacyLibraryRolesPreserveHybridJavaButDropNativeContainer(t *testing.T) {
	for _, os := range []string{"osx", "windows", "linux"} {
		t.Run(os, func(t *testing.T) {
			version := officialLegacy1122(t)
			plan, err := Build(version, meta.Platform{OS: os, Arch: "amd64"})
			if err != nil {
				t.Fatal(err)
			}
			if plan.Graphics.Family != "lwjgl2" {
				t.Fatal(plan.Graphics)
			}
			speech := false
			for _, library := range plan.Libraries {
				c, _ := meta.ParseCoordinate(library.Coordinate)
				if library.Role == LibraryClasspath && (c.Artifact == "lwjgl-platform" || c.Artifact == "jinput-platform") {
					t.Fatal("native-only container became Java classpath", library)
				}
				if library.Role == LibraryNativeArchive && slices.Contains(plan.Classpath, library.File.Path) {
					t.Fatal("extracted native leaked into classpath")
				}
				if c.Group == "com.mojang" && c.Artifact == "text2speech" && library.Role == LibraryClasspath {
					speech = true
				}
			}
			for _, file := range plan.Files {
				if strings.HasSuffix(file.Path, "lwjgl-platform-2.9.4-nightly-20150209.jar") {
					t.Fatal("unused packaging container still downloaded")
				}
			}
			if !speech {
				t.Fatal("hybrid Java artifact removed")
			}
		})
	}
}
func TestModernNativeJarsRemainSelectedClasspathComponents(t *testing.T) {
	for _, id := range []string{"1.20.4", "1.21.1"} {
		plan, err := Build(fixture(t, id), meta.Platform{OS: "osx", Arch: "amd64"})
		if err != nil {
			t.Fatal(err)
		}
		if plan.Graphics.Family != "lwjgl3" {
			t.Fatal(plan.Graphics)
		}
		natives := 0
		for _, library := range plan.Libraries {
			if library.Role == LibraryNativeClasspath {
				natives++
				if !slices.Contains(plan.Classpath, library.File.Path) {
					t.Fatal("modern native omitted from classpath")
				}
			}
		}
		if natives != len(plan.NativeJARs) || natives < 7 || len(plan.Natives) != 0 {
			t.Fatal(natives, len(plan.NativeJARs), len(plan.Natives))
		}
	}
}
func TestConditionalVersionsAreSelectedBeforeConflictChecking(t *testing.T) {
	version := fixture(t, "1.20.4")
	first := selectedFixtureLibrary(t, "example:conditional:1")
	first.Rules = osRule(t, "osx")
	second := selectedFixtureLibrary(t, "example:conditional:2")
	second.Rules = osRule(t, "windows")
	version.Libraries = []meta.Library{first, second}
	for _, os := range []string{"osx", "windows"} {
		plan, err := Build(version, meta.Platform{OS: os, Arch: "amd64"})
		if err != nil {
			t.Fatal(err)
		}
		if len(plan.Libraries) != 1 || len(plan.Classpath) != 2 {
			t.Fatal(plan.Libraries, plan.Classpath)
		}
		expected := first.Name
		if os == "windows" {
			expected = second.Name
		}
		if plan.Libraries[0].Coordinate != expected {
			t.Fatal(plan.Libraries)
		}
	}
	// Reusing the source metadata for a second target must retain both variants.
	if len(version.Libraries) != 2 || len(version.Libraries[0].Rules) != 1 {
		t.Fatal("source declarations mutated")
	}
	second.Rules = nil
	first.Rules = nil
	version.Libraries = []meta.Library{first, second}
	if _, err := Build(version, meta.Platform{OS: "osx", Arch: "amd64"}); err == nil {
		t.Fatal("two active versions silently reordered")
	}
}
func TestSelectedRoleDedupAndNativeExclusionOwnership(t *testing.T) {
	version := fixture(t, "1.20.4")
	library := selectedFixtureLibrary(t, "example:java:1")
	version.Libraries = []meta.Library{library, library}
	plan, err := Build(version, meta.Platform{OS: "osx", Arch: "amd64"})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Libraries) != 1 || len(plan.Classpath) != 2 {
		t.Fatal("same identity role duplicated")
	}
	native := meta.Library{Name: "example:extract:1", Natives: map[string]string{"osx": "natives-osx"}}
	native.Downloads.Classifiers = map[string]meta.Download{"natives-osx": {URL: "https://libraries.minecraft.net/native.jar", SHA1: strings.Repeat("b", 40), Size: 10}}
	native.Extract.Exclude = []string{"META-INF/"}
	version.Libraries = []meta.Library{native}
	plan, err = Build(version, meta.Platform{OS: "osx", Arch: "amd64"})
	if err != nil {
		t.Fatal(err)
	}
	native.Extract.Exclude[0] = "changed/"
	if plan.Libraries[0].Exclude[0] != "META-INF/" || plan.Natives[0].Exclude[0] != "META-INF/" {
		t.Fatal("source exclusion slice escaped")
	}
	plan.Libraries[0].Exclude[0] = "other/"
	if plan.Natives[0].Exclude[0] != "META-INF/" {
		t.Fatal("role and projection share mutable exclusions")
	}
}
func TestRoleEvidenceUsesSelectedCoordinatesAndNeverClaimsLoaded(t *testing.T) {
	version := officialLegacy1122(t)
	plan, err := Build(version, meta.Platform{OS: "osx", Arch: "amd64"})
	if err != nil {
		t.Fatal(err)
	}
	var snapshot diagnostics.Snapshot
	ctx := diagnostics.WithObserver(t.Context(), snapshot.Apply)
	reportPlan(ctx, plan)
	if err := snapshot.Validate(); err != nil {
		t.Fatal(err)
	}
	native := false
	for _, artifact := range snapshot.Artifacts {
		if strings.HasPrefix(artifact.Coordinate, "org.lwjgl.lwjgl:lwjgl-platform:") {
			native = true
			if artifact.Role != "native-archive" || artifact.State != "planned" || artifact.Published {
				t.Fatal(artifact)
			}
		}
	}
	if !native {
		t.Fatal("selected native coordinate evidence missing")
	}
}
