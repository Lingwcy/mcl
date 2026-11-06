package install

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha1"
	"fmt"
	"os"
	"strings"
	"testing"

	"mcl/internal/core/download"
	"mcl/internal/core/meta"
)

func TestCleanupNativeGenerationsPreservesCurrentAndUserFiles(t *testing.T) {
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	keep := "natives/ready-" + rand.Text()
	old := "natives/ready-" + rand.Text()
	temporary := "natives/.prepare-" + rand.Text()
	for _, name := range []string{keep, old, temporary, "natives/user-files"} {
		if err := root.MkdirAll(name, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := CleanupNatives(root, "natives", keep); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{old, temporary} {
		if _, err := root.Stat(name); !os.IsNotExist(err) {
			t.Fatalf("abandoned directory remains: %s", name)
		}
	}
	for _, name := range []string{keep, "natives/user-files"} {
		if _, err := root.Stat(name); err != nil {
			t.Fatalf("kept directory lost: %s", name)
		}
	}
	if err := CleanupNatives(root, "natives", "somewhere/else"); err == nil {
		t.Fatal("unconfined keep path accepted")
	}
}

func nativeFixture(t *testing.T, root *os.Root, entries map[string]string, symlink bool) NativeArchive {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, body := range entries {
		header := &zip.FileHeader{Name: name, Method: zip.Deflate}
		if symlink {
			header.SetMode(os.ModeSymlink | 0o777)
		}
		f, err := w.CreateHeader(header)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := root.WriteFile("native.jar", buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return NativeArchive{File: download.File{Path: "native.jar", Download: meta.Download{URL: "https://example.com/native.jar", Size: int64(buf.Len()), SHA1: fmt.Sprintf("%x", sha1.Sum(buf.Bytes()))}}, Exclude: []string{"META-INF/", "skip/"}}
}

func TestPrepareNativeFilesAndExclusions(t *testing.T) {
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	archive := nativeFixture(t, root, map[string]string{"lib/native.dylib": "native data", "META-INF/MANIFEST.MF": "ignored", "skip/file": "ignored"}, false)
	directory, err := PrepareNatives(t.Context(), root, "instances/a/natives", []NativeArchive{archive})
	if err != nil {
		t.Fatal(err)
	}
	data, err := root.ReadFile(directory + "/lib/native.dylib")
	if err != nil || string(data) != "native data" {
		t.Fatalf("native data %q: %v", data, err)
	}
	if _, err := root.Stat(directory + "/META-INF"); !os.IsNotExist(err) {
		t.Fatal("metadata extracted")
	}
	if _, err := root.Stat(directory + "/skip"); !os.IsNotExist(err) {
		t.Fatal("excluded entry extracted")
	}
}

func TestRejectUnsafeNativeArchivesAndCleanTemporary(t *testing.T) {
	for _, tc := range []struct {
		name    string
		entries map[string]string
		symlink bool
	}{
		{"traversal", map[string]string{"../escape": "bad"}, false},
		{"absolute", map[string]string{"/escape": "bad"}, false},
		{"backslash", map[string]string{`..\escape`: "bad"}, false},
		{"link", map[string]string{"lib.so": "../../escape"}, true},
		{"case collision", map[string]string{"Lib.so": "one", "lib.so": "two"}, false},
		{"expansion", map[string]string{"lib.so": strings.Repeat("x", (64<<20)+1)}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, err := os.OpenRoot(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			defer root.Close()
			archive := nativeFixture(t, root, tc.entries, tc.symlink)
			if _, err := PrepareNatives(t.Context(), root, "natives", []NativeArchive{archive}); err == nil {
				t.Fatal("invalid archive accepted")
			}
			f, err := root.Open("natives")
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			entries, _ := f.ReadDir(-1)
			if len(entries) != 0 {
				t.Fatalf("temporary extraction retained: %v", entries)
			}
		})
	}
}

func TestCanceledOrCorruptNativesNeverPublish(t *testing.T) {
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	archive := nativeFixture(t, root, map[string]string{"lib.so": "native"}, false)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := PrepareNatives(ctx, root, "natives", []NativeArchive{archive}); err == nil {
		t.Fatal("canceled preparation succeeded")
	}
	if err := root.WriteFile("native.jar", []byte("bad"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := PrepareNatives(t.Context(), root, "natives", []NativeArchive{archive}); err == nil {
		t.Fatal("corrupted archive accepted")
	}
}
