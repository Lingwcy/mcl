package platform

import (
	"context"
	"path/filepath"
)

const javaBinary = "java"
const systemJavaShim = ""

func registeredJava(context.Context) []string {
	paths := []string{"/etc/alternatives/java"}
	for _, pattern := range []string{"/usr/lib/jvm/*", "/usr/java/*", "/opt/java/*", "/opt/jdk*"} {
		matches, _ := filepath.Glob(pattern)
		for _, home := range matches {
			paths = append(paths, JavaExecutable(home))
		}
	}
	return paths
}
