package java

import (
	"mcl/internal/core/meta"
	"strings"
	"testing"
)

func fixtureVersion(t *testing.T, s string) Version {
	t.Helper()
	v, err := ParseVersion(s)
	if err != nil {
		t.Fatal(err)
	}
	return v
}
func TestJavaRequirementsIntersectWithoutWidening(t *testing.T) {
	target := meta.Platform{OS: "linux", Arch: "amd64", Features: map[string]bool{"demo": true}}
	base := GameRequirement(8, target)
	limited := Requirement{Purpose: "game", Target: target, Constraints: []Constraint{{Source: "loader-game", Majors: []int{8, 17}, Maximum: &Bound{Version: fixtureVersion(t, "8u400"), Inclusive: true}}}}
	merged, err := base.Intersect(limited)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		version string
		allowed bool
	}{{"1.8.0_341", true}, {"8u400-b99", true}, {"8u402", false}, {"17.0.12", false}} {
		v := fixtureVersion(t, tc.version)
		r := Runtime{Version: tc.version, Major: v.Major(), Arch: "amd64"}
		if got := merged.CheckRuntime(r) == nil; got != tc.allowed {
			t.Fatal(tc.version, got)
		}
	}
	bad := GameRequirement(17, target)
	if _, err := base.Intersect(bad); err == nil {
		t.Fatal("disjoint sets reset to a default")
	}
	cloned := merged.Clone()
	cloned.Target.Features["demo"] = false
	cloned.Constraints[0].Minimum.Version.Numbers[0] = 21
	if !merged.Target.Features["demo"] || merged.Constraints[0].Minimum.Version.Major() != 8 {
		t.Fatal("requirements alias mutable source")
	}
}
func TestExclusiveUpdateAndUnknownBuildBoundaries(t *testing.T) {
	target := meta.Platform{OS: "linux", Arch: "amd64"}
	v := fixtureVersion(t, "8u400")
	impossible := GameRequirement(8, target)
	impossible.Constraints = append(impossible.Constraints, Constraint{Source: "loader-game", Minimum: &Bound{Version: v, Inclusive: false}, Maximum: &Bound{Version: v, Inclusive: true}})
	if err := impossible.Validate(); err == nil {
		t.Fatal("empty interval accepted")
	}
	requirement := GameRequirement(8, target)
	requirement.Constraints = append(requirement.Constraints, Constraint{Source: "loader-game", Maximum: &Bound{Version: fixtureVersion(t, "8u400-b06"), Inclusive: true}})
	for _, tc := range []struct {
		version string
		allowed bool
	}{{"8u400", false}, {"8u400-b06", true}, {"8u400-b07", false}, {"8u341", true}} {
		err := requirement.CheckRuntime(Runtime{Version: tc.version, Major: 8, Arch: "amd64"})
		if (err == nil) != tc.allowed {
			t.Fatal(tc.version, err)
		}
	}
}
func TestJDKAndVMRequirementsFilterBeforeDownloadAndAfterProbe(t *testing.T) {
	requirement := ProcessorRequirement(17, meta.Platform{OS: "linux", Arch: "amd64"})
	requirement.Constraints = append(requirement.Constraints, Constraint{Source: "loader-tool", NeedJDK: true, VMs: []string{"hotspot"}})
	p := Package{Provider: "temurin", Version: "17.0.12+7", Major: 17, Kind: "jre", OS: "linux", Arch: "amd64", VM: "hotspot", SHA256: strings.Repeat("a", 64), Size: 1024, URL: "https://github.com/adoptium/temurin17-binaries/releases/download/jdk-17.0.12%2B7/runtime.tar.gz"}
	if err := requirement.CheckPackage(p); err == nil {
		t.Fatal("JRE offered for a compiler requirement")
	}
	p.Kind = "jdk"
	if err := requirement.CheckPackage(p); err != nil {
		t.Fatal(err)
	}
	r := Runtime{Version: "17.0.12+7", Major: 17, Arch: "amd64", VM: "hotspot", Compiler: "unknown"}
	if err := requirement.CheckRuntime(r); err == nil {
		t.Fatal("unknown compiler capability accepted")
	}
	r.Compiler = "present"
	if err := requirement.CheckRuntime(r); err != nil {
		t.Fatal(err)
	}
	r.VM = "openj9"
	if err := requirement.CheckRuntime(r); err == nil {
		t.Fatal("wrong actual VM accepted")
	}
	if _, err := requirement.Intersect(GameRequirement(17, requirement.Target)); err == nil {
		t.Fatal("game requirement leaked into processor")
	}
}
