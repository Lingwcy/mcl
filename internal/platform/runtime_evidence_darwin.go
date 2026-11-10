package platform

import "golang.org/x/sys/unix"

func hostArchitecture() (string, string) {
	if arm, err := unix.SysctlUint32("hw.optional.arm64"); err == nil && arm == 1 {
		return "arm64", "sysctl-hardware"
	}
	if machine, err := unix.Sysctl("hw.machine"); err == nil {
		// On translated processes hw.machine may be x86_64. Never use that as
		// Intel-host evidence when the independent ARM capability read failed.
		arm, probeErr := unix.SysctlUint32("hw.optional.arm64")
		if probeErr == nil && arm == 0 && kernelArchitecture(machine) == "amd64" {
			return "amd64", "sysctl-hardware"
		}
	}
	return "", ""
}
func desktopEvidence(_ func(string) string) (string, string) {
	return "quartz-unchecked", "quartz-onscreen"
}
