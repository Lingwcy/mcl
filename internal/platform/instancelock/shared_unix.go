//go:build darwin || linux

package instancelock

import (
	"errors"
	"golang.org/x/sys/unix"
	"os"
)

func trySharedLock(f *os.File) error {
	err := unix.Flock(int(f.Fd()), unix.LOCK_SH|unix.LOCK_NB)
	if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
		return ErrBusy
	}
	return err
}
