// Package java inspects a user-selected JVM and checks its runtime requirements.
package java

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"mcl/internal/core/meta"
)

type Runtime struct {
	Executable    string
	Version       string
	Major         int
	Arch          string
	Vendor        string
	VM            string
	EarlyAccess   bool
	HomeDirectory string
}

// Probe executes exactly the selected executable with fixed diagnostic arguments.
// Output, execution time and inherited pipe lifetime are all bounded.
func Probe(ctx context.Context, executable string) (Runtime, error) {
	executable = strings.TrimSpace(executable)
	if !filepath.IsAbs(executable) {
		return Runtime{}, errors.New("请选择 Java 可执行文件的绝对路径")
	}
	resolved, err := filepath.EvalSymlinks(executable)
	if err != nil {
		return Runtime{}, fmt.Errorf("无法找到 Java 可执行文件: %w", err)
	}
	stat, err := os.Stat(resolved)
	if err != nil || !stat.Mode().IsRegular() {
		return Runtime{}, errors.New("Java 路径不是普通可执行文件")
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, resolved, "-XshowSettings:properties", "-version")
	cmd.Env = Environment(os.Environ())
	cmd.WaitDelay = time.Second
	var output boundedOutput
	cmd.Stdout, cmd.Stderr = &output, &output
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return Runtime{}, fmt.Errorf("Java 检查已取消或超时: %w", ctx.Err())
		}
		return Runtime{}, fmt.Errorf("Java 检查失败: %w", err)
	}
	if output.truncated {
		return Runtime{}, errors.New("Java 诊断输出超过 64 KiB")
	}
	runtime, err := ParseProperties(output.String())
	if err != nil {
		return Runtime{}, err
	}
	runtime.Executable = resolved
	return runtime, nil
}

func ParseProperties(output string) (Runtime, error) {
	properties := make(map[string]string)
	for line := range strings.SplitSeq(output, "\n") {
		key, value, found := strings.Cut(strings.TrimSpace(line), "=")
		if found {
			properties[strings.TrimSpace(key)] = strings.TrimSpace(value)
		}
	}
	version := properties["java.version"]
	number := version
	if strings.HasPrefix(number, "1.") {
		number = strings.TrimPrefix(number, "1.")
	}
	if index := strings.IndexAny(number, ".-+_"); index >= 0 {
		number = number[:index]
	}
	major, err := strconv.Atoi(number)
	arch := meta.NormalizeArch(properties["os.arch"])
	if err != nil || major <= 0 || (arch != "amd64" && arch != "arm64" && arch != "386") {
		return Runtime{}, errors.New("无法从 Java 诊断中识别版本与 CPU 架构")
	}
	return Runtime{
		Version: version, Major: major, Arch: arch, Vendor: properties["java.vendor"], VM: runtimeVM(properties["java.vm.name"]),
		EarlyAccess:   strings.Contains(strings.ToLower(version), "-ea"),
		HomeDirectory: properties["java.home"],
	}, nil
}

func (r Runtime) Check(requiredMajor int, platform meta.Platform) error {
	if r.Major < requiredMajor {
		return fmt.Errorf("该版本需要 Java %d 或更高版本，当前为 Java %s", requiredMajor, r.Version)
	}
	if r.Arch != meta.NormalizeArch(platform.Arch) {
		return fmt.Errorf("Java 架构 %s 与游戏目标架构 %s 不匹配", r.Arch, platform.Arch)
	}
	if r.EarlyAccess {
		return errors.New("当前 Java 是抢先体验版本，请选择稳定版 Java")
	}
	return nil
}

type boundedOutput struct {
	bytes.Buffer
	truncated bool
}

func (b *boundedOutput) Write(data []byte) (int, error) {
	n := len(data)
	remaining := (64 << 10) - b.Len()
	if n > remaining {
		b.truncated = true
		data = data[:remaining]
	}
	b.Buffer.Write(data)
	return n, nil
}
