package ui

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	_ "image/jpeg"
	"math"
	"strconv"

	"mcl/internal/app"
	"mcl/internal/core/auth"

	"github.com/egoist/mygo/ui"
)

//go:embed default_wallpaper.jpg
var defaultWallpaperData []byte

var defaultWallpaperBitmap = decodeDefaultWallpaper()

func decodeDefaultWallpaper() *ui.Bitmap {
	bmp, err := ui.DecodeBitmap(defaultWallpaperData)
	if err != nil {
		panic("invalid embedded default wallpaper: " + err.Error())
	}
	return bmp
}

// WallpaperActions loads, imports, selects, shuffles, configures, and removes
// home-page wallpapers outside the UI thread, delivering callbacks on the UI thread.
type WallpaperActions struct {
	Load        func(done func(app.WallpaperSnapshot, error)) func()
	Add         func(done func(app.WallpaperSnapshot, error)) func()
	Select      func(id string, done func(app.WallpaperSnapshot, error)) func()
	Shuffle     func(done func(app.WallpaperSnapshot, error)) func()
	SaveOptions func(options app.WallpaperOptions, done func(app.WallpaperSnapshot, error)) func()
	Remove      func(id string, done func(app.WallpaperSnapshot, error)) func()
}

type wallpaperState struct {
	actions           WallpaperActions
	snapshot          app.WallpaperSnapshot
	options           app.WallpaperOptions
	draftInitialized  bool
	opacityFloat      float64
	blurFloat         float64
	dimFloat          float64
	active            *ui.Bitmap
	thumbs            map[string]*ui.Bitmap
	showAll           bool
	loaded            bool
	busy              bool
	optionsSaving     bool
	optionsPending    bool
	closed            bool
	generation        uint64
	optionsGeneration uint64
	cancel            func()
	cancelOptions     func()
	error             string
}

const wallpaperTileHeight float32 = 62

func (s *Shell) wallpaperGalleryColumns(c *ui.Context) int {
	winW, _ := c.Size()
	if winW <= 0 {
		winW = DefaultWidth
	}
	sideW := railWidth + chromeEdge
	if !s.sidebarCollapsed {
		sideW += secondarySidebarWidth(c)
	}
	availW := winW - sideW - 44 - 20
	cols := int((availW + 8) / (104 + 8))
	return max(4, min(cols, 6))
}

func (s *Shell) ConnectWallpapers(actions WallpaperActions) {
	s.wallpapers.actions = actions
	s.loadWallpapers()
}

func (s *Shell) cancelWallpapers() {
	w := &s.wallpapers
	w.generation++
	w.optionsGeneration++
	if w.cancel != nil {
		w.cancel()
		w.cancel = nil
	}
	if w.cancelOptions != nil {
		w.cancelOptions()
		w.cancelOptions = nil
	}
	w.busy = false
	w.optionsSaving = false
	w.optionsPending = false
}

func (s *Shell) wallpaperOptions() app.WallpaperOptions {
	return s.wallpapers.options.Normalized()
}

func (s *Shell) activeWallpaperBitmap() *ui.Bitmap {
	if s.wallpapers.active != nil {
		return s.wallpapers.active
	}
	return defaultWallpaperBitmap
}

func (s *Shell) activeWallpaperKey() string {
	if s.wallpapers.active != nil && s.wallpapers.snapshot.ActiveID != "" {
		return s.wallpapers.snapshot.ActiveID
	}
	return "default"
}

func (s *Shell) loadWallpapers() {
	w := &s.wallpapers
	if w.closed || w.actions.Load == nil {
		return
	}
	s.runWallpaperAction(func(done func(app.WallpaperSnapshot, error)) func() {
		return w.actions.Load(done)
	}, "")
}

func (s *Shell) addWallpaper() {
	w := &s.wallpapers
	if w.closed || w.busy || w.actions.Add == nil {
		return
	}
	s.runWallpaperAction(w.actions.Add, "已添加墙纸")
}

func (s *Shell) selectWallpaper(id string) {
	w := &s.wallpapers
	if w.closed || w.busy || w.actions.Select == nil {
		return
	}
	if !s.wallpaperOptions().Shuffle && w.snapshot.ActiveID == id {
		return
	}
	s.runWallpaperAction(func(done func(app.WallpaperSnapshot, error)) func() {
		return w.actions.Select(id, done)
	}, "")
}

