package fileio

import (
	"os"
	"strings"
	"testing"
)

func publicationRoots(t *testing.T) (*os.Root, *os.Root) {
	t.Helper()
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { root.Close() })
	for _, name := range []string{"source", "target"} {
		if err := root.Mkdir(name, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	source, err := root.OpenRoot("source")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { source.Close() })
	target, err := root.OpenRoot("target")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { target.Close() })
	return source, target
}

func TestAtomicPublicationShortUnicodeAndLongNames(t *testing.T) {
	for _, name := range []string{"0", "1", "x", "中文", "😀", strings.Repeat("a", 120) + ".jar"} {
		t.Run(name, func(t *testing.T) {
			source, target := publicationRoots(t)
			if err := source.WriteFile("staged", []byte("fixed bytes"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := MoveNoReplace(source, "staged", target, name); err != nil {
				t.Fatal("publish failed", err)
			}
			data, err := target.ReadFile(name)
			if err != nil || string(data) != "fixed bytes" {
				t.Fatal("published bytes lost", err)
			}
			if err := source.WriteFile("second", []byte("foreign bytes"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := MoveNoReplace(source, "second", target, name); err == nil {
				t.Fatal("non-overwrite publication replaced a target")
			}
			data, err = target.ReadFile(name)
			if err != nil || string(data) != "fixed bytes" {
				t.Fatal("existing target changed", err)
			}
			if err := ReplaceManagedFile(source, "second", target, name); err != nil {
				t.Fatal("managed replacement failed", err)
			}
			data, err = target.ReadFile(name)
			if err != nil || string(data) != "foreign bytes" {
				t.Fatal("replacement bytes lost", err)
			}
		})
	}
}

func TestDirectoryPublicationPreservesExistingEmptyTarget(t *testing.T) {
	source, target := publicationRoots(t)
	if err := source.Mkdir("s", 0o700); err != nil {
		t.Fatal(err)
	}
	if err := target.Mkdir("0", 0o700); err != nil {
		t.Fatal(err)
	}
	if err := MoveNoReplace(source, "s", target, "0"); err == nil {
		t.Fatal("existing empty directory replaced")
	}
	if _, err := source.Stat("s"); err != nil {
		t.Fatal("failed move lost source", err)
	}
	if err := MoveNoReplace(source, "s", target, "1"); err != nil {
		t.Fatal("short directory name publication failed", err)
	}
}
