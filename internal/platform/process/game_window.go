package process

import "mcl/internal/core/launch"

// Only a window observed between two checks of the same registered process
// may complete window discovery. Visibility never proves a usable game menu.
func observeProcessWindow(expected launch.ProcessIdentity, alive func(launch.ProcessIdentity) (bool, error), observe func(int) (bool, error)) (bool, error) {
	if err := expected.Validate(); err != nil {
		return false, err
	}
	running, err := alive(expected)
	if err != nil || !running {
		return false, err
	}
	visible, err := observe(expected.PID)
	if err != nil || !visible {
		return false, err
	}
	return alive(expected)
}

func visibleClientWindow(pid, owner int, visible, minimized, cloaked bool, width, height int64) bool {
	return owner == pid && visible && !minimized && !cloaked && width >= 32 && height >= 32
}
