package process

import (
	"errors"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"

	"mcl/internal/core/launch"
)

// GameWindowProbe observes onscreen Quartz window metadata. The PID and bounds
// are available without inspecting window titles or capturing screen contents.
func GameWindowProbe() func(launch.ProcessIdentity) (bool, error) {
	load := sync.OnceValues(loadWindowList)
	return func(expected launch.ProcessIdentity) (bool, error) {
		api, err := load()
		if err != nil {
			return false, err
		}
		visible, err := api.visible(expected.PID)
		if err != nil || !visible {
			return false, err
		}
		// Recheck the registered process identity so a reused PID cannot
		// complete this launch.
		return Running(expected)
	}
}

type quartzRect struct{ x, y, width, height float64 }

type windowListAPI struct {
	copyList                    func(uint32, uint32) uintptr
	release                     func(uintptr)
	count                       func(uintptr) int
	item                        func(uintptr, int) uintptr
	value                       func(uintptr, uintptr) uintptr
	number                      func(uintptr, int, unsafe.Pointer) bool
	rect                        func(uintptr, *quartzRect) bool
	pidKey, boundsKey, alphaKey uintptr
}

func loadWindowList() (*windowListAPI, error) {
	const frameworks = "/System/Library/Frameworks/"
	cg, err := purego.Dlopen(frameworks+"CoreGraphics.framework/CoreGraphics", purego.RTLD_LAZY|purego.RTLD_LOCAL)
	if err != nil {
		return nil, err
	}
	cf, err := purego.Dlopen(frameworks+"CoreFoundation.framework/CoreFoundation", purego.RTLD_LAZY|purego.RTLD_LOCAL)
	if err != nil {
		_ = purego.Dlclose(cg)
		return nil, err
	}
	loaded := false
	defer func() {
		if !loaded {
			_ = purego.Dlclose(cf)
			_ = purego.Dlclose(cg)
		}
	}()
	api := new(windowListAPI)
	for _, fn := range []struct {
		library uintptr
		name    string
		target  any
	}{
		{cg, "CGWindowListCopyWindowInfo", &api.copyList},
		{cg, "CGRectMakeWithDictionaryRepresentation", &api.rect},
		{cf, "CFRelease", &api.release},
		{cf, "CFArrayGetCount", &api.count},
		{cf, "CFArrayGetValueAtIndex", &api.item},
		{cf, "CFDictionaryGetValue", &api.value},
		{cf, "CFNumberGetValue", &api.number},
	} {
		symbol, err := purego.Dlsym(fn.library, fn.name)
		if err != nil {
			return nil, err
		}
		purego.RegisterFunc(fn.target, symbol)
	}
	for _, key := range []struct {
		name   string
		target *uintptr
	}{
		{"kCGWindowOwnerPID", &api.pidKey},
		{"kCGWindowBounds", &api.boundsKey},
		{"kCGWindowAlpha", &api.alphaKey},
	} {
		symbol, err := purego.Dlsym(cg, key.name)
		if err != nil {
			return nil, err
		}
		*key.target = **(**uintptr)(unsafe.Pointer(&symbol)) // exported CFStringRef
	}
	loaded = true
	return api, nil
}

func (api *windowListAPI) visible(pid int) (bool, error) {
	const (
		onScreenOnly    = 1 << 0
		excludeDesktop  = 1 << 4
		cfNumberSInt32  = 3
		cfNumberFloat64 = 6
	)
	list := api.copyList(onScreenOnly|excludeDesktop, 0)
	if list == 0 {
		return false, errors.New("当前桌面会话无法读取窗口列表")
	}
	defer api.release(list)
	for index, count := 0, api.count(list); index < count; index++ {
		window := api.item(list, index)
		var owner int32
		if value := api.value(window, api.pidKey); value == 0 ||
			!api.number(value, cfNumberSInt32, unsafe.Pointer(&owner)) || int(owner) != pid {
			continue
		}
		var alpha float64
		if value := api.value(window, api.alphaKey); value == 0 ||
			!api.number(value, cfNumberFloat64, unsafe.Pointer(&alpha)) || alpha <= 0 {
			continue
		}
		var bounds quartzRect
		// Ignore tiny drawable placeholders while allowing fullscreen and
		// borderless game windows regardless of their Quartz layer.
		if value := api.value(window, api.boundsKey); value != 0 &&
			api.rect(value, &bounds) && bounds.width >= 32 && bounds.height >= 32 {
			return true, nil
		}
	}
	return false, nil
}
