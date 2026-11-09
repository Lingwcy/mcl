package app

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"mcl/internal/core/java"
	"mcl/internal/core/meta"
)

// JavaResolver runs entirely in the preparation worker. Preferred paths are
// hints, never a reason to launch with the wrong major, architecture or an EA.
type JavaResolver func(context.Context, int, meta.Platform, []string, func(java.Progress)) (java.Runtime, error)

func (s *JavaService) Ensure(ctx context.Context, major int, target meta.Platform, preferred []string, report func(java.Progress)) (java.Runtime, error) {
	if major < 8 {
		return java.Runtime{}, errors.New("当前启动管线不支持 Java 8 之前的运行环境")
	}
	emit := func(stage string) {
		if report != nil {
			report(java.Progress{Stage: stage})
		}
	}
	emit("java-discovery")
	inventory, err := s.inventory(ctx, preferred)
	if err != nil {
		return java.Runtime{}, fmt.Errorf("发现可用 Java: %w", err)
	}
	if runtime, ok := matchingJava(inventory.Environments, major, target, preferred); ok {
		return runtime, nil
	}
	if err := ctx.Err(); err != nil {
		return java.Runtime{}, err
	}
	emit("java-catalog")
	catalog, err := s.Catalog(ctx, major, target)
	if err != nil {
		return java.Runtime{}, fmt.Errorf("自动安装 Java %d: %w", major, err)
	}
	var selected *java.Package
	for _, p := range catalog.Packages {
		if p.Major != major || p.OS != target.OS || p.Arch != meta.NormalizeArch(target.Arch) {
			continue
		}
		if err := p.Validate(); err != nil {
			return java.Runtime{}, err
		}
		// Prefer the smaller runtime; a JDK is suitable when no JRE is published.
		if selected == nil || p.Kind == "jre" && selected.Kind != "jre" {
			copy := p
			selected = &copy
		}
	}
	if selected == nil {
		return java.Runtime{}, fmt.Errorf("官方目录暂无适合 %s/%s 的 Java %d；可在 Java 管理中添加对应环境后重试", target.OS, target.Arch, major)
	}
	emit("java-download")
	environment, err := s.Install(ctx, *selected, func(p java.Progress) {
		if report == nil {
			return
		}
		stage := "java-download"
		if p.Stage == "解压并检查 Java" {
			stage = "java-extract"
			p.Bytes, p.Total = 0, 0
		}
		report(java.Progress{Stage: stage, Bytes: p.Bytes, Total: p.Total})
	})
	if err != nil {
		return java.Runtime{}, fmt.Errorf("自动安装 Java %d: %w", major, err)
	}
	if err := environment.Runtime.Check(major, target); err != nil {
		return java.Runtime{}, err
	}
	return environment.Runtime, ctx.Err()
}

func matchingJava(environments []JavaEnvironment, major int, target meta.Platform, preferred []string) (java.Runtime, bool) {
	var candidates []java.Runtime
	for _, environment := range environments {
		runtime := environment.Runtime
		if runtime.Major == major && runtime.Check(major, target) == nil {
			candidates = append(candidates, runtime)
		}
	}
	// Honor compatible explicit instance/global choices, otherwise choose the
	// newest installed patch of the metadata's exact recommended major.
	for _, path := range preferred {
		if path == "" {
			continue
		}
		for _, candidate := range candidates {
			if candidate.Executable == path {
				return candidate, true
			}
		}
	}
	slices.SortFunc(candidates, func(a, b java.Runtime) int {
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

func javaVersionNumbers(version string) []uint64 {
	var numbers []uint64
	for _, part := range strings.FieldsFunc(version, func(r rune) bool { return !unicode.IsDigit(r) }) {
		value, _ := strconv.ParseUint(part, 10, 64)
		numbers = append(numbers, value)
	}
	return numbers
}
