package platform

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

var globalMemoryStatusEx = windows.NewLazySystemDLL("kernel32.dll").NewProc("GlobalMemoryStatusEx")

// PhysicalMemory returns the host's installed RAM in bytes.
func PhysicalMemory() (uint64, error) {
	// MEMORYSTATUSEX has fixed-width fields on both 32- and 64-bit Windows.
	var status struct {
		Length, Load                                    uint32
		TotalPhysical, AvailablePhysical                uint64
		TotalPageFile, AvailablePageFile                uint64
		TotalVirtual, AvailableVirtual, ExtendedVirtual uint64
	}
	status.Length = uint32(unsafe.Sizeof(status))
	ok, _, err := globalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&status)))
	if ok == 0 {
		return 0, err
	}
	return status.TotalPhysical, nil
}
