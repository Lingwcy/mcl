package process

import (
	"errors"
	"os"
	"strconv"

	"golang.org/x/sys/windows"
	"mcl/internal/core/launch"
)

func Inspect(pid int) (launch.ProcessIdentity, error) {
	if pid < 1 || pid > 1<<31-1 {
		return launch.ProcessIdentity{}, errors.New("invalid PID")
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, uint32(pid))
	if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
		return launch.ProcessIdentity{}, os.ErrNotExist
	}
	if err != nil {
		return launch.ProcessIdentity{}, err
	}
	defer windows.CloseHandle(h)
	return inspectProcessHandle(pid, h)
}

func inspectProcessHandle(pid int, h windows.Handle) (launch.ProcessIdentity, error) {
	status, err := windows.WaitForSingleObject(h, 0)
	if err != nil {
		return launch.ProcessIdentity{}, err
	}
	if status == windows.WAIT_OBJECT_0 {
		return launch.ProcessIdentity{}, os.ErrNotExist
	}
	var created, exited, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(h, &created, &exited, &kernel, &user); err != nil {
		return launch.ProcessIdentity{}, err
	}
	stamp := uint64(created.HighDateTime)<<32 | uint64(created.LowDateTime)
	if stamp == 0 {
		return launch.ProcessIdentity{}, errors.New("invalid process creation time")
	}
	return launch.ProcessIdentity{PID: pid, OS: "windows", Stamp: strconv.FormatUint(stamp, 10)}, nil
}
