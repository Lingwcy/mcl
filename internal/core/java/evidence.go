package java

import (
	"strings"

	"mcl/internal/core/diagnostics"
)

func runtimeVM(name string) string {
	name = strings.ToLower(name)
	switch {
	case strings.Contains(name, "openj9"):
		return "openj9"
	case strings.Contains(name, "hotspot") || strings.Contains(name, "openjdk"):
		return "hotspot"
	}
	return "unknown"
}
func (r Runtime) DiagnosticFacts(tool bool) diagnostics.Facts {
	version := r.Version
	if r.RuntimeVersion != "" {
		if _, err := r.NormalizedVersion(); err == nil {
			version = r.RuntimeVersion
		}
	}
	if (diagnostics.Facts{JavaVersion: version}).Validate() != nil {
		version = "unknown"
	}
	vm := r.VM
	if vm == "" {
		vm = "unknown"
	}
	if tool {
		return diagnostics.Facts{ToolJavaVersion: version, ToolJavaArch: r.Arch, ToolJavaVM: vm, ToolCompiler: r.Compiler}
	}
	return diagnostics.Facts{JavaVersion: version, JavaArch: r.Arch, JavaVM: vm, Compiler: r.Compiler}
}