func (s *Shell) shuffleWallpaper() {
	w := &s.wallpapers
	if w.closed || w.busy || w.actions.Shuffle == nil || len(w.snapshot.Wallpapers) == 0 {
		return
	}
	s.runWallpaperAction(w.actions.Shuffle, "")
}

func (s *Shell) removeWallpaper(id string) {
	w := &s.wallpapers
	if w.closed || w.busy || w.actions.Remove == nil || id == "" {
		return
	}
	s.runWallpaperAction(func(done func(app.WallpaperSnapshot, error)) func() {
		return w.actions.Remove(id, done)
	}, "已移除墙纸")
}

func (s *Shell) saveWallpaperOptions() {
	w := &s.wallpapers
	if w.closed || w.actions.SaveOptions == nil {
		return
	}
	if w.optionsSaving {
		w.optionsPending = true
		return
	}
	w.optionsPending = false
	target := w.options.Normalized()
	w.options = target
	w.optionsGeneration++
	generation := w.optionsGeneration
	w.optionsSaving = true
	w.cancelOptions = w.actions.SaveOptions(target, func(snap app.WallpaperSnapshot, err error) {
		if w.closed || generation != w.optionsGeneration {
			return
		}
		w.optionsSaving, w.cancelOptions = false, nil
		if err != nil {
			if !errors.Is(err, context.Canceled) {
				w.error = err.Error()
			}
		} else {
			w.loaded = true
			s.applyWallpaperSnapshot(snap)
		}
		if w.optionsPending {
			s.saveWallpaperOptions()
		}
	})
}

func (s *Shell) runWallpaperAction(call func(func(app.WallpaperSnapshot, error)) func(), successToast string) {
	w := &s.wallpapers
	if w.cancel != nil {
		w.cancel()
		w.cancel = nil
	}
	w.generation++
	generation := w.generation
	w.busy, w.error = true, ""
	w.cancel = call(func(snap app.WallpaperSnapshot, err error) {
		if w.closed || generation != w.generation {
			return
		}
		w.busy, w.cancel = false, nil
		if err != nil {
			if !errors.Is(err, context.Canceled) {
				w.error = err.Error()
			}
			return
		}
		w.loaded = true
		s.applyWallpaperSnapshot(snap)
		if successToast != "" {
			s.Notify(ToastSuccess, successToast)
		}
	})
}

func (s *Shell) applyWallpaperSnapshot(snap app.WallpaperSnapshot) {
	w := &s.wallpapers
	w.snapshot = snap
	if !w.optionsSaving && !w.optionsPending {
		w.options = snap.Options.Normalized()
		w.opacityFloat = float64(w.options.Opacity)
		w.blurFloat = float64(w.options.Blur)
		w.dimFloat = float64(w.options.Dim)
		w.draftInitialized = true
	} else {
		// Preserve draft effect sliders while saving; keep external Shuffle changes in sync.
		w.options.Shuffle = snap.Options.Shuffle
	}
	w.active = nil
	if len(snap.ActiveData) > 0 {
		if bmp, err := ui.DecodeBitmap(snap.ActiveData); err == nil {
			w.active = bmp
		}
	}
	thumbs := make(map[string]*ui.Bitmap, len(snap.Thumbnails))
	for id, raw := range snap.Thumbnails {
		if len(raw) > 0 {
			if bmp, err := ui.DecodeBitmap(raw); err == nil {
				thumbs[id] = bmp
			}
		}
	}
	w.thumbs = thumbs
}

func (s *Shell) openHomePage() {
	s.cancelInstanceSettingsIcon()
	s.page = homePage
}

