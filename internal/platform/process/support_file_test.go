package process

import (
	"os"
	"testing"
)

func TestSupportCleanupIsOwnedAndIdempotent(t *testing.T) {
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	f, err := root.OpenFile("private.args", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		t.Fatal(err)
	}
	if _, err := f.Write([]byte("non-secret support data")); err != nil {
		f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if err := removeSupportFile(root, "private.args", info); err != nil {
		t.Fatal(err)
	}
	if _, err := root.Lstat("private.args"); !os.IsNotExist(err) {
		t.Fatal("owned support file remains", err)
	}
	if err := removeSupportFile(root, "private.args", info); err != nil {
		t.Fatal(err)
	}
	if err := root.Mkdir("private.args", 0o700); err != nil {
		t.Fatal(err)
	}
	if err := removeSupportFile(root, "private.args", info); err == nil {
		t.Fatal("directory replacement removed")
	}
	if entry, err := root.Lstat("private.args"); err != nil || !entry.IsDir() {
		t.Fatal("foreign directory lost", err)
	}
}

func TestSupportCleanupPreservesForeignReplacement(t *testing.T) {
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	f, err := root.OpenFile("support.jar", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	info, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	if err := root.Rename("support.jar", "original.jar"); err != nil {
		t.Fatal(err)
	}
	replacement, err := root.OpenFile("support.jar", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	replacement.Close()
	if err := removeSupportFile(root, "support.jar", info); err == nil {
		t.Fatal("replacement identity ignored")
	}
	if _, err := root.Stat("support.jar"); err != nil {
		t.Fatal("foreign replacement removed")
	}
	if err := removeSupportFile(root, "support.jar", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := root.Stat("support.jar"); err != nil {
		t.Fatal("unowned collision removed")
	}
	if err := removeSupportFile(root, "original.jar", info); err != nil {
		t.Fatal(err)
	}
}
