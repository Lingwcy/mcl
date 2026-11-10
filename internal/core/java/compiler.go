package java

import (
	"os"
	"path/filepath"
	"strings"
)

// Compiler evidence is structural only: present means a regular executable
// compiler file in the same Java home, not that compilation has been executed.
func inspectCompiler(r Runtime) string {
	if !filepath.IsAbs(r.Executable) {
		return "unknown"
	}
	base := filepath.Dir(filepath.Dir(r.Executable))
	home := base
	reported := r.HomeDirectory
	if reported != "" {
		resolved, err := filepath.EvalSymlinks(reported)
		if err != nil {
			return "unknown"
		}
		if !sameJavaDirectory(resolved, base) && !sameJavaDirectory(resolved, filepath.Join(base, "jre")) {
			return "unknown"
		}
	}
	if r.Major == 8 && filepath.Base(base) == "jre" {
		home = filepath.Dir(base)
	}
	name := filepath.Join(home, "bin", compilerName())
	info, err := os.Stat(name)
	if os.IsNotExist(err) {
		return "absent"
	}
	if err != nil {
		return "unknown"
	}
	if !info.Mode().IsRegular() || !compilerExecutable(info) {
		return "absent"
	}
	resolved, err := filepath.EvalSymlinks(name)
	if err != nil {
		return "unknown"
	}
	relative, err := filepath.Rel(home, resolved)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return "unknown"
	}
	opened, err := os.Stat(resolved)
	if err != nil || !os.SameFile(info, opened) {
		return "unknown"
	}
	return "present"
}

func sameJavaDirectory(a, b string) bool {
	left, le := os.Stat(a)
	right, re := os.Stat(b)
	return le == nil && re == nil && left.IsDir() && right.IsDir() && os.SameFile(left, right)
}
