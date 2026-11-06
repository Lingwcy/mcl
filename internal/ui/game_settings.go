package ui

import (
	"mcl/internal/core/launch"

	"github.com/egoist/mygo/ui"
)

type gameSettingsState struct {
	gc        launch.GCMode
	arguments string
}

var gcChoices = []struct {
	mode launch.GCMode
	name string
}{
	{launch.GCZGC, "尽量使用 ZGC"},
	{launch.GCGenerationalZGC, "尽量使用分代 ZGC"},
	{launch.GCG1, "标准 G1GC"},
	{launch.GCTunedG1, "调优 G1GC"},
	{launch.GCNone, "不指定（可自定义）"},
}

func gcChoiceName(mode launch.GCMode) string {
	for _, choice := range gcChoices {
		if choice.mode == mode.Effective() {
			return choice.name
		}
	}
	return "未知策略"
}

func (s *Shell) gameSettingsView(c *ui.Context) {
	t, draft := s.theme, &s.gameSettings
	blocked := s.loadErr != nil || s.save == nil
	ui.Scroll(c.Key("game-settings-scroll")).Grow(1).MinHeight(0).FillWidth().Padding(18, 24, 24).Gap(8).Children(func() {
		settingsGroupLabel(c, "启动")
		ui.Column(c.Key("game-jvm-settings")).FillWidth().Radius(10).Border(0.5, colors(t).ListDivider).Clip().Children(func() {
			gameSettingRow(c, "collector", "内存管理", func() {
				ui.Spacer(c)
				button := ui.ButtonBase(c.Key("game-gc-mode")).Label("JVM 内存管理").Width(190).MinWidth(0).Height(26).
					Radius(6).Padding(0, 9).Gap(8).Background(t.Surface).Disabled(blocked).
					Transition(ui.ElementTransition{Animation: &motionColor, Colors: true})
				button.Menu(func(menu *ui.Menu) {
					for _, choice := range gcChoices {
						if menu.Item(choice.name).Checked(draft.gc.Effective() == choice.mode).Chosen() {
							draft.gc = choice.mode
							s.saveSettings()
						}
					}
				}).Children(func() {
					ui.Text(c, gcChoiceName(draft.gc)).FontSize(12).Grow(1).MinWidth(0).MaxLines(1).Ellipsis("…")
					ui.Icon(c, iconChevronDown).Size(12, 12).Shrink(0).TextColor(colors(t).Metadata)
				})
			})
			gameSettingsDivider(c)
			gameSettingRow(c, "arguments", "JVM 参数", func() {
				ui.TextInput(c.Key("custom-jvm-arguments"), &draft.arguments).Label("自定义 JVM 参数").Placeholder("无").Grow(1).MinWidth(0).Height(26).
					Disabled(blocked).OnChange(s.saveSettings).OnSubmit(s.saveSettings)
			})
		})
		if s.noticeFail && s.notice != "" {
			ui.Text(c, s.notice).FontSize(11).TextColor(t.Danger).MaxLines(3)
		}
	})
}

func gameSettingRow(c *ui.Context, key, label string, controls func()) {
	ui.Row(c.Key(key)).FillWidth().Height(44).Shrink(0).Padding(0, 14).Gap(10).Children(func() {
		ui.Text(c, label).FontSize(12).Width(80).Shrink(0)
		ui.Row(c).Grow(1).MinWidth(0).Gap(8).Children(controls)
	})
}

func gameSettingsDivider(c *ui.Context) {
	ui.Box(c).FillWidth().Height(0.5).Margin(0, 14).Background(colors(c.Theme()).ListDivider)
}

func settingsGroupLabel(c *ui.Context, label string) ui.Element {
	return ui.Text(c, label).FontSize(11).TextColor(colors(c.Theme()).Metadata).Margin(0, 2)
}
