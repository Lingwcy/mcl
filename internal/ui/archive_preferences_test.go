package ui

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"strings"
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

func TestInitialArchiveFailureReturnsToVersionsAndShowsCause(t *testing.T) {
	shell := NewShell(app.DefaultSettings(), t.TempDir(), nil, nil)
	project := market.Project{Source: market.Modrinth, Kind: market.Modpack, ID: "project", Title: "Pack"}
	shell.page = marketPage
	shell.market.selected = &project
	var complete func(app.ArchiveImportPlan, error)
	shell.market.actions.InspectArchive = func(_ app.ArchiveImportRequest, _ app.Settings, _ func(download.Progress), done func(app.ArchiveImportPlan, error)) func() {
		complete = done
		return func() {}
	}
	shell.selectMarketInstall(project, market.Version{ID: "version"})
	complete(app.ArchiveImportPlan{}, errors.New("synthetic manifest failure"))
	if shell.market.showInstall || shell.market.install != nil || shell.market.tab != 1 || shell.page != marketPage {
		t.Fatal("failed read left an empty confirmation page")
	}
	if len(shell.toasts.pending) != 1 || shell.toasts.pending[0].Type != string(ToastError) || !strings.Contains(shell.toasts.pending[0].Title, "synthetic manifest failure") {
		t.Fatal("failure cause was hidden", shell.toasts.pending)
	}
}
func TestArchiveCanceledOrObsoleteReadsDoNotShowFailure(t *testing.T) {
	for _, obsolete := range []bool{false, true} {
		shell := NewShell(app.DefaultSettings(), t.TempDir(), nil, nil)
		var complete func(app.ArchiveImportPlan, error)
		shell.market.actions.InspectArchive = func(_ app.ArchiveImportRequest, _ app.Settings, _ func(download.Progress), done func(app.ArchiveImportPlan, error)) func() {
			complete = done
			return func() {}
		}
		shell.selectMarketInstall(market.Project{Source: market.Modrinth, Kind: market.Modpack, ID: "project", Title: "Pack"}, market.Version{ID: "version"})
		replacement := &marketInstallState{}
		shell.page = gameSettingsPage
		if obsolete {
			shell.market.install = replacement
		}
		if obsolete {
			complete(app.ArchiveImportPlan{}, errors.New("obsolete failure"))
		} else {
			complete(app.ArchiveImportPlan{}, context.Canceled)
		}
		if len(shell.toasts.pending) != 0 || shell.page != gameSettingsPage {
			t.Fatal("obsolete/canceled failure affected current page")
		}
		if obsolete && shell.market.install != replacement {
			t.Fatal("late failure removed a newer task")
		}
	}
}
func TestArchiveRereadFailureKeepsEditorChoices(t *testing.T) {
	shell := NewShell(app.DefaultSettings(), t.TempDir(), nil, nil)
	plan := &app.ArchiveImportPlan{}
	i := &marketInstallState{project: market.Project{Kind: market.Modpack}, archivePlan: plan, archiveName: "Edited", archiveDescription: "Draft", optional: map[string]bool{"optional.jar": true}}
	shell.market.install = i
	shell.market.showInstall = true
	shell.failArchiveInspection(i, errors.New("synthetic reread failure"))
	if shell.market.install != i || !shell.market.showInstall || i.archivePlan != plan || i.archiveName != "Edited" || i.archiveDescription != "Draft" || !i.optional["optional.jar"] || len(shell.toasts.pending) != 1 {
		t.Fatal("failed re-read discarded confirmation edits")
	}
}
