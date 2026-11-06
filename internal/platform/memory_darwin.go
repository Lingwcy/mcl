package platform

import "golang.org/x/sys/unix"

// PhysicalMemory returns the host's installed RAM in bytes.
func PhysicalMemory() (uint64, error) {
	return unix.SysctlUint64("hw.memsize")
}
