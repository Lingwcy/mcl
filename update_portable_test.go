package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestPromotePortableKeepsTheUsersFileName(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "mcl-0.0.5-windows-amd64.exe")
	write(t, exe, "old")
	write(t, filepath.Join(dir, archiveExe), "new")

	if err := promotePortable(exe, 42); err != nil {
		t.Fatal(err)
	}
	if got := read(t, exe); got != "new" {
		t.Fatalf("executable holds %q, want the update", got)
	}
	if _, err := os.Stat(filepath.Join(dir, archiveExe)); !os.IsNotExist(err) {
		t.Fatalf("the downloaded mcl.exe should be renamed away: %v", err)
	}
	aside := filepath.Join(dir, ".mcl-0.0.5-windows-amd64.exe.old-42")
	if got := read(t, aside); got != "old" {
		t.Fatalf("old executable aside holds %q", got)
	}
	// MyGo deletes these leftovers at the next launch.
	if matched, _ := filepath.Match(".*.old-*", filepath.Base(aside)); !matched {
		t.Fatalf("%s does not match MyGo's leftover pattern", aside)
	}
}

func TestPromotePortableWithoutDownloadChangesNothing(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "mcl-0.0.5-windows-arm64.exe")
	write(t, exe, "old")
	if err := promotePortable(exe, 1); err == nil {
		t.Fatal("expected an error without a downloaded mcl.exe")
	}
	if got := read(t, exe); got != "old" {
		t.Fatalf("executable changed to %q", got)
	}
}

func TestPromotePortableInPlaceIsANoOp(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "MCL.EXE")
	write(t, exe, "updated in place")
	if err := promotePortable(exe, 1); err != nil {
		t.Fatal(err)
	}
	if got := read(t, exe); got != "updated in place" {
		t.Fatalf("executable changed to %q", got)
	}
}

func TestRemoveAddedResourcesKeepsExistingFiles(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "Icon.png"), "the user's")
	before := existingNames(dir)
	for _, name := range shippedResources {
		if !strings.EqualFold(name, "icon.png") {
			write(t, filepath.Join(dir, name), "unpacked")
		}
	}
	write(t, filepath.Join(dir, "notes.txt"), "unrelated")

	removeAddedResources(dir, before)
	entries, _ := os.ReadDir(dir)
	var left []string
	for _, e := range entries {
		left = append(left, e.Name())
	}
	slices.Sort(left)
	if want := []string{"Icon.png", "notes.txt"}; !slices.Equal(left, want) {
		t.Fatalf("left %v, want %v", left, want)
	}
}

func TestShippedResourcesMatchTheResourcesDirectory(t *testing.T) {
	entries, err := os.ReadDir("resources")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if e.IsDir() {
			// Platform directories (windows, windows-amd64, ...) would ship
			// more files that the portable cleanup must know about.
			t.Fatalf("resources/%s: update shippedResources for directories", e.Name())
		}
		names = append(names, e.Name())
	}
	if !slices.Equal(names, shippedResources) {
		t.Fatalf("resources holds %v, shippedResources lists %v", names, shippedResources)
	}
}
