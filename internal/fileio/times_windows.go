package fileio

import (
	"golang.org/x/sys/windows"
	"os"
	"time"
)

func SetFileTimes(file *os.File, atime, mtime time.Time) error {
	a, m := windows.NsecToFiletime(atime.UnixNano()), windows.NsecToFiletime(mtime.UnixNano())
	return windows.SetFileTime(windows.Handle(file.Fd()), nil, &a, &m)
}
