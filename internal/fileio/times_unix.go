//go:build darwin || linux

package fileio

import (
	"golang.org/x/sys/unix"
	"os"
	"time"
)

func SetFileTimes(file *os.File, atime, mtime time.Time) error {
	return unix.Futimes(int(file.Fd()), []unix.Timeval{unix.NsecToTimeval(atime.UnixNano()), unix.NsecToTimeval(mtime.UnixNano())})
}
