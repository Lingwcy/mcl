package platform

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
)

// JavaCandidates reads registration locations, never recursively scans a disk.
// Every candidate is independently inspected by the application service.
func JavaCandidates(ctx context.Context) ([]string, error) {
	paths := registeredJava(ctx)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if home := os.Getenv("JAVA_HOME"); filepath.IsAbs(home) {
		paths = append(paths, JavaExecutable(home))
	}
	if executable, err := exec.LookPath(javaBinary); err == nil && filepath.IsAbs(executable) {
		paths = append(paths, executable)
	}
	for _, directory := range filepath.SplitList(os.Getenv("PATH")) {
		if filepath.IsAbs(directory) {
			paths = append(paths, filepath.Join(directory, javaBinary))
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		for _, pattern := range []string{
			".sdkman/candidates/java/*", ".jenv/versions/*", ".jdks/*",
			".asdf/installs/java/*", ".local/share/mise/installs/java/*",
		} {
			matches, _ := filepath.Glob(filepath.Join(home, filepath.FromSlash(pattern)))
			for _, directory := range matches {
				paths = append(paths, JavaExecutable(directory))
			}
		}
	}
	// Running Apple's placeholder without an installed JVM can open an OS
	// installation prompt. Registered Java homes provide the real binaries.
	return slices.DeleteFunc(paths, func(path string) bool { return path == systemJavaShim }), nil
}

// JavaExecutable accepts either a Java home or a macOS JDK bundle.
func JavaExecutable(home string) string {
	bundle := filepath.Join(home, "Contents", "Home")
	if info, err := os.Stat(bundle); err == nil && info.IsDir() {
		home = bundle
	}
	return filepath.Join(home, "bin", javaBinary)
}
