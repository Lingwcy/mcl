package app

import (
	"context"
	"errors"

	"mcl/internal/core/java"
	"mcl/internal/core/meta"
)

func (s *JavaService) EnsureProcessor(ctx context.Context, minimum int, target meta.Platform, preferred []string, report func(java.Progress)) (java.Runtime, error) {
	requirement := java.ProcessorRequirement(minimum, target)
	if fixed, ok := java.RequirementFromContext(ctx); ok && fixed.Purpose == "processor" {
		var err error
		requirement, err = requirement.Intersect(fixed)
		if err != nil {
			return java.Runtime{}, err
		}
	}
	return s.EnsureRequirement(ctx, requirement, preferred, report)
}
func processorDownloadMajor(minimum int) (int, error) {
	requirement := java.ProcessorRequirement(minimum, meta.Platform{OS: "linux", Arch: "amd64"})
	majors, err := requirement.DownloadMajors()
	if err != nil {
		return 0, err
	}
	if len(majors) == 0 {
		return 0, errors.New("没有适用的稳定安装工具Java")
	}
	return majors[0], nil
}
func matchingProcessorJava(environments []JavaEnvironment, minimum int, target meta.Platform, preferred []string) (java.Runtime, bool) {
	return matchingRequirement(environments, java.ProcessorRequirement(minimum, target), preferred)
}