func (s *Shell) homeChromeBackdrop(c *ui.Context) {
	p := colors(s.theme)
	opts := s.wallpaperOptions()
	opacity := float32(opts.Opacity) / 100
	dim := float32(opts.Dim) / 100
	tint := p.Chrome.Alpha(0.42)
	if s.theme.Dark {
		tint = p.Chrome.Alpha(0.50)
	}
	bmp := s.activeWallpaperBitmap()
	key := s.activeWallpaperKey()
	ui.Box(c.Key("home-chrome-backdrop")).Fill().Clip().Children(func() {
		homeDefaultBackdrop(c.Key("chrome-default-bg"), p).Fill()
		ui.Image(c.Key("chrome-wallpaper-"+key), bmp).
			Absolute().Top(-48).Left(-48).Right(-48).Bottom(-48).
			Fit(ui.Cover).Opacity(opacity).Blur(24).Cache()
		if dim > 0 {
			ui.Box(c.Key("chrome-dim")).Absolute().Top(0).Left(0).Right(0).Bottom(0).Background(ui.RGBA(0, 0, 0, dim))
		}
		ui.Box(c.Key("chrome-glass-tint")).Absolute().Top(0).Left(0).Right(0).Bottom(0).Background(tint)
	})
}

func (s *Shell) homeView(c *ui.Context) {
	p := colors(s.theme)
	opts := s.wallpaperOptions()
	opacity := float32(opts.Opacity) / 100
	blur := float32(opts.Blur)
	dim := float32(opts.Dim) / 100

	bmp := s.activeWallpaperBitmap()
	key := s.activeWallpaperKey()

	ui.Box(c.Key("home-workspace")).Fill().Radius(workspaceRadius).Clip().Children(func() {
		homeDefaultBackdrop(c.Key("home-default-bg"), p).Fill().Radius(workspaceRadius)
		pad := blur * 2
		bg := ui.Image(c.Key("home-wallpaper-"+key), bmp).
			Absolute().Top(-pad).Left(-pad).Right(-pad).Bottom(-pad).
			Fit(ui.Cover).Opacity(opacity).Transition(motionArtwork)
		if blur > 0 {
			bg.Blur(blur)
		} else {
			bg.Radius(workspaceRadius)
		}
		if dim > 0 {
			ui.Box(c.Key("home-dim")).Absolute().Top(0).Left(0).Right(0).Bottom(0).
				Radius(workspaceRadius).Background(ui.RGBA(0, 0, 0, dim))
		}
		ui.Column(c.Key("home-overlay")).Absolute().Top(0).Left(0).Right(0).Bottom(0).
			Padding(14, 18, 18, 18).Children(func() {
			ui.Row(c).FillWidth().Justify(ui.End).Gap(6).Children(func() {
				wpBtn := ui.ButtonBase(c.Key("home-wallpaper-shortcut")).
					Label(s.tr("个性化设置")).Tooltip(s.tr("个性化")).
					Size(26, 26).Radius(7).Center()
				s.homeCornerButtonMotion(wpBtn, p).OnClick(func() {
					s.page, s.sidebarCollapsed = wallpaperSettingsPage, false
				}).Children(func() {
					ui.Icon(c, iconPalette).Size(13, 13).TextColor(p.Icon)
				})
			})
			ui.Spacer(c)
			ui.Row(c.Key("home-actions")).FillWidth().Justify(ui.End).Gap(10).AlignItems(ui.End).Children(func() {
				s.homeAccountCard(c)
				s.homeLaunchCard(c)
			})
		})
	})
}

func (s *Shell) homeCornerButtonMotion(btn ui.Element, p interfaceColors) ui.Element {
	fill := p.Raised.Alpha(0.68)
	if btn.IsDisabled() {
		fill = p.Raised.Alpha(0.45)
	} else if btn.Pressed() {
		fill = p.Pressed.Alpha(0.86)
	} else if btn.Hovered() {
		fill = p.Raised.Alpha(0.90)
	}
	return btn.Background(fill).Border(0.5, p.Border.Alpha(0.70)).
		Transition(ui.ElementTransition{Animation: &motionColor, Colors: true})
}

func homeDefaultBackdrop(c *ui.Context, p interfaceColors) ui.Element {
	return ui.Box(c).LinearGradient(ui.LinearGradient{
		Angle: 145,
		Stops: []ui.GradientStop{
			{Color: p.Canvas, At: 0},
			{Color: p.Sidebar, At: 0.55},
			{Color: p.Selected, At: 1},
		},
	})
}

