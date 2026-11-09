package ui

import (
	"errors"
	"strconv"
	"strings"

	"mcl/internal/app"
	"mcl/internal/core/launch"

	"github.com/egoist/mygo/ui"
)

type gameSettingsState struct {
	directoryID      string
	customResolution bool
	gc               launch.GCMode
	arguments        string
	options          launch.GameSettings
	width            string
	height           string
}

func newGameSettingsState(settings app.Settings) gameSettingsState {
	width, height := settings.Game.Resolution()
	return gameSettingsState{
		directoryID: settings.GameDirectoryID, customResolution: !gameResolutionPreset(width, height),
		gc: settings.GCMode, arguments: settings.JVMArguments, options: settings.Game,
		width: strconv.Itoa(width), height: strconv.Itoa(height),
	}
}

func (g gameSettingsState) preferences() (launch.GameSettings, error) {
	value := g.options
	width, err := strconv.Atoi(strings.TrimSpace(g.width))
	if err != nil {
		return value, errors.New("游戏窗口宽度应为 320–16384 的整数")
	}
	height, err := strconv.Atoi(strings.TrimSpace(g.height))
	if err != nil {
		return value, errors.New("游戏窗口高度应为 200–16384 的整数")
	}
	if width < 320 || width > 16384 || height < 200 || height > 16384 {
		return value, errors.New("游戏分辨率宽度应为 320–16384，高度应为 200–16384")
	}
	// Keep legacy zero/default dimensions until a real resolution is selected.
	if value.Width == 0 && value.Height == 0 && width == 854 && height == 480 {
		width, height = 0, 0
	}
	value.Width, value.Height = width, height
	value.CustomInfo, value.ServerAddress = strings.TrimSpace(value.CustomInfo), strings.TrimSpace(value.ServerAddress)
	return value, value.Validate()
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

var gamePriorityChoices = []downloadChoice{
	{string(launch.PriorityLow), "低"},
	{string(launch.PriorityBelowNormal), "低于正常"},
	{string(launch.PriorityNormal), "正常"},
	{string(launch.PriorityAboveNormal), "高于正常"},
	{string(launch.PriorityHigh), "高"},
}

var gameResolutions = []struct{ width, height int }{{854, 480}, {1280, 720}, {1600, 900}, {1920, 1080}, {2560, 1440}, {3840, 2160}}

func gameResolutionPreset(width, height int) bool {
	for _, preset := range gameResolutions {
		if preset.width == width && preset.height == height {
			return true
		}
	}
	return false
}
func (s *Shell) gameSettingsView(c *ui.Context) {
	t, draft := s.theme, &s.gameSettings
	blocked := s.loadErr != nil || s.save == nil
	ui.Scroll(c.Key("game-settings-scroll")).Grow(1).MinHeight(0).FillWidth().Padding(18, 24, 24).Gap(12).Children(func() {
		ui.Column(c.Key("game-directory-section")).FillWidth().Shrink(0).Gap(6).Children(func() { s.gameDirectoriesView(c, blocked) })
		ui.Column(c.Key("game-window-settings")).FillWidth().Shrink(0).Radius(10).Border(0.5, colors(t).ListDivider).Clip().Children(func() {
			gameSettingRow(c, "resolution", "分辨率", func() {
				ui.Spacer(c)
				if draft.customResolution {
					ui.TextInput(c.Key("game-width"), &draft.width).Label("游戏窗口宽度").Width(60).Height(26).Disabled(blocked).OnChange(s.saveSettings).OnSubmit(s.saveSettings)
					ui.Text(c, "×").FontSize(12).TextColor(colors(t).Metadata)
					ui.TextInput(c.Key("game-height"), &draft.height).Label("游戏窗口高度").Width(60).Height(26).Disabled(blocked).OnChange(s.saveSettings).OnSubmit(s.saveSettings)
				}
				label := draft.width + " × " + draft.height
				width := float32(144)
				if draft.customResolution {
					label = "自定义"
					width = 86
				}
				ui.ButtonBase(c.Key("resolution-presets")).Label("游戏分辨率").Width(width).MinWidth(0).Height(26).Radius(6).Padding(0, 9).Gap(8).Background(t.Surface).Disabled(blocked).
					Menu(func(menu *ui.Menu) {
						for _, preset := range gameResolutions {
							w, h := strconv.Itoa(preset.width), strconv.Itoa(preset.height)
							if menu.Item(w + " × " + h).Checked(!draft.customResolution && draft.width == w && draft.height == h).Chosen() {
								draft.customResolution = false
								draft.width, draft.height = w, h
								s.saveSettings()
							}
						}
						if menu.Item("自定义").Checked(draft.customResolution).Chosen() {
							draft.customResolution = true
						}
					}).Children(func() {
					ui.Text(c, label).FontSize(12).Grow(1).MinWidth(0).MaxLines(1).Ellipsis("…")
					ui.Icon(c, iconChevronDown).Size(12, 12).Shrink(0).TextColor(colors(t).Metadata)
				})
			})
			gameSettingsDivider(c)
			gameSettingRow(c, "fullscreen", "全屏启动", func() {
				ui.Spacer(c)
				ui.Switch(c.Key("game-fullscreen"), &draft.options.Fullscreen).Label("全屏启动").Size(30, 18).Disabled(blocked).OnClick(s.saveSettings)
			})
			gameSettingsDivider(c)
			gameSettingRow(c, "custom-info", "自定义信息", func() {
				ui.TextInput(c.Key("game-custom-info"), &draft.options.CustomInfo).Label("自定义游戏信息").Placeholder("默认").Grow(1).MinWidth(0).Height(26).Disabled(blocked).OnChange(s.saveSettings).OnSubmit(s.saveSettings)
			})
			gameSettingsDivider(c)
			gameSettingRow(c, "auto-join", "自动加入服务器", func() {
				ui.Spacer(c)
				ui.Switch(c.Key("game-auto-join"), &draft.options.AutoJoinServer).Label("自动加入服务器").Size(30, 18).Disabled(blocked).OnClick(s.saveSettings)
			})
			if draft.options.AutoJoinServer {
				gameSettingsDivider(c)
				gameSettingRow(c, "server-address", "服务器", func() {
					ui.TextInput(c.Key("game-server-address"), &draft.options.ServerAddress).Label("服务器地址").Placeholder("mc.example.com:25565").Grow(1).MinWidth(0).Height(26).Disabled(blocked).OnChange(s.saveSettings).OnSubmit(s.saveSettings)
				})
			}
		})
		ui.Column(c.Key("game-jvm-settings")).FillWidth().Shrink(0).Radius(10).Border(0.5, colors(t).ListDivider).Clip().Children(func() {

			gameSettingRow(c, "automatic-memory", "游戏内存", func() { ui.Spacer(c); ui.Text(c, "自动分配").FontSize(12).TextColor(t.TextMuted) })
			gameSettingsDivider(c)
			s.memoryStatusView(c)
			gameSettingsDivider(c)
			gameSettingRow(c, "priority", "进程优先级", func() {
				ui.Spacer(c)
				downloadSettingMenu(c, "game-process-priority", "游戏进程优先级", string(draft.options.Priority), gamePriorityChoices, blocked, func(value string) { draft.options.Priority = launch.ProcessPriority(value); s.saveSettings() })
			})
			gameSettingsDivider(c)
			gameSettingRow(c, "show-logs", "启动后显示日志", func() {
				ui.Spacer(c)
				ui.Switch(c.Key("game-show-logs"), &draft.options.ShowLogs).Label("启动后显示游戏日志").Size(30, 18).Disabled(blocked).OnClick(s.saveSettings)
			})
			gameSettingsDivider(c)
			gameSettingRow(c, "collector", "内存管理", func() {
				ui.Spacer(c)
				choices := make([]downloadChoice, 0, len(gcChoices))
				for _, choice := range gcChoices {
					choices = append(choices, downloadChoice{string(choice.mode), choice.name})
				}
				downloadSettingMenu(c, "game-gc-mode", "JVM 内存管理", string(draft.gc.Effective()), choices, blocked, func(value string) { draft.gc = launch.GCMode(value); s.saveSettings() })
			})
			gameSettingsDivider(c)
			gameSettingRow(c, "arguments", "JVM 参数", func() {
				ui.TextInput(c.Key("custom-jvm-arguments"), &draft.arguments).Label("自定义 JVM 参数").Placeholder("无").Grow(1).MinWidth(0).Height(26).Disabled(blocked).OnChange(s.saveSettings).OnSubmit(s.saveSettings)
			})
		})
	})
}

func gameSettingRow(c *ui.Context, key, label string, controls func()) {
	ui.Row(c.Key(key)).FillWidth().Height(44).Shrink(0).Padding(0, 14).Gap(10).Children(func() {
		ui.Text(c, label).FontSize(12).Width(84).Shrink(0)
		ui.Row(c).Grow(1).MinWidth(0).Gap(8).Children(controls)
	})
}

func gameSettingsDivider(c *ui.Context) {
	ui.Box(c).FillWidth().Height(0.5).Margin(0, 14).Background(colors(c.Theme()).ListDivider)
}

func settingsGroupLabel(c *ui.Context, label string) ui.Element {
	return ui.Text(c, label).FontSize(11).TextColor(colors(c.Theme()).Metadata).Margin(0, 2)
}
