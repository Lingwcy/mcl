package ui

import (
	"path/filepath"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"mcl/internal/app"
)

type page string

const (
	homePage              page = "home"
	libraryPage           page = "library"
	accountsPage          page = "accounts"
	activityPage          page = "activity"
	settingsActivityPage  page = "settings-activity"
	otherActivityPage     page = "other-activity"
	logsPage              page = "logs"
	launchPage            page = "launch"
	settingsPage          page = "settings"
	gameSettingsPage      page = "game-settings"
	wallpaperSettingsPage page = "wallpaper-settings"
	i18nSettingsPage      page = "i18n-settings"
	downloadSettingsPage  page = "download-settings"
	newInstancePage       page = "new-instance"
	existingGamesPage     page = "existing-games"
	directoryPage         page = "directory-transfer"
	instanceSettingsPage  page = "instance-settings"
	marketPage            page = "market"
	serversPage           page = "servers"
	creditsPage           page = "credits"
	aboutPage             page = "about"
)

func (p page) inSettings() bool {
	return p == settingsPage || p == gameSettingsPage || p == wallpaperSettingsPage || p == i18nSettingsPage || p == downloadSettingsPage || p == accountsPage || p == settingsActivityPage || p == otherActivityPage || p == creditsPage || p == aboutPage
}

const (
	DefaultWidth            = 1040
	DefaultHeight           = 680
	MinWidth                = 680
	MinHeight               = 440
	TitlebarHeight  float32 = 34
	railWidth       float32 = 40
	chromeEdge      float32 = 4
	workspaceRadius float32 = 10
	sidebarWidth    float32 = 208
	minSidebarWidth float32 = 156
)

type SaveSettings func(app.Settings, func(error))

// Shell owns persistent view data on the UI thread, never build-scoped elements.
type Shell struct {
	deletedInstances map[string]bool
	window           *mygo.Window
	toasts           toastState
	memory           memoryState
	creation         creationState
	existing         existingGamesState
	directories      directoryState
	instanceEditor   *instanceSettingsState
	page             page
	locale           string
	theme            *ui.Theme
	applyAppearance  func(bool)
	settings         app.Settings
	i18n             app.I18nSettings
	gameSettings     gameSettingsState
	downloadSettings downloadSettingsState
	downloadTools    downloadToolsState
	downloadJobs     downloadTaskState
	gameDirectories  gameDirectoriesState
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
	servers          serverState
	wallpapers       wallpaperState
	accounts         accountState
	java             javaState
	sidebarCollapsed bool
	version          string
	updates          Updates
}

func NewShell(settings app.Settings, dataDir string, loadErr error, save SaveSettings) *Shell {
	return &Shell{
		creation:        creationState{installNow: true},
		gameDirectories: gameDirectoriesState{rows: []app.GameDirectory{{Name: "内置目录", Path: filepath.Join(dataDir, "instances")}}},
		page:            libraryPage,
		locale:          settings.I18n.EffectiveLanguage([]string{app.LangZhHans}),
		theme:           compactTheme(false), settings: settings, i18n: settings.I18n,
		dataDir: dataDir, loadErr: loadErr, save: save,
		gameSettings:     newGameSettingsState(settings),
		downloadSettings: newDownloadSettingsState(settings.Download),
	}
}

