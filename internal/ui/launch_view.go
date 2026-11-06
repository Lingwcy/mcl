package ui

import (
	"fmt"
	"math"

	"mcl/internal/app"
	"mcl/internal/core/instance"

	"github.com/egoist/mygo/ui"
)

func (s *Shell) launchGameSnapshot() (app.GameSnapshot, bool) {
	if id := s.installation.launchGameID; id != "" {
		for _, game := range s.games.rows {
			if game.ID == id {
				return game, true
			}
		}
	}
	return app.GameSnapshot{}, false
}

func (s *Shell) waitingForGameWindow(game app.GameSnapshot, found bool) bool {
	i := &s.installation
	return i.launching && i.launchGameID != "" && (!found || game.Running && game.Window == app.GameWindowPending)
}

func (s *Shell) returnToLaunchInstance() {
	s.library.selectedID = s.installation.instanceID
	s.openHome()
}

func (s *Shell) retryLaunch() {
	s.library.selectedID = s.installation.instanceID
	s.startGame()
}

func (s *Shell) launchLogs() {
	s.showLogs(s.installation.launchGameID)
}

func (s *Shell) stopLaunch() {
	i := &s.installation
	if i.active && i.requestLaunchStop != nil {
		i.requestLaunchStop()
	} else if i.launchGameID != "" {
		s.stopGame(i.launchGameID)
	}
	s.returnToLaunchInstance()
}

func (s *Shell) launchView(c *ui.Context) {
	i, t := &s.installation, s.theme
	palette := colors(t)
	game, hasGame := s.launchGameSnapshot()
	waiting := s.waitingForGameWindow(game, hasGame)
	busy := i.active || waiting
	exited := !i.active && hasGame && !game.Running
	earlyExit := exited && game.Window == app.GameWindowPending
	failed := i.failed || exited && !game.Exit.Stopped && (earlyExit || game.Exit.Code != 0)
	running := !i.active && hasGame && game.Running
	windowOpened := running && game.Window == app.GameWindowVisible
	bar := launchBarIdle
	switch {
	case exited || failed:
		bar = launchBarClosed
	case running && !waiting:
		bar = launchBarReady
	case busy && !i.canceling:
		bar = launchBarPreparing
	}
	status := installationStage(i.progress.Stage)
	detail := ""
	switch {
	case i.canceling:
		status = "正在取消准备…"
	case waiting:
		status = "等待游戏窗口…"
	case !i.active:
		status = i.message
		switch {
		case exited && game.Exit.Stopped:
			status = "游戏已停止"
		case exited && failed:
			status = "游戏异常退出"
			if earlyExit {
				status = "游戏在窗口出现前退出"
			}
			detail = fmt.Sprintf("退出代码 %d", game.Exit.Code)
			if game.Exit.Error != "" {
				detail += " · " + game.Exit.Error
			}
		case failed:
			status, detail = "启动未完成", i.message
		case exited:
			status = "游戏已退出"
		case windowOpened:
			status = "游戏窗口已打开"
		case running:
			status = "游戏进程运行中"
		}
	}
	ui.Column(c.Key(struct{ Generation uint64 }{i.generation})).Fill().Gap(12).Children(func() {
		ui.Spacer(c)
		ui.Column(c.Key("launch-center")).FillWidth().MaxWidth(460).AlignSelf(ui.Center).Center().Gap(12).
			Transition(motionLaunchEnter).Children(func() {
			art := ui.Column(c.Key("launch-artwork")).Height(88).FillWidth().Center()
			hop := float32(0)
			if busy && !i.canceling && !c.Preferences().ReduceMotion {
				hop = art.Keyframes("hop", mascotHop, ui.KeyframeOptions{Repeat: ui.RepeatForever})
			}
			art.Translate(0, hop*9).Scale(1-hop*0.025, 1+hop*0.025).Children(func() {
				var value instance.Instance
				for _, row := range s.library.snapshot.Instances {
					if row.ID == i.instanceID {
						value = row
						break
					}
				}
				if value.ID != "" {
					s.instanceAvatar(c, value, 72)
				} else {
					ui.Image(c, brandLogo).Size(64, 64).Fit(ui.Contain)
				}
			})
			ui.Text(c, i.name).FontSize(20).FontWeight(600).MaxLines(1).Ellipsis("…").MinWidth(0)
			ui.Row(c.Key("launch-status")).Height(22).Gap(7).Center().Children(func() {
				switch {
				case busy:
					busyIndicator(c, 13)
				case failed:
					ui.Icon(c, iconClose).Size(15, 15).TextColor(t.Danger)
				case windowOpened:
					ui.Icon(c, iconCheck).Size(15, 15).TextColor(t.Success)
				}
				ui.Text(c, status).FontSize(12).TextColor(t.TextMuted).MaxLines(2)
				if i.active {
					if progress := launchProgressDetail(i); progress != "" {
						ui.Text(c, progress).FontSize(11).TextColor(palette.Metadata)
					}
				}
			})
			if detail != "" {
				ui.Text(c, detail).FontSize(12).TextColor(t.Danger).MaxLines(5).Selectable()
			}
			ui.Row(c.Key("launch-actions")).Height(32).Shrink(0).Gap(4).Center().Children(func() {
				iconButton(c, "launch-logs", "打开终端日志", iconLog, s.launchLogs)
				stopLabel := "停止游戏"
				if i.active {
					stopLabel = "停止启动准备"
				}
				canStop := i.active && !i.canceling && i.requestLaunchStop != nil ||
					i.launchGameID != "" && (!hasGame || game.Running && !game.Stopping)
				iconButton(c, "stop-launch", stopLabel, iconSquare, s.stopLaunch).Disabled(!canStop)
				iconButton(c, "leave-launch", "退出启动页，返回实例", iconBack, s.returnToLaunchInstance)
			})
			if !busy && !running {
				ui.PrimaryButton(c.Key("retry-launch"), "重新启动").Height(28).OnClick(s.retryLaunch)
			}
		})
		ui.Spacer(c)
		s.launchProgressBar(c, bar)
	})
}

