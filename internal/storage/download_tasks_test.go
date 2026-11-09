package storage

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"mcl/internal/core/instance"
	"mcl/internal/core/tasks"
)

func storedTask() tasks.Group {
	return tasks.Group{Schema: 1, ID: instance.NewID(), Intent: tasks.Intent{Kind: "files", Name: "fixture", Payload: json.RawMessage("[]")}, State: "paused", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
}
func TestDownloadTaskStoreKeepsCorruptionAndRejectsOccupiedHistory(t *testing.T) {
	root := t.TempDir()
	store := NewDownloadTaskStore(root)
	group := storedTask()
	lease, err := store.Lock(t.Context(), group.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Create(t.Context(), group); err != nil {
		t.Fatal(err)
	}
	if second, err := store.Lock(t.Context(), group.ID); err == nil {
		second.Close()
		t.Fatal("same task admitted in two workers")
	}
	lease.Close()
	if err := store.Remove(t.Context(), group.ID); err == nil {
		t.Fatal("paused task removed")
	}
	name := filepath.Join(root, "download-tasks-v1", group.ID+".json")
	raw := []byte("{bad-json")
	if err := os.WriteFile(name, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.List(t.Context()); err == nil {
		t.Fatal("corrupt history hidden")
	}
	if err := store.Create(t.Context(), storedTask()); err == nil {
		t.Fatal("new task discarded corrupt record")
	}
	actual, _ := os.ReadFile(name)
	if string(actual) != string(raw) {
		t.Fatal("corrupt file replaced")
	}
}
func TestDownloadCoordinationAliasesAndCanceledWait(t *testing.T) {
	root := t.TempDir()
	coordination := NewDownloadCoordination(root)
	first, err := coordination.Target(t.Context(), "cache/A.jar")
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if second, err := coordination.Target(ctx, "cache/a.jar"); !errors.Is(err, context.Canceled) {
		if second != nil {
			second.Close()
		}
		t.Fatal(err)
	}
	shared, err := coordination.Parts(t.Context(), false)
	if err != nil {
		t.Fatal(err)
	}
	if exclusive, err := coordination.Parts(t.Context(), true); err == nil {
		exclusive.Close()
		t.Fatal("cleanup acquired while active part shared lease exists")
	}
	shared.Close()
	exclusive, err := coordination.Parts(t.Context(), true)
	if err != nil {
		t.Fatal(err)
	}
	exclusive.Close()
}
func TestDownloadTaskStoreLimitsAndRejectsSymlinkRecords(t *testing.T) {
	store := NewDownloadTaskStore(t.TempDir())
	group := storedTask()
	group.Files = []tasks.File{{Key: "invalid", Path: "../escape", State: "downloading", Total: 1}}
	if err := store.Create(t.Context(), group); err == nil {
		t.Fatal("invalid file identity persisted")
	}
	group = storedTask()
	if err := store.Create(t.Context(), group); err != nil {
		t.Fatal(err)
	}
	name := filepath.Join(store.root, "download-tasks-v1", group.ID+".json")
	external := filepath.Join(t.TempDir(), "secret.json")
	if err := os.WriteFile(external, []byte("do not read"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(name); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, name); err != nil {
		t.Skip(err)
	}
	if _, err := store.List(t.Context()); err == nil {
		t.Fatal("linked history accepted")
	}
	data, _ := os.ReadFile(external)
	if string(data) != "do not read" {
		t.Fatal("external record changed")
	}
}
