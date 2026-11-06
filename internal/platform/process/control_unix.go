//go:build darwin || linux

package process

import (
	"os/exec"
	"syscall"
)

type group struct{ pid int }

func configure(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} }
func attach(cmd *exec.Cmd, beforeResume func() error) (controller, error) {
	if err := beforeResume(); err != nil {
		return nil, err
	}
	return group{pid: cmd.Process.Pid}, nil
}
func (g group) terminate() error { return syscall.Kill(-g.pid, syscall.SIGTERM) }
func (g group) kill() error      { return syscall.Kill(-g.pid, syscall.SIGKILL) }
func (g group) close()           {}
func (g group) commit() error    { return nil }
