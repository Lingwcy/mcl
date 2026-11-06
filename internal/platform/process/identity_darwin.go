package process

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
	"mcl/internal/core/launch"
)

func Inspect(pid int) (launch.ProcessIdentity, error) {
	if pid < 1 || pid > 1<<31-1 {
		return launch.ProcessIdentity{}, errors.New("invalid PID")
	}
	info, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil {
		if errors.Is(unix.Kill(pid, 0), unix.ESRCH) {
			return launch.ProcessIdentity{}, os.ErrNotExist
		}
		return launch.ProcessIdentity{}, fmt.Errorf("read process creation identity: %w", err)
	}
	// XNU SZOMB (bsd/sys/proc.h): the process has exited, awaiting reaping.
	if info.Proc.P_stat == 5 {
		return launch.ProcessIdentity{}, os.ErrNotExist
	}
	if info.Proc.P_pid != int32(pid) || info.Proc.P_starttime.Sec <= 0 {
		return launch.ProcessIdentity{}, errors.New("unavailable process creation identity")
	}
	boot, err := unix.Sysctl("kern.bootsessionuuid")
	if err != nil {
		return launch.ProcessIdentity{}, err
	}
	identity := launch.ProcessIdentity{PID: pid, OS: "darwin", Stamp: fmt.Sprintf("%s:%d:%d", boot, info.Proc.P_starttime.Sec, info.Proc.P_starttime.Usec)}
	if err := identity.Validate(); err != nil {
		return launch.ProcessIdentity{}, err
	}
	return identity, nil
}
