package platform

import "golang.org/x/sys/windows"

func hostArchitecture() (string, string) {
	var process, native uint16
	if err := windows.IsWow64Process2(windows.CurrentProcess(), &process, &native); err != nil {
		return "", ""
	}
	arch := ""
	switch native {
	case 0x8664:
		arch = "amd64"
	case 0xaa64:
		arch = "arm64"
	case 0x14c:
		arch = "386"
	}
	if arch == "" {
		return "", ""
	}
	return arch, "windows-native-machine"
}
func desktopEvidence(_ func(string) string) (string, string) {
	return "win32-unchecked", "win32-pid-window"
}
