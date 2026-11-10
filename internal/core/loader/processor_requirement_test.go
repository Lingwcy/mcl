package loader

import (
	"context"
	"mcl/internal/core/download"
	"mcl/internal/core/java"
	"mcl/internal/core/meta"
	"testing"
)

func TestProcessorRequirementSurvivesResolverMutation(t *testing.T) {
	target := meta.Platform{OS: "linux", Arch: "amd64"}
	maximum, err := java.ParseVersion("17.0.12")
	if err != nil {
		t.Fatal(err)
	}
	requirement := java.ProcessorRequirement(17, target)
	requirement.Constraints = append(requirement.Constraints, java.Constraint{Source: "loader-tool", NeedJDK: true, Maximum: &java.Bound{Version: maximum, Inclusive: true}})
	ctx, err := java.WithRequirement(t.Context(), requirement)
	if err != nil {
		t.Fatal(err)
	}
	installer := NewForgeInstallerWithRequirements(nil, nil, nil, func(_ context.Context, r java.Requirement, _ []string, _ func(java.Progress)) (java.Runtime, error) {
		r.Constraints[len(r.Constraints)-1].Maximum.Version.Numbers[0] = 25
		r.Constraints[len(r.Constraints)-1].NeedJDK = false
		return java.Runtime{Version: "21.0.1", Major: 21, Arch: "amd64", Compiler: "absent"}, nil
	})
	if _, err := installer.processorRuntime(ctx, 17, target, nil, nil); err == nil {
		t.Fatal("mutable callback widened upper bound or JDK capability")
	}
	if requirement.Constraints[len(requirement.Constraints)-1].Maximum.Version.Major() != 17 {
		t.Fatal("caller requirement was mutated")
	}
}

func TestProcessorIncludesMergedProfileGameBaseline(t *testing.T) {
	root, fixed := stagedProcessorJar(t, map[string][]byte{"test/Main.class": syntheticClass(52, 0)})
	target := meta.Platform{OS: "linux", Arch: "amd64"}
	selected, runs := 0, 0
	installer := NewForgeInstallerWithRequirements(nil, nil, func(ctx context.Context, _ string, _ string, _ []string) error {
		requirement, ok := java.RequirementFromContext(ctx)
		if !ok || requirement.Purpose != "processor" {
			t.Fatal("runner lost fixed requirement")
		}
		runs++
		return nil
	}, func(_ context.Context, r java.Requirement, _ []string, _ func(java.Progress)) (java.Runtime, error) {
		selected++
		if err := r.CheckRuntime(java.Runtime{Version: "17.0.12", Major: 17, Arch: "amd64"}); err == nil {
			t.Fatal("merged Java21 profile was ignored")
		}
		return java.Runtime{Executable: root.Name() + "/tool-java", Version: "21.0.8", Major: 21, Arch: "amd64"}, nil
	})
	base := meta.Version{}
	base.Java.Major = 17
	p := preparedProcessor{jar: "tool.jar", classpath: []string{"tool.jar"}, inputs: []string{"tool.jar"}}
	err := installer.runProcessors(t.Context(), root, root, root.Name(), "", target, base, []preparedProcessor{p}, map[string]meta.Download{"tool.jar": fixed}, func(download.Progress) {}, nil, 21)
	if err != nil || selected != 1 || runs != 1 {
		t.Fatal(err, selected, runs)
	}
}

func TestGameContextDoesNotConstrainIndependentProcessorJava(t *testing.T) {
	target := meta.Platform{OS: "linux", Arch: "amd64"}
	ctx, err := java.WithRequirement(t.Context(), java.GameRequirement(8, target))
	if err != nil {
		t.Fatal(err)
	}
	requirement, err := processorRequirement(ctx, 17, target)
	if err != nil {
		t.Fatal(err)
	}
	if err := requirement.CheckRuntime(java.Runtime{Version: "17.0.12", Major: 17, Arch: "amd64"}); err != nil {
		t.Fatal(err)
	}
}
