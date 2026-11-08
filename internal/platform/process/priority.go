package process

import (
	"errors"
	"fmt"
	"os"

	"mcl/internal/core/launch"
)

var errPriorityIdentityChanged = errors.New("设置游戏优先级时进程身份已改变，已取消交接")

// prepareChildPriority runs inside attach's beforeResume callback. Production
// always uses a gated/suspended child which has not been reaped, so its PID
// cannot be reused between identity validation and the native priority call.
// The function neither requests elevation nor adjusts a reopened/foreign game.
func prepareChildPriority(pid int, priority launch.ProcessPriority, register func(launch.ProcessIdentity) error, inspect func(int) (launch.ProcessIdentity, error), apply func(launch.ProcessIdentity, launch.ProcessPriority) error) (string, error) {
	if err := priority.Validate(); err != nil {
		return "", err
	}
	if register == nil {
		if priority != launch.PriorityNormal {
			return "", errors.New("自定义游戏优先级需要受控身份交接")
		}
		return "", nil
	}
	identity, err := inspect(pid)
	if err != nil {
		return "", err
	}
	if err := identity.Validate(); err != nil {
		return "", err
	}
	if identity.PID != pid {
		return "", errPriorityIdentityChanged
	}
	if err := register(identity); err != nil {
		return "", err
	}
	if err := apply(identity, priority); err != nil {
		if errors.Is(err, errPriorityIdentityChanged) {
			return "", err
		}
		reason := "系统未接受设置"
		if errors.Is(err, os.ErrPermission) {
			reason = "系统权限不足"
		}
		return fmt.Sprintf("游戏优先级「%s」未能确认生效（%s），游戏继续运行", priority.Label(), reason), nil
	}
	return "", nil
}
