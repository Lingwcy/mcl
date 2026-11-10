package platform

import (
	"mcl/internal/core/diagnostics"
	"mcl/internal/core/meta"
	"os"
)

// RuntimeEvidence is collected once during assembly. HostArch means the CPU
// reported by the OS (possibly a VM), never a promise of physical bare metal.
// Desktop values describe API/environment hints, not a usable graphics session.
func RuntimeEvidence() diagnostics.Facts {
	arch, source := hostArchitecture()
	f := diagnostics.Facts{LauncherArch: meta.CurrentPlatform().Arch, HostArch: arch, HostArchSource: source}
	f.Desktop, f.WindowObserver = desktopEvidence(os.Getenv)
	if f.Validate() != nil {
		return diagnostics.Facts{LauncherArch: meta.CurrentPlatform().Arch}
	}
	return f
}
func kernelArchitecture(machine string) string {
	switch machine {
	case "x86_64", "amd64":
		return "amd64"
	case "aarch64", "arm64":
		return "arm64"
	case "i386", "i486", "i586", "i686":
		return "386"
	case "armv7l", "armv6l":
		return "arm"
	}
	return ""
}
