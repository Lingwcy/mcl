package app

import (
	"context"
	"mcl/internal/core/java"
	"mcl/internal/core/meta"
	"testing"
)

func TestFixedJavaRangeAppliesBeforeExplicitPreference(t *testing.T) {
	target := meta.Platform{OS: "linux", Arch: "amd64"}
	maxVersion, err := java.ParseVersion("8u400")
	if err != nil {
		t.Fatal(err)
	}
	requirement := java.GameRequirement(8, target)
	requirement.Constraints = append(requirement.Constraints, java.Constraint{Source: "loader-game", Maximum: &java.Bound{Version: maxVersion, Inclusive: true}})
	environments := []JavaEnvironment{
		{Runtime: java.Runtime{Executable: "newer-preference", Version: "8u442", Major: 8, Arch: "amd64"}},
		{Runtime: java.Runtime{Executable: "compatible", Version: "8u341", Major: 8, Arch: "amd64"}},
	}
	got, ok := matchingRequirement(environments, requirement, []string{"newer-preference"})
	if !ok || got.Executable != "compatible" {
		t.Fatal(got, ok)
	}
	ctx, err := java.WithRequirement(t.Context(), requirement)
	if err != nil {
		t.Fatal(err)
	}
	fixed, err := fixedGameRequirement(ctx, 8, target)
	if err != nil {
		t.Fatal(err)
	}
	resolver := func(context.Context, int, meta.Platform, []string, func(java.Progress)) (java.Runtime, error) {
		return environments[0].Runtime, nil
	}
	if _, err := resolveRequiredGameJava(ctx, resolver, 8, fixed, nil, nil); err == nil {
		t.Fatal("legacy callback bypassed fixed upper bound")
	}
}
func TestInventoryLabelNeverRedirectsProbedExecutable(t *testing.T) {
	service := &JavaService{installer: &java.Installer{Directory: t.TempDir()}}
	runtime := java.Runtime{Executable: "/selected/bin/java", HomeDirectory: "/unverified/other-home", Compiler: "unknown"}
	got := service.environment(runtime)
	if got.Runtime.Executable != runtime.Executable || got.Kind != "Java" {
		t.Fatal(got)
	}
}
func TestJDKRequirementDoesNotAcceptPreferredJRE(t *testing.T) {
	requirement := java.ProcessorRequirement(17, meta.Platform{OS: "linux", Arch: "amd64"})
	requirement.Constraints = append(requirement.Constraints, java.Constraint{Source: "loader-tool", NeedJDK: true})
	env := []JavaEnvironment{
		{Runtime: java.Runtime{Executable: "jre", Version: "17.0.12", Major: 17, Arch: "amd64", Compiler: "absent"}},
		{Runtime: java.Runtime{Executable: "jdk", Version: "17.0.12", Major: 17, Arch: "amd64", Compiler: "present"}},
	}
	runtime, ok := matchingRequirement(env, requirement, []string{"jre"})
	if !ok || runtime.Executable != "jdk" {
		t.Fatal(runtime, ok)
	}
}
