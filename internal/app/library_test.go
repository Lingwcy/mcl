package app_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"mcl/internal/app"
	"mcl/internal/core/meta"
	"mcl/internal/storage"
)

type fixedCatalog struct {
	manifest meta.Manifest
	version  meta.Version
}

func (c fixedCatalog) Manifest(ctx context.Context, _ bool) (meta.Catalog, error) {
	return meta.Catalog{Manifest: c.manifest}, ctx.Err()
}

func (c fixedCatalog) Version(ctx context.Context, _ meta.ManifestEntry) (meta.Version, error) {
	return c.version, ctx.Err()
}

func TestCreateAndReloadInstanceFromOfficialMetadata(t *testing.T) {
	t.Parallel()
	read := func(name string) []byte {
		t.Helper()
		data, err := os.ReadFile(filepath.Join("..", "core", "meta", "testdata", name))
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	manifest, err := meta.ParseManifest(read("manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	version, err := meta.ParseVersion(read("1.21.1.json"))
	if err != nil {
		t.Fatal(err)
	}
	store := storage.NewInstanceStore(t.TempDir())
	catalog := fixedCatalog{manifest, version}
	service := app.NewLibraryService(catalog, store, meta.Platform{OS: "osx", Arch: "arm64"})
	value, err := service.Create(t.Context(), app.CreateInstance{Name: "  新世界  ", VersionID: "1.21.1", Settings: app.DefaultSettings()})
	if err != nil || value.Name != "新世界" || value.RequiredJava != 21 || value.Version.ID != "1.21.1" {
		t.Fatalf("create from metadata: %+v, %v", value, err)
	}
	snapshot, err := service.Load(t.Context(), false)
	if err != nil || len(snapshot.Instances) != 1 || len(snapshot.Releases) != 2 {
		t.Fatalf("reload library: %+v, %v", snapshot, err)
	}
	if _, err := service.Rename(t.Context(), value.ID, "  renamed  "); err != nil {
		t.Fatal(err)
	}
	listed, err := store.List(t.Context())
	if err != nil || listed[0].Name != "renamed" {
		t.Fatalf("rename persistence: %+v, %v", listed, err)
	}
	if _, err := service.Create(t.Context(), app.CreateInstance{Name: "unknown", VersionID: "unknown", Settings: app.DefaultSettings()}); err == nil {
		t.Fatal("unknown version cannot create a partially initialized instance")
	}
}
