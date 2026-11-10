package ui

import (
	"errors"
	"testing"

	"mcl/internal/app"
	"mcl/internal/core/download"
	"mcl/internal/core/install"
	"mcl/internal/core/instance"
	"mcl/internal/core/meta"

	"github.com/egoist/mygo/ui"
)

func TestInstallationCancelRetryAndNavigation(t *testing.T) {
	s := NewShell(app.DefaultSettings(), t.TempDir(), nil, nil)
	id := instance.NewID()
	s.library.snapshot.Instances = []instance.Instance{{ID: id, Name: "安装验收", Version: meta.ManifestEntry{ID: "1.20.4"}}}
	s.library.selectedID = id
	var report func(install.Progress)
	var complete func(install.Receipt, error)
	var oldComplete func(install.Receipt, error)
	canceled := false
	s.ConnectInstallations(InstallationActions{Start: func(instanceID string, p func(install.Progress), done func(install.Receipt, error)) func() {
		if instanceID != id {
			t.Fatal("wrong instance")
		}
		report, complete = p, done
		return func() { canceled = true }
	}})
	tester := ui.NewTester(s.View, 860, 564)
	click(t, tester, "安装游戏文件")
	report(install.Progress{Stage: "assets", Progress: download.Progress{Files: 3, TotalFiles: 10, Bytes: 30, TotalBytes: 100}})
	tester.Frame()
	if !tester.HasText("下载与校验资产对象") {
		t.Fatal(tester.Texts())
	}
	click(t, tester, "设置")
	click(t, tester, "下载与任务")
	click(t, tester, "取消任务")
	if !canceled || !s.installation.active {
		t.Fatal("cancellation prematurely declared complete")
	}
	oldComplete = complete
	complete(install.Receipt{}, errors.New("canceled"))
	tester.Frame()
	click(t, tester, "再次校验 / 重试")
	oldComplete(install.Receipt{}, errors.New("stale result"))
	tester.Frame()
	if !s.installation.active || s.installation.message != "" {
		t.Fatal("obsolete task overwrote retry")
	}
	complete(install.Receipt{InstanceID: id, VersionID: "1.20.4"}, nil)
	tester.Frame()
	click(t, tester, "游戏库")
	if s.installation.active || !tester.HasText("安装验收") {
		t.Fatal(tester.Texts())
	}
}