func launchProgressDetail(i *installationState) string {
	p := i.progress
	if p.TotalBytes > 0 {
		return fmt.Sprintf("%s / %s", formatBytes(p.Bytes), formatBytes(p.TotalBytes))
	}
	if p.TotalFiles > 0 {
		return fmt.Sprintf("%d / %d", p.Files, p.TotalFiles)
	}
	return ""
}

type launchBarState uint8

const (
	launchBarIdle launchBarState = iota
	launchBarPreparing
	launchBarReady
	launchBarClosed
)

func (s *Shell) launchProgressBar(c *ui.Context, state launchBarState) {
	const segmentWidth float32 = 0.26
	palette := colors(s.theme)
	host := ui.Box(c.Key("launch-bar")).Label("启动状态").FillWidth().Height(10).Shrink(0).Background(palette.Control)
	reduceMotion := c.Preferences().ReduceMotion
	var center, span, readyTint float32
	switch state {
	case launchBarPreparing:
		position := float32(0.5)
		if !reduceMotion {
			position = host.Keyframes("bounce", launchBarBounce, ui.KeyframeOptions{
				Repeat: ui.RepeatForever, Alternate: true,
			})
		}
		center = host.Jump("center", segmentWidth/2+position*(1-segmentWidth))
		span = host.Jump("span", segmentWidth)
		host.Jump("ready-tint", 0)
	case launchBarReady:
		// Center first, then grow a shared span so both ends move together
		// even when the last loading frame was against one edge.
		center = host.AnimateTo("center", 0.5, motionLaunchBarCenter)
		span = host.AnimateTo("span", 1, motionLaunchBarOpen)
		readyTint = host.AnimateTo("ready-tint", 1, motionColor)
	case launchBarClosed:
		center, span = 0.5, 1
	}
	if span <= 0 {
		return
	}
	animated := !reduceMotion && state != launchBarClosed
	var phase float32
	if animated {
		// The same phase continues through the success transition.
		phase = host.Loop("neon", launchBarNeonPeriod, ui.Linear)
	}
	span = min(1, span)
	left := max(0, center-span/2)
	right := min(1, center+span/2)
	host.Draw(func(p *ui.Painter, bounds ui.Rect) {
		fill := ui.Rect{X: bounds.X + left*bounds.W, Y: bounds.Y, W: (right - left) * bounds.W, H: bounds.H}
		radius := min(fill.W/2, 5*(1-(right-left)))
		if state == launchBarClosed {
			p.Fill(fill, palette.Danger, radius)
			return
		}
		if animated {
			drawLaunchNeon(p, bounds, fill, palette, radius, phase, readyTint)
			return
		}
		p.FillGradient(fill, ui.LinearGradient{Stops: []ui.GradientStop{
			{At: 0, Color: palette.LaunchStart.Mix(palette.Success, readyTint)},
			{At: 0.5, Color: palette.LaunchMiddle.Mix(palette.Success, readyTint)},
			{At: 1, Color: palette.LaunchEnd.Mix(palette.Success, readyTint)},
		}, Angle: 90}, radius)
	})
}

