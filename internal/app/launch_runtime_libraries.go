package app

import (
	"context"
	"os"

	"mcl/internal/core/install"
	"mcl/internal/core/launch"
)

func verifyLaunchLibraries(ctx context.Context, options launch.Options) error {
	plan, err := install.Build(options.Version, options.Platform)
	if err != nil {
		return err
	}
	if len(plan.LocalLibraries) == 0 && len(plan.RuntimeLookups) == 0 {
		return ctx.Err()
	}
	root, err := os.OpenRoot(options.Root)
	if err != nil {
		return err
	}
	defer root.Close()
	return install.VerifyLocalLibraries(ctx, root, plan)
}
