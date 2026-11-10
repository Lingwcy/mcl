package platform

import "golang.org/x/sys/unix"

func hostArchitecture() (string, string) {
	var info unix.Utsname
	if err := unix.Uname(&info); err != nil {
		return "", ""
	}
	arch := kernelArchitecture(unix.ByteSliceToString(info.Machine[:]))
	if arch == "" {
		return "", ""
	}
	return arch, "uname-kernel"
}
func desktopEvidence(env func(string) string) (string, string) {
	wayland, x11 := env("WAYLAND_DISPLAY") != "", env("DISPLAY") != ""
	switch {
	case wayland && x11:
		return "wayland-and-x11-env", "unsupported"
	case wayland:
		return "wayland-env", "unsupported"
	case x11:
		return "x11-env", "unsupported"
	default:
		return "no-display-env", "unsupported"
	}
}
