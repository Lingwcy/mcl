package platform

import (
	"context"
	"errors"
	"mcl/internal/app"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var globalMemoryStatusEx = windows.NewLazySystemDLL("kernel32.dll").NewProc("GlobalMemoryStatusEx")

// PhysicalMemory returns the host's installed RAM in bytes.
type physicalMemoryStatus struct {
	Length, Load                                    uint32
	TotalPhysical, AvailablePhysical                uint64
	TotalPageFile, AvailablePageFile                uint64
	TotalVirtual, AvailableVirtual, ExtendedVirtual uint64
}

func readPhysicalMemoryStatus() (physicalMemoryStatus, error) {
	var status physicalMemoryStatus
	status.Length = uint32(unsafe.Sizeof(status))
	ok, _, err := globalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&status)))
	if ok == 0 {
		if err == nil || errors.Is(err, syscall.Errno(0)) {
			err = errors.New("无法读取系统内存统计")
		}
		return status, err
	}
	return status, nil
}
func PhysicalMemory() (uint64, error) {
	status, err := readPhysicalMemoryStatus()
	return status.TotalPhysical, err
}
func NewMemoryProbe() app.MemoryProbe {
	return func(ctx context.Context) (app.MemoryStatus, error) {
		if err := ctx.Err(); err != nil {
			return app.MemoryStatus{}, err
		}
		status, err := readPhysicalMemoryStatus()
		if err != nil {
			return app.MemoryStatus{}, errors.New("无法读取系统内存统计")
		}
		if err := ctx.Err(); err != nil {
			return app.MemoryStatus{}, err
		}
		return app.MemoryStatus{TotalBytes: status.TotalPhysical, AvailableBytes: status.AvailablePhysical}, nil
	}
}
