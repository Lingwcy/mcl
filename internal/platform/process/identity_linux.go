package process

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"mcl/internal/core/launch"
)

func Inspect(pid int) (launch.ProcessIdentity, error) {
	if pid < 1 || pid > 1<<31-1 {
		return launch.ProcessIdentity{}, errors.New("invalid PID")
	}
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return launch.ProcessIdentity{}, err
	}
	// comm can contain spaces and parentheses; numeric fields follow its last ).
	end := strings.LastIndexByte(string(data), ')')
	if end < 0 {
		return launch.ProcessIdentity{}, errors.New("invalid proc stat")
	}
	fields := strings.Fields(string(data[end+1:]))
	if len(fields) <= 19 {
		return launch.ProcessIdentity{}, errors.New("short proc stat")
	}
	if fields[0] == "Z" || fields[0] == "X" {
		return launch.ProcessIdentity{}, os.ErrNotExist
	}
	ticks, err := strconv.ParseUint(fields[19], 10, 64)
	if err != nil || ticks == 0 {
		return launch.ProcessIdentity{}, errors.New("invalid process start ticks")
	}
	boot, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if err != nil {
		return launch.ProcessIdentity{}, err
	}
	identity := launch.ProcessIdentity{PID: pid, OS: "linux", Stamp: strings.TrimSpace(string(boot)) + ":" + strconv.FormatUint(ticks, 10)}
	if err := identity.Validate(); err != nil {
		return launch.ProcessIdentity{}, err
	}
	return identity, nil
}
