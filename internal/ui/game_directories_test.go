package ui

import (
	"errors"
	"testing"

	"mcl/internal/app"
	"mcl/internal/core/instance"
)

func TestCreationDirectoryReadsOnlyCommittedDefault(t *testing.T) {
	var complete func(error)
	initial := app.DefaultSettings()
	shell := NewShell(initial, t.TempDir(), nil, func(_ app.Settings, done func(error)) { complete = done })
	next := instance.NewID()
	shell.gameSettings.directoryID = next
	shell.saveSettings()
	if shell.creationSettings().GameDirectoryID != "" {
		t.Fatal("creation consumed an uncommitted location")
	}
	complete(errors.New("save failed"))
	if shell.creationSettings().GameDirectoryID != "" {
		t.Fatal("failed save changed the creation location")
	}
	shell.saveSettings()
	complete(nil)
	if shell.creationSettings().GameDirectoryID != next {
		t.Fatal("persisted default did not reach creation")
	}
	explicit := ""
	shell.creation.directoryID = &explicit
	if shell.creationSettings().GameDirectoryID != "" || shell.settings.GameDirectoryID != next {
		t.Fatal("one-off directory choice changed the global default")
	}
}

func TestCreationDirectorySelectionReachesRequestWithoutChangingDefault(t *testing.T) {
	shell := NewShell(app.DefaultSettings(), t.TempDir(), nil, nil)
	first, second := instance.NewID(), instance.NewID()
	shell.gameDirectories.rows = []app.GameDirectory{{Name: "内置目录"}, {ID: first, Name: "游戏盘", Path: "/games/one"}, {ID: second, Name: "游戏盘", Path: "/games/two"}}
	shell.chooseCreationGameDirectory(second)
	if shell.creationSettings().GameDirectoryID != second || shell.settings.GameDirectoryID != "" {
		t.Fatal("single creation choice did not preserve stable identity and global default")
	}
	shell.chooseCreationGameDirectory("")
	if shell.creation.directoryID == nil || shell.creationSettings().GameDirectoryID != "" {
		t.Fatal("explicit built-in selection was treated as an unset default")
	}
	shell.gameDirectories.rows[2].Problem = "磁盘离线"
	shell.chooseCreationGameDirectory(second)
	if shell.creationSettings().GameDirectoryID != "" {
		t.Fatal("unavailable directory replaced the prior selection")
	}
	shell.chooseCreationGameDirectory(instance.NewID())
	if shell.creationSettings().GameDirectoryID != "" {
		t.Fatal("unregistered location reached the creation request")
	}
}
