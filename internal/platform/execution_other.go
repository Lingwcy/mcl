//go:build !darwin

package platform

import (
	"context"
	"errors"
	"mcl/internal/core/meta"
)

func CheckExecutionPlatform(ctx context.Context, host, target meta.Platform) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if host.OS != target.OS || meta.NormalizeArch(host.Arch) != meta.NormalizeArch(target.Arch) {
		return errors.New("系统不支持该执行架构回退")
	}
	return nil
}
