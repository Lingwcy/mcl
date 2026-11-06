package platform

import "golang.org/x/sys/unix"

// PhysicalMemory returns the host's installed RAM in bytes.
func PhysicalMemory() (uint64, error) {
	var info unix.Sysinfo_t
	if err := unix.Sysinfo(&info); err != nil {
		return 0, err
	}
	return uint64(info.Totalram) * uint64(info.Unit), nil
}
