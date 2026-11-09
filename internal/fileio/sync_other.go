//go:build !darwin && !linux && !windows

package fileio

import (
	"errors"
	"os"
)

func SyncDirectory(root *os.Root) error { return errors.New("此平台尚未实现目录提交同步") }
