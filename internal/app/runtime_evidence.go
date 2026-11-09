package app

import (
	"context"
	"mcl/internal/core/diagnostics"
	"mcl/internal/core/java"
)

type runtimePurposeKey struct{}

func reportRuntimeFacts(ctx context.Context, r java.Runtime) {
	tool, _ := ctx.Value(runtimePurposeKey{}).(bool)
	diagnostics.SetFacts(ctx, r.DiagnosticFacts(tool))
}