func (s *Shell) homeAccountCard(c *ui.Context) {
	a, t := &s.accounts, s.theme
	p := colors(t)
	profile, hasAccount := a.data.Active()

	accountName := s.tr("未选择角色")
	accountMode := s.tr("点击添加账户")
	if hasAccount {
		accountName = profile.Name
		accountMode = s.tr("离线模式")
		if profile.Kind == auth.Microsoft {
			accountMode = s.tr("正版账户")
		}
	}

	cardBorder := ui.RGBA(255, 255, 255, 0.45)
	if t.Dark {
		cardBorder = ui.RGBA(255, 255, 255, 0.18)
	}

	ui.Row(c.Key("home-account-card")).Width(172).Height(52).Radius(10).
		Background(p.Raised.Alpha(0.84)).Border(0.5, cardBorder).
		Shadow(0, 6, 20, 0, p.Shadow).Padding(6, 7, 6, 10).Gap(4).AlignItems(ui.Center).Children(func() {
		mainBtn := ui.ButtonBase(c.Key("home-account-open")).Label(s.tr("账户 · "+accountName)).Tooltip(s.tr("打开账户管理")).
			Grow(1).MinWidth(0).FillHeight().Radius(7).Padding(0, 4).Gap(8).Justify(ui.Start)
		mainFill := ui.Transparent
		if mainBtn.Hovered() {
			mainFill = p.Control.Alpha(0.65)
		}
		mainBtn.Background(mainFill).Transition(ui.ElementTransition{Animation: &motionColor, Colors: true}).
			OnClick(s.openAccounts).Children(func() {
			s.accountAvatar(c, a.data.ActiveID, 28)
			ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
				ui.Text(c, accountName).FontSize(12).FontWeight(600).TextColor(t.Text).MinWidth(0).SingleLine()
				ui.Text(c, accountMode).FontSize(10).TextColor(p.Secondary).MinWidth(0).SingleLine()
			})
		})

		ui.Column(c).Shrink(0).FillHeight().Justify(ui.Start).Children(func() {
			switchBtn := ui.ButtonBase(c.Key("home-account-switch")).Label(s.tr("切换账户")).Tooltip(s.tr("切换账户")).
				Size(20, 20).Radius(5).Center()
			rowMotion(c, switchBtn, p.Control.Alpha(0.85)).Menu(func(menu *ui.Menu) {
				for _, prof := range a.data.Profiles {
					id := prof.ID()
					mode := s.tr("离线")
					if prof.Kind == auth.Microsoft {
						mode = s.tr("微软")
					}
					if menu.Item(prof.Name + " · " + mode).Checked(id == a.data.ActiveID).Chosen() {
						if id != a.data.ActiveID {
							s.changeAccount(id, a.actions.Select)
						}
					}
				}
				if len(a.data.Profiles) > 0 {
					menu.Separator()
				}
				if menu.Item(s.tr("账户管理…")).Chosen() {
					s.openAccounts()
				}
			}).Children(func() {
				ui.Icon(c, iconSwitch).Size(11, 11).TextColor(p.Icon)
			})
		})
	})
}

