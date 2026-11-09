package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func makeExistingRoot(t *testing.T, parent string) string {
	t.Helper()
	directory := filepath.Join(parent, ".minecraft")
	if err := os.MkdirAll(filepath.Join(directory, "versions", "1.20.4"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "versions", "1.20.4", "1.20.4.json"), []byte(`{"id":"1.20.4","mainClass":"net.minecraft.client.main.Main"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	return directory
}
func TestExistingSourceRegistrationNeverAdoptsOrMutatesForeignRoots(t *testing.T) {
	data, parent := t.TempDir(), t.TempDir()
	directory := makeExistingRoot(t, parent)
	store := NewExistingGamesStore(data)
	value, err := store.Add(t.Context(), parent)
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := filepath.EvalSymlinks(directory)
	if err != nil {
		t.Fatal(err)
	}
	if value.Path != canonical {
		t.Fatal("single immediate Minecraft child not resolved explicitly")
	}
	if _, err := store.Add(t.Context(), directory); err == nil {
		t.Fatal("duplicate canonical root was registered")
	}
	report, err := store.Scan(t.Context(), value.ID)
	if err != nil || len(report.Candidates) != 1 {
		t.Fatal("read-only scan failed", report, err)
	}
	if err := store.Remove(t.Context(), value.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(directory, "versions", "1.20.4", "1.20.4.json")); err != nil {
		t.Fatal("removing discovery registration deleted original instance", err)
	}
	for _, name := range []string{"mcl-games-v1", "owner.json", ".mcl-games-init.lock", "existing-sources.lock", "existing-sources.json"} {
		if _, err := os.Stat(filepath.Join(directory, name)); !os.IsNotExist(err) {
			t.Fatal("source root received launcher state", name)
		}
	}
	if _, err := os.Stat(filepath.Join(data, "existing-sources.json")); err != nil {
		t.Fatal("registration wasn't saved in app data")
	}
}
func TestExistingSourceCorruptRegistryIsNotSilentlyReplaced(t *testing.T) {
	data := t.TempDir()
	store := NewExistingGamesStore(data)
	name := filepath.Join(data, "existing-sources.json")
	raw := []byte("{not-json")
	if err := os.WriteFile(name, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.List(t.Context()); err == nil {
		t.Fatal("corruption ignored")
	}
	directory := makeExistingRoot(t, t.TempDir())
	if _, err := store.Add(t.Context(), directory); err == nil {
		t.Fatal("new source overwrote corrupt config")
	}
	actual, _ := os.ReadFile(name)
	if string(actual) != string(raw) {
		t.Fatal("original corrupt config changed")
	}
}
func TestExistingSourceMissingDiskAndOverlapsAreNotFallbacks(t *testing.T) {
	store := NewExistingGamesStore(t.TempDir())
	directory := makeExistingRoot(t, t.TempDir())
	source, err := store.Add(t.Context(), directory)
	if err != nil {
		t.Fatal(err)
	}
	child := filepath.Join(directory, "nested")
	if err := os.MkdirAll(filepath.Join(child, "versions"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Add(t.Context(), child); err == nil {
		t.Fatal("nested overlapping source accepted")
	}
	if err := os.Rename(directory, directory+"-offline"); err != nil {
		t.Fatal(err)
	}
	report, err := store.Scan(t.Context(), source.ID)
	if err != nil || len(report.Problems) == 0 || len(report.Candidates) != 0 {
		t.Fatal("missing disk was hidden or replaced with fallback")
	}
	if _, err := os.Stat(directory); !os.IsNotExist(err) {
		t.Fatal("scan recreated an unavailable source")
	}
}

func TestExistingSourceRejectsUncreatedDataDirectoryThroughAlias(t *testing.T) {
	parent := t.TempDir()
	directory := makeExistingRoot(t, parent)
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(directory, alias); err != nil {
		t.Skip("symlink privilege unavailable", err)
	}
	store := NewExistingGamesStore(filepath.Join(alias, "new-launcher-data"))
	if _, err := store.Add(t.Context(), directory); err == nil {
		t.Fatal("source registration created launcher data inside its own read-only source")
	}
	if _, err := os.Stat(filepath.Join(directory, "new-launcher-data")); !os.IsNotExist(err) {
		t.Fatal("source directory was modified before overlap rejection")
	}
}
