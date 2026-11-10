package app

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"mcl/internal/core/diagnostics"
	"mcl/internal/core/download"
	"mcl/internal/core/java"
	"mcl/internal/core/meta"
)

type JavaResolver func(context.Context, int, meta.Platform, []string, func(java.Progress)) (java.Runtime, error)

func (s *JavaService) Ensure(ctx context.Context, major int, target meta.Platform, preferred []string, report func(java.Progress)) (java.Runtime, error) {
	requirement := java.GameRequirement(major, target)
	if fixed, ok := java.RequirementFromContext(ctx); ok {
		var err error
		requirement, err = requirement.Intersect(fixed)
		if err != nil {
			return java.Runtime{}, err
		}
	}
	return s.EnsureRequirement(ctx, requirement, preferred, report)
}

// EnsureRequirement applies the SAME established requirement to explicit
// preferences, discovered environments, catalog candidates and the final Probe.
func (s *JavaService) EnsureRequirement(ctx context.Context, requirement java.Requirement, preferred []string, report func(java.Progress)) (java.Runtime, error) {
	requirement = requirement.Clone()
	if err := requirement.Validate(); err != nil {
		return java.Runtime{}, err
	}
	if err := ctx.Err(); err != nil {
		return java.Runtime{}, err
	}
	ctx = context.WithValue(ctx, runtimePurposeKey{}, requirement.Purpose == "processor")
	ctx, evidence := diagnostics.Start(ctx, "java", requirement.Summary())
	defer evidence.End()
	deliver := func(p java.Progress) {
		if requirement.Purpose == "processor" {
			p.Stage = "processor-" + p.Stage
		}
		if report != nil {
			report(p)
		}
	}
	emit := func(stage string) { deliver(java.Progress{Stage: stage}) }
	emit("java-discovery")
	inventory, err := s.inventory(ctx, preferred)
	if err != nil {
		return java.Runtime{}, fmt.Errorf("发现可用Java: %w", err)
	}
	if runtime, ok := matchingRequirement(inventory.Environments, requirement, preferred); ok {
		if err := ctx.Err(); err != nil {
			return java.Runtime{}, err
		}
		evidence.Work()
		reportRuntimeFacts(ctx, runtime)
		return runtime, nil
	}
	majors, err := requirement.DownloadMajors()
	if err != nil {
		return java.Runtime{}, err
	}
	if len(majors) == 0 {
		return java.Runtime{}, errors.New("固定Java需求没有受支持的稳定下载主版本")
	}
	var failures []error
	for _, major := range majors {
		for _, provider := range java.Providers() {
			emit("java-catalog")
			catalog, err := s.catalog.CatalogFor(ctx, provider, major, requirement.Target)
			if ctx.Err() != nil {
				return java.Runtime{}, ctx.Err()
			}
			if err != nil {
				failures = append(failures, fmt.Errorf("%s目录: %w", provider, err))
				if len(catalog.Packages) == 0 {
					continue
				}
			}
			packages := slices.Clone(catalog.Packages)
			slices.SortStableFunc(packages, func(a, b java.Package) int {
				if a.Kind != b.Kind {
					if a.Kind == "jre" {
						return -1
					}
					return 1
				}
				av, ae := a.ReleaseVersion()
				bv, be := b.ReleaseVersion()
				if ae == nil && be == nil {
					return bv.Compare(av)
				}
				if ae == nil {
					return -1
				}
				if be == nil {
					return 1
				}
				return 0
			})
			for _, p := range packages {
				if p.Major != major {
					continue
				}
				if err := requirement.CheckPackage(p); err != nil {
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
					deliver(java.Progress{Stage: stage, Bytes: p.Bytes, Total: p.Total})
				})
				if ctx.Err() != nil {
					return java.Runtime{}, ctx.Err()
				}
				if err != nil {
					file := p.DownloadFile()
					if observeErr := download.Observe(ctx, download.Event{Key: download.FileKey(file), Path: file.Path, State: "skipped", Total: p.Size}); observeErr != nil {
						return java.Runtime{}, observeErr
					}
					failures = append(failures, fmt.Errorf("%s安装: %w", provider, err))
					continue
				}
				if err := requirement.CheckRuntime(environment.Runtime); err != nil {
					failures = append(failures, err)
					continue
				}
				evidence.Work()
				reportRuntimeFacts(ctx, environment.Runtime)
				return environment.Runtime, nil
			}
		}
	}
	if len(failures) > 0 {
		return java.Runtime{}, fmt.Errorf("各官方来源未能满足固定Java需求: %w", errors.Join(failures...))
	}
	return java.Runtime{}, errors.New("官方目录暂无满足固定需求的稳定Java")
}

func matchingJava(environments []JavaEnvironment, major int, target meta.Platform, preferred []string) (java.Runtime, bool) {
	return matchingRequirement(environments, java.GameRequirement(major, target), preferred)
}
func matchingRequirement(environments []JavaEnvironment, requirement java.Requirement, preferred []string) (java.Runtime, bool) {
	if requirement.Validate() != nil {
		return java.Runtime{}, false
	}
	var candidates []java.Runtime
	for _, environment := range environments {
		if requirement.CheckRuntime(environment.Runtime) == nil {
			candidates = append(candidates, environment.Runtime)
		}
	}
	for _, path := range preferred {
		if path != "" {
			for _, candidate := range candidates {
				if candidate.Executable == path {
					return candidate, true
				}
			}
		}
	}
	slices.SortFunc(candidates, func(a, b java.Runtime) int {
		if requirement.Purpose == "processor" && a.Major != b.Major {
			if a.Major < b.Major {
				return -1
			}
			return 1
		}
		if comparison := java.CompareRuntimeVersions(b, a); comparison != 0 {
			return comparison
		}
		return strings.Compare(a.Executable, b.Executable)
	})
	if len(candidates) == 0 {
		return java.Runtime{}, false
	}
	return candidates[0], true
}
