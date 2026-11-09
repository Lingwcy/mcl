package app

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"mcl/internal/core/diagnostics"
	"mcl/internal/core/download"
	"mcl/internal/core/java"
	"mcl/internal/core/meta"
)

// JavaResolver runs entirely in the preparation worker. Preferred paths are
// hints, never a reason to launch with the wrong major, architecture or an EA.
type JavaResolver func(context.Context, int, meta.Platform, []string, func(java.Progress)) (java.Runtime, error)

func (s *JavaService) Ensure(ctx context.Context, major int, target meta.Platform, preferred []string, report func(java.Progress)) (java.Runtime, error) {
	ctx, evidence := diagnostics.Start(ctx, "java", "game-runtime")
	defer evidence.End()
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
		evidence.Work()
		reportRuntimeFacts(ctx, runtime)
		return runtime, nil
	}
	if err := ctx.Err(); err != nil {
		return java.Runtime{}, err
	}

	var failures []error
	for _, provider := range java.Providers() {
		emit("java-catalog")
		catalog, err := s.catalog.CatalogFor(ctx, provider, major, target)
		if ctx.Err() != nil {
			return java.Runtime{}, ctx.Err()
		}
		if err != nil {
			failures = append(failures, fmt.Errorf("%s 目录: %w", provider, err))
			if len(catalog.Packages) == 0 {
				continue
			}
		}
		packages := slices.Clone(catalog.Packages)
		slices.SortStableFunc(packages, func(a, b java.Package) int {
			if a.Kind == b.Kind {
				return 0
			}
			if a.Kind == "jre" {
				return -1
			}
			return 1
		})
		for _, p := range packages {
			if p.Major != major || p.OS != target.OS || p.Arch != meta.NormalizeArch(target.Arch) {
				continue
			}
			if err := p.Validate(); err != nil {
				failures = append(failures, err)
				continue
			}
			emit("java-download")
			environment, err := s.Install(ctx, p, func(p java.Progress) {
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
			if ctx.Err() != nil {
				return java.Runtime{}, ctx.Err()
			}
			if err != nil {
				file := p.DownloadFile()
				if observeErr := download.Observe(ctx, download.Event{Key: download.FileKey(file), Path: file.Path, State: "skipped", Total: p.Size}); observeErr != nil {
					return java.Runtime{}, observeErr
				}
				failures = append(failures, fmt.Errorf("%s 安装: %w", provider, err))
				continue
			}
			if environment.Runtime.Major != major {
				return java.Runtime{}, errors.New("Java 安装结果主版本与请求不符")
			}
			if err := environment.Runtime.Check(major, target); err != nil {
				failures = append(failures, err)
				continue
			}
			evidence.Work()
			reportRuntimeFacts(ctx, environment.Runtime)
			return environment.Runtime, nil
		}
	}
	if len(failures) > 0 {
		return java.Runtime{}, fmt.Errorf("自动准备 Java %d（%s/%s）各来源均未完成: %w", major, target.OS, target.Arch, errors.Join(failures...))
	}
	return java.Runtime{}, fmt.Errorf("各官方发行商暂无 %s/%s 的稳定 Java %d", target.OS, target.Arch, major)
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
