package fileio

import (
	"os"

	"golang.org/x/sys/unix"
)

func moveNoReplace(source *os.Root, old string, target *os.Root, name string) error {
	f, err := source.Open(".")
	if err != nil {
		return err
	}
	defer f.Close()
	t, err := target.Open(".")
	if err != nil {
		return err
	}
	defer t.Close()
	return unix.RenameatxNp(int(f.Fd()), old, int(t.Fd()), name, unix.RENAME_EXCL)
}

func replaceManagedFile(source *os.Root, old string, target *os.Root, name string) error {
	src, err := source.Open(".")
	if err != nil {
		return err
	}
	defer src.Close()
	dst, err := target.Open(".")
	if err != nil {
		return err
	}
	defer dst.Close()
	return unix.Renameat(int(src.Fd()), old, int(dst.Fd()), name)
}
