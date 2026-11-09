package storage_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"mcl/internal/app"
	"mcl/internal/storage"
)

func TestGameDirectoriesKeepOldInstancesAndRouteExternalAssets(t *testing.T) {
	root := filepath.Join(t.TempDir(), "应用数据")
	directory := storage.NewGameDirectoryStore(root)
	location, err := directory.Add(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	store := storage.NewInstanceStore(filepath.Join(root, "instances"))
	old, external := testInstance("旧实例"), testInstance("外部实例")
	external.GameDirectoryID = location.ID
	if err := store.Create(t.Context(), old); err != nil {
		t.Fatal(err)
	}
	if err := store.Create(t.Context(), external); err != nil {
		t.Fatal(err)
	}
	assets := storage.NewAssetStore(filepath.Join(root, "instances"))
	for _, value := range []struct{ id, want string }{{old.ID, filepath.Join(root, "instances", old.ID, "game")}, {external.ID, filepath.Join(location.Path, external.ID, "game")}} {
		got, err := assets.Directory(t.Context(), value.id)
		if err != nil || got != value.want {
			t.Fatalf("game route %q: %q %v", value.id, got, err)
		}
	}
	game := filepath.Join(location.Path, external.ID, "game")
	if err := os.Mkdir(filepath.Join(game, "resourcepacks"), 0700); err != nil {
		t.Fatal(err)
	}
	payload := filepath.Join(game, "resourcepacks", "local.zip")
	if err := os.WriteFile(payload, []byte("local file"), 0600); err != nil {
		t.Fatal(err)
	}
	snapshot, err := assets.List(t.Context(), external.ID)
	if err != nil || len(snapshot.Files) != 1 {
		t.Fatalf("external asset scan: %+v %v", snapshot, err)
	}
	result, err := assets.Change(t.Context(), app.AssetMutation{InstanceID: external.ID, File: snapshot.Files[0], Action: app.AssetRemove})
	if err != nil {
		t.Fatal(err)
	}
	backup := filepath.Join(location.Path, external.ID, "removed-assets", result.BackupID, "payload")
	if data, err := os.ReadFile(backup); err != nil || string(data) != "local file" {
		t.Fatalf("same-volume backup: %v", err)
	}
	if err := assets.Restore(t.Context(), external.ID, result.BackupID); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(payload); err != nil || string(data) != "local file" {
		t.Fatalf("restore: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(root, "instances", external.ID, "game")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("external instance silently created a central game directory")
	}
	if err := directory.Remove(t.Context(), location.ID); err == nil {
		t.Fatal("removed a location with a live instance association")
	}
	listed, err := storage.NewInstanceStore(filepath.Join(root, "instances")).List(t.Context())
	if err != nil || len(listed) != 2 {
		t.Fatal("central inventory lost an instance", err)
	}
}

func TestGameDirectoriesRejectMissingVolumeAndCopiedApplication(t *testing.T) {
	root := filepath.Join(t.TempDir(), "data")
	store := storage.NewGameDirectoryStore(root)
	location, err := store.Add(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	value := testInstance("外部")
	value.GameDirectoryID = location.ID
	if err := storage.NewInstanceStore(filepath.Join(root, "instances")).Create(t.Context(), value); err != nil {
		t.Fatal(err)
	}
	if err := store.Rename(t.Context(), location.ID, "工作盘"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "game-directories.json"))
	if err != nil {
		t.Fatal(err)
	}
	other := t.TempDir()
	if err := os.WriteFile(filepath.Join(other, "game-directories.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := storage.NewGameDirectoryStore(other).Directory(t.Context(), location.ID); err == nil {
		t.Fatal("copied registry bypassed the original application's session locks")
	}
	if err := os.Rename(location.Path, location.Path+".offline"); err != nil {
		t.Fatal(err)
	}
	if _, err := storage.NewAssetStore(filepath.Join(root, "instances")).Directory(t.Context(), value.ID); err == nil {
		t.Fatal("unavailable location fell back to another game directory")
	}
	rows, err := store.List(t.Context())
	if err != nil || len(rows) != 2 || rows[1].Problem == "" || rows[1].Instances != 1 {
		t.Fatalf("offline location lost its registration: %+v %v", rows, err)
	}
}

func TestGameDirectoriesNeverAdoptUnmarkedOrProtectedFolders(t *testing.T) {
	root := t.TempDir()
	store := storage.NewGameDirectoryStore(root)
	parent := t.TempDir()
	child := filepath.Join(parent, "mcl-games-v1")
	if err := os.Mkdir(child, 0700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(child, "world.txt")
	if err := os.WriteFile(marker, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Add(t.Context(), parent); err == nil {
		t.Fatal("adopted an unmarked directory")
	}
	if data, err := os.ReadFile(marker); err != nil || string(data) != "keep" {
		t.Fatal("unknown game data changed")
	}
	if _, err := store.Add(t.Context(), root); err == nil {
		t.Fatal("registered the application data subtree")
	}
	if _, err := os.Lstat(filepath.Join(root, "mcl-games-v1")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("created a rejected location")
	}
}

func TestGameDirectoryDefaultUpgradeAndNonDestructiveUnregister(t *testing.T) {
	root := t.TempDir()
	registry := storage.NewGameDirectoryStore(root)
	location, err := registry.Add(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "settings.json")
	original := []byte("{\"version\":4,\"settings\":{}}")
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	settings := app.DefaultSettings()
	settings.GameDirectoryID = location.ID
	repository := storage.NewSettingsStore(path)
	if err := repository.Save(t.Context(), settings); err != nil {
		t.Fatal(err)
	}
	copies, err := filepath.Glob(filepath.Join(root, "settings-v4-*.json.bak"))
	if err != nil || len(copies) != 1 {
		t.Fatal("missing original schema-four backup")
	}
	if got, err := os.ReadFile(copies[0]); err != nil || string(got) != string(original) {
		t.Fatal("original settings backup changed")
	}
	if err := registry.Remove(t.Context(), location.ID); err == nil {
		t.Fatal("removed the persisted default")
	}
	settings.GameDirectoryID = ""
	if err := repository.Save(t.Context(), settings); err != nil {
		t.Fatal(err)
	}
	if err := registry.Remove(t.Context(), location.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(location.Path, "owner.json")); err != nil {
		t.Fatal("unregister deleted the owned directory")
	}
	reattached, err := registry.Add(t.Context(), filepath.Dir(location.Path))
	if err != nil || reattached.ID != location.ID {
		t.Fatal("reattach did not preserve identity", err)
	}
}