func (s *Shell) homeLaunchCard(c *ui.Context) {
	t := s.theme
	p := colors(t)
	inst, hasInst := s.selectedInstance()
	label, detail, action, disabled := s.launchButton()

	title := label
	subtitle := detail
	if !hasInst {
		title = "启动游戏"
		subtitle = "尚未选择游戏实例"
	} else if label == "启动游戏" {
		subtitle = inst.Name
		if inst.Name != inst.Version.ID {
			subtitle = inst.Name + " · " + inst.Version.ID
		}
	}
	title = s.tr(title)
	subtitle = s.tr(subtitle)

	surface := darkColors.Canvas.Alpha(0.76)
	hoverSurface := darkColors.Control.Alpha(0.88)
	borderColor := ui.RGBA(255, 255, 255, 0.26)

	ui.Row(c.Key("home-launch-card")).Width(198).Height(52).Radius(10).
		Background(surface).Border(0.5, borderColor).
		Shadow(0, 8, 24, 0, p.Shadow).Padding(6, 7, 6, 10).Gap(4).AlignItems(ui.Center).Children(func() {
		launchBtn := ui.ButtonBase(c.Key("home-launch-action")).Label(title).Tooltip(title+"\n"+subtitle).
			Grow(1).MinWidth(0).FillHeight().Radius(7).Padding(0, 0, 0, 20).Justify(ui.Center).Disabled(disabled)
		btnFill := ui.Transparent
		scale := float32(1)
		if !disabled {
			if launchBtn.Pressed() {
				scale = 0.98
				btnFill = darkColors.Hover.Alpha(0.35)
			} else if launchBtn.Hovered() {
				btnFill = hoverSurface.Alpha(0.35)
			}
		}
		launchBtn.Background(btnFill).
			Transition(ui.ElementTransition{Animation: &motionColor, Colors: true}).
			AnimatePose(ui.Pose{Scale: scale}, motionPress).
			OnClick(action).Children(func() {
			ui.Column(c).Grow(1).MinWidth(0).AlignItems(ui.Center).Gap(2).Children(func() {
				ui.Text(c, title).FontSize(13).FontWeight(600).TextColor(darkColors.Text).MinWidth(0).SingleLine()
				ui.Text(c, subtitle).FontSize(10).TextColor(darkColors.Secondary).MinWidth(0).SingleLine()
			})
		})

		ui.Column(c).Shrink(0).FillHeight().Justify(ui.Start).Children(func() {
			pickBtn := ui.ButtonBase(c.Key("home-instance-menu")).Label(s.tr("选择游戏实例")).Tooltip(s.tr("切换或新建实例")).
				Size(20, 20).Radius(5).Center()
			pickFill := darkColors.Control.Alpha(0.80)
			if pickBtn.Pressed() {
				pickFill = darkColors.Pressed
			} else if pickBtn.Hovered() {
				pickFill = darkColors.Hover
			}
			pickBtn.Background(pickFill).
				Transition(ui.ElementTransition{Animation: &motionColor, Colors: true}).
				Menu(func(menu *ui.Menu) {
					for _, value := range s.library.snapshot.Instances {
						id := value.ID
						itemLabel := value.Name
						if value.Name != value.Version.ID {
							itemLabel = value.Name + " · " + value.Version.ID
						}
						if menu.Item(itemLabel).Checked(hasInst && inst.ID == id).Chosen() {
							s.selectInstance(id)
						}
					}
					if len(s.library.snapshot.Instances) > 0 {
						menu.Separator()
					}
					if menu.Item(s.tr("新建实例…")).Chosen() {
						s.openNewInstance()
					}
					if menu.Item(s.tr("管理全部实例…")).Chosen() {
						s.openHome()
					}
				}).Children(func() {
				ui.Icon(c, iconPlus).Size(11, 11).TextColor(darkColors.Text)
			})
		})
	})
}

