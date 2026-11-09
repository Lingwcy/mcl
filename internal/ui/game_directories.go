package ui

import (
	"context"
	"errors"
	"path/filepath"
	"slices"

	"github.com/egoist/mygo/ui"
	"mcl/internal/app"
	"mcl/internal/core/instance"
)

type GameDirectoryActions struct {
	List         func(func([]app.GameDirectory, error)) func()
	Add          func(func(app.GameDirectory, error)) func()
	Rename       func(string, string, func(error)) func()
	Remove       func(string, func(error)) func()
	Open         func(string, func(error)) func()
	OpenInstance func(string, func(error)) func()
}
type gameDirectoriesState struct {
	actions                   GameDirectoryActions
	rows                      []app.GameDirectory
	busy                      bool
	cancel                    func()
	generation                uint64
	editing, name             string
	pickerKey, pickerSelected string
	pickerOpen                bool
}

func (s *Shell) ConnectGameDirectories(actions GameDirectoryActions) {
	s.gameDirectories.actions = actions
	s.refreshGameDirectories()
}
func (s *Shell) refreshGameDirectories() {
	d := &s.gameDirectories
	if d.busy || d.actions.List == nil {
		return
	}
	s.gameDirectoryTask(func(done func(error)) func() {
		return d.actions.List(func(rows []app.GameDirectory, err error) {
			if err == nil {
				d.rows = slices.Clone(rows)
			}
			done(err)
		})
	}, nil)
}

// All callbacks are scheduled on the window by the entrypoint, and carry no
// build-scoped Context/Element. A single slot prevents overlapping mutations.
func (s *Shell) gameDirectoryTask(run func(func(error)) func(), success func()) {
	d := &s.gameDirectories
	if d.busy {
		return
	}
	d.busy = true
	d.generation++
	generation := d.generation
	d.cancel = run(func(err error) {
		if generation != d.generation {
			return
		}
		d.busy = false
		d.cancel = nil
		if err != nil {
			if !errors.Is(err, context.Canceled) {
				s.Notify(ToastError, err.Error())
			}
			return
		}
		if success != nil {
			success()
		}
	})
}
func (s *Shell) addGameDirectory() {
	d := &s.gameDirectories
	if d.busy || d.actions.Add == nil {
		return
	}
	s.gameDirectoryTask(func(done func(error)) func() {
		return d.actions.Add(func(value app.GameDirectory, err error) {
			if err == nil {
				d.rows = append(d.rows, value)
			}
			done(err)
		})
	}, nil)
}
func (s *Shell) renameGameDirectory(id string) {
	d := &s.gameDirectories
	if d.busy || d.actions.Rename == nil {
		return
	}
	name := d.name
	s.gameDirectoryTask(func(done func(error)) func() { return d.actions.Rename(id, name, done) }, func() { d.editing = ""; s.refreshGameDirectories() })
}
func (s *Shell) removeGameDirectory(id string) {
	d := &s.gameDirectories
	if d.busy || d.actions.Remove == nil {
		return
	}
	s.gameDirectoryTask(func(done func(error)) func() { return d.actions.Remove(id, done) }, func() {
		d.rows = slices.DeleteFunc(d.rows, func(row app.GameDirectory) bool { return row.ID == id })
		s.Notify(ToastSuccess, "已移除目录登记，磁盘文件保留")
	})
}
func (s *Shell) openGameLocation(id string) {
	d := &s.gameDirectories
	if d.busy || d.actions.Open == nil {
		return
	}
	s.gameDirectoryTask(func(done func(error)) func() { return d.actions.Open(id, done) }, nil)
}
func (s *Shell) openInstanceGameDirectory(id string) {
	d := &s.gameDirectories
	if d.busy || d.actions.OpenInstance == nil {
		return
	}
	s.gameDirectoryTask(func(done func(error)) func() { return d.actions.OpenInstance(id, done) }, nil)
}
func (s *Shell) gameDirectoryName(id string) string {
	for _, row := range s.gameDirectories.rows {
		if row.ID == id {
			return row.Name
		}
	}
	if id == "" {
		return "内置目录"
	}
	return "目录未登记"
}
func (s *Shell) instanceGamePath(value instance.Instance) string {
	if value.GameDirectoryID == "" {
		return filepath.Join(s.dataDir, "instances", value.ID, "game")
	}
	for _, row := range s.gameDirectories.rows {
		if row.ID == value.GameDirectoryID {
			return filepath.Join(row.Path, value.ID, "game")
		}
	}
	return ""
}
func (s *Shell) creationGameDirectoryID() string {
	if s.creation.directoryID != nil {
		return *s.creation.directoryID
	}
	return s.settings.GameDirectoryID
}
func (s *Shell) creationSettings() app.Settings {
	value := s.settings
	value.GameDirectoryID = s.creationGameDirectoryID()
	return value
}

