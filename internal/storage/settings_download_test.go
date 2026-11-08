package storage_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"mcl/internal/core/download"
	"mcl/internal/storage"
)

func TestDownloadPolicyUpgradeBacksUpSchemaTwo(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	legacy := `{"version":2,"settings":{"max_memory_mib":4096,"game":{"width":1280,"height":720},"download":{"concurrency":2}}}`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	store := storage.NewSettingsStore(path)
	settings, err := store.Load(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if settings.Download.Concurrency != 2 || settings.Download.SpeedLimitKiB != 0 || settings.Download.Source != download.SourceOfficial {
		t.Fatal("schema two defaults changed")
	}
	settings.Download.SpeedLimitKiB, settings.Download.Source = 1024, download.SourceMirrorFirst
	settings.Download.Proxy = download.Proxy{Mode: download.ProxyHTTP, Host: "127.0.0.1", Port: 8080}
	if err := store.Save(t.Context(), settings); err != nil {
		t.Fatal(err)
	}
	backups, err := filepath.Glob(filepath.Join(filepath.Dir(path), "settings-v2-*.json.bak"))
	if err != nil || len(backups) != 1 {
		t.Fatalf("missing schema two backup: %v", err)
	}
	data, err := os.ReadFile(backups[0])
	if err != nil || string(data) != legacy {
		t.Fatal("backup did not retain the original bytes")
	}
	got, err := storage.NewSettingsStore(path).Load(t.Context())
	if err != nil || got != settings || got.Game.Width != 1280 || got.MaxMemoryMiB != 4096 {
		t.Fatalf("download policy lost other settings: %+v, %v", got, err)
	}
}

func TestCacheDirectorySchemaThreeUpgradePreservesDownloadPolicy(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	legacy := []byte("{\"version\":3,\"settings\":{\"download\":{\"speed_limit_kib\":1024,\"source\":\"mirror-first\"}}}")
	if err := os.WriteFile(path, legacy, 0o600); err != nil {
		t.Fatal(err)
	}
	store := storage.NewSettingsStore(path)
	settings, err := store.Load(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	settings.Download.CacheDirectory = t.TempDir()
	if err := store.Save(t.Context(), settings); err != nil {
		t.Fatal(err)
	}
	backups, err := filepath.Glob(filepath.Join(filepath.Dir(path), "settings-v3-*.json.bak"))
	if err != nil || len(backups) != 1 {
		t.Fatal("missing schema-three recovery copy")
	}
	old, err := os.ReadFile(backups[0])
	if err != nil || !bytes.Equal(old, legacy) {
		t.Fatal("upgrade backup changed original bytes")
	}
	got, err := store.Load(t.Context())
	if err != nil || got != settings {
		t.Fatal("cache preference discarded download policy")
	}
}
