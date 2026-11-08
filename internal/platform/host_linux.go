package platform

import "golang.org/x/sys/unix"

func operatingSystemVersion() (string, error) {
	var identity unix.Utsname
	if err := unix.Uname(&identity); err != nil {
		return "", err
	}
	// Linux JVM os.version is uname release, not a distribution's VERSION_ID.
	return unix.ByteSliceToString(identity.Release[:]), nil
}
