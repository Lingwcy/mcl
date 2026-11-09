package app

import (
	"testing"

	"mcl/internal/core/instance"
	"mcl/internal/core/market"
	"mcl/internal/core/modpack"
)

func TestUpdateTaskIdentityBindsFilesButNotEphemeralURLs(t *testing.T) {
	file := market.InstallFile{Source: market.Modrinth, ProjectID: "project", VersionID: "fixed", Path: "mods/example.jar", File: market.File{URL: "https://cdn.modrinth.com/file.jar", SHA1: "1111111111111111111111111111111111111111", Size: 12}}
	plan := market.UpdatePlan{Request: market.UpdateRequest{InstanceID: instance.NewID(), GameVersion: "1.20.4", Kind: market.Mod}, ReceiptSHA256: "receipt", Changes: []market.UpdateChange{{After: file, Enabled: true}}}
	before := UpdateTaskIdentity(plan)
	plan.Changes[0].After.File.URL = "https://cdn.modrinth.com/file.jar?temporary=changed"
	if UpdateTaskIdentity(plan) != before {
		t.Fatal("temporary URL changed fixed update identity")
	}
	plan.Changes[0].Enabled = false
	if UpdateTaskIdentity(plan) == before {
		t.Fatal("disabled state not bound")
	}
	plan.Changes[0].Enabled = true
	plan.Changes[0].After.VersionID = "other"
	if UpdateTaskIdentity(plan) == before {
		t.Fatal("version identity not bound")
	}
}
func TestPackTaskIdentityUsesSelectedFilesAndPinsOptionalChoice(t *testing.T) {
	one := modpack.File{Path: "mods/required.jar", SHA1: "1111111111111111111111111111111111111111", Size: 4, URLs: []string{"https://cdn.modrinth.com/a"}}
	optional := modpack.File{Path: "mods/optional.jar", SHA1: "2222222222222222222222222222222222222222", Size: 7, Optional: true}
	plan := ArchiveImportPlan{Archive: market.File{SHA1: "3333333333333333333333333333333333333333", Size: 100}, Files: []modpack.File{one, optional}}
	fixed := PackTaskIdentity(plan)
	published := plan
	published.Files = []modpack.File{one}
	published.Files[0].URLs = nil
	if PackTaskIdentity(published) != fixed {
		t.Fatal("unselected optional file treated as published requirement")
	}
	plan.SelectedOptional = []string{optional.Path}
	if PackTaskIdentity(plan) == fixed {
		t.Fatal("optional selection not bound")
	}
}
func TestTaskTargetFingerprintAllowsNameEditButRejectsMovedGame(t *testing.T) {
	value := instance.Instance{ID: instance.NewID(), Name: "first"}
	before := taskInstanceFingerprint(value)
	value.Name = "renamed"
	if taskInstanceFingerprint(value) != before {
		t.Fatal("cosmetic rename invalidated resource task")
	}
	value.GameDirectoryID = instance.NewID()
	if taskInstanceFingerprint(value) == before {
		t.Fatal("changed payload location accepted")
	}
}