// The selector is an anchored panel, not a native one-line menu: players
// need both the registered name and the actual installation path to choose.
func (s *Shell) gameDirectoryMenu(c *ui.Context, key, selected string, blocked bool, choose func(string)) {
	d, t := &s.gameDirectories, s.theme
	button := ui.ButtonBase(c.Key(key)).Label("选择游戏目录").Grow(1).MinWidth(0).Height(28).Padding(0, 5).Gap(7).Radius(5).Disabled(blocked)
	fill := ui.Transparent
	if !blocked && button.Hovered() {
		fill = t.SurfaceHover
	}
	rowMotion(c, button, fill).OnClick(func() {
		wasOpen := d.pickerKey == key && d.pickerOpen
		d.pickerKey, d.pickerSelected, d.pickerOpen = key, selected, !wasOpen
		s.creation.kindOpen, s.creation.versionOpen, s.creation.loaderOpen, s.creation.iconOpen = false, false, false, false
		if !wasOpen {
			s.refreshGameDirectories()
		}
	}).Children(func() {
		ui.Text(c, s.gameDirectoryName(selected)).FontSize(12).TextColor(t.TextMuted).Grow(1).MinWidth(0).MaxLines(1).Ellipsis("…")
		ui.Icon(c, iconChevronDown).Size(10, 10).Shrink(0).TextColor(colors(t).Icon)
	})
	if d.pickerKey != key {
		return
	}
	width, height := c.Size()
	creationPopover(c.Key(key+"-panel"), button, &d.pickerOpen, min(440, width-32), func() {
		ui.Column(c.Key("directory-choice-panel")).FillWidth().Gap(6).Padding(6).Children(func() {
			ui.Row(c).FillWidth().Height(26).Gap(6).Children(func() {
				ui.Text(c, "选择游戏目录").FontSize(12).FontWeight(600).Grow(1)
				if d.busy {
					busyIndicator(c, 12)
				}
				iconButton(c, "refresh-directory-choices", "刷新游戏目录", iconRefresh, s.refreshGameDirectories).Disabled(d.busy)
			})
			ui.Scroll(c.Key("directory-choice-scroll")).FillWidth().Height(min(float32(max(1, len(d.rows)))*56, min(280, height*.5))).Children(func() {
				ui.RadioGroup(c.Key("directory-choices"), func() {
					for index, row := range d.rows {
						if index > 0 {
							gameSettingsDivider(c)
						}
						commit := func() {
							if !d.pickerOpen || row.Problem != "" {
								return
							}
							d.pickerOpen = false
							choose(row.ID)
						}
						radio := ui.RadioBase(c.Key("directory-"+row.ID), &d.pickerSelected, row.ID).Label(row.Name+" · "+row.Path).FillWidth().MinHeight(54).Padding(8, 10).Gap(10).Radius(6).Disabled(blocked || row.Problem != "").OnClick(commit).OnChange(commit)
						background := ui.Transparent
						if radio.Hovered() {
							background = t.SurfaceHover
						}
						rowMotion(c, radio, background).Children(func() {
							selected := d.pickerSelected == row.ID
							ring := ui.Box(c).Size(16, 16).Shrink(0).Radius(8).Border(1.5, t.Border).Center()
							if selected {
								ring.Background(t.Accent).BorderColor(t.Accent)
							}
							ring.Children(func() {
								if selected {
									ui.Box(c).Size(6, 6).Radius(3).Background(colors(t).OnAccent)
								}
							})
							ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
								ui.Text(c, row.Name).FontSize(12)
								// Paths wrap rather than being hidden behind an ellipsis/tooltip.
								ui.Text(c, row.Path).FontSize(10).TextColor(colors(t).Metadata).FillWidth()
								if row.Problem != "" {
									ui.Text(c, row.Problem).FontSize(11).TextColor(t.Danger).FillWidth()
								}
							})
						})
					}
				}).Label("游戏安装位置").FillWidth().Gap(0)
			})
			ui.Button(c.Key("manage-game-directories"), "管理游戏目录…").Height(26).FontSize(11).FillWidth().OnClick(func() { d.pickerOpen = false; s.page = gameSettingsPage; s.refreshGameDirectories() })
		})
	})
}

