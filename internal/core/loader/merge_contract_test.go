package loader

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"

	"mcl/internal/core/install"
	"mcl/internal/core/meta"
)

func mergeContractLibrary(t *testing.T, name string) meta.Library {
	t.Helper()
	c, err := meta.ParseCoordinate(name)
	if err != nil {
		t.Fatal(err)
	}
	lib := meta.Library{Name: name}
	lib.Downloads.Artifact = &meta.Download{Path: c.Path(), URL: "https://maven.fabricmc.net/" + c.Path(), SHA1: strings.Repeat("b", 40), Size: 100}
	return lib
}
func mergeContractRule(t *testing.T, target string) []meta.Rule {
	t.Helper()
	var rules []meta.Rule
	if err := json.Unmarshal([]byte("[{\"action\":\"allow\",\"os\":{\"name\":\""+target+"\"}}]"), &rules); err != nil {
		t.Fatal(err)
	}
	return rules
}
func mergeContractProfile(t *testing.T) (Profile, meta.Version) {
	t.Helper()
	_, base, _, _ := legacyFixture(t)
	p := Profile{Request: Request{Type: "fabric", Version: "0.16.0", GameVersion: base.ID, BaseSHA1: strings.Repeat("a", 40)}, ID: "fabric-fixture", MainClass: "net.fabricmc.loader.impl.launch.knot.KnotClient", Libraries: []meta.Library{mergeContractLibrary(t, "net.fabricmc:fabric-loader:0.16.0")}}
	return p, base
}

func TestMergePreservesConditionalVersionsUntilFinalSelection(t *testing.T) {
	p, base := mergeContractProfile(t)
	base.Libraries = []meta.Library{mergeContractLibrary(t, "com.google.guava:guava:99.0-fixture")}
	first := mergeContractLibrary(t, "com.google.guava:guava:17.0")
	first.Rules = mergeContractRule(t, "windows")
	second := mergeContractLibrary(t, "com.google.guava:guava:21.0")
	second.Rules = mergeContractRule(t, "linux")
	p.Libraries = append(p.Libraries, first, second)
	merged, err := p.Merge(base)
	if err != nil {
		t.Fatal("mutually exclusive versions rejected before rules", err)
	}
	for _, target := range []string{"windows", "linux", "osx"} {
		plan, err := install.Build(merged, meta.Platform{OS: target, Arch: "amd64"})
		if err != nil {
			t.Fatal(err)
		}
		var names []string
		for _, lib := range plan.Libraries {
			if strings.HasPrefix(lib.Coordinate, "com.google.guava:guava:") {
				names = append(names, lib.Coordinate)
			}
		}
		want := []string(nil)
		if target == "windows" {
			want = []string{first.Name}
		} else if target == "linux" {
			want = []string{second.Name}
		}
		if !slices.Equal(names, want) {
			t.Fatalf("%s selected %v, want %v; parent must not reappear when a child group is inactive", target, names, want)
		}
	}
	// Same profile is now an actual overlap on Linux, not a new merge policy.
	p.Libraries[1].Rules = mergeContractRule(t, "linux")
	merged, err = p.Merge(base)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := install.Build(merged, meta.Platform{OS: "linux", Arch: "amd64"}); err == nil {
		t.Fatal("simultaneous versions silently chose first/newest")
	}
}

func TestLoaderOverridesExactModulesWithoutVersionOrdering(t *testing.T) {
	p, base := mergeContractProfile(t)
	// Explicit synthetic version inversions: test ownership, not compatibility.
	base.Libraries = []meta.Library{
		mergeContractLibrary(t, "org.ow2.asm:asm:99.0-fixture"),
		mergeContractLibrary(t, "org.ow2.asm:asm:99.0-fixture:sources"),
		mergeContractLibrary(t, "org.ow2.asm:asm:99.0-fixture@zip"),
		mergeContractLibrary(t, "org.ow2.asm:asm-debug-all:5.2"),
		mergeContractLibrary(t, "com.google.guava:guava:99.0-fixture"),
		mergeContractLibrary(t, "org.scala-lang:scala-library:99.0-fixture"),
	}
	p.Libraries = append(p.Libraries,
		mergeContractLibrary(t, "org.ow2.asm:asm:5.0.3"),
		mergeContractLibrary(t, "com.google.guava:guava:17.0"),
		mergeContractLibrary(t, "org.scala-lang:scala-library:2.11.1"),
	)
	merged, err := p.Merge(base)
	if err != nil {
		t.Fatal(err)
	}
	var actual []string
	for _, lib := range merged.Libraries {
		actual = append(actual, lib.Name)
	}
	want := []string{p.Libraries[0].Name, p.Libraries[1].Name, p.Libraries[2].Name, p.Libraries[3].Name, base.Libraries[1].Name, base.Libraries[2].Name, base.Libraries[3].Name}
	if !slices.Equal(actual, want) {
		t.Fatalf("fixed Loader order/classifier/extension distinction lost: %v", actual)
	}
}

