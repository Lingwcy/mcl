package process

import (
	"errors"
	"runtime"
	"unsafe"

	"mcl/internal/core/launch"

	"golang.org/x/sys/windows"
)

var (
	windowUser32        = windows.NewLazySystemDLL("user32.dll")
	windowGetClientRect = windowUser32.NewProc("GetClientRect")
	windowIsIconic      = windowUser32.NewProc("IsIconic")
	windowMonitor       = windowUser32.NewProc("MonitorFromWindow")
	windowDWM           = windows.NewLazySystemDLL("dwmapi.dll")
	windowGetAttribute  = windowDWM.NewProc("DwmGetWindowAttribute")
	// One callback for the lifetime of this package. Per-probe NewCallback
	// allocations cannot be freed and would leak during the observation ticker.
	windowEnumCallback = windows.NewCallback(enumerateProcessWindow)
)

type winClientRect struct{ left, top, right, bottom int32 }
type windowEnumeration struct {
	pid, count int
	found      bool
	failure    error
}

func GameWindowProbe() func(launch.ProcessIdentity) (bool, error) {
	return func(expected launch.ProcessIdentity) (bool, error) {
		return observeProcessWindow(expected, Running, windowsProcessWindow)
	}
}

func windowsProcessWindow(pid int) (bool, error) {
	for _, proc := range []*windows.LazyProc{windowGetClientRect, windowIsIconic, windowGetAttribute, windowMonitor} {
		if proc.Find() != nil {
			return false, errors.New("当前桌面会话不支持窗口可见性观测")
		}
	}
	state := &windowEnumeration{pid: pid}
	err := windows.EnumWindows(windowEnumCallback, unsafe.Pointer(state))
	runtime.KeepAlive(state)
	if state.found {
		return true, nil
	}
	if state.failure != nil {
		return false, state.failure
	}
	if err != nil {
		return false, errors.New("无法枚举当前桌面窗口")
	}
	return false, nil
}

func enumerateProcessWindow(hwnd windows.HWND, argument unsafe.Pointer) uintptr {
	state := (*windowEnumeration)(argument)
	state.count++
	if state.count > 8192 {
		state.failure = errors.New("桌面窗口数量超过观测边界")
		return 0
	}
	var owner uint32
	if _, err := windows.GetWindowThreadProcessId(hwnd, &owner); err != nil || int(owner) != state.pid {
		return 1
	}
	if !windows.IsWindowVisible(hwnd) {
		return 1
	}
	iconic, _, _ := windowIsIconic.Call(uintptr(hwnd))
	if iconic != 0 {
		return 1
	}
	monitor, _, _ := windowMonitor.Call(uintptr(hwnd), 0) // MONITOR_DEFAULTTONULL: require intersection with a display.
	if monitor == 0 {
		return 1
	}
	var cloaked uint32
	result, _, _ := windowGetAttribute.Call(uintptr(hwnd), 14, uintptr(unsafe.Pointer(&cloaked)), unsafe.Sizeof(cloaked)) // DWMWA_CLOAKED
	if int32(result) < 0 {
		state.failure = errors.New("无法判断游戏窗口的桌面可见性")
		return 1
	}
	var rect winClientRect
	ok, _, _ := windowGetClientRect.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&rect)))
	if ok == 0 {
		state.failure = errors.New("无法读取游戏窗口客户区边界")
		return 1
	}
	if visibleClientWindow(state.pid, int(owner), true, false, cloaked != 0, int64(rect.right)-int64(rect.left), int64(rect.bottom)-int64(rect.top)) {
		state.found = true
		return 0
	}
	return 1
}
