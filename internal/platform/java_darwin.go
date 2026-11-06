package platform

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const javaBinary = "java"
const systemJavaShim = "/usr/bin/java"

func registeredJava(ctx context.Context) []string {
	var paths []string
	task, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	cmd := exec.CommandContext(task, "/usr/libexec/java_home", "-V")
	cmd.WaitDelay = time.Second
	if output, err := cmd.CombinedOutput(); err == nil && len(output) < 64<<10 {
		for line := range strings.SplitSeq(string(output), "\n") {
			// java_home -V lists each registered JVM with its absolute home.
			if at := strings.Index(line, " /"); at >= 0 {
				paths = append(paths, JavaExecutable(strings.TrimSpace(line[at+1:])))
			}
		}
	}
	patterns := []string{
		"/Library/Java/JavaVirtualMachines/*",
		"/System/Library/Java/JavaVirtualMachines/*",
		"/opt/homebrew/opt/openjdk*/libexec/*.jdk",
		"/usr/local/opt/openjdk*/libexec/*.jdk",
	}
	if home, err := os.UserHomeDir(); err == nil {
		patterns = append(patterns, filepath.Join(home, "Library/Java/JavaVirtualMachines/*"))
	}
	for _, pattern := range patterns {
		matches, _ := filepath.Glob(pattern)
		for _, home := range matches {
			paths = append(paths, JavaExecutable(home))
		}
	}
	return paths
}
