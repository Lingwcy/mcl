package install

import (
	"crypto/sha1"
	"fmt"
	"os"
	"path"
	"strings"
	"testing"

	"mcl/internal/core/download"
	"mcl/internal/core/meta"
)

func TestRuntimeLookupRechecksActualBytesWithoutDownloading(t *testing.T) {
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	directory := "cache/loader-runtimes/" + strings.Repeat("a", 64) + "/ready-" + strings.Repeat("A", 26) + "/libraries"
	coordinate, _ := meta.ParseCoordinate("net.minecraftforge:forge:1.20.4-49.0.3:client")
	name := path.Join(directory, coordinate.Path())
	data := []byte("synthetic generated client, never executed")
	if err := root.MkdirAll(path.Dir(name), 0700); err != nil {
		t.Fatal(err)
	}
	if err := root.WriteFile(name, data, 0600); err != nil {
		t.Fatal(err)
	}
	file := download.File{Path: name, Download: meta.Download{Path: coordinate.Path(), Size: int64(len(data)), SHA1: fmt.Sprintf("%x", sha1.Sum(data))}}
	plan := Plan{RuntimeLookups: []download.File{file}}
	if err := VerifyLocalLibraries(t.Context(), root, plan); err != nil {
		t.Fatal(err)
	}
	wrong := append([]byte(nil), data...)
	wrong[0] = 'S'
	if err := root.WriteFile(name, wrong, 0600); err != nil {
		t.Fatal(err)
	}
	if err := VerifyLocalLibraries(t.Context(), root, plan); err == nil {
		t.Fatal("changed lookup accepted")
	}
	if err := root.Remove(name); err != nil {
		t.Fatal(err)
	}
	if err := VerifyLocalLibraries(t.Context(), root, plan); err == nil {
		t.Fatal("missing lookup accepted")
	}
	other := path.Join(path.Dir(name), "other.jar")
	if err := root.WriteFile(other, data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := root.Symlink("other.jar", name); err != nil {
		t.Skip(err)
	}
	if err := VerifyLocalLibraries(t.Context(), root, plan); err == nil {
		t.Fatal("symlink lookup accepted")
	}
}