func (s *Shell) wallpaperSettingsView(c *ui.Context) {
	w, t := &s.wallpapers, s.theme
	p := colors(t)
	opts := s.wallpaperOptions()
	if !w.draftInitialized {
		w.opacityFloat = float64(opts.Opacity)
		w.blurFloat = float64(opts.Blur)
		w.dimFloat = float64(opts.Dim)
		w.draftInitialized = true
	}

	ui.Scroll(c.Key("wallpaper-settings-scroll")).Grow(1).MinHeight(0).FillWidth().Padding(16, 22, 20).Gap(14).Children(func() {
		cols := s.wallpaperGalleryColumns(c)
		compactLimit := max(cols*2-3, 3)

		// Section 1: My Wallpapers photo tile gallery
		ui.Column(c.Key("wallpaper-gallery-section")).FillWidth().Shrink(0).Gap(7).Children(func() {
			ui.Row(c.Key("wallpaper-settings-header")).FillWidth().Height(22).Shrink(0).Gap(8).Children(func() {
				settingsGroupLabel(c, s.tr("我的墙纸")).Grow(1)
				if w.busy {
					busyIndicator(c, 12)
				}
				if len(w.snapshot.Wallpapers) > compactLimit {
					toggleLabel := s.tr("显示全部 (" + strconv.Itoa(len(w.snapshot.Wallpapers)) + ")")
					if w.showAll {
						toggleLabel = s.tr("收起")
					}
					toggleBtn := ui.ButtonBase(c.Key("toggle-all-wallpapers")).Label(toggleLabel).Height(22).Padding(0, 8).Radius(5)
					rowMotion(c, toggleBtn, ui.Transparent).OnClick(func() { w.showAll = !w.showAll }).Children(func() {
						ui.Text(c, toggleLabel).FontSize(11).TextColor(p.AccentInk)
					})
				}
			})

			if w.error != "" {
				ui.Text(c, w.error).FontSize(11).TextColor(t.Danger).MaxLines(2)
			}

			wallpapers := w.snapshot.Wallpapers
			if !w.showAll && len(wallpapers) > compactLimit {
				wallpapers = wallpapers[:compactLimit]
			}

			ui.Column(c.Key("wallpaper-gallery-card")).FillWidth().Shrink(0).Radius(10).
				Border(0.5, p.ListDivider).Background(t.Surface).Padding(10).Children(func() {
				ui.Grid(c.Key("wallpaper-gallery")).Columns(cols).Gap(8).FillWidth().Children(func() {
					s.addWallpaperTile(c)
					if len(w.snapshot.Wallpapers) > 0 {
						s.shuffleWallpaperTile(c, opts.Shuffle)
					}
					s.defaultWallpaperTile(c, !opts.Shuffle && w.snapshot.ActiveID == "")
					for _, item := range wallpapers {
						s.wallpaperPhotoTile(c, item, !opts.Shuffle && w.snapshot.ActiveID == item.ID)
					}
				})
			})
		})

		// Section 2: Background Effects
		ui.Column(c.Key("wallpaper-effects-section")).FillWidth().Shrink(0).Gap(7).Children(func() {
			ui.Row(c).FillWidth().Height(22).Shrink(0).Gap(8).Children(func() {
				settingsGroupLabel(c, s.tr("背景效果")).Grow(1)
				if opts.Opacity != 100 || opts.Blur != 0 || opts.Dim != 0 {
					resetBtn := ui.ButtonBase(c.Key("reset-wallpaper-effects")).Label(s.tr("恢复默认效果")).Height(22).Padding(0, 8).Radius(5)
					rowMotion(c, resetBtn, ui.Transparent).OnClick(func() {
						w.options.Opacity = 100
						w.options.Blur = 0
						w.options.Dim = 0
						w.opacityFloat = 100
						w.blurFloat = 0
						w.dimFloat = 0
						s.saveWallpaperOptions()
					}).Children(func() {
						ui.Text(c, s.tr("恢复默认")).FontSize(11).TextColor(p.Metadata)
					})
				}
			})

			ui.Column(c.Key("wallpaper-effects")).FillWidth().Shrink(0).Radius(10).
				Border(0.5, p.ListDivider).Background(t.Surface).Clip().Children(func() {
				wallpaperEffectRow(c, "wp-shuffle-row", "混合随机切换", func() {
					ui.Spacer(c)
					if len(w.snapshot.Wallpapers) > 1 {
						ui.Button(c.Key("wp-shuffle-now"), s.tr("换一张")).Height(22).FontSize(11).
							Disabled(w.busy || w.actions.Shuffle == nil).OnClick(s.shuffleWallpaper)
					}
					shuffleOn := opts.Shuffle
					ui.Switch(c.Key("wp-shuffle"), &shuffleOn).Label(s.tr("混合随机切换")).Size(30, 18).
						Disabled(w.busy || len(w.snapshot.Wallpapers) == 0).OnClick(func() {
						if shuffleOn {
							s.shuffleWallpaper()
						} else {
							w.options.Shuffle = false
							s.saveWallpaperOptions()
						}
					})
				})
				gameSettingsDivider(c)
				wallpaperEffectRow(c, "wp-opacity-row", "背景不透明度", func() {
					ui.Spacer(c)
					slider := ui.Slider(c.Key("wp-opacity"), &w.opacityFloat, 15, 100).Label(s.tr("背景不透明度")).Step(1).Width(176).Shrink(0)
					val := int(math.Round(w.opacityFloat))
					ui.Textf(c, "%d%%", val).FontSize(11).TextColor(p.Metadata).Width(42).TextAlign(ui.End).Shrink(0)
					if slider.Changed() && val != w.options.Opacity {
						w.options.Opacity = val
						s.saveWallpaperOptions()
					}
				})
				gameSettingsDivider(c)
				wallpaperEffectRow(c, "wp-blur-row", "背景模糊", func() {
					ui.Spacer(c)
					slider := ui.Slider(c.Key("wp-blur"), &w.blurFloat, 0, 24).Label(s.tr("背景模糊")).Step(1).Width(176).Shrink(0)
					val := int(math.Round(w.blurFloat))
					ui.Textf(c, "%d px", val).FontSize(11).TextColor(p.Metadata).Width(42).TextAlign(ui.End).Shrink(0)
					if slider.Changed() && val != w.options.Blur {
						w.options.Blur = val
						s.saveWallpaperOptions()
					}
				})
				gameSettingsDivider(c)
				wallpaperEffectRow(c, "wp-dim-row", "背景压暗", func() {
					ui.Spacer(c)
					slider := ui.Slider(c.Key("wp-dim"), &w.dimFloat, 0, 70).Label(s.tr("背景压暗")).Step(1).Width(176).Shrink(0)
					val := int(math.Round(w.dimFloat))
					ui.Textf(c, "%d%%", val).FontSize(11).TextColor(p.Metadata).Width(42).TextAlign(ui.End).Shrink(0)
					if slider.Changed() && val != w.options.Dim {
						w.options.Dim = val
						s.saveWallpaperOptions()
					}
				})
			})
		})
	})
}

