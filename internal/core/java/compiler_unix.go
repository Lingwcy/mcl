//go:build !windows

package java

import "os"

func compilerName() string                     { return "javac" }
func compilerExecutable(info os.FileInfo) bool { return info.Mode().Perm()&0111 != 0 }
