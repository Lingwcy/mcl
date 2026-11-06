package ui

import (
	"strings"

	"mcl/internal/app"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

type page string

const (
	libraryPage          page = "library"
	accountsPage         page = "accounts"
	activityPage         page = "activity"
	settingsActivityPage page = "settings-activity"
	logsPage             page = "logs"
	launchPage           page = "launch"
	settingsPage         page = "settings"
	gameSettingsPage     page = "game-settings"
	newInstancePage      page = "new-instance"
	versionsPage         page = "versions"
	instanceSettingsPage page = "instance-settings"
	marketPage           page = "market"
)

func (p page) inSettings() bool {
	return p == settingsPage || p == gameSettingsPage || p == accountsPage || p == settingsActivityPage
}

const (
	DefaultWidth            = 1040
	DefaultHeight           = 680
	MinWidth                = 680
	MinHeight               = 440
	titlebarHeight  float32 = 44
	railWidth       float32 = 52
	sidebarWidth    float32 = 208
	minSidebarWidth float32 = 156
)

type SaveSettings func(app.Settings, func(error))

// Shell owns persistent view data on the UI thread, never build-scoped elements.
type Shell struct {
	window           *mygo.Window
	creation         creationState
	page             page
	theme            *ui.Theme
	applyAppearance  func(bool)
	settings         app.Settings
	gameSettings     gameSettingsState
	dataDir          string
	loadErr          error
	save             SaveSettings
	saving           bool
	savePending      bool
	notice           string
	noticeFail       bool
	library          libraryState
	installation     installationState
	games            gameState
	assets           assetState
	market           marketState
	accounts         accountState
	java             javaState
	sidebarCollapsed bool
}

func NewShell(settings app.Settings, dataDir string, loadErr error, save SaveSettings) *Shell {
	return &Shell{
		creation: creationState{installNow: true},
		page:     libraryPage, theme: compactTheme(false), settings: settings,
		dataDir: dataDir, loadErr: loadErr, save: save,
		gameSettings: gameSettingsState{gc: settings.GCMode, arguments: settings.JVMArguments},
	}
}

func (s *Shell) BindWindow(window *mygo.Window) {
	s.window = window
	window.OnMaximize(window.Invalidate)
	window.OnUnmaximize(window.Invalidate)
	window.Invalidate()
}

func (s *Shell) BindAppearance(dark bool, apply func(bool)) {
	s.SetAppearance(dark)
	s.applyAppearance = apply
}

func (s *Shell) SetAppearance(dark bool) { s.theme = compactTheme(dark) }

func (s *Shell) toggleAppearance() {
	s.SetAppearance(!s.theme.Dark)
	if s.applyAppearance != nil {
		s.applyAppearance(s.theme.Dark)
	}
}

func (s *Shell) View(c *ui.Context) {
	t := s.theme
	if c.Preferences().HighContrast {
		contrast := *t
		contrast.Scrollbar = t.Scrollbar.Alpha(1.9)
		contrast.ScrollbarWidth = 6
		t = &contrast
	}
	c.SetTheme(t)
	c.Root().Background(chromeColor(t))
	shell := ui.Column(c.Key("app-shell")).Fill().Gap(0)
	entrance := shell.Keyframes("entrance", appEntrance)
	shell.Children(func() {
		s.titlebar(c)
		// Keep the host for the exit snapshot, but let input reach the
		// workspace beneath it. The live launch page owns its own input.
		ui.Column(c.Key("launch-layer")).Absolute().Top(titlebarHeight).Bottom(0).Left(0).Right(0).PassThrough().Children(func() {
			if s.page == launchPage {
				ui.Column(c.Key("launch-workspace")).Fill().Radius(12).Clip().Background(t.Background).
					Transition(motionLaunchExit).Children(func() { s.launchView(c) })
			}
		})
		if s.page == launchPage {
			return
		}
		content := ui.Row(c).Grow(1).MinHeight(0).FillWidth().Padding(0, 6, 6, 0).Gap(0)
		if entrance < 1 {
			content.Translate(0, 4*(1-entrance)).Opacity(0.78 + 0.22*entrance)
		}
		content.Children(func() {
			s.navigationRail(c)
			sidebarWidth := secondarySidebarWidth(c)
			workspace := ui.Row(c.Key("workspace")).Grow(1).MinWidth(0).FillHeight().Radius(12).Clip().Background(t.Background)
			target := float32(1)
			if s.sidebarCollapsed {
				target = 0
			}
			shown := max(0, min(workspace.AnimateTo("sidebar", target, motionPanel), 1))
			workspace.Children(func() {
				if shown > 0.001 || !s.sidebarCollapsed {
					// MyGo's stationary nested clips use the innermost radii.
					// Keep the animation viewport's outside corners rounded too.
					ui.Row(c.Key("sidebar-viewport")).Width(sidebarWidth*shown).Shrink(0).FillHeight().Radius(12, 0, 0, 12).Clip().Disabled(s.sidebarCollapsed).Children(func() {
						s.sidebar(c).Margin(0, 0, 0, sidebarWidth*(shown-1))
					})
				}
				ui.Column(c.Key("main-panel")).Grow(1).MinWidth(0).FillHeight().Background(t.Background).Children(func() {
					if s.loadErr != nil {
						ui.Text(c, "无法读取设置，原文件已保留").FontSize(11).TextColor(t.Danger).Margin(12, 24, 0)
					}
					ui.Column(c.Key(s.contentIdentity())).Grow(1).MinHeight(0).FillWidth().Transition(motionPage).Children(func() {
						switch s.page {
						case libraryPage:
							s.instanceView(c)
						case marketPage:
							s.marketView(c)
						case newInstancePage:
							s.newInstanceView(c)
						case versionsPage:
							s.versionsView(c)
						case instanceSettingsPage:
							s.instanceSettingsView(c)
						case settingsPage:
							s.javaView(c)
						case gameSettingsPage:
							s.gameSettingsView(c)
						case accountsPage:
							s.accountsView(c)
						case activityPage, settingsActivityPage:
							s.activityView(c)
						case logsPage:
							s.logView(c)
						}
					})
				})
			})
		})
	})
	s.instanceSearchDialog(c)
}

func (s *Shell) empty(c *ui.Context, heading, title string) {
	t := s.theme
	ui.Column(c).Label(heading).Grow(1).FillWidth().Padding(24).Center().Gap(10).Children(func() {
		ui.Icon(c, iconUser).Size(26, 26).TextColor(colors(t).IconMuted)
		ui.Text(c, title).FontSize(12).TextColor(t.TextMuted)
	})
}

func (s *Shell) clearNotice() { s.notice, s.noticeFail = "", false }

func (s *Shell) saveSettings() {
	if s.loadErr != nil || s.save == nil {
		return
	}
	// Serialize persistence with Java selection. Coalesce edits made during a
	// write into the latest draft; never disable the input or lose its focus.
	s.clearNotice()
	if s.saving {
		s.savePending = true
		return
	}
	s.savePending = false
	snapshot := s.settings
	snapshot.GCMode, snapshot.JVMArguments = s.gameSettings.gc, strings.TrimSpace(s.gameSettings.arguments)
	if err := snapshot.Validate(); err != nil {
		s.notice, s.noticeFail = err.Error(), true
		return
	}
	if snapshot == s.settings {
		return
	}
	s.saving, s.notice, s.noticeFail = true, "", false
	s.save(snapshot, func(err error) {
		s.saving = false
		if err != nil {
			s.notice, s.noticeFail = err.Error(), true
		} else {
			s.settings = snapshot
		}
		if s.savePending {
			s.saveSettings()
		}
	})
}