func drawLaunchNeon(p *ui.Painter, track, fill ui.Rect, palette interfaceColors, radius, phase, readyTint float32) {
	start := palette.LaunchStart.Mix(palette.Success, readyTint)
	middle := palette.LaunchMiddle.Mix(palette.LaunchReadyMint, readyTint)
	end := palette.LaunchEnd.Mix(palette.LaunchReadyAqua, readyTint)
	var stops [9]ui.GradientStop
	for n := range stops {
		x := float32(n) / float32(len(stops)-1)
		wave := launchLightWave(x-phase)*2 - 1
		tint := middle.Mix(start, max(0, wave)).Mix(end, max(0, -wave))
		crest := launchLightWave(2*x - phase + 0.2)
		stops[n] = ui.GradientStop{At: x, Color: tint.Mix(palette.LaunchGlint, 0.30*crest*crest)}
	}
	p.FillGradient(fill, ui.LinearGradient{Angle: 90, Stops: stops[:]}, radius)
	p.FillGradient(fill, ui.LinearGradient{Stops: []ui.GradientStop{
		{At: 0, Color: palette.Chrome.Alpha(0.18)},
		{At: 0.40, Color: palette.LaunchGlint.Alpha(0)},
		{At: 1, Color: palette.LaunchGlint.Alpha(0.32)},
	}}, radius)

	// Lay out the texture in track coordinates so the short bar and the
	// expanding edges reveal pixels without dragging a trail behind them.
	const inset float32 = 5
	step := max(11, track.W/90)
	first := int((fill.X - track.X) / step)
	last := int((fill.X + fill.W - track.X) / step)
	for row := range 2 {
		for cell := first; cell <= last; cell++ {
			seed := uint32(cell+1)*0x9e3779b9 ^ uint32(row+1)*0x85ebca6b
			seed ^= seed >> 16
			x := track.X + step*(float32(cell)+float32(seed&255)/256)
			size := 1.0 + 0.5*float32(seed>>8&3)
			if x < fill.X+inset || x+size > fill.X+fill.W-inset {
				continue
			}
			y := fill.Y + 1.5 + float32(row)*4.5 + float32(seed>>10&3)*0.25
			twinkle := launchLightWave(phase + float32(seed>>12&255)/256)
			p.Fill(ui.Rect{X: x, Y: y, W: size, H: size},
				palette.LaunchParticle.Alpha(0.18+0.40*twinkle*twinkle), 0)
		}
	}
}

func launchLightWave(phase float32) float32 {
	return 0.5 + 0.5*float32(math.Sin(2*math.Pi*float64(phase)))
}
