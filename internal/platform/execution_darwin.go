package platform

import (
	"context"
	"errors"
	"os"

	"mcl/internal/core/meta"
)

// CheckExecutionPlatform is read-only: never installs Rosetta or prompts for
// system changes. Executing the already selected JVM remains the OS's job.
func CheckExecutionPlatform(ctx context.Context, host, target meta.Platform) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if host.OS == target.OS && meta.NormalizeArch(host.Arch) == meta.NormalizeArch(target.Arch) {
		return nil
	}
	if host.OS != "osx" || target.OS != "osx" || meta.NormalizeArch(host.Arch) != "arm64" || meta.NormalizeArch(target.Arch) != "amd64" {
		return errors.New("系统不支持该执行架构回退")
	}
	for _, name := range []string{"/Library/Apple/usr/libexec/oah/libRosettaRuntime", "/Library/Apple/usr/share/rosetta/rosetta"} {
		info, err := os.Stat(name)
		if err == nil && info.Mode().IsRegular() {
			return nil
		}
	}
	return errors.New("此版本需要 x86-64 Java 和本地库；系统未检测到 Rosetta 2，请安装 Rosetta 2 后重试")
}
