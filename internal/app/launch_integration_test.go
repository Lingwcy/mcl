//go:build integration

package app_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"mcl/internal/app"
	"mcl/internal/core/download"
	"mcl/internal/core/install"
	"mcl/internal/core/java"
	"mcl/internal/core/meta"
	"mcl/internal/platform"
	"mcl/internal/platform/process"
	"mcl/internal/storage"
)

func TestRealPreparation(t *testing.T) {
	root, executable := os.Getenv("MCL_INTEGRATION_DATA_DIR"), os.Getenv("MCL_INTEGRATION_JAVA")
	if root == "" || executable == "" {
		t.Skip("set MCL_INTEGRATION_DATA_DIR and MCL_INTEGRATION_JAVA; install 1.20.4 into this test directory first")
	}
	root, err := filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	instances := storage.NewInstanceStore(filepath.Join(root, "instances"))
	values, err := instances.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	id := ""
	for _, value := range values {
		if value.Version.ID == "1.20.4" {
			id = value.ID
			break
		}
	}
	if id == "" {
		t.Skip("the integration data directory has no 1.20.4 test instance")
	}
	catalog := meta.NewClient(filepath.Join(root, "cache/metadata"))
	installer := app.NewInstallationService(catalog, instances, storage.NewInstallationStore(root), download.NewClient(root), meta.CurrentPlatform(), storage.NewSessionStore(root, process.Running), nil)
	javas := app.NewJavaService(storage.NewJavaStore(root), platform.JavaCandidates, &java.Installer{Directory: filepath.Join(root, "java")}, java.NewCatalogClient())
	service := app.NewLaunchService(installer, javas.Ensure, platform.NewMemoryProbe(), nil, nil)
	settings := app.DefaultSettings()
	settings.JavaPath = executable
	var files, assets download.Progress
	prepared, err := service.Prepare(t.Context(), id, settings, func(p install.Progress) {
		if p.Stage == "files" {
			files = p.Progress
		}
		if p.Stage == "assets" {
			assets = p.Progress
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Close()
	preview, err := prepared.Preview()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(preview.Arguments, "<登录后提供>") || preview.Executable == "" || preview.Directory == "" {
		t.Fatal("preparation did not produce an unresolved authenticated preview")
	}
	if files.Files != 54 || assets.Files != 3788 || files.Reused != files.Files || assets.Reused != assets.Files {
		t.Fatalf("preparation did not reverify/reuse official installation: %+v %+v", files, assets)
	}
	t.Logf("real Java probe and official 1.20.4 preparation passed: %d base files, %d assets, all reverified/reused; no session or game process created", files.Files, assets.Files)
}
