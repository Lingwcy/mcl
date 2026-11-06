package ui

import (
	"github.com/egoist/mygo/ui"
)

func (s *Shell) libraryStatus(c *ui.Context) {
	l := &s.library
	if l.busy {
		ui.Row(c.Key("library-busy")).Gap(7).Transition(motionReveal).Children(func() {
			busyIndicator(c, 12)
			ui.Text(c, "正在处理…").FontSize(11).TextColor(s.theme.TextMuted)
		})
	}
	if l.message != "" {
		color := s.theme.TextMuted
		if l.failed {
			color = s.theme.Danger
		}
		ui.Text(c, l.message).FontSize(11).TextColor(color).MaxLines(3)
	}
}

func (s *Shell) versionsView(c *ui.Context) {
	l, t := &s.library, s.theme
	ui.Column(c).Grow(1).FillWidth().Padding(8, 28, 20).Gap(14).Children(func() {
		ui.Row(c).Children(func() {
			ui.Text(c, "切换版本").FontSize(18).Grow(1)
			ui.Button(c.Key("refresh-library"), "刷新版本").Height(24).Disabled(l.busy).OnClick(func() { s.refreshLibrary(true) })
		})
		s.libraryStatus(c)
		rows := l.snapshot.Instances
		if len(rows) == 0 {
			ui.Text(c, "尚未添加游戏").FontSize(12).TextColor(t.TextMuted)
		}
		l.pickerList.Key = func(index int) any { return rows[index].ID }
		l.pickerList.Label = func(index int) string { return rows[index].Name }
		ui.List(c.Key("instance-list"), &l.pickerList, len(rows), func(index int) {
			value := rows[index]
			ui.ButtonBase(c.Key("pick-"+value.ID)).Label("选择实例 "+value.Name).FillWidth().Height(44).Padding(0, 14).Gap(12).BorderWidth(0, 0, 1, 0).BorderColor(t.Border).
				OnClick(func() { s.selectInstance(value.ID) }).Children(func() {
				ui.Icon(c, iconCube).Size(16, 16).TextColor(colors(t).AccentInk)
				ui.Text(c, value.Name).Grow(1).MaxLines(1).Ellipsis("…")
				ui.Text(c, value.Version.ID).FontSize(12).TextColor(t.TextMuted)
			})
		}).Grow(1).FillWidth().Radius(10).Border(1, t.Border).Background(t.Surface)
	})
}

func (s *Shell) instanceSettingsView(c *ui.Context) {
	value, found := s.selectedInstance()
	if !found {
		s.empty(c, "版本设置", "尚未选择实例")
		return
	}
	l, t := &s.library, s.theme
	ui.Scroll(c).Grow(1).FillWidth().Padding(18, 24, 24).Gap(18).Children(func() {
		s.libraryStatus(c)
		section(c, "实例", func() {
			ui.Row(c).Gap(12).Children(func() {
				ui.Text(c, "名称").Width(70)
				ui.TextInput(c.Key("rename-input-"+value.ID), &l.editName).Label("实例名称").Grow(1).Height(26).Disabled(l.busy).OnChange(func() { l.editing = value.ID })
				ui.Button(c.Key("save-name-"+value.ID), "保存名称").Height(26).Disabled(l.busy || l.editing == "").OnClick(s.renameInstance)
			})
			ui.Divider(c)
			ui.Textf(c, "Minecraft %s · %s · Java %d+ · 自动内存", value.Version.ID, instanceType(value), value.RequiredJava).FontSize(12).TextColor(t.TextMuted)
			if runtime := s.instanceRuntimeStatus(value); runtime != "" {
				ui.Text(c, runtime).FontSize(11).TextColor(t.TextMuted)
			}
		})
		section(c, "运行环境", func() {
			ui.Row(c).Gap(12).Children(func() {
				ui.Text(c, "Java 路径").Width(70)
				ui.TextInput(c.Key("instance-java-"+value.ID), &l.editJava).Label("实例 Java 路径").Placeholder("留空使用全局 Java").Grow(1).MinWidth(0).Height(26).Disabled(l.busy)
			})
			ui.Row(c).Gap(8).Children(func() {
				ui.Button(c.Key("save-runtime"), "保存运行设置").Height(26).Disabled(l.busy || l.actions.SaveRuntime == nil).OnClick(s.saveInstanceRuntime)
				ui.Button(c.Key("configure-java"), "全局 Java 设置").Height(26).OnClick(func() { s.page = settingsPage })
			})
		})
		section(c, "游戏文件", func() {

			ui.Button(c.Key("repair-install"), "安装 / 修复").Height(26).Disabled(s.installation.active || s.installation.actions.Start == nil || !supportedPackLoader(value)).OnClick(s.startInstallation)
			ui.Button(c.Key("check-launch"), "检查启动条件").Height(26).Width(140).Disabled(s.installation.active || s.installation.actions.Check == nil || !supportedPackLoader(value)).OnClick(s.startLaunchCheck)
		})
	})
}
