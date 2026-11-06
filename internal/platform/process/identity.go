package process

import (
	"errors"
	"os"
	"runtime"

	"mcl/internal/core/launch"
)

// Running distinguishes a dead/reused PID from an unreadable process. Unknown
// identity or permissions never become permission to modify an occupied game.
func Running(expected launch.ProcessIdentity) (bool, error) {
	if err := expected.Validate(); err != nil {
		return false, err
	}
	if expected.OS != runtime.GOOS {
		return false, errors.New("无法在当前系统核验其他平台的游戏会话")
	}
	current, err := Inspect(expected.PID)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return current == expected, nil
}
