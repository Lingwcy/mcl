package process

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

func childNice(pid int) (int, error) {
	// Linux's raw getpriority syscall returns 20-nice, unlike libc and Darwin.
	value, err := unix.Getpriority(unix.PRIO_PROCESS, pid)
	return 20 - value, err
}

// Called only by the single-use gate, with the exec goroutine OS-thread locked.
// The calling thread may not be the thread-group leader that the parent set.
// exec inherits the caller's nice; otherwise Go scheduling could undo the user's
// selected priority. New JVM threads inherit this value after exec.
func inheritGatePriority() error {
	nice, err := childNice(os.Getpid())
	if err != nil {
		return err
	}
	if err := unix.Setpriority(unix.PRIO_PROCESS, 0, nice); err != nil {
		return err
	}
	actual, err := childNice(0)
	if err != nil {
		return err
	}
	if actual != nice {
		return errors.New("game handoff priority was not inherited")
	}
	return nil
}
