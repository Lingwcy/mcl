package install

import (
	"errors"

	"mcl/internal/core/meta"
)

var ErrNativeArchitecture = errors.New("游戏本地库不支持目标架构")

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
			return host, err
		}
		return compatible, nil
	}
	return host, nil
}