func (s *Shell) gameDirectoryAvailable(id string) bool {
	for _, row := range s.gameDirectories.rows {
		if row.ID == id {
			return row.Problem == ""
		}
	}
	return false
}
func (s *Shell) chooseCreationGameDirectory(id string) {
	if s.library.busy {
		return
	}
	if !s.gameDirectoryAvailable(id) {
		s.Notify(ToastError, "游戏目录不可用，请刷新并重新选择")
		return
	}
	s.creation.directoryID = &id
}

func (s *Shell) gameDirectoriesView(c *ui.Context, blocked bool) {
	d, t := &s.gameDirectories, s.theme
	ui.Row(c.Key("game-locations-header")).FillWidth().Height(24).Gap(6).Children(func() {
		ui.Text(c, "游戏目录").FontSize(12).Grow(1)
		if d.busy {
			busyIndicator(c, 12)
		}
		iconButton(c, "refresh-game-locations", "刷新游戏目录", iconRefresh, s.refreshGameDirectories).Disabled(d.busy)
		iconButton(c, "add-game-location", "添加游戏目录", iconPlus, s.addGameDirectory).Disabled(blocked || d.busy || d.actions.Add == nil)
	})
	ui.Column(c.Key("game-locations")).FillWidth().Shrink(0).Radius(10).Border(0.5, colors(t).ListDivider).Clip().Children(func() {
		for i, row := range d.rows {
			if i > 0 {
				gameSettingsDivider(c)
			}
			ui.Row(c.Key("location-"+row.ID)).FillWidth().MinHeight(48).Padding(6, 14).Gap(9).Children(func() {
				ui.Icon(c, iconFolder).Size(14, 14).Shrink(0).TextColor(colors(t).Icon)
				ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
					if d.editing == row.ID && row.ID != "" {
						ui.TextInput(c.Key("location-name-"+row.ID), &d.name).Label("游戏目录名称").FillWidth().Height(25).Disabled(d.busy).OnSubmit(func() { s.renameGameDirectory(row.ID) })
					} else {
						label := row.Name
						if s.gameSettings.directoryID == row.ID {
							label += " · 默认"
						}
						ui.Text(c, label).FontSize(12).MaxLines(1).Ellipsis("…")
					}
					ui.Text(c, row.Path).FontSize(10).TextColor(colors(t).Metadata).MaxLines(1).Ellipsis("…")
					if row.Problem != "" {
						ui.Text(c, row.Problem).FontSize(11).TextColor(t.Danger).MaxLines(2)
					}
				})
				if d.editing == row.ID && row.ID != "" {
					iconButton(c, "save-location-name", "保存目录名称", iconCheck, func() { s.renameGameDirectory(row.ID) }).Disabled(d.busy)
					iconButton(c, "cancel-location-name", "取消重命名", iconClose, func() { d.editing = "" }).Disabled(d.busy)
				} else {
					iconButton(c, "open-location", "打开游戏目录", iconFolder, func() { s.openGameLocation(row.ID) }).Disabled(d.busy || row.Problem != "" || d.actions.Open == nil)
					services := c.Services()
					ui.ButtonBase(c.Key("location-menu")).Label("游戏目录操作").Tooltip("目录操作").Size(25, 25).Radius(5).Center().Disabled(blocked || d.busy).
						Menu(func(menu *ui.Menu) {
							if menu.Item("设为默认").Checked(s.gameSettings.directoryID == row.ID).Disabled(row.Problem != "").Chosen() {
								s.gameSettings.directoryID = row.ID
								s.saveSettings()
							}
							if menu.Item("复制路径").Chosen() {
								services.WriteClipboard(row.Path)
							}
							if row.ID != "" {
								if menu.Item("重命名…").Chosen() {
									d.editing, d.name = row.ID, row.Name
								}
								if menu.Item("移除登记…").Disabled(row.Instances > 0 || s.gameSettings.directoryID == row.ID || s.settings.GameDirectoryID == row.ID).Chosen() {
									s.removeGameDirectory(row.ID)
								}
							}
						}).Children(func() { ui.Icon(c, iconSliders).Size(14, 14).TextColor(colors(t).Icon) })
				}
			})
		}
	})
}
