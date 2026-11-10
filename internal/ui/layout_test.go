package ui

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"mcl/internal/app"
)

func TestWorkspaceNavigationAndSecondarySidebar(t *testing.T) {
	shell := NewShell(app.DefaultSettings(), t.TempDir(), nil, nil)
	tester := ui.NewTester(shell.View, DefaultWidth, DefaultHeight)
	rail, railFound := tester.Find("主导航")
	sidebar, sidebarFound := tester.Find("副侧栏")
	if !railFound || !sidebarFound || rail.X+rail.W > sidebar.X+1 || rail.W >= sidebar.W {
		t.Fatalf("navigation rail and secondary sidebar must have separate columns: %+v, %+v", rail, sidebar)
	}
	click(t, tester, "市场")
	if shell.page != marketPage || tester.HasText("搜索实例") {
		t.Fatal("market must replace the instance sidebar with asset categories")
	}
	click(t, tester, "光影")
	if _, found := tester.Find("搜索市场"); shell.market.kind != app.AssetShader || !found {
		t.Fatal("market category navigation must retain the search controls")
	}
	click(t, tester, "收起或展开侧栏")
	tester.Advance(2 * time.Second)
	tester.Frame()
	if _, found := tester.Find("副侧栏"); found {
		t.Fatal("secondary sidebar should collapse independently of the navigation rail")
	}
	click(t, tester, "Home")
	tester.Advance(2 * time.Second)
	tester.Frame()
	if !tester.HasText("实例") || shell.sidebarCollapsed {
		t.Fatal("Home must reopen the instance sidebar")
	}
	tester.SetSize(MinWidth, MinHeight)
	click(t, tester, "设置")
	click(t, tester, "Java 管理")
	if _, found := tester.Find("添加本机 Java"); !found {
		t.Fatal("settings should remain reachable at the minimum window size")
	}
}
