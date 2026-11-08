package process

import (
	"errors"

	"golang.org/x/sys/windows"
	"mcl/internal/core/launch"
)

func priorityClass(priority launch.ProcessPriority) (uint32, error) {
	switch priority {
	case launch.PriorityLow:
		return windows.IDLE_PRIORITY_CLASS, nil
	case launch.PriorityBelowNormal:
		return windows.BELOW_NORMAL_PRIORITY_CLASS, nil
	case launch.PriorityNormal:
		return windows.NORMAL_PRIORITY_CLASS, nil
	case launch.PriorityAboveNormal:
		return windows.ABOVE_NORMAL_PRIORITY_CLASS, nil
	case launch.PriorityHigh:
		return windows.HIGH_PRIORITY_CLASS, nil
	default:
		return 0, priority.Validate()
	}
}
func setChildPriority(identity launch.ProcessIdentity, priority launch.ProcessPriority) error {
	class, err := priorityClass(priority)
	if err != nil {
		return err
	}
	handle, err := windows.OpenProcess(windows.PROCESS_SET_INFORMATION|windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, uint32(identity.PID))
	if err != nil {
		return err
	}
	defer windows.CloseHandle(handle)
	// Creation identity and the scheduling call use the same native handle.
	current, err := inspectProcessHandle(identity.PID, handle)
	if err != nil || current != identity {
		return errors.Join(errPriorityIdentityChanged, err)
	}
	if err := windows.SetPriorityClass(handle, class); err != nil {
		return err
	}
	actual, err := windows.GetPriorityClass(handle)
	if err != nil {
		return err
	}
	if actual != class {
		return errors.New("系统返回的游戏进程优先级与请求不同")
	}
	return nil
}