func wallpaperEffectRow(c *ui.Context, key, label string, controls func()) {
	ui.Row(c.Key(key)).FillWidth().Height(38).Shrink(0).Padding(0, 14).Gap(10).Children(func() {
		ui.Text(c, tr(c, label)).FontSize(12).Width(96).Shrink(0)
		ui.Row(c).Grow(1).MinWidth(0).Gap(8).AlignItems(ui.Center).Children(controls)
	})
}

func wallpaperSelectedBadge(c *ui.Context, p interfaceColors) {
	ui.Box(c).Absolute().Bottom(5).Right(5).Size(16, 16).Radius(8).Background(p.Accent).Center().Children(func() {
		ui.Icon(c, iconCheck).Size(10, 10).TextColor(p.OnAccent)
	})
}

func (s *Shell) addWallpaperTile(c *ui.Context) {
	w, t := &s.wallpapers, s.theme
	p := colors(t)
	btn := ui.ButtonBase(c.Key("tile-add")).Label(s.tr("添加墙纸")).Tooltip(s.tr("从本地选择一张或多张图片作为墙纸")).
		FillWidth().Height(wallpaperTileHeight).Radius(7).Center().
		Border(0.5, p.Divider).Disabled(w.busy || w.actions.Add == nil)
	rowMotion(c, btn, p.Control).OnClick(s.addWallpaper).Children(func() {
		ui.Column(c).Center().Gap(4).Children(func() {
			ui.Box(c).Size(24, 20).Children(func() {
				ui.Icon(c, iconImage).Size(18, 18).TextColor(p.IconMuted)
				ui.Box(c).Absolute().Top(-2).Right(-2).Size(11, 11).Radius(6).Background(p.Control).Center().Children(func() {
					ui.Icon(c, iconPlus).Size(8, 8).TextColor(p.Secondary)
				})
			})
			ui.Text(c, s.tr("添加墙纸")).FontSize(10).TextColor(p.Secondary)
		})
	})
}

func (s *Shell) shuffleWallpaperTile(c *ui.Context, active bool) {
	w, t := &s.wallpapers, s.theme
	p := colors(t)
	photos := w.snapshot.Wallpapers
	btn := ui.ButtonBase(c.Key("tile-shuffle")).Label(s.tr("混合随机切换")).Tooltip(s.tr("每次启动随机切换，点击立即换一张")).
		FillWidth().Height(wallpaperTileHeight).Radius(7).Clip().
		Disabled(w.busy || w.actions.Shuffle == nil || len(photos) == 0)
	if active {
		btn.Border(2, p.Accent)
	} else {
		btn.Border(0.5, p.Divider)
	}
	btn.OnClick(s.shuffleWallpaper).Children(func() {
		ui.Grid(c).Columns(2).RowTracks(ui.Fr(1), ui.Fr(1)).Gap(0).Fill().Children(func() {
			for cellIdx := range 4 {
				wp := photos[cellIdx%len(photos)]
				thumb := w.thumbs[wp.ID]
				ui.Box(c.Key(cellIdx)).Fill().Clip().Background(p.Control).Children(func() {
					if thumb != nil {
						ui.Image(c.Key("cell"), thumb).Fill().Fit(ui.Cover)
					}
				})
			}
		})
		ui.Box(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).Center().Children(func() {
			ui.Box(c).Size(26, 26).Radius(13).Background(ui.RGBA(0, 0, 0, 0.48)).Center().Children(func() {
				ui.Icon(c, iconRefresh).Size(13, 13).TextColor(ui.RGB(255, 255, 255))
			})
		})
		if active {
			wallpaperSelectedBadge(c, p)
		}
	})
}

