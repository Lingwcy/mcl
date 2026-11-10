package app

import (
	"context"
	"mcl/internal/core/java"
	"mcl/internal/core/meta"
)

func fixedGameRequirement(ctx context.Context, major int, target meta.Platform) (java.Requirement, error) {
	requirement := java.GameRequirement(major, target)
	if fixed, ok := java.RequirementFromContext(ctx); ok {
		return requirement.Intersect(fixed)
	}
	return requirement, requirement.Validate()
}
func resolveRequiredGameJava(ctx context.Context, resolve JavaResolver, major int, requirement java.Requirement, preferred []string, report func(java.Progress)) (java.Runtime, error) {
	child, err := java.WithRequirement(ctx, requirement)
	if err != nil {
		return java.Runtime{}, err
	}
	runtime, err := resolve(child, major, requirement.Target, preferred, report)
	if err != nil {
		return java.Runtime{}, err
	}
	if err := requirement.CheckRuntime(runtime); err != nil {
		return java.Runtime{}, err
	}
	return runtime, nil
}
