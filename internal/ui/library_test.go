package ui

import (
	"errors"
	"testing"

	"mcl/internal/app"
	"mcl/internal/core/instance"
	"mcl/internal/core/java"
	"mcl/internal/core/meta"

	"github.com/egoist/mygo/ui"
)

func TestLibraryCreateRenameAndRefresh(t *testing.T) {
	shell := NewShell(app.DefaultSettings(), t.TempDir(), nil, nil)
	var rows []instance.Instance
	var request app.CreateInstance
	shell.ConnectLibrary(LibraryActions{
		Load: func(_ bool, done func(app.LibrarySnapshot, error)) {
			done(app.LibrarySnapshot{Instances: rows, Releases: []meta.ManifestEntry{{ID: "1.21.11"}, {ID: "1.21.1"}, {ID: "1.20.4"}}}, nil)
		},
		Create: func(value app.CreateInstance, done func(instance.Instance, error)) {
			request = value
			row := instance.Instance{ID: instance.NewID(), Name: value.Name, Version: meta.ManifestEntry{ID: value.VersionID}, RequiredJava: 21, MaxMemoryMiB: value.Settings.MaxMemoryMiB}
			rows = append(rows, row)
			done(row, nil)
		},
		Rename: func(id, name string, done func(instance.Instance, error)) {
			rows[0].Name = name
			done(rows[0], nil)
		},
	})
	tester := ui.NewTester(shell.View, 1080, 1000)
	click(t, tester, "新建实例")
	click(t, tester, "选择游戏版本")
	click(t, tester, "搜索版本")
	tester.Type("1.21.1")
	tester.Key(0, ui.KeyEnter)
	click(t, tester, "新实例名称")
	tester.Command("selectAll")
	tester.Type("生存世界")
	click(t, tester, "创建并安装")
	if request.Name != "生存世界" || request.VersionID != "1.21.1" || request.Settings.MaxMemoryMiB != 0 || !tester.HasText("生存世界") {
		t.Fatalf("create did not publish an isolated instance: %+v, %v", request, tester.Texts())
	}
	click(t, tester, "版本设置")
	click(t, tester, "实例名称")
	tester.Command("selectAll")
	tester.Type("重命名世界")
	click(t, tester, "保存名称")
	click(t, tester, "账户")
	click(t, tester, "游戏库")
	click(t, tester, "切换版本")
	click(t, tester, "刷新版本")
	if !tester.HasText("重命名世界") || shell.library.editing != "" {
		t.Fatalf("rename lost identity across navigation and refresh: %v", tester.Texts())
	}
}

func TestObsoleteJavaScanCannotOverwriteNewInventory(t *testing.T) {
	shell := NewShell(app.DefaultSettings(), t.TempDir(), nil, nil)
	var completions []func(app.JavaInventory, error)
	shell.ConnectJava(JavaActions{Inventory: func(_ string, done func(app.JavaInventory, error)) func() {
		completions = append(completions, done)
		return func() {}
	}})
	shell.refreshJava()
	current := app.JavaInventory{Environments: []app.JavaEnvironment{{Runtime: java.Runtime{Version: "21"}}}}
	completions[1](current, nil)
	completions[0](app.JavaInventory{}, errors.New("obsolete"))
	if shell.java.scanning || shell.java.message != "" || len(shell.java.inventory.Environments) != 1 {
		t.Fatal("obsolete scan changed the latest Java inventory")
	}
}

func TestFailedRefreshKeepsPreviouslyLoadedInstances(t *testing.T) {
	shell := NewShell(app.DefaultSettings(), t.TempDir(), nil, nil)
	shell.library.snapshot.Instances = []instance.Instance{{ID: "kept", Name: "keep this instance"}}
	shell.library.actions.Load = func(_ bool, done func(app.LibrarySnapshot, error)) {
		done(app.LibrarySnapshot{}, errors.New("cannot read instance metadata"))
	}
	shell.refreshLibrary(true)
	if len(shell.library.snapshot.Instances) != 1 || !shell.library.failed {
		t.Fatal("read failure must not silently replace the last visible snapshot")
	}
}
