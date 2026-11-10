package install

import (
	"errors"
	"fmt"

	"mcl/internal/core/meta"
)

var ErrNativeArchitecture = errors.New("游戏本地库不支持目标架构")

// PlatformFallbackError exposes the failed candidate, while retaining both
// causes for diagnostics. An ARM limitation must not hide a broken x64 plan.
type PlatformFallbackError struct{ Native, Fallback error }

func (e *PlatformFallbackError) Error() string {
	return fmt.Sprintf("macOS x86-64 后备安装计划无法生成: %v", e.Fallback)
}
func (e *PlatformFallbackError) Unwrap() []error { return []error{e.Native, e.Fallback} }

// SelectPlatform never mixes architectures or changes version metadata. Only
// Apple's supported x86-64 translation is a candidate, and only after the native
// target fails specifically on architecture and the complete x86-64 plan works.
// The caller must additionally verify OS translation availability.
func SelectPlatform(version meta.Version, host meta.Platform) (meta.Platform, error) {
	host.Arch = meta.NormalizeArch(host.Arch)
	if _, err := Build(version, host); err != nil {
		if host.OS != "osx" || host.Arch != "arm64" || !errors.Is(err, ErrNativeArchitecture) {
			return host, err
		}
		compatible := host
		compatible.Arch = "amd64"
		if _, fallbackErr := Build(version, compatible); fallbackErr != nil {
			return host, &PlatformFallbackError{Native: err, Fallback: fallbackErr}
		}
		return compatible, nil
	}
	return host, nil
}
