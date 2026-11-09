package ui

import (
	"bytes"
	"image"
	"image/png"
	"testing"

	"mcl/internal/app"
	"mcl/internal/core/download"
	"mcl/internal/core/install"
	"mcl/internal/core/instance"
	"mcl/internal/core/market"
)

func archiveEditorPNG(t *testing.T) []byte {
	t.Helper()
	var output bytes.Buffer
	if err := png.Encode(&output, image.NewRGBA(image.Rect(0, 0, 80, 80))); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}
func TestPackVersionSelectionReadsImmediatelyAndPinsDirectory(t *testing.T) {
	shell := NewShell(app.DefaultSettings(), t.TempDir(), nil, nil)
	selected := instance.NewID()
	shell.market.importDirectoryID = &selected
	calls := 0
	var request app.ArchiveImportRequest
	shell.market.actions.InspectArchive = func(r app.ArchiveImportRequest, _ app.Settings, _ func(download.Progress), _ func(app.ArchiveImportPlan, error)) func() {
		calls++
		request = r
		return func() {}
	}
	shell.selectMarketInstall(market.Project{Source: market.Modrinth, Kind: market.Modpack, ID: "project", Title: "Pack"}, market.Version{ID: "fixed-version"})
	if calls != 1 || request.ProjectID != "project" || request.VersionID != "fixed-version" || request.GameDirectoryID == nil || *request.GameDirectoryID != selected || !shell.market.install.busy {
		t.Fatal("extra manual inspection step or unfixed location", calls, request)
	}
	selected = instance.NewID()
	if *request.GameDirectoryID == selected {
		t.Fatal("later directory draft changed pending request")
	}
}
func TestArchiveImageIsPreservedByMetadataAndCopiedIntoImport(t *testing.T) {
	shell := NewShell(app.DefaultSettings(), t.TempDir(), nil, nil)
	var ready func([]byte, error)
	requests := 0
	shell.market.actions.InstanceIcon = func(_ string, done func([]byte, error)) func() { requests++; ready = done; return func() {} }
	i := &marketInstallState{project: market.Project{Source: market.Modrinth, Kind: market.Modpack, ID: "project", IconURL: "https://cdn.modrinth.com/icon.png"}}
	shell.market.install = i
	shell.loadArchiveProjectIcon(i)
	data := archiveEditorPNG(t)
	ready(data, nil)
	plan := app.ArchiveImportPlan{Request: app.ArchiveImportRequest{Kind: market.Modpack}, Instance: instance.Instance{ID: instance.NewID(), Name: "Pack", Description: "Summary"}}
	shell.initializeArchiveEditor(i, plan)
	i.archivePlan = &plan
	if requests != 1 || i.archiveIconBusy || i.archiveBitmap == nil || !bytes.Equal(i.archiveIcon, data) {
		t.Fatal("metadata replaced project icon or restarted its request")
	}
	var imported app.ArchiveImportPlan
	shell.market.actions.ImportArchive = func(p app.ArchiveImportPlan, _ func(install.Progress), _ func(app.PackImportResult, error)) func() {
		imported = p
		return func() {}
	}
	shell.importMarketArchive()
	if !bytes.Equal(imported.Request.Icon, data) || !bytes.Equal(imported.Instance.Icon, data) {
		t.Fatal("project thumbnail not persisted in fixed import plan")
	}
	data[0] ^= 0xff
	if bytes.Equal(imported.Request.Icon, data) {
		t.Fatal("icon ownership still aliases callback memory")
	}
}
func TestArchiveIconRejectsCanceledAndReplacedResults(t *testing.T) {
	shell := NewShell(app.DefaultSettings(), t.TempDir(), nil, nil)
	var ready []func([]byte, error)
	canceled := 0
	shell.market.actions.InstanceIcon = func(_ string, done func([]byte, error)) func() {
		ready = append(ready, done)
		return func() { canceled++ }
	}
	i := &marketInstallState{project: market.Project{Kind: market.Modpack, IconURL: "https://cdn.modrinth.com/first.png"}}
	shell.market.install = i
	shell.loadArchiveProjectIcon(i)
	shell.cancelArchiveIcon(i)
	i.archiveIconChosen = true
	i.archiveIconStyle = "cat"
	ready[0](archiveEditorPNG(t), nil)
	if i.archiveBitmap != nil || i.archiveIconStyle != "cat" || len(i.archiveIcon) != 0 || canceled != 1 {
		t.Fatal("late image replaced explicit preset")
	}
	i.archiveIconChosen = false
	i.archiveIconStyle = ""
	i.archiveIconURL = ""
	shell.loadArchiveProjectIcon(i)
	replacement := &marketInstallState{}
	shell.market.install = replacement
	ready[1](archiveEditorPNG(t), nil)
	if len(i.archiveIcon) != 0 || replacement.archiveBitmap != nil {
		t.Fatal("result affected obsolete/new import")
	}
}
