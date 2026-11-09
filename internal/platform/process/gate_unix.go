//go:build darwin || linux

package process

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"runtime"
	"syscall"
)

func startCommand(executable string, args []string, tracked bool) (*exec.Cmd, func() error, func(), error) {
	if !tracked {
		return exec.Command(executable, args...), func() error { return nil }, func() {}, nil
	}
	self, err := os.Executable()
	if err != nil {
		return nil, nil, nil, err
	}
	data, err := json.Marshal(gateMessage{Executable: executable, Arguments: args})
	if err != nil || len(data) > maxGateBytes {
		return nil, nil, nil, errors.New("launch handoff exceeds supported size")
	}
	cmd := exec.Command(self, gateFlag)
	writer, err := cmd.StdinPipe()
	if err != nil {
		return nil, nil, nil, err
	}
	resume := func() error {
		n, err := writer.Write(data)
		closeErr := writer.Close()
		if err != nil {
			return err
		}
		if n != len(data) {
			return io.ErrShortWrite
		}
		return closeErr
	}
	return cmd, resume, func() { writer.Close() }, nil
}

func gateExec(executable string, args []string) error {
	// Linux nice is per native thread. Keep the exec goroutine on one OS
	// thread and copy the parent's verified leader setting to that thread.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := inheritGatePriority(); err != nil {
		return err
	}
	return syscall.Exec(executable, append([]string{executable}, args...), os.Environ())
}
