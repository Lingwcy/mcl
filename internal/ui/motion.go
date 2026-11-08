package ui

import (
	"math"
	"time"

	"github.com/egoist/mygo/ui"
)

// Keep motion in the native frame scheduler. Idle views own no timers or
// animation goroutines; MyGo also applies the system Reduce Motion preference.
var (
	motionPress  = ui.Animation{Spring: ui.SpringOptions{Duration: 260 * time.Millisecond, Bounce: 0.20}}
	motionLaunch = ui.Animation{Spring: ui.SpringOptions{Duration: 460 * time.Millisecond, Bounce: 0.16}}
	motionPanel  = ui.Animation{Spring: ui.SpringOptions{Duration: 300 * time.Millisecond}}
	motionColor  = ui.Animation{Duration: 140 * time.Millisecond, Ease: ui.EaseOut}
	motionFade   = ui.Animation{Duration: 180 * time.Millisecond, Ease: ui.EaseOut}
	motionPage   = ui.ElementTransition{
		Duration: 200 * time.Millisecond, Ease: ui.EaseOut, Colors: true,
		Enter: &ui.Motion{X: 8, Opacity: 0}, Exit: &ui.Motion{X: -4, Opacity: 0},
	}
	motionToast = ui.ElementTransition{
		Duration: 180 * time.Millisecond, Ease: ui.EaseOut,
		Enter: &ui.Motion{X: 8, Opacity: 0}, Exit: &ui.Motion{X: 6, Opacity: 0},
	}
	motionReveal = ui.ElementTransition{
		Duration: 180 * time.Millisecond, Ease: ui.EaseOut, Colors: true,
		Enter: &ui.Motion{Y: 4, Opacity: 0},
	}
	motionArtwork = ui.ElementTransition{
		Duration: 180 * time.Millisecond, Ease: ui.EaseOut, Colors: true,
		Enter: &ui.Motion{Scale: 0.94, Opacity: 0},
	}
	motionLaunchEnter = ui.ElementTransition{
		Animation: &motionLaunch,
		Enter:     &ui.Motion{Y: 16, Scale: 0.96, Opacity: 0},
	}
	motionLaunchExit = ui.ElementTransition{
		Duration: 240 * time.Millisecond,
		Ease:     ui.EaseOut,
		Exit:     &ui.Motion{Y: 12, Scale: 0.98, Opacity: 0},
	}
)

// Small controls compress under the pointer and spring back on release.
// Large rows use a much smaller compression so text remains easy to track.
func controlMotion(c *ui.Context, button ui.Element, fill ui.Color, enabled bool) ui.Element {
	scale := float32(1)
	enabled = enabled && !button.IsDisabled()
	if enabled {
		if button.Pressed() {
			scale = 0.91
		} else if button.Hovered() {
			scale = 1.035
		}
	}
	return button.Background(controlFill(c.Theme(), button, fill)).
		Transition(ui.ElementTransition{Animation: &motionColor, Colors: true}).
		AnimatePose(ui.Pose{Scale: scale}, motionPress)
}

func rowMotion(c *ui.Context, button ui.Element, fill ui.Color) ui.Element {
	scale := float32(1)
	if !button.IsDisabled() && button.Pressed() {
		scale = 0.99
	}
	return button.Background(controlFill(c.Theme(), button, fill)).
		Transition(ui.ElementTransition{Animation: &motionColor, Colors: true}).
		AnimatePose(ui.Pose{Scale: scale}, motionPress)
}

// Progress follows actual reported work. It never advances on a timer and
// intentionally has no spring overshoot or artificial completion delay.
func smoothProgress(c *ui.Context, value float64) ui.Element {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		value = 0
	}
	host := ui.Box(c).FillWidth().Height(4).Clip()
	target := float32(max(0, min(value, 1)))
	shown := host.AnimateTo("progress", target, motionFade)
	if shown > target {
		shown = host.Jump("progress", target)
	}
	host.Children(func() { ui.Progress(c, float64(shown)).Fill() })
	return host
}

func busyIndicator(c *ui.Context, size float32) {
	icon := ui.Icon(c.Key("busy-indicator"), iconRefresh).Size(size, size).TextColor(colors(c.Theme()).Icon)
	if !c.Preferences().ReduceMotion {
		icon.Rotate(icon.Loop("rotation", 1100*time.Millisecond, ui.Linear) * 360)
	}
}

// Both tracks run once on stable elements of the app window.
var appEntrance = []ui.Keyframe{
	{At: 0, Value: 0},
	{At: 220 * time.Millisecond, Value: 1, Ease: ui.EaseOut},
}

var appLogoEntrance = []ui.Keyframe{
	{At: 0, Value: 0.9},
	{At: 150 * time.Millisecond, Value: 1.04, Ease: ui.EaseOut},
	{At: 280 * time.Millisecond, Value: 1, Ease: ui.EaseInOut},
}

const (
	launchBarNeonPeriod = 2100 * time.Millisecond
	launchBarCenterTime = 145 * time.Millisecond
)

var (
	launchBarBounce = []ui.Keyframe{
		{At: 0, Value: 0},
		{At: 740 * time.Millisecond, Value: 1, Ease: ui.CubicBezier(0.38, 0.04, 0.68, 0.94)},
	}
	motionLaunchBarCenter = ui.Animation{Duration: launchBarCenterTime, Ease: ui.EaseOut}
	motionLaunchBarOpen   = ui.Animation{
		Delay: launchBarCenterTime, Spring: ui.SpringOptions{Duration: 390 * time.Millisecond, Bounce: 0.12},
	}
)

