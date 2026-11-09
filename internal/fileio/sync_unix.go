//go:build darwin || linux

package fileio

import "os"

// SyncDirectory orders directory entry publication before a recovery checkpoint.
// It does not turn multiple filesystem operations into a hardware transaction.
func SyncDirectory(root *os.Root) error {
	file, err := root.Open(".")
	if err != nil {
		return err
	}
	defer file.Close()
	return file.Sync()
}
