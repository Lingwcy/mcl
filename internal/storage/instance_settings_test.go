package storage_test

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"mcl/internal/app"
	"mcl/internal/core/instance"
	"mcl/internal/core/launch"
	"mcl/internal/storage"
)

func TestInstancePreferencesPreserveIdentityRuntimeAndWorld(t *testing.T) {
	for _, pack := range []bool{false, true} {
		name := "loader"
		if pack {
			name = "pack"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			store := storage.NewInstanceStore(filepath.Join(root, "instances"))
			original := testInstance("original")
			runtime := instance.Loader{Type: "fabric", Version: "0.16.9"}
			if pack {
				original.Pack = &instance.Pack{Source: "modrinth", ProjectID: "fixed-pack", VersionID: "fixed-version", ArchiveSHA1: strings.Repeat("a", 40), Loaders: []instance.Loader{runtime}}
			} else {
				original.Loader = &runtime
			}
			if err := store.Create(t.Context(), original); err != nil {
				t.Fatal(err)
			}
			world := filepath.Join(root, "instances", original.ID, "game", "world.dat")
			if err := os.WriteFile(world, []byte("keep world"), 0o600); err != nil {
				t.Fatal(err)
			}
			sessions := storage.NewSessionStore(root, func(launch.ProcessIdentity) (bool, error) { return false, nil })
			service := app.NewInstanceSettingsService(store, sessions)
			expected := app.PreferencesForInstance(original)
			desired := expected
			desired.Name, desired.JavaPath = "  renamed  ", filepath.Join(root, "Java 中文", "bin", "java")
			var upload bytes.Buffer
			if err := png.Encode(&upload, image.NewNRGBA(image.Rect(0, 0, 16, 12))); err != nil {
				t.Fatal(err)
			}
			desired.Icon = upload.Bytes()
			updated, err := service.Save(t.Context(), original.ID, expected, desired)
			if err != nil {
				t.Fatal(err)
			}
			configuration, err := png.DecodeConfig(bytes.NewReader(updated.Icon))
			if err != nil || configuration.Width != 80 || configuration.Height != 80 {
				t.Fatalf("icon was not normalized: %+v, %v", configuration, err)
			}
			want := original
			want.Name, want.JavaPath, want.Icon = "renamed", desired.JavaPath, updated.Icon
			if !reflect.DeepEqual(updated, want) {
				t.Fatalf("preferences changed unrelated fields: got %+v, want %+v", updated, want)
			}
			loaded, err := service.Load(t.Context(), original.ID)
			if err != nil || !reflect.DeepEqual(loaded, want) {
				t.Fatalf("preferences did not survive reload: %+v, %v", loaded, err)
			}
			if data, err := os.ReadFile(world); err != nil || string(data) != "keep world" {
				t.Fatalf("editing moved or modified game data: %v", err)
			}
			desired.Name = "stale edit"
			if _, err := service.Save(t.Context(), original.ID, expected, desired); !errors.Is(err, app.ErrInstanceSettingsChanged) {
				t.Fatalf("stale preferences must not overwrite a newer save: %v", err)
			}
			still, err := service.Load(t.Context(), original.ID)
			if err != nil || !reflect.DeepEqual(still, want) {
				t.Fatal("a rejected edit altered the document")
			}
		})
	}
}

func TestInstancePreferencesRespectReservationPendingRecoveryAndCancellation(t *testing.T) {
	root := t.TempDir()
	store := storage.NewInstanceStore(filepath.Join(root, "instances"))
	original := testInstance("original")
	if err := store.Create(t.Context(), original); err != nil {
		t.Fatal(err)
	}
	sessions := storage.NewSessionStore(root, func(launch.ProcessIdentity) (bool, error) { return false, nil })
	service := app.NewInstanceSettingsService(store, sessions)
	expected := app.PreferencesForInstance(original)
	desired := expected
	desired.Name = "renamed"
	lease, err := sessions.Reserve(t.Context(), original.ID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { lease.Close() })
	if _, err := service.Save(t.Context(), original.ID, expected, desired); err == nil {
		t.Fatal("editing ignored an existing preparation or game reservation")
	}
	if err := lease.Close(); err != nil {
		t.Fatal(err)
	}
	pending := filepath.Join(root, "instances", original.ID, "asset-update.pending.json")
	if err := os.WriteFile(pending, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Save(t.Context(), original.ID, expected, desired); err == nil {
		t.Fatal("editing bypassed unfinished asset recovery")
	}
	if err := os.Remove(pending); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := service.Save(ctx, original.ID, expected, desired); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled edit: %v", err)
	}
	for _, bad := range []app.InstancePreferences{
		{Name: ""}, {Name: "valid", JavaPath: "relative/java"},
		{Name: "valid", IconStyle: "not-a-preset"}, {Name: "valid", Icon: []byte("not an image")},
	} {
		if _, err := service.Save(t.Context(), original.ID, expected, bad); err == nil {
			t.Fatalf("accepted invalid preferences: %+v", bad)
		}
	}
	still, err := store.Get(t.Context(), original.ID)
	if err != nil || !reflect.DeepEqual(still, original) {
		t.Fatalf("a rejected edit altered the original: %+v, %v", still, err)
	}
}

func TestUnchangedInstanceIconIsNotResampledByNameEdits(t *testing.T) {
	root := t.TempDir()
	store := storage.NewInstanceStore(filepath.Join(root, "instances"))
	original := testInstance("original")
	var icon bytes.Buffer
	if err := png.Encode(&icon, image.NewNRGBA(image.Rect(0, 0, 16, 12))); err != nil {
		t.Fatal(err)
	}
	original.Icon = icon.Bytes()
	if err := store.Create(t.Context(), original); err != nil {
		t.Fatal(err)
	}
	sessions := storage.NewSessionStore(root, func(launch.ProcessIdentity) (bool, error) { return false, nil })
	service := app.NewInstanceSettingsService(store, sessions)
	expected := app.PreferencesForInstance(original)
	desired := app.PreferencesForInstance(original)
	desired.Name = "new name"
	updated, err := service.Save(t.Context(), original.ID, expected, desired)
	if err != nil || !bytes.Equal(updated.Icon, original.Icon) {
		t.Fatalf("name edit changed unedited icon bytes: %v", err)
	}
}
