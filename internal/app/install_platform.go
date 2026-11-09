package app

import (
	"context"
	"errors"
	"fmt"

	"mcl/internal/core/install"
	"mcl/internal/core/meta"
)

func (s *InstallationService) targetPlatform(ctx context.Context, version meta.Version) (meta.Platform, error) {
	target, err := install.SelectPlatform(version, s.platform)
	if err != nil {
		return target, fmt.Errorf("客户端平台兼容性: %w", err)
	}
	if target.Arch != meta.NormalizeArch(s.platform.Arch) {
		if s.CheckExecutionPlatform == nil {
			return target, errors.New("跨架构执行支持检查未配置")
		}
		if err := s.CheckExecutionPlatform(ctx, s.platform, target); err != nil {
			return target, err
		}
	}
	return target, ctx.Err()
}
func (s *InstallationService) resolvedPlatform(resolved resolvedVersion) meta.Platform {
	if resolved.platform.OS != "" {
		return resolved.platform
	}
	return s.platform
}
