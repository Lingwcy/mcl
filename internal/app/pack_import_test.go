package app_test

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"mcl/internal/app"
	"mcl/internal/core/download"
	"mcl/internal/core/java"
	"mcl/internal/core/launch"
	"mcl/internal/core/market"
	"mcl/internal/core/meta"
	"mcl/internal/storage"
)

func TestMultiMCImportRetainsInstanceWhenRuntimeFailsAndResumes(t *testing.T) {
	fixture := newInstallFixture(t)
	data, err := os.ReadFile("../core/meta/testdata/1.20.4.json")
	if err != nil {
		t.Fatal(err)
	}
	version, err := meta.ParseVersion(data)
	if err != nil {
		t.Fatal(err)
	}
	catalog := fixedCatalog{manifest: meta.Manifest{Versions: []meta.ManifestEntry{fixture.value.Version}}, version: version}
	instances := storage.NewInstanceStore(filepath.Join(fixture.root, "instances"))
	sessions := storage.NewSessionStore(fixture.root, func(launch.ProcessIdentity) (bool, error) { return false, nil })
	archives := app.NewArchiveImporter(app.NewMarketService(market.NewClient("")), catalog, instances, sessions, storage.NewArchiveStore(fixture.root), download.NewClient(fixture.root))
	var dataZIP bytes.Buffer
	writer := zip.NewWriter(&dataZIP)
	for name, content := range map[string]string{"Example/mmc-pack.json": `{"formatVersion":1,"components":[{"uid":"net.minecraft","version":"1.20.4"}]}`, "Example/instance.cfg": "name=多格式导入\nPreLaunchCommand=must-not-run", "Example/minecraft/config/example.json": "{}"} {
		file, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		file.Write([]byte(content))
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	name := filepath.Join(t.TempDir(), "local.zip")
	if err := os.WriteFile(name, dataZIP.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	plan, err := archives.InspectLocal(t.Context(), name, app.DefaultSettings())
	if err != nil {
		t.Fatal(err)
	}
	description := "用户描述"
	plan.Request.Description = &description
	denied := errors.New("fixture Java download declined")
	failJava := func(context.Context, int, meta.Platform, []string, func(java.Progress)) (java.Runtime, error) {
		return java.Runtime{}, denied
	}
	result, err := app.NewPackImportService(archives, fixture.service, failJava).Run(t.Context(), plan, nil)
	if !errors.Is(err, denied) || !result.Imported || result.RuntimeReady || result.Instance.ID == "" {
		t.Fatalf("failed runtime concealed imported instance: %+v %v", result, err)
	}
	current, err := instances.Get(t.Context(), result.Instance.ID)
	if err != nil || current.Pack.Source != "multimc" || !current.Pack.Local || current.Description != description {
		t.Fatalf("import identity/preferences lost: %+v %v", current, err)
	}
	if _, err := os.Stat(filepath.Join(fixture.root, "instances", current.ID, "game", "config/example.json")); err != nil {
		t.Fatal("Prism payload was not published", err)
	}
	javaOK := func(ctx context.Context, major int, target meta.Platform, _ []string, _ func(java.Progress)) (java.Runtime, error) {
		return java.Runtime{Executable: filepath.Join(fixture.root, "fake-java"), Version: "17.0.1", Major: 17, Arch: target.Arch}, ctx.Err()
	}
	ready, err := app.NewPackImportService(archives, fixture.service, javaOK).Resume(t.Context(), current.ID, nil)
	if err != nil || !ready.Imported || !ready.RuntimeReady || ready.Instance.ID != current.ID {
		t.Fatalf("resume did not install the existing instance: %+v %v", ready, err)
	}
	values, err := instances.List(t.Context())
	if err != nil || len(values) != 2 {
		t.Fatal("resume created a duplicate instance", err)
	}
}