func (s *Shell) BindWindow(window *mygo.Window) {
	s.window = window
	s.page = homePage
	mygo.App.SetLanguages(app.SupportedLanguages...)
	s.locale = s.i18n.EffectiveLanguage(mygo.App.PreferredLanguages())
	mygo.App.SetLocale(s.locale)
	window.OnFocus(func() { s.memory.blurred = false; s.syncMemoryObservation(); window.Invalidate() })
	window.OnBlur(func() { s.memory.blurred = true; s.syncMemoryObservation(); window.Invalidate() })
	window.OnClosed(func() {
		s.cancelArchiveIcon(s.market.install)
		s.downloadTools.closed = true
		s.cancelDownloadTools()
		s.memory.blurred = true
		s.syncMemoryObservation()
		s.downloadJobs.closed = true
		s.closeDownloadTask()
		s.existing.closed = true
		s.cancelExistingGames()
		s.servers.closed = true
		s.cancelServers()
		s.wallpapers.closed = true
		s.cancelWallpapers()
		s.directories.closed = true
		s.directories.generation++
		if s.directories.cancel != nil {
			s.directories.cancel()
		}
	})
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
	loc := s.locale
	if loc == "" {
		loc = app.LangZhHans
	}
	*ui.Local(c.Root(), "mcl-locale", func() string { return loc }) = loc
	if s.downloadJobs.selected != "" && s.downloadJobs.owner != s.page {
		s.closeDownloadTask()
	}
	if s.page != directoryPage && s.directories.busy && !s.directories.running {
		s.cancelDirectoryPreview()
	}
	if s.page != existingGamesPage && s.existing.busy {
		s.cancelExistingGames()
	}
	s.syncMemoryObservation()
	if s.page != downloadSettingsPage && s.downloadTools.operation != "" {
		clearing := s.downloadTools.operation == "clear"
		s.cancelDownloadTools()
		s.downloadTools.message = "操作已取消"
		if clearing {
			s.Notify(ToastWarning, "清理已中断，可能已处理部分缓存")
		}

	}
	t := s.theme
	if c.Preferences().HighContrast {
		contrast := *t
		contrast.Scrollbar = t.Scrollbar.Alpha(1.9)
		contrast.ScrollbarWidth = 6
		t = &contrast
	}
	c.SetTheme(t)
	homeGlass := s.page == homePage
	if homeGlass {
		c.Root().Background(t.Background)
		s.homeChromeBackdrop(c)
	} else {
		c.Root().Background(chromeColor(t))
	}
	shell := ui.Column(c.Key("app-shell")).Fill().Gap(0)
	if homeGlass {
		shell.Absolute().Top(0).Left(0).Right(0).Bottom(0)
	}
	entrance := shell.Keyframes("entrance", appEntrance)
	shell.Children(func() {
		s.titlebar(c)
		// Keep the host for the exit snapshot, but let input reach the
		// workspace beneath it. The live launch page owns its own input.
		ui.Column(c.Key("launch-layer")).Absolute().Top(TitlebarHeight).Bottom(0).Left(0).Right(0).PassThrough().Children(func() {
			if s.page == launchPage {
				ui.Column(c.Key("launch-workspace")).Fill().Radius(workspaceRadius).Clip().Background(t.Background).
					Transition(motionLaunchExit).Children(func() { s.launchView(c) })
			}
		})
		if s.page == launchPage {
			return
		}
		content := ui.Row(c).Grow(1).MinHeight(0).FillWidth().Padding(0, chromeEdge, chromeEdge, 0).Gap(0)
		if entrance < 1 {
			content.Translate(0, 4*(1-entrance)).Opacity(0.78 + 0.22*entrance)
		}
		content.Children(func() {
			s.navigationRail(c)
			sidebarWidth := secondarySidebarWidth(c)
			workspaceFill := t.Background
			if homeGlass {
				workspaceFill = ui.Transparent
			}
			workspace := ui.Row(c.Key("workspace")).Grow(1).MinWidth(0).FillHeight().Radius(workspaceRadius).Clip().Background(workspaceFill)
			if homeGlass {
				rim := ui.RGBA(255, 255, 255, 0.32)
				if t.Dark {
					rim = colors(t).Border.Alpha(0.45)
				}
				workspace.Border(0.5, rim)
			}
			target := float32(1)
			if s.sidebarCollapsed {
				target = 0
			}
			shown := max(0, min(workspace.AnimateTo("sidebar", target, motionPanel), 1))
			workspace.Children(func() {
				if !homeGlass && (shown > 0.001 || !s.sidebarCollapsed) {
					// MyGo's stationary nested clips use the innermost radii.
					// Keep the animation viewport's outside corners rounded too.
					ui.Row(c.Key("sidebar-viewport")).Width(sidebarWidth*shown).Shrink(0).FillHeight().Radius(workspaceRadius, 0, 0, workspaceRadius).Clip().Disabled(s.sidebarCollapsed).Children(func() {
						s.sidebar(c).Margin(0, 0, 0, sidebarWidth*(shown-1))
					})
				}
				mainFill := t.Background
				if homeGlass {
					mainFill = ui.Transparent
				}
				ui.Column(c.Key("main-panel")).Grow(1).MinWidth(0).FillHeight().Background(mainFill).Children(func() {
					ui.Column(c.Key(s.contentIdentity())).Grow(1).MinHeight(0).FillWidth().Transition(motionPage).Children(func() {
						switch s.page {
						case homePage:
							s.homeView(c)
						case libraryPage:
							s.instanceView(c)
						case marketPage:
							s.marketView(c)
						case serversPage:
							s.serversView(c)
						case existingGamesPage:
							s.existingGamesView(c)
						case directoryPage:
							s.directoryView(c)
						case newInstancePage:
							s.newInstanceView(c)
						case instanceSettingsPage:
							s.instanceSettingsView(c)
						case settingsPage:
							s.javaView(c)
						case gameSettingsPage:
							s.gameSettingsView(c)
						case wallpaperSettingsPage:
							s.wallpaperSettingsView(c)
						case i18nSettingsPage:
							s.i18nSettingsView(c)
						case downloadSettingsPage:
							s.downloadSettingsView(c)
						case accountsPage:
							s.accountsView(c)
						case activityPage, settingsActivityPage:
							s.activityView(c)
						case otherActivityPage:
							s.legacyActivityView(c)
						case logsPage:
							s.logView(c)
						case creditsPage:
							s.creditsView(c)
						case aboutPage:
							s.aboutView(c)
						}
					})
				})
			})
		})
	})
	s.instanceSearchDialog(c)
	s.feedbackToasts(c)
}

func (s *Shell) empty(c *ui.Context, heading, title string) {
	t := s.theme
	ui.Column(c).Label(s.tr(heading)).Grow(1).FillWidth().Padding(24).Center().Gap(10).Children(func() {
		ui.Icon(c, iconUser).Size(26, 26).TextColor(colors(t).IconMuted)
		ui.Text(c, s.tr(title)).FontSize(12).TextColor(t.TextMuted)
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
	game, err := s.gameSettings.preferences()
	if err != nil {
		s.notice, s.noticeFail = err.Error(), true
		return
	}
	downloads, err := s.downloadSettings.preferences()
	if err != nil {
		s.notice, s.noticeFail = err.Error(), true
		return
	}
	snapshot := s.settings
	snapshot.GCMode, snapshot.JVMArguments = s.gameSettings.gc, strings.TrimSpace(s.gameSettings.arguments)
	snapshot.Game, snapshot.Download, snapshot.I18n = game, downloads, s.i18n
	snapshot.GameDirectoryID = s.gameSettings.directoryID
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
			downloadsChanged := s.settings.Download != snapshot.Download
			s.settings = snapshot
			if downloadsChanged {
				s.downloadPreferencesCommitted()
			}
		}
		if s.savePending {
			s.saveSettings()
		}
	})
}
