package install

import (
	"errors"
	"strings"

	"mcl/internal/core/archive"
	"mcl/internal/core/meta"
)

// Generic legacy classifiers bundle both x86 bitnesses; they never imply ARM.
// Explicitly named CPU suffixes must match the requested target exactly.
func legacyNativeMatches(classifier string, p meta.Platform) (bool, error) {
	base := strings.TrimPrefix(classifier, "natives-")
	if base == classifier {
		return false, errors.New("unsupported legacy native classifier")
	}
	arch := ""
	for _, suffix := range []struct{ name, arch string }{
		{"-x86_64", "amd64"}, {"-aarch64", "arm64"}, {"-arm64", "arm64"}, {"-amd64", "amd64"}, {"-x64", "amd64"}, {"-x86", "386"}, {"-64", "amd64"}, {"-32", "386"},
	} {
		if strings.HasSuffix(base, suffix.name) {
			base = strings.TrimSuffix(base, suffix.name)
			arch = suffix.arch
			break
		}
	}
	if base == "macos" {
		base = "osx"
	}
	if base != "windows" && base != "linux" && base != "osx" {
		return false, errors.New("unknown legacy native operating system")
	}
	target := meta.NormalizeArch(p.Arch)
	if arch == "" {
		return base == p.OS && (target == "amd64" || target == "386"), nil
	}
	return base == p.OS && arch == target, nil
}

func validateNativeExclusions(exclusions []string) error {
	if len(exclusions) > 128 {
		return errors.New("native extract exclusion list exceeds 128")
	}
	for _, prefix := range exclusions {
		name := strings.TrimSuffix(prefix, "/")
		if !archive.SafePath(name) {
			return errors.New("unsafe native extract exclusion prefix")
		}
	}
	return nil
}
