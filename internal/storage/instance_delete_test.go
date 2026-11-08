package storage_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"mcl/internal/app"
	"mcl/internal/core/launch"
	"mcl/internal/storage"
)

func TestDeleteInstanceIncludesExternalGameAndBackupsNotSharedData(t *testing.T) {
	root := t.TempDir()
	registry := storage.NewGameDirectoryStore(root)
	location, err := registry.Add(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	instances := storage.NewInstanceStore(filepath.Join(root, "instances"))
	value, other := testInstance("删除目标"), testInstance("保留")
	value.GameDirectoryID = location.ID
	if err := instances.Create(t.Context(), value); err != nil {
		t.Fatal(err)
	}
	if err := instances.Create(t.Context(), other); err != nil {
		t.Fatal(err)
	}
	payload := filepath.Join(location.Path, value.ID)
	for _, relative := range []string{"game/saves/world/level.dat", "game/mods/example.jar", "removed-assets/backup/payload", "asset-updates/old/payload"} {
		target := filepath.Join(payload, relative)
		if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, []byte("owned"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	shared := filepath.Join(root, "cache", "shared")
	if err := os.MkdirAll(filepath.Dir(shared), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(shared, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	trash := t.TempDir()
	calls := 0
	sessions := storage.NewSessionStore(root, func(launch.ProcessIdentity) (bool, error) { return false, nil })
	service := app.NewInstanceDeletionService(storage.NewInstanceDeletionStore(root), sessions, func(path string) error { calls++; return os.Rename(path, filepath.Join(trash, strconv.Itoa(calls))) })
	plan, err := service.Inspect(t.Context(), value.ID)
	if err != nil {
		t.Fatal(err)
	}
	if plan.ExternalPayload != payload || plan.GamePath != filepath.Join(payload, "game") {
		t.Fatal("wrong deletion scope")
	}
	if err := service.Delete(t.Context(), plan); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatal("external payload or central record was not deleted")
	}
	if _, err := os.Stat(filepath.Join(trash, "1", "game/saves/world/level.dat")); err != nil {
		t.Fatal("owned world was excluded", err)
	}
	if _, err := os.Stat(filepath.Join(trash, "1", "asset-updates/old/payload")); err != nil {
		t.Fatal("instance backup was excluded", err)
	}
	if _, err := instances.Get(t.Context(), value.ID); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("instance record still exists")
	}
	if _, err := instances.Get(t.Context(), other.ID); err != nil {
		t.Fatal("other instance was changed", err)
	}
	if data, err := os.ReadFile(shared); err != nil || string(data) != "keep" {
		t.Fatal("shared cache was deleted")
	}
}
func TestDeleteInstancePartialTrashBlocksNormalUseAndCanResume(t *testing.T) {
	root := t.TempDir()
	locations := storage.NewGameDirectoryStore(root)
	location, err := locations.Add(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	value := testInstance("重试")
	value.GameDirectoryID = location.ID
	instances := storage.NewInstanceStore(filepath.Join(root, "instances"))
	if err := instances.Create(t.Context(), value); err != nil {
		t.Fatal(err)
	}
	sessions := storage.NewSessionStore(root, func(launch.ProcessIdentity) (bool, error) { return false, nil })
	calls := 0
	fail := true
	trash := t.TempDir()
	service := app.NewInstanceDeletionService(storage.NewInstanceDeletionStore(root), sessions, func(path string) error {
		calls++
		if fail && calls == 2 {
			return errors.New("trash unavailable")
		}
		return os.Rename(path, filepath.Join(trash, strconv.Itoa(calls)))
	})
	plan, err := service.Inspect(t.Context(), value.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Delete(t.Context(), plan); err == nil {
		t.Fatal("partial deletion reported success")
	}
	if lease, err := sessions.Reserve(t.Context(), value.ID); err == nil {
		lease.Close()
		t.Fatal("unfinished deletion allowed launch/repair")
	}
	plan, err = service.Inspect(t.Context(), value.ID)
	if err != nil || !plan.Pending || !plan.PayloadGone {
		t.Fatal("deletion cannot resume", err)
	}
	fail = false
	if err := service.Delete(t.Context(), plan); err != nil {
		t.Fatal(err)
	}
	if calls != 3 {
		t.Fatal("resume tried to delete the already removed external payload again")
	}
}
func TestDeleteInstanceRefusesCompetingLeaseAndStaleConfirmation(t *testing.T) {
	root := t.TempDir()
	instances := storage.NewInstanceStore(filepath.Join(root, "instances"))
	value := testInstance("原名称")
	if err := instances.Create(t.Context(), value); err != nil {
		t.Fatal(err)
	}
	sessions := storage.NewSessionStore(root, func(launch.ProcessIdentity) (bool, error) { return false, nil })
	calls := 0
	service := app.NewInstanceDeletionService(storage.NewInstanceDeletionStore(root), sessions, func(string) error { calls++; return nil })
	lease, err := sessions.Reserve(t.Context(), value.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Inspect(t.Context(), value.ID); err == nil {
		t.Fatal("active preparation lease allowed deletion")
	}
	lease.Close()
	plan, err := service.Inspect(t.Context(), value.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := instances.Rename(t.Context(), value.ID, "其他进程已修改"); err != nil {
		t.Fatal(err)
	}
	if err := service.Delete(t.Context(), plan); err == nil || calls != 0 {
		t.Fatal("stale confirmation deleted changed instance")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := service.Inspect(ctx, value.ID); !errors.Is(err, context.Canceled) {
		t.Fatal("canceled deletion inspection continued")
	}
}