var mascotHop = []ui.Keyframe{
	{At: 0, Value: 0},
	{At: 180 * time.Millisecond, Value: -1, Ease: ui.EaseOut},
	{At: 400 * time.Millisecond, Value: 0, Ease: ui.EaseIn},
	{At: 490 * time.Millisecond, Value: -0.16, Ease: ui.EaseOut},
	{At: 580 * time.Millisecond, Value: 0, Ease: ui.EaseIn},
	{At: 1250 * time.Millisecond, Value: 0},
}

var successPop = []ui.Keyframe{
	{At: 0, Value: 0.8},
	{At: 180 * time.Millisecond, Value: 1.06, Ease: ui.EaseOut},
	{At: 290 * time.Millisecond, Value: 0.99, Ease: ui.EaseInOut},
	{At: 380 * time.Millisecond, Value: 1, Ease: ui.EaseOut},
}

// The pixel cat is reserved for ongoing work, rather than animating the
// titlebar forever. Its stretch and shadow share a single timeline.
func loadingStatus(c *ui.Context, label string) {
	ui.Column(c.Key("loading-status")).Label(label).Grow(1).MinHeight(0).FillWidth().Center().Gap(10).
		Transition(motionReveal).Children(func() {
		stage := ui.Column(c.Key("loading-mascot")).Size(56, 60).Center().Gap(5)
		hop := stage.Keyframes("hop", mascotHop, ui.KeyframeOptions{Repeat: ui.RepeatForever})
		stage.Children(func() {
			ui.Image(c, brandLogo).Size(38, 38).Translate(0, hop*7).Scale(1-hop*0.035, 1+hop*0.035)
			ui.Box(c).Size(23, 3).Radius(2).Background(colors(c.Theme()).Shadow).Scale(1+hop*0.24, 1)
		})
		ui.Text(c.Key("loading-label"), label).FontSize(12).TextColor(c.Theme().TextMuted).
			ContentTransition(ui.ContentTransition{Animation: &motionFade})
		ui.Row(c).Height(8).Gap(5).Children(func() {
			for index := range 3 {
				dot := ui.Box(c.Key(index)).Size(3, 3).Radius(1).Background(c.Theme().Accent)
				level := float32(0)
				if !c.Preferences().ReduceMotion {
					phase := dot.Loop("pulse", 1250*time.Millisecond, ui.Linear)
					level = float32((1 + math.Sin(float64(phase)*2*math.Pi-float64(index)*0.8)) / 2)
				}
				dot.Opacity(0.35+level*0.55).Translate(0, -level*2)
			}
		})
	})
}

// Fixed, clipped skeletons describe the real list geometry without fake
// project names or counts. Only one pulse channel drives the entire group.
func loadingList(c *ui.Context, market bool) {
	height, iconSize := float32(48), float32(30)
	if market {
		height, iconSize = marketProjectRowHeight, marketProjectIconSize
	}
	host := ui.Column(c.Key("loading-list")).Label("正在读取列表").Grow(1).MinHeight(0).FillWidth().Clip().Transition(motionReveal)
	opacity := float32(0.5)
	if !c.Preferences().ReduceMotion {
		phase := host.Loop("pulse", 1400*time.Millisecond, ui.Linear)
		opacity += float32(math.Sin(float64(phase)*2*math.Pi)) * 0.12
	}
	host.Children(func() {
		for index := range 6 {
			ui.Row(c.Key(index)).FillWidth().Height(height).Shrink(0).Padding(8, 12).Gap(12).Opacity(opacity).Children(func() {
				fill := colors(c.Theme()).Skeleton
				ui.Box(c).Size(iconSize, iconSize).Radius(6).Background(fill)
				ui.Column(c).Grow(1).MinWidth(0).Gap(8).Children(func() {
					ui.Box(c).Width(float32(112 + index%3*24)).MaxWidth(160).Height(9).Radius(4).Background(fill)
					ui.Box(c).Width(float32(180 - index%3*25)).MaxWidth(180).Height(6).Radius(3).Background(fill)
				})
				ui.Box(c).Size(34, 7).Radius(3).Background(fill)
			})
		}
	})
}

// Navigation identity excludes input text, progress and loading state. Those
// updates must not rebuild the page identity or discard focus/scroll state.
type contentIdentity struct {
	page                  page
	instance, screen      string
	source, kind, project string
	version               string
}

func (s *Shell) contentIdentity() contentIdentity {
	id := contentIdentity{page: s.page}
	switch s.page {
	case settingsPage:
		if s.java.downloads {
			id.screen = "java-download"
		}
	case libraryPage:
		id.instance = s.library.selectedID
		switch {
		case s.assets.showRepair:
			id.screen = "repair"
		case s.assets.showUpdate:
			id.screen = "update"
		case s.assets.showDiagnostic:
			id.screen = "diagnostic"
		}
	case marketPage:
		m := &s.market
		id.source, id.kind = string(m.source), string(m.kind)
		switch {
		case m.showKey:
			id.screen = "key"
		case m.showInstall && m.install != nil:
			id.screen = "install"
		case m.selected != nil:
			id.project = m.selected.ID
			if m.version != nil {
				id.version = m.version.version.ID
			}
		}
	case newInstancePage:
		switch s.creation.mode {
		case 1:
			id.screen = "import"
		case 2:
			id.screen = "download"
		}
	case instanceSettingsPage:
		id.instance = s.library.selectedID
	}
	return id
}
