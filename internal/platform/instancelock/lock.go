// Package instancelock coordinates instance mutation across launcher processes.
package instancelock

import (
	"errors"
	"os"
	"path"
	"path/filepath"
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
func acquire(root *os.Root, name string) (*Lock, error) { return acquireMode(root, name, false) }

// AcquireDownloadCache takes ownership of root. Shared leases cover complete
// download jobs; maintenance requires an exclusive lease on the same inode.
// AcquireDownloadCacheInit serializes creating and validating the owned child.
// The parent lock is never unlinked, just like the per-cache maintenance lock.
func AcquireDownloadCacheInit(root *os.Root) (*Lock, error) {
	return acquire(root, ".mcl-cache-init.lock")
}

func AcquireDownloadCache(root *os.Root, exclusive bool) (*Lock, error) {
	return acquireMode(root, "cache.lock", !exclusive)
}

func acquireMode(root *os.Root, name string, shared bool) (*Lock, error) {
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
	lockFile := tryLock
	if shared {
		lockFile = trySharedLock
	}
	if err := lockFile(f); err != nil {
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

// AcquireGameDirectories serializes location registration and new instance
// publication. The lock inode is retained across processes.
func AcquireGameDirectories(directory string) (*Lock, error) {
	return acquireDirectory(directory, "game-directories.lock")
}

// AcquireGameDirectoryInit takes ownership of the selected parent handle.
func AcquireGameDirectoryInit(root *os.Root) (*Lock, error) {
	return acquire(root, ".mcl-games-init.lock")
}

// AcquireInstanceDeletion is outside the directory sent to Trash, so the final
// move stays serialized after releasing the instance's internal Windows lock.
func AcquireInstanceDeletion(directory, id string) (*Lock, error) {
	if !instance.ValidID(id) {
		return nil, errors.New("invalid instance ID")
	}
	return acquireDirectory(filepath.Join(directory, "instance-deletions"), id+".lock")
}
