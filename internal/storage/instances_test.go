package storage_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"mcl/internal/core/instance"
	"mcl/internal/core/meta"
	"mcl/internal/storage"
)

func TestInstanceIsolationAndRename(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "中文 游戏")
	store := storage.NewInstanceStore(root)
	first, second := testInstance("生存 世界"), testInstance("生存 世界")
	for _, value := range []instance.Instance{first, second} {
		if err := store.Create(t.Context(), value); err != nil {
			t.Fatal(err)
		}
		if stat, err := os.Stat(filepath.Join(root, value.ID, "game")); err != nil || !stat.IsDir() {
			t.Fatalf("missing isolated game folder: %v", err)
		}
	}
	marker := filepath.Join(root, first.ID, "game", "world.txt")
	if err := os.WriteFile(marker, []byte("keep my world"), 0o600); err != nil {
		t.Fatal(err)
	}
	renamed, err := store.Rename(t.Context(), first.ID, "新名字 / 独立世界")
	if err != nil || renamed.ID != first.ID || renamed.Version != first.Version || !renamed.CreatedAt.Equal(first.CreatedAt) {
		t.Fatalf("rename changed stable identity: %+v, %v", renamed, err)
	}
	if data, err := os.ReadFile(marker); err != nil || string(data) != "keep my world" {
		t.Fatalf("rename moved or changed game data: %v", err)
	}
	listed, err := storage.NewInstanceStore(root).List(t.Context())
	if err != nil || len(listed) != 2 {
		t.Fatalf("reload isolated instances: %+v, %v", listed, err)
	}
	if err := store.Create(t.Context(), first); !errors.Is(err, os.ErrExist) {
		t.Fatalf("duplicate ID must not overwrite an instance: %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := store.Rename(ctx, first.ID, "must not persist"); !errors.Is(err, context.Canceled) {
		t.Fatalf("rename cancellation: %v", err)
	}
}

func TestInstanceStorePreservesCorruptionAndRejectsEscapes(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	store := storage.NewInstanceStore(root)
	value := testInstance("original")
	if err := store.Create(t.Context(), value); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, value.ID, "instance.json")
	if err := os.WriteFile(path, []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Rename(t.Context(), value.ID, "new"); err == nil {
		t.Fatal("rename must not overwrite unreadable metadata")
	}
	if got, _ := os.ReadFile(path); string(got) != "{broken" {
		t.Fatal("corrupt original was overwritten")
	}
	for _, id := range []string{"../escape", "not-an-id", ""} {
		if _, err := store.Rename(t.Context(), id, "new"); err == nil {
			t.Fatalf("accepted invalid ID %q", id)
		}
	}
	outside := t.TempDir()
	outsidePath := filepath.Join(outside, "instance.json")
	if err := os.WriteFile(outsidePath, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	linkID := instance.NewID()
	if err := os.Symlink(outside, filepath.Join(root, linkID)); err != nil {
		t.Skipf("symlink test unavailable: %v", err)
	}
	if _, err := store.Rename(t.Context(), linkID, "escape"); err == nil {
		t.Fatal("instance symlink escaped the managed root")
	}
	if got, _ := os.ReadFile(outsidePath); !reflect.DeepEqual(got, []byte("outside")) {
		t.Fatal("outside file changed")
	}
}

func testInstance(name string) instance.Instance {
	return instance.Instance{
		ID: instance.NewID(), Name: name, RequiredJava: 17, MaxMemoryMiB: 4096,
		Version:   meta.ManifestEntry{ID: "1.20.4", Type: "release", URL: "https://example.com/version.json", SHA1: "0000000000000000000000000000000000000000"},
		CreatedAt: time.Now().UTC(),
	}
}
