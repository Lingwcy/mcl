package ui

import (
	"fmt"

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

func (s *Shell) instanceSettingsView(c *ui.Context) {
	value, found := s.selectedInstance()
	state, t := s.instanceEditor, s.theme
	if !found || state == nil || state.id != value.ID {
		s.empty(c, "实例设置", "尚未选择实例")
		return
	}
	reason := s.instanceEditBlockReason(state.id)
	disabled := !state.loaded || state.loading || state.conflict || reason != "" || s.library.actions.SavePreferences == nil
	ui.Scroll(c.Key("instance-settings-"+state.id)).Grow(1).MinHeight(0).FillWidth().Padding(18, 24, 24).Gap(16).Children(func() {
		ui.Row(c.Key("instance-settings-header")).FillWidth().Height(26).Gap(8).Children(func() {
			iconButton(c, "back-instance", "返回实例", iconBack, s.openHome)
			ui.Text(c, "实例设置").FontSize(12).Grow(1).MinWidth(0)
			if state.saving {
				busyIndicator(c, 12)
			}
			iconButton(c, "reload-instance-settings", "重新读取并放弃未保存修改", iconRefresh, func() { s.loadInstanceSettings(value) }).Disabled(state.loading || state.saving || s.library.actions.LoadInstance == nil)
		})
		if state.loading {
			loadingStatus(c, "正在读取实例…")
			return
		}
		if state.loadError != "" {
			ui.Text(c, state.loadError).FontSize(12).TextColor(t.Danger).MaxLines(4)
			return
		}
		ui.Column(c.Key("instance-identity-group")).FillWidth().Gap(8).Children(func() {
			settingsGroupLabel(c, "基本信息")
			ui.Column(c).FillWidth().Radius(10).Border(0.5, colors(t).ListDivider).Clip().Children(func() {
				ui.Row(c.Key("instance-icon-row")).FillWidth().Height(64).Padding(0, 14).Gap(10).Children(func() {
					ui.Text(c, "图标").FontSize(12).Grow(1).MinWidth(0)
					if state.iconBusy {
						busyIndicator(c, 12)
					}
					button := ui.ButtonBase(c.Key("edit-instance-icon")).Label("更换实例图标").Tooltip("更换图标").Size(44, 44).Shrink(0).Radius(9).Center().Disabled(disabled || state.iconBusy)
					controlMotion(c, button, ui.Transparent, !disabled && !state.iconBusy).OnClick(func() { state.iconOpen = !state.iconOpen }).Children(func() {
						drawInstanceArtwork(c, instanceSettingsIconStyle(state), state.bitmap, 40)
					})
					creationPopover(c.Key("instance-icon-popover"), button, &state.iconOpen, 270, func() { s.instanceSettingsIconPicker(c, state, disabled) })
				})
				gameSettingsDivider(c)
				gameSettingRow(c, "name", "名称", func() {
					ui.TextInput(c.Key("instance-name"), &state.name).Label("实例名称").Grow(1).MinWidth(0).Height(26).FontSize(12).Disabled(disabled).
						OnChange(func() { s.saveInstancePreferences(state) }).OnSubmit(func() { s.saveInstancePreferences(state) })
				})
				gameSettingsDivider(c)
				gameSettingRow(c, "game-version", "Minecraft", func() {
					instanceSettingValue(c, state.value.Version.ID)
				})
				gameSettingsDivider(c)
				gameSettingRow(c, "loader", "加载器", func() {
					label := "原版"
					if required, ok := state.value.RequiredLoader(); ok {
						label = marketLoaderLabel(required.Type) + " " + required.Version
					}
					instanceSettingValue(c, label)
				})
			})
		})
		ui.Column(c.Key("instance-runtime-group")).FillWidth().Gap(8).Children(func() {
			settingsGroupLabel(c, "运行环境")
			ui.Column(c).FillWidth().Radius(10).Border(0.5, colors(t).ListDivider).Clip().Children(func() {
				gameSettingRow(c, "java", "Java 偏好", func() {
					ui.TextInput(c.Key("instance-java"), &state.javaPath).Label("实例 Java 路径").Placeholder("自动选择").Grow(1).MinWidth(0).Height(26).FontSize(12).Disabled(disabled).
						OnChange(func() { s.saveInstancePreferences(state) }).OnSubmit(func() { s.saveInstancePreferences(state) })
					ui.ButtonBase(c.Key("instance-java-choice")).Label("选择实例 Java").Tooltip("选择 Java").Size(24, 24).Shrink(0).Radius(5).Center().Disabled(disabled).Menu(func(menu *ui.Menu) {
						if menu.Item("自动选择").Checked(state.javaPath == "").Chosen() {
							state.javaPath = ""
							s.saveInstancePreferences(state)
						}
						for _, environment := range s.java.inventory.Environments {
							runtime := environment.Runtime
							label := "Java " + runtime.Version + " · " + javaArchLabel(runtime.Arch) + " · " + compactJavaPath(runtime.Executable)
							if menu.Item(label).Checked(state.javaPath == runtime.Executable).Chosen() {
								state.javaPath = runtime.Executable
								s.saveInstancePreferences(state)
							}
						}
					}).Children(func() { ui.Icon(c, iconChevronDown).Size(12, 12).TextColor(colors(t).Icon) })
				})
				gameSettingsDivider(c)
				gameSettingRow(c, "memory", "游戏内存", func() { instanceSettingValue(c, "自动分配") })
			})
			ui.Row(c).FillWidth().Gap(10).Children(func() {
				ui.Text(c, fmt.Sprintf("Java %d · 启动时检查兼容性", state.value.RequiredJava)).FontSize(11).TextColor(t.TextMuted).Grow(1).MinWidth(0).MaxLines(2)
				ui.Button(c.Key("manage-instance-java"), "Java 管理").Height(26).FontSize(11).Shrink(0).OnClick(func() { s.page = settingsPage })
			})
		})
		ui.Column(c.Key("instance-files-group")).FillWidth().Gap(8).Children(func() {
			settingsGroupLabel(c, "游戏文件")
			ui.Column(c).FillWidth().Radius(10).Border(0.5, colors(t).ListDivider).Clip().Children(func() {
				gameSettingRow(c, "files", "安装文件", func() {
					ui.Spacer(c)
					ui.Button(c.Key("repair-install"), "安装 / 修复").Height(26).FontSize(12).Disabled(state.saving || reason != "" || s.installation.active || s.installation.actions.Start == nil || !supportedPackLoader(value)).OnClick(s.startInstallation)
				})
				gameSettingsDivider(c)
				gameSettingRow(c, "launch", "启动条件", func() {
					ui.Spacer(c)
					ui.Button(c.Key("check-launch"), "检查").Label("检查启动条件").Height(26).FontSize(12).Disabled(state.saving || reason != "" || s.installation.active || s.installation.actions.Check == nil || !supportedPackLoader(value)).OnClick(s.startLaunchCheck)
				})
			})
		})
		message := state.validation
		if message == "" {
			message = state.saveError
		}
		if message != "" {
			ui.Row(c.Key("instance-settings-error")).FillWidth().Gap(10).Children(func() {
				ui.Text(c, message).FontSize(12).TextColor(t.Danger).Grow(1).MinWidth(0).MaxLines(4)
				if state.saveError != "" && !state.conflict {
					ui.Button(c.Key("retry-instance-settings"), "重试").Height(26).Shrink(0).Disabled(state.saving || disabled).OnClick(func() { s.saveInstancePreferences(state) })
				}
			})
		} else if reason != "" {
			ui.Text(c, reason).FontSize(12).TextColor(t.TextMuted).MaxLines(3)
		}
	})
}

func instanceSettingValue(c *ui.Context, label string) {
	ui.Spacer(c)
	ui.Text(c, label).FontSize(12).TextColor(c.Theme().TextMuted).MinWidth(0).MaxLines(1).Ellipsis("…")
}

func (s *Shell) instanceSettingsIconPicker(c *ui.Context, state *instanceSettingsState, disabled bool) {
	ui.Column(c).Width(250).Padding(7).Gap(10).Children(func() {
		ui.Row(c).FillWidth().Gap(8).Children(func() {
			ui.Text(c, "实例图标").FontSize(11).TextColor(s.theme.TextMuted).Grow(1)
			ui.Button(c.Key("automatic-icon"), "自动").Height(23).FontSize(11).Disabled(disabled).OnClick(func() { s.chooseInstanceSettingsIcon(state, "") })
		})
		ui.Row(c).FillWidth().Wrap().Gap(6).Children(func() {
			for _, style := range []string{"grass", "fabric", "forge", "neoforge", "quilt", "cat", "leaf", "spark", "craft"} {
				button := ui.ButtonBase(c.Key(style)).Label("图标 "+style).Tooltip(instancePresetLabel(style)).Size(38, 38).Radius(9).Center().Border(1, ui.Transparent).Disabled(disabled)
				if state.iconStyle == style && len(state.icon) == 0 {
					button.BorderColor(s.theme.Accent)
				}
				controlMotion(c, button, ui.Transparent, !disabled).OnClick(func() { s.chooseInstanceSettingsIcon(state, style) }).Children(func() { drawInstanceArtwork(c, style, nil, 30) })
			}
		})
		ui.Button(c.Key("instance-custom-icon"), "选择图片…").Height(26).FontSize(11).FillWidth().Disabled(disabled || state.iconBusy || s.library.actions.PickIcon == nil).OnClick(func() { s.pickInstanceSettingsIcon(state) })
	})
}
