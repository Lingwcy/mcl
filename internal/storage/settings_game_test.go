package storage_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"mcl/internal/app"
	"mcl/internal/core/download"
	"mcl/internal/core/launch"
	"mcl/internal/storage"
)

func TestSettingsVersionOneMigrationPreservesLegacyValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	legacy := []byte(`{"version":1,"settings":{"max_memory_mib":4096,"jvm_arguments":"-Dexample=true"}}`)
	if err := os.WriteFile(path, legacy, 0o600); err != nil {
		t.Fatal(err)
	}
	store := storage.NewSettingsStore(path)
	settings, err := store.Load(t.Context())
	if err != nil || settings.MaxMemoryMiB != 4096 || settings.JVMArguments != "-Dexample=true" || settings.Game != (launch.GameSettings{}) || settings.Download != (download.Preferences{}) {
		t.Fatalf("legacy values changed: %+v, %v", settings, err)
	}
	settings.Game = launch.GameSettings{Width: 1920, Height: 1080, Fullscreen: true, AutoJoinServer: true, ServerAddress: "[::1]:25565"}
	settings.Download = download.Preferences{Concurrency: 128}
	if err := store.Save(t.Context(), settings); err != nil {
		t.Fatal(err)
	}
	backups, err := filepath.Glob(filepath.Join(filepath.Dir(path), "settings-v1-*.json.bak"))
	if err != nil || len(backups) != 1 {
		t.Fatalf("missing legacy recovery copy: %v, %v", backups, err)
	}
	backup, err := os.ReadFile(backups[0])
	if err != nil || string(backup) != string(legacy) {
		t.Fatalf("legacy recovery copy did not preserve the original: %v", err)
	}
	got, err := storage.NewSettingsStore(path).Load(t.Context())
	if err != nil || got != settings {
		t.Fatalf("new settings did not round-trip: %+v, %v", got, err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var document struct{ Version int }
	if err := json.Unmarshal(data, &document); err != nil || document.Version != 6 {
		t.Fatalf("expected an explicit schema upgrade: %s, %v", data, err)
	}
}

func TestInvalidNewSettingsCannotBeSilentlyOverwritten(t *testing.T) {
	for _, document := range []string{
		`{"version":2,"settings":{"download":{"concurrency":129}}}`,
		`{"version":2,"settings":{"game":{"width":1280,"height":0}}}`,
		`{"version":2,"settings":{"game":{"auto_join_server":true,"server_address":"https://host"}}}`,
	} {
		path := filepath.Join(t.TempDir(), "settings.json")
		if err := os.WriteFile(path, []byte(document), 0o600); err != nil {
			t.Fatal(err)
		}
		store := storage.NewSettingsStore(path)
		if _, err := store.Load(t.Context()); err == nil {
			t.Fatal("invalid document was accepted")
		}
		if err := store.Save(t.Context(), app.DefaultSettings()); err == nil {
			t.Fatal("invalid original settings were overwritten")
		}
		data, err := os.ReadFile(path)
		if err != nil || string(data) != document {
			t.Fatal("original settings were not preserved")
		}
	}
}

func TestGamePriorityAndAutoLogsUpgradePreservesSchemaFive(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "settings.json")
	original := []byte("{\"version\":5,\"settings\":{\"game\":{\"width\":1280,\"height\":720},\"download\":{\"concurrency\":4}}}")
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	repository := storage.NewSettingsStore(path)
	settings, err := repository.Load(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if settings.Game.Priority != launch.PriorityNormal || settings.Game.ShowLogs {
		t.Fatal("new startup behavior was enabled for an old configuration")
	}
	settings.Game.Priority, settings.Game.ShowLogs = launch.PriorityBelowNormal, true
	if err := repository.Save(t.Context(), settings); err != nil {
		t.Fatal(err)
	}
	copies, err := filepath.Glob(filepath.Join(root, "settings-v5-*.json.bak"))
	if err != nil || len(copies) != 1 {
		t.Fatal("missing schema five rollback copy")
	}
	if got, err := os.ReadFile(copies[0]); err != nil || string(got) != string(original) {
		t.Fatal("rollback copy changed the original")
	}
	got, err := repository.Load(t.Context())
	if err != nil || got != settings || got.Game.Width != 1280 || got.Download.Concurrency != 4 {
		t.Fatal("startup preferences lost unrelated settings", err)
	}
	got.Game.Priority = launch.ProcessPriority("realtime")
	if err := repository.Save(t.Context(), got); err == nil {
		t.Fatal("invalid process priority was saved")
	}
}
