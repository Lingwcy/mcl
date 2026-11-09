//go:build !darwin && !linux && !windows

package fileio

import (
	"errors"
	"os"
)

func moveNoReplace(*os.Root, string, *os.Root, string) error {
	return errors.New("atomic no-replace move is unsupported on this platform")
}

func replaceManagedFile(source *os.Root, old string, target *os.Root, name string) error {
	return moveNoReplace(source, old, target, name)
}