func (s *Shell) defaultWallpaperTile(c *ui.Context, active bool) {
	w, t := &s.wallpapers, s.theme
	p := colors(t)
	btn := ui.ButtonBase(c.Key("tile-default")).Label(s.tr("mcl 默认壁纸")).Tooltip(s.tr("mcl 动漫默认壁纸")).
		FillWidth().Height(wallpaperTileHeight).Radius(7).Clip().Disabled(w.busy)
	if active {
		btn.Border(2, p.Accent)
	} else {
		btn.Border(0.5, p.Divider)
	}
	btn.OnClick(func() { s.selectWallpaper("") }).Children(func() {
		ui.Image(c.Key("thumb"), defaultWallpaperBitmap).Fill().Fit(ui.Cover)
		ui.Text(c, s.tr("默认")).FontSize(9).TextColor(ui.RGB(255, 255, 255)).
			Padding(1, 5).Radius(4).Background(ui.RGBA(0, 0, 0, 0.52)).
			Absolute().Bottom(5).Left(5)
		if active {
			wallpaperSelectedBadge(c, p)
		}
	})
}

func (s *Shell) wallpaperPhotoTile(c *ui.Context, wp app.Wallpaper, active bool) {
	w, t := &s.wallpapers, s.theme
	p := colors(t)
	id := wp.ID
	thumb := w.thumbs[id]
	tileRoot := ui.Box(c.Key("tile-" + id)).FillWidth().Height(wallpaperTileHeight)
	tileRoot.Children(func() {
		pickBtn := ui.ButtonBase(c.Key("pick")).Label(s.tr("使用墙纸 " + wp.Name)).
			Tooltip(fmt.Sprintf("%s\n%d × %d", wp.Name, wp.Width, wp.Height)).
			Fill().Radius(7).Clip().Disabled(w.busy)
		if active {
			pickBtn.Border(2, p.Accent)
		} else {
			pickBtn.Border(0.5, p.Divider)
		}
		pickBtn.ContextMenu(func(menu *ui.Menu) {
			if menu.Item(s.tr("设为墙纸")).Chosen() {
				s.selectWallpaper(id)
			}
			menu.Separator()
			if menu.Item(s.tr("移除墙纸")).Chosen() {
				s.removeWallpaper(id)
			}
		}).OnClick(func() { s.selectWallpaper(id) }).Children(func() {
			if thumb != nil {
				ui.Image(c.Key("thumb"), thumb).Fill().Fit(ui.Cover).Transition(motionArtwork)
			} else {
				ui.Box(c).Fill().Background(p.Control).Center().Children(func() {
					ui.Icon(c, iconImage).Size(18, 18).TextColor(p.IconMuted)
				})
			}
			if active {
				wallpaperSelectedBadge(c, p)
			}
		})

		if tileRoot.Hovered() && !w.busy && w.actions.Remove != nil {
			rmBtn := ui.ButtonBase(c.Key("remove")).Label(s.tr("移除 "+wp.Name)).Tooltip(s.tr("移除墙纸")).
				Absolute().Top(5).Right(5).Size(18, 18).Radius(9).Center()
			rmFill := ui.RGBA(0, 0, 0, 0.56)
			if rmBtn.Pressed() {
				rmFill = ui.RGBA(0, 0, 0, 0.85)
			} else if rmBtn.Hovered() {
				rmFill = ui.RGBA(0, 0, 0, 0.74)
			}
			rmBtn.Background(rmFill).OnClick(func() { s.removeWallpaper(id) }).Children(func() {
				ui.Icon(c, iconClose).Size(10, 10).TextColor(ui.RGB(255, 255, 255))
			})
		}
	})
}
