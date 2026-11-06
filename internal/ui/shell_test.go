package ui

import (
	"errors"
	"path/filepath"
	"testing"

	"mcl/internal/app"
	"mcl/internal/core/java"
	"mcl/internal/core/launch"

	"github.com/egoist/mygo/ui"
)

func TestJVMArgumentsAutosaveAcrossNavigationAndComposition(t *testing.T) {
	var saved app.Settings
	var complete func(error)
	shell := NewShell(app.DefaultSettings(), t.TempDir(), nil, func(settings app.Settings, done func(error)) {
		saved, complete = settings, done
	})
	path := filepath.Join(t.TempDir(), "中文 Java", "bin", "java")
	shell.settings.JavaPath = path
	tester := ui.NewTester(shell.View, 1080, 720)
	click(t, tester, "设置")
	click(t, tester, "自定义 JVM 参数")
	tester.Command("selectAll")
	tester.Compose("-D玩家=测试", 7)
	tester.Type("-D玩家=测试")
	click(t, tester, "游戏库")
	click(t, tester, "设置")
	if saved.JavaPath != path || saved.JVMArguments != "-D玩家=测试" {
		t.Fatalf("input was lost across builds/navigation: %+v", saved)
	}
	if !shell.saving || complete == nil {
		t.Fatal("expected background save to remain pending")
	}
	tester.Frame()
	complete(nil)
	tester.Frame()
	if shell.settings.JVMArguments != saved.JVMArguments || shell.saving || shell.notice != "" {
		t.Fatalf("save did not finish: %v", tester.Texts())
	}
	if tester.HasText("最大内存") || tester.HasText("初始内存") || tester.HasText("保存") {
		t.Fatal("settings should use a small group label and save automatically")
	}
	tester.SetSize(820, 510)
	tester.SetScale(2)
	tester.SetDark(true)
	click(t, tester, "账户")
	click(t, tester, "设置")
	tester.Frame()
}

func TestSaveValidationAndFailure(t *testing.T) {
	var calls int
	var complete func(error)
	shell := NewShell(app.DefaultSettings(), t.TempDir(), nil, func(_ app.Settings, done func(error)) {
		calls++
		complete = done
	})
	tester := ui.NewTester(shell.View, 1080, 720)
	click(t, tester, "设置")
	click(t, tester, "自定义 JVM 参数")
	tester.Command("selectAll")
	tester.Type("-Xmx4096m")
	if calls != 0 || !tester.HasText("堆大小由启动器自动管理，JVM 参数不能覆盖内存分配") {
		t.Fatalf("invalid input should not start a save: calls=%d, %v", calls, tester.Texts())
	}
	click(t, tester, "自定义 JVM 参数")
	tester.Command("selectAll")
	tester.Type("-Dexample=true")
	if calls != 1 || complete == nil {
		t.Fatal("expected one save request")
	}
	// Edits while saving stay interactive and produce one latest snapshot.
	shell.gameSettings.gc = launch.GCG1
	shell.saveSettings()
	if calls != 1 {
		t.Fatal("a pending save must not be submitted again")
	}
	complete(nil)
	if calls != 2 || complete == nil || !shell.saving {
		t.Fatal("latest draft was not flushed after the pending save")
	}
	complete(errors.New("磁盘写入失败"))
	tester.Frame()
	if shell.saving || !tester.HasText("磁盘写入失败") || tester.HasText("设置已保存") {
		t.Fatalf("failure should not be reported as success: %v", tester.Texts())
	}
}

func TestJavaSelectionFlushesQueuedGameSettings(t *testing.T) {
	var writes []app.Settings
	var complete func(error)
	shell := NewShell(app.DefaultSettings(), t.TempDir(), nil, func(settings app.Settings, done func(error)) {
		writes = append(writes, settings)
		complete = done
	})
	path := filepath.Join(t.TempDir(), "java", "bin", "java")
	shell.defaultJava(app.JavaEnvironment{Runtime: java.Runtime{Executable: path}})
	shell.gameSettings.gc = launch.GCG1
	shell.saveSettings()
	if len(writes) != 1 {
		t.Fatal("settings writes must be serialized")
	}
	complete(nil)
	if len(writes) != 2 || writes[1].JavaPath != path || writes[1].GCMode != launch.GCG1 {
		t.Fatal("queued autosave lost the Java selection or collector")
	}
	complete(nil)
	if shell.saving || shell.settings.JavaPath != path || shell.settings.GCMode != launch.GCG1 {
		t.Fatal("autosave did not commit the latest settings")
	}
}

func TestUnreadableSettingsDisableSaving(t *testing.T) {
	var saved bool
	shell := NewShell(app.DefaultSettings(), t.TempDir(), errors.New("invalid JSON"), func(app.Settings, func(error)) {
		saved = true
	})
	tester := ui.NewTester(shell.View, 1080, 720)
	click(t, tester, "设置")
	shell.saveSettings()
	if saved || !tester.HasText("无法读取设置，原文件已保留") {
		t.Fatalf("unreadable settings must be preserved: saved=%v, %v", saved, tester.Texts())
	}
}

func click(t *testing.T, tester *ui.Tester, label string) {
	t.Helper()
	if err := tester.Click(label); err != nil {
		t.Fatal(err)
	}
}
