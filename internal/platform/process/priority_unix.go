//go:build darwin || linux

package process

import (
	"errors"

	"golang.org/x/sys/unix"
	"mcl/internal/core/launch"
)

func priorityNice(priority launch.ProcessPriority) (int, error) {
	switch priority {
	case launch.PriorityLow:
		return 5, nil
	case launch.PriorityBelowNormal:
		return 1, nil
	case launch.PriorityNormal:
		return 0, nil
	case launch.PriorityAboveNormal:
		return -1, nil
	case launch.PriorityHigh:
		return -5, nil
	default:
		return 0, priority.Validate()
	}
}

// Only called for our own unreaped, blocked child. Inspect again after the
// persistent registration callback, before altering any scheduling attribute.
func setChildPriority(identity launch.ProcessIdentity, priority launch.ProcessPriority) error {
	nice, err := priorityNice(priority)
	if err != nil {
		return err
	}
	current, err := Inspect(identity.PID)
	if err != nil || current != identity {
		return errors.Join(errPriorityIdentityChanged, err)
	}
	if err := unix.Setpriority(unix.PRIO_PROCESS, identity.PID, nice); err != nil {
		return err
	}
	actual, err := childNice(identity.PID)
	if err != nil {
		return err
	}
	if actual != nice {
		return errors.New("系统返回的游戏进程优先级与请求不同")
	}
	return nil
}
