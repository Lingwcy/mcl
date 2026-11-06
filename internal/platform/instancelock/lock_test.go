package instancelock_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"mcl/internal/core/instance"
	"mcl/internal/platform/instancelock"
)

func TestLockAnchorCannotBeReplacedOrRedirected(t *testing.T) {
	directory := t.TempDir()
	id := instance.NewID()
	child := filepath.Join(directory, "instances", id, "runtime")
	if err := os.MkdirAll(child, 0o700); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	lock, err := instancelock.Acquire(root, id)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := instancelock.Acquire(root, id); !errors.Is(err, instancelock.ErrBusy) {
		t.Fatalf("same process bypassed independent file lock: %v", err)
	}
	if err := lock.Close(); err != nil {
		t.Fatal(err)
	}
	if err := lock.Close(); err != nil {
		t.Fatal(err)
	}
	anchor := filepath.Join(child, "instance.lock")
	if _, err := os.Stat(anchor); err != nil {
		t.Fatal("lock inode was removed")
	}
	if err := os.WriteFile(anchor, []byte("preserve this content"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := instancelock.Acquire(root, id); err == nil {
		t.Fatal("nonempty file accepted as lock anchor")
	}
	data, _ := os.ReadFile(anchor)
	if string(data) != "preserve this content" {
		t.Fatal("lock attempt truncated an unrelated file")
	}
	if err := os.Remove(anchor); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(child, "redirected"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("redirected", anchor); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := instancelock.Acquire(root, id); err == nil {
		t.Fatal("symlink lock anchor accepted")
	}
}
