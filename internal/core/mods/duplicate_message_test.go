package mods

import (
	"archive/zip"
	"bytes"
	"errors"
	"strings"
	"testing"
)

func testDescriptorArchive(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var out bytes.Buffer
	writer := zip.NewWriter(&out)
	for name, text := range files {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte(text)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func TestDuplicateGuardNamesBothConflictingFiles(t *testing.T) {
	paths := []string{"mods/aether-1.20.1-1.5.1-neoforge.jar", "mods/aether-1.20.1-1.5.2-neoforge.jar"}
	old := descriptorFile(paths[0], "forge", Mod{ID: "aether", Version: "1.20.1-1.5.1"})
	next := descriptorFile(paths[1], "forge", Mod{ID: "aether", Version: "1.20.1-1.5.2"})
	runtime := Runtime{Loader: "forge", GameVersion: "1.20.1", LoaderVersion: "47.4.13"}
	for _, file := range []File{old, next} {
		if err := GuardTransition(t.Context(), nil, []File{file}, runtime, nil); err != nil {
			t.Fatal("single provider was called duplicate", err)
		}
	}
	for _, files := range [][]File{{old, next}, {next, old}} {
		err := GuardTransition(t.Context(), nil, files, runtime, nil)
		if !errors.Is(err, ErrUnsafeChange) {
			t.Fatal("true duplicate accepted", err)
		}
		for _, name := range paths {
			if !strings.Contains(err.Error(), name) {
				t.Fatal("one conflict file was hidden", err)
			}
		}
	}
	// Turning off one file removes the definite duplicate; the guard does not
	// automatically select a preferred version or mutate either input.
	old.Enabled = false
	if err := GuardTransition(t.Context(), nil, []File{old, next}, runtime, nil); err != nil {
		t.Fatal(err)
	}
}
func TestSameJarAlternativeDescriptorsDoNotCreateDuplicateProvider(t *testing.T) {
	// Explicitly synthetic ZIP. This is a regression control, not the Aether JAR.
	data := testDescriptorArchive(t, map[string]string{
		"META-INF/mods.toml": "modLoader=\"javafml\"\nloaderVersion=\"*\"\n[[mods]]\nmodId=\"aether\"\nversion=\"1.0.0\"\n",
		"fabric.mod.json":    "{\"schemaVersion\":1,\"id\":\"aether\",\"version\":\"1.0.0\"}",
	})
	parsed, err := Read(t.Context(), bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	file := File{Path: "mods/synthetic-multiloader.jar", Enabled: true, Archive: parsed}
	if err := GuardTransition(t.Context(), nil, []File{file}, Runtime{Loader: "forge"}, nil); err != nil {
		t.Fatal(err)
	}
}
