//go:build !darwin && !linux && !windows

package fileio

import (
	"errors"
	"os"
	"time"
)

func SetFileTimes(file *os.File, atime, mtime time.Time) error {
	return errors.New("此平台尚未支持按文件句柄复制时间")
}
