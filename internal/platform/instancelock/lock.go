// Package instancelock coordinates instance mutation across launcher processes.
package instancelock

import (
	"errors"
	"os"
	"path"
	"sync"

	"mcl/internal/core/instance"
)

var ErrBusy = errors.New("instance is occupied by another launcher operation")

type Lock struct {
	file *os.File
	root *os.Root
	once sync.Once
	err  error
}

// Acquire never unlinks the lock file: replacing its inode would let another
// process acquire a different lock for the same instance while this one lives.
func Acquire(root *os.Root, id string) (*Lock, error) {
	if !instance.ValidID(id) {
		return nil, errors.New("invalid instance ID")
	}
	child, err := root.OpenRoot(path.Join("instances", id))
	if err != nil {
		return nil, err
	}
	if err := child.MkdirAll("runtime", 0o700); err != nil {
		child.Close()
		return nil, err
	}
	return acquire(child, "runtime/instance.lock")
}

// AcquireAccounts serializes account writes and credential changes across
// launcher processes using the same application data directory.
func AcquireAccounts(directory string) (*Lock, error) {
	return acquireDirectory(directory, "accounts.lock")
}

// AcquireJava serializes Java registration changes across launcher processes.
func AcquireJava(directory string) (*Lock, error) {
	return acquireDirectory(directory, "java.lock")
}

func acquireDirectory(directory, name string) (*Lock, error) {
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, err
	}
	return acquire(root, name)
}

// acquire takes ownership of root, including on failure.
func acquire(root *os.Root, name string) (*Lock, error) {
	if info, err := root.Lstat(name); err == nil && info.Mode()&os.ModeSymlink != 0 {
		root.Close()
		return nil, errors.New("lock must not be a symlink")
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		root.Close()
		return nil, err
	}
	f, err := openLock(root, name)
	if err != nil {
		root.Close()
		return nil, err
	}
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() != 0 {
		f.Close()
		root.Close()
		return nil, errors.New("lock must be an empty regular file")
	}
	if err := tryLock(f); err != nil {
		f.Close()
		root.Close()
		return nil, err
	}
	return &Lock{file: f, root: root}, nil
}

func (l *Lock) Close() error {
	l.once.Do(func() { l.err = errors.Join(unlock(l.file), l.file.Close(), l.root.Close()) })
	return l.err
}