func TestLoaderMergeSnapshotDoesNotAliasEitherSource(t *testing.T) {
	p, base := mergeContractProfile(t)
	p.Arguments = meta.Arguments{Game: []meta.Argument{{Values: []string{"--loader-fixture", "value"}, Rules: mergeContractRule(t, "linux")}}, JVM: []meta.Argument{{Values: []string{"-Dloader=fixture"}, Rules: []meta.Rule{{Action: "allow", Features: map[string]bool{"fixture": true}}}}}}
	p.Libraries[0].Rules = mergeContractRule(t, "linux")
	p.Libraries[0].Extract.Exclude = []string{"META-INF/"}
	base.Libraries = []meta.Library{mergeContractLibrary(t, "example:base:1")}
	base.Libraries[0].Natives = map[string]string{"windows": "natives-windows"}
	base.Libraries[0].Downloads.Classifiers = map[string]meta.Download{"natives-windows": {URL: "https://libraries.minecraft.net/fixture.jar", SHA1: strings.Repeat("c", 40), Size: 50}}
	base.RuntimeLookups = nil
	beforeBase := base.Clone()
	beforeProfileBytes, _ := json.Marshal(p)
	merged, err := p.Merge(base)
	if err != nil {
		t.Fatal(err)
	}
	if merged.LoaderProfile != p.Fingerprint() {
		t.Fatal("merged profile identity lost")
	}
	merged.Arguments.Game[len(merged.Arguments.Game)-1].Values[0] = "changed"
	merged.Arguments.Game[len(merged.Arguments.Game)-1].Rules[0].OS.Name = "osx"
	merged.Arguments.JVM[0].Values[0] = "changed"
	merged.Arguments.JVM[len(merged.Arguments.JVM)-1].Rules[0].Features["fixture"] = false
	merged.Libraries[0].Downloads.Artifact.SHA1 = strings.Repeat("d", 40)
	merged.Libraries[0].Rules[0].OS.Name = "osx"
	merged.Libraries[0].Extract.Exclude[0] = "changed"
	merged.Libraries[1].Downloads.Artifact.Size = 200
	merged.Libraries[1].Natives["windows"] = "changed"
	merged.Libraries[1].Downloads.Classifiers["natives-windows"] = meta.Download{}
	merged.Downloads["client"] = meta.Download{}
	merged.Sources = append(merged.Sources, meta.ManifestEntry{ID: "changed"})
	if base.Logging.Client != nil {
		merged.Logging.Client.File.ID = "changed"
	}
	if !reflect.DeepEqual(base, beforeBase) {
		t.Fatal("result aliases official base")
	}
	afterProfileBytes, _ := json.Marshal(p)
	if string(afterProfileBytes) != string(beforeProfileBytes) {
		t.Fatal("result aliases cached Loader profile")
	}
	fresh, err := p.Merge(base)
	if err != nil {
		t.Fatal(err)
	}
	fixed := fresh.Clone()
	p.Arguments.Game[0].Values[0] = "reverse change"
	p.Libraries[0].Downloads.Artifact.Size = 300
	base.Libraries[0].Natives["windows"] = "reverse change"
	if !reflect.DeepEqual(fresh, fixed) {
		t.Fatal("source changes mutate already built effective version")
	}
}

func TestLoaderCannotBeMergedTwice(t *testing.T) {
	p, base := mergeContractProfile(t)
	merged, err := p.Merge(base)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Merge(merged); err == nil {
		t.Fatal("second Loader layer appended duplicate game/JVM template")
	}
	if _, err := p.Merge(merged.Clone()); err == nil {
		t.Fatal("snapshot lost Loader layer identity")
	}
	if _, err := p.Merge(base); err != nil {
		t.Fatal("independent re-read base must still work", err)
	}
}
