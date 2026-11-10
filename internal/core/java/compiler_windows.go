package java

import "os"

func compilerName() string                  { return "javac.exe" }
func compilerExecutable(_ os.FileInfo) bool { return true }
