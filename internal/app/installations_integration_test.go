//go:build integration

package app_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"mcl/internal/app"
	"mcl/internal/core/download"
	"mcl/internal/core/install"
	"mcl/internal/core/meta"
	"mcl/internal/platform/process"
	"mcl/internal/storage"
)

// This explicitly invoked integration test downloads official game files into
// an isolated directory, then corrupts its own client copy to exercise repair.
// It never executes Minecraft or uses any account credentials.
func TestOfficialInstallAndRepair(t *testing.T) {
	root := os.Getenv("MCL_INTEGRATION_DATA_DIR")
	if root == "" {
		t.Skip("set MCL_INTEGRATION_DATA_DIR to an isolated test directory")
	}
	root, err := filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	id := os.Getenv("MCL_INTEGRATION_VERSION")
	if id == "" {
		id = "1.20.4"
	}
	catalog := meta.NewClient(filepath.Join(root, "cache/metadata"))
	instances := storage.NewInstanceStore(filepath.Join(root, "instances"))
	library := app.NewLibraryService(catalog, instances, meta.CurrentPlatform())
	value, err := library.Create(t.Context(), app.CreateInstance{Name: "官方安装验收 " + id, VersionID: id, Settings: app.DefaultSettings()})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("created instance %s version %s data=%s", value.ID, id, root)
	service := app.NewInstallationService(catalog, instances, storage.NewInstallationStore(root), download.NewClient(root), meta.CurrentPlatform(), storage.NewSessionStore(root, process.Running), nil)
	var stage string
	var last time.Time
	report := func(p install.Progress) {
		if stage != p.Stage || time.Since(last) > 5*time.Second || p.Files == p.TotalFiles {
			t.Logf("stage=%s files=%d/%d verified=%d/%d reused=%d transferred=%d", p.Stage, p.Files, p.TotalFiles, p.Bytes, p.TotalBytes, p.Reused, p.Transferred)
			stage, last = p.Stage, time.Now()
		}
	}
	if _, err := service.Run(t.Context(), value.ID, report); err != nil {
		t.Fatal(err)
	}
	version, err := catalog.Version(t.Context(), value.Version)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := install.Build(version, meta.CurrentPlatform())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, plan.ClientPath), []byte("deliberately corrupted integration fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	var fileProgress, assetProgress download.Progress
	if _, err := service.Run(t.Context(), value.ID, func(p install.Progress) {
		report(p)
		if p.Stage == "files" {
			fileProgress = p.Progress
		}
		if p.Stage == "assets" {
			assetProgress = p.Progress
		}
	}); err != nil {
		t.Fatal(err)
	}
	if fileProgress.Transferred != version.Downloads["client"].Size || fileProgress.Reused != fileProgress.TotalFiles-1 || assetProgress.Reused != assetProgress.TotalFiles || assetProgress.Transferred != 0 {
		t.Fatalf("repair did not reuse correct files: files=%+v assets=%+v", fileProgress, assetProgress)
	}
	if err := os.Remove(filepath.Join(root, plan.LoggingPath)); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Run(t.Context(), value.ID, func(p install.Progress) {
		report(p)
		if p.Stage == "files" {
			fileProgress = p.Progress
		}
		if p.Stage == "assets" {
			assetProgress = p.Progress
		}
	}); err != nil {
		t.Fatal(err)
	}
	if fileProgress.Transferred != version.Logging.Client.File.Size || fileProgress.Reused != fileProgress.TotalFiles-1 || assetProgress.Transferred != 0 || assetProgress.Reused != assetProgress.TotalFiles {
		t.Fatalf("missing log config repair: files=%+v assets=%+v", fileProgress, assetProgress)
	}
	opened, err := os.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer opened.Close()
	for _, f := range plan.Files {
		if err := download.Verify(t.Context(), opened, f); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("PASS official %s installation, corrupt client and deleted logging config repair; reuse of all %d asset objects", id, assetProgress.TotalFiles)
}
