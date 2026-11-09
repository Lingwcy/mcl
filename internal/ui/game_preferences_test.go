package ui

import (
	"path/filepath"
	"testing"

	"mcl/internal/app"
	"mcl/internal/core/java"
)

func TestGameAndDownloadDraftsCoalesceAfterJavaSelection(t *testing.T) {
	var writes []app.Settings
	var done func(error)
	shell := NewShell(app.DefaultSettings(), t.TempDir(), nil, func(settings app.Settings, complete func(error)) {
		writes = append(writes, settings)
		done = complete
	})
	path := filepath.Join(t.TempDir(), "java", "bin", "java")
	shell.defaultJava(app.JavaEnvironment{Runtime: java.Runtime{Executable: path}})
	shell.gameSettings.width, shell.gameSettings.height = "1280", "720"
	shell.gameSettings.options.Fullscreen = true
	shell.gameSettings.options.AutoJoinServer = true
	shell.gameSettings.options.ServerAddress = "example.com:25566"
	shell.downloadSettings.automatic, shell.downloadSettings.concurrency = false, "2"
	shell.saveSettings()
	if len(writes) != 1 {
		t.Fatal("drafts started a concurrent settings write")
	}
	done(nil)
	if len(writes) != 2 || writes[1].JavaPath != path || writes[1].Game.Width != 1280 || writes[1].Game.Height != 720 || !writes[1].Game.Fullscreen || writes[1].Game.ServerAddress != "example.com:25566" || writes[1].Download.Concurrency != 2 {
		t.Fatalf("queued settings lost a draft: %+v", writes)
	}
	done(nil)
	persisted := shell.settings
	shell.downloadSettings.concurrency = "0"
	shell.saveSettings()
	if len(writes) != 2 || !shell.noticeFail || shell.settings != persisted {
		t.Fatal("invalid draft changed persisted launch/download settings")
	}
	shell.downloadSettings.automatic = true
	shell.gameSettings.width, shell.gameSettings.height = "0", "0"
	shell.saveSettings()
	if len(writes) != 2 || !shell.noticeFail || shell.settings != persisted {
		t.Fatal("zero UI dimensions bypassed validation")
	}
}
