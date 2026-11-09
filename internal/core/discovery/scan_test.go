package discovery

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeVersion(t *testing.T, root, folder string, value map[string]any, jar bool) {
	t.Helper()
	directory := filepath.Join(root, "versions", folder)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, folder+".json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	if jar {
		if err := os.WriteFile(filepath.Join(directory, folder+".jar"), []byte("not-an-executable-jar"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}
func scanFixture(t *testing.T, directory string) Report {
	t.Helper()
	root, err := os.OpenRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	report, err := Scan(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	return report
}
func candidateNamed(t *testing.T, report Report, name string) Candidate {
	t.Helper()
	for _, row := range report.Candidates {
		if row.Folder == name {
			return row
		}
	}
	t.Fatal("candidate missing", name)
	return Candidate{}
}
func TestDiscoveryKeepsForeignFilesAndIncompleteCandidates(t *testing.T) {
	directory := t.TempDir()
	for _, child := range []string{"assets", "libraries"} {
		if err := os.Mkdir(filepath.Join(directory, child), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	writeVersion(t, directory, "1.20.4", map[string]any{"id": "1.20.4", "mainClass": "net.minecraft.client.main.Main"}, true)
	writeVersion(t, directory, "Fabric instance", map[string]any{"id": "different-document-id", "inheritsFrom": "1.20.4", "mainClass": "net.fabricmc.loader.impl.launch.knot.KnotClient", "libraries": []any{map[string]any{"name": "net.fabricmc:fabric-loader:0.16.0"}}}, false)
	writeVersion(t, directory, "Missing client", map[string]any{"id": "Missing client", "mainClass": "net.minecraft.client.main.Main"}, false)
	original := filepath.Join(directory, "versions", "Fabric instance", "Fabric instance.json")
	before, err := os.ReadFile(original)
	if err != nil {
		t.Fatal(err)
	}
	report := scanFixture(t, directory)
	child := candidateNamed(t, report, "Fabric instance")
	if child.DocumentID != "different-document-id" || child.GameVersion != "1.20.4" || len(child.Loaders) != 1 || child.Loaders[0].Kind != "fabric" || !strings.HasSuffix(child.ClientPath, filepath.Join("1.20.4", "1.20.4.jar")) {
		t.Fatal("parent or loader evidence was lost", child)
	}
	if child.Status != "incomplete" {
		t.Fatal("ID mismatch was hidden")
	}
	if candidateNamed(t, report, "Missing client").Status != "incomplete" {
		t.Fatal("missing client disappeared or looked runnable")
	}
	after, _ := os.ReadFile(original)
	if string(before) != string(after) {
		t.Fatal("discovery rewrote source metadata")
	}
	if _, err := os.Stat(filepath.Join(directory, "versions", "different-document-id")); !os.IsNotExist(err) {
		t.Fatal("discovery renamed a foreign instance")
	}
	again := scanFixture(t, directory)
	if candidateNamed(t, again, "Fabric instance").ID != child.ID {
		t.Fatal("stable source candidate identity changed")
	}
}
func TestDiscoveryCyclesUnsafeParentsAndCorruptionAreVisible(t *testing.T) {
	directory := t.TempDir()
	writeVersion(t, directory, "a", map[string]any{"id": "a", "inheritsFrom": "b"}, false)
	writeVersion(t, directory, "b", map[string]any{"id": "b", "inheritsFrom": "a"}, false)
	writeVersion(t, directory, "unsafe", map[string]any{"id": "unsafe", "inheritsFrom": "../outside"}, false)
	writeVersion(t, directory, "broken", map[string]any{"id": "broken"}, false)
	if err := os.WriteFile(filepath.Join(directory, "versions", "broken", "broken.json"), []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	report := scanFixture(t, directory)
	if !strings.Contains(strings.Join(candidateNamed(t, report, "a").Problems, " "), "循环") {
		t.Fatal("inheritance cycle was hidden")
	}
	for _, name := range []string{"unsafe", "broken"} {
		if candidateNamed(t, report, name).Status != "unreadable" {
			t.Fatal("unsafe/unreadable entry omitted", name)
		}
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := Scan(ctx, root); !errors.Is(err, context.Canceled) {
		t.Fatal("scan did not honor cancellation", err)
	}
}
func TestDiscoveryRefusesVersionDirectoryAndMetadataLinks(t *testing.T) {
	directory, outside := t.TempDir(), t.TempDir()
	writeVersion(t, directory, "linked-json", map[string]any{"id": "linked-json"}, false)
	foreign := filepath.Join(outside, "foreign.json")
	if err := os.WriteFile(foreign, []byte(`{"id":"foreign"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(directory, "versions", "linked-json", "linked-json.json")
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(foreign, target); err != nil {
		t.Skip("symlink privilege unavailable", err)
	}
	if err := os.Symlink(outside, filepath.Join(directory, "versions", "linked-folder")); err != nil {
		t.Fatal(err)
	}
	report := scanFixture(t, directory)
	for _, name := range []string{"linked-json", "linked-folder"} {
		if candidateNamed(t, report, name).Status != "unreadable" {
			t.Fatal("foreign link was followed", name)
		}
	}
}
