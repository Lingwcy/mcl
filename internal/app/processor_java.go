package app

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"mcl/internal/core/diagnostics"
	"mcl/internal/core/java"
	"mcl/internal/core/meta"
)

// EnsureProcessor never changes the game's selected Java, instance preference,
// or global default. A tool needs a compatible stable JVM, not the game major.
// Missing tools use the same serialized, pinned Temurin installer as game Java.
func (s *JavaService) EnsureProcessor(ctx context.Context, minimum int, target meta.Platform, preferred []string, report func(java.Progress)) (java.Runtime, error) {
	ctx = context.WithValue(ctx, runtimePurposeKey{}, true)
	ctx, evidence := diagnostics.Start(ctx, "java", "processor-runtime")
	defer evidence.End()
	if err := ctx.Err(); err != nil {
		return java.Runtime{}, err
	}
	major, err := processorDownloadMajor(minimum)
	if err != nil {
		return java.Runtime{}, err
	}
	if report != nil {
		report(java.Progress{Stage: "processor-java-discovery"})
	}
	inventory, err := s.inventory(ctx, preferred)
	if err != nil {
		return java.Runtime{}, fmt.Errorf("发现安装工具Java: %w", err)
	}
	if runtime, ok := matchingProcessorJava(inventory.Environments, minimum, target, preferred); ok {
		reportRuntimeFacts(ctx, runtime)
		evidence.Work()
		return runtime, ctx.Err()
	}
	return s.Ensure(ctx, major, target, preferred, func(p java.Progress) {
		if report != nil {
			p.Stage = "processor-" + p.Stage
			report(p)
		}
	})
}

func processorDownloadMajor(minimum int) (int, error) {
	if minimum < 8 || minimum > 25 {
		return 0, errors.New("安装工具Java要求超出当前8–25支持范围")
	}
	if minimum == 8 {
		return 8, nil
	}
	for _, major := range []int{17, 21, 25} {
		if major >= minimum {
			return major, nil
		}
	}
	return 0, errors.New("没有适用的官方稳定安装工具Java")
}

func matchingProcessorJava(environments []JavaEnvironment, minimum int, target meta.Platform, preferred []string) (java.Runtime, bool) {
	var candidates []java.Runtime
	for _, environment := range environments {
		runtime := environment.Runtime
		compatible := runtime.Major >= minimum && runtime.Major <= 25
		if minimum == 8 {
			compatible = runtime.Major == 8
		}
		if compatible && runtime.Check(minimum, target) == nil {
			candidates = append(candidates, runtime)
		}
	}
	for _, path := range preferred {
		for _, candidate := range candidates {
			if path != "" && candidate.Executable == path {
				return candidate, true
			}
		}
	}
	slices.SortFunc(candidates, func(a, b java.Runtime) int {
		if a.Major != b.Major {
			return a.Major - b.Major
		}
		if comparison := slices.Compare(javaVersionNumbers(b.Version), javaVersionNumbers(a.Version)); comparison != 0 {
			return comparison
		}
		return strings.Compare(a.Executable, b.Executable)
	})
	if len(candidates) == 0 {
		return java.Runtime{}, false
	}
	return candidates[0], true
}
