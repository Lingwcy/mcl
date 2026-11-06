package storage_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"mcl/internal/core/instance"
	"mcl/internal/core/launch"
	"mcl/internal/storage"
)

func sessionData(t *testing.T) (string, launch.Record) {
	t.Helper()
	root := t.TempDir()
	id := instance.NewID()
	if err := os.MkdirAll(filepath.Join(root, "instances", id, "game"), 0o700); err != nil {
		t.Fatal(err)
	}
	r := launch.Record{ID: instance.NewID(), InstanceID: id, VersionID: "1.20.4", VersionSHA1: strings.Repeat("a", 40), NativeDirectory: "instances/" + id + "/natives/ready-" + strings.Repeat("A", 26), Process: launch.ProcessIdentity{PID: 123, OS: runtime.GOOS, Stamp: "original:birth"}, CreatedAt: time.Now().UTC()}
	return root, r
}

func TestSessionReopenPIDReuseAndStaleFileCleanup(t *testing.T) {
	root, record := sessionData(t)
	stamp := record.Process.Stamp
	running := func(identity launch.ProcessIdentity) (bool, error) { return identity.Stamp == stamp, nil }
	store := storage.NewSessionStore(root, running)
	lease, err := store.Reserve(t.Context(), record.InstanceID)
	if err != nil {
		t.Fatal(err)
	}
	if err := lease.Track(t.Context(), record); err != nil {
		t.Fatal(err)
	}
	argumentFile := filepath.Join(root, "instances", record.InstanceID, "runtime", "jvm-"+strings.Repeat("A", 26)+".args")
	if err := os.WriteFile(argumentFile, []byte("JVM options"), 0o600); err != nil {
		t.Fatal(err)
	}
	notes := filepath.Join(filepath.Dir(argumentFile), "notes.args")
	if err := os.WriteFile(notes, []byte("user file"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := lease.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := storage.NewSessionStore(root, running)
	if _, err := reopened.Reserve(t.Context(), record.InstanceID); err == nil {
		t.Fatal("live session was overwritten")
	}
	if _, err := os.Stat(argumentFile); err != nil {
		t.Fatal("live session runtime file removed")
	}
	active, err := reopened.Active(t.Context())
	if err != nil || len(active) != 1 || active[0].ID != record.ID {
		t.Fatalf("active records %+v %v", active, err)
	}
	// Same PID, different creation identity: the unrelated process stays alive,
	// but it must not keep this old instance reserved.
	stamp = "new:birth"
	lease, err = reopened.Reserve(t.Context(), record.InstanceID)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	if _, err := os.Stat(argumentFile); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("abandoned JVM file retained")
	}
	if _, err := os.Stat(notes); err != nil {
		t.Fatal("nonmanaged file removed")
	}
	if _, err := os.Stat(filepath.Join(root, "instances", record.InstanceID, "session.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("stale PID record retained")
	}
}

func TestUnknownProcessAndCorruptSessionNeverPermitMutation(t *testing.T) {
	root, record := sessionData(t)
	store := storage.NewSessionStore(root, func(launch.ProcessIdentity) (bool, error) { return true, nil })
	lease, err := store.Reserve(t.Context(), record.InstanceID)
	if err != nil {
		t.Fatal(err)
	}
	if err := lease.Track(t.Context(), record); err != nil {
		t.Fatal(err)
	}
	lease.Close()
	filename := filepath.Join(root, "instances", record.InstanceID, "session.json")
	before, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	unknown := storage.NewSessionStore(root, func(launch.ProcessIdentity) (bool, error) { return false, os.ErrPermission })
	if _, err := unknown.Reserve(t.Context(), record.InstanceID); !errors.Is(err, os.ErrPermission) {
		t.Fatalf("unknown process treated as dead: %v", err)
	}
	after, _ := os.ReadFile(filename)
	if string(after) != string(before) {
		t.Fatal("unverifiable session changed")
	}
	for _, body := range []string{`{"schema":99}`, `{broken`, string(before) + " trailing"} {
		if err := os.WriteFile(filename, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := store.Reserve(t.Context(), record.InstanceID); err == nil {
			t.Fatal("corrupt session accepted")
		}
		data, _ := os.ReadFile(filename)
		if string(data) != body {
			t.Fatal("corrupt session replaced")
		}
	}
}

func TestCanceledTrackAndForeignRecord(t *testing.T) {
	root, record := sessionData(t)
	store := storage.NewSessionStore(root, func(launch.ProcessIdentity) (bool, error) { return false, nil })
	lease, err := store.Reserve(t.Context(), record.InstanceID)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := lease.Track(ctx, record); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled track: %v", err)
	}
	foreign := record
	foreign.InstanceID = instance.NewID()
	if err := lease.Track(t.Context(), foreign); err == nil {
		t.Fatal("foreign session accepted")
	}
	if err := lease.Track(t.Context(), record); err != nil {
		t.Fatal(err)
	}
	if err := lease.Track(t.Context(), record); err == nil {
		t.Fatal("duplicate tracking accepted")
	}
}
