package process

import (
	"errors"
	"os"
)

// Never remove a colliding pre-existing file or a replacement published by
// someone else while the game ran. The instance lease handles app writers;
// this identity check also preserves externally replaced paths.
func removeSupportFile(root *os.Root, name string, identity os.FileInfo) error {
	if identity == nil {
		return nil
	}
	current, err := root.Lstat(name)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !current.Mode().IsRegular() || !os.SameFile(identity, current) {
		return errors.New("启动支持文件已变化，已保留现有文件")
	}
	return root.Remove(name)
}
