package process

import (
	"errors"
	"os/exec"
)

func startCommand(executable string, args []string, _ bool) (*exec.Cmd, func() error, func(), error) {
	// Windows uses CREATE_SUSPENDED instead; attach registers identity before
	// resuming the primary thread, without an extra helper process.
	return exec.Command(executable, args...), func() error { return nil }, func() {}, nil
}
func gateExec(string, []string) error { return errors.New("Windows uses suspended process handoff") }
