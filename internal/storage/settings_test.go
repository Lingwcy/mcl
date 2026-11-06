package storage_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mcl/internal/app"
	"mcl/internal/storage"
)

func TestSettingsRoundTrip(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "中文 directory", "settings.json")
	store := storage.NewSettingsStore(path)
	initial, err := store.Load(t.Context())
	if err != nil || initial != app.DefaultSettings() {
		t.Fatalf("load missing settings: got %+v, %v", initial, err)
	}
	if _, err := os.Stat(filepath.Dir(path)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("loading should not create the data directory: %v", err)
	}
	want := app.Settings{JavaPath: filepath.Join(t.TempDir(), "Java 目录", "bin", "java"), MaxMemoryMiB: 8192}
	if err := store.Save(t.Context(), want); err != nil {
		t.Fatal(err)
	}
	got, err := storage.NewSettingsStore(path).Load(t.Context())
	if err != nil || got != want {
		t.Fatalf("reload settings: got %+v, %v; want %+v", got, err, want)
	}
	want.MaxMemoryMiB = 2048
	if err := store.Save(t.Context(), want); err != nil {
		t.Fatal(err)
	}
	got, err = store.Load(t.Context())
	if err != nil || got != want {
		t.Fatalf("replace settings: got %+v, %v; want %+v", got, err, want)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil || len(entries) != 1 {
		t.Fatalf("temporary files leaked: %v, %v", entries, err)
	}
}

func TestUnreadableSettingsArePreserved(t *testing.T) {
	t.Parallel()
	for name, document := range map[string]string{
		"malformed":        "{broken",
		"future version":   `{"version":99,"settings":{"java_path":"","max_memory_mib":4096}}`,
		"invalid memory":   `{"version":1,"settings":{"java_path":"","max_memory_mib":0}}`,
		"trailing content": `{"version":1,"settings":{"java_path":"","max_memory_mib":4096}} {}`,
		"oversized":        strings.Repeat(" ", (1<<20)+1),
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "settings.json")
			if err := os.WriteFile(path, []byte(document), 0o600); err != nil {
				t.Fatal(err)
			}
			store := storage.NewSettingsStore(path)
			if _, err := store.Load(t.Context()); err == nil {
				t.Fatal("expected load to fail")
			}
			if err := store.Save(t.Context(), app.DefaultSettings()); err == nil {
				t.Fatal("save should refuse to overwrite unreadable settings")
			}
			got, err := os.ReadFile(path)
			if err != nil || string(got) != document {
				t.Fatalf("original document changed: %v", err)
			}
		})
	}
}

func TestCanceledSaveKeepsPreviousSettings(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "settings.json")
	store := storage.NewSettingsStore(path)
	want := app.DefaultSettings()
	if err := store.Save(t.Context(), want); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := store.Save(ctx, app.Settings{MaxMemoryMiB: 8192}); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
	got, err := store.Load(t.Context())
	if err != nil || got != want {
		t.Fatalf("canceled save changed settings: %+v, %v", got, err)
	}
}
