package ui

import (
	"fmt"
	"time"

	"mcl/internal/branding"

	"github.com/egoist/mygo/ui"
)

// UpdatePhase is where an in-app update is. Updaters that own a window of
// their own, such as the macOS one, stay idle and handle Check themselves.
type UpdatePhase int

const (
	UpdateIdle UpdatePhase = iota
	UpdateChecking
	UpdateLatest
	UpdateAvailable
	UpdateDownloading
	UpdateReady
	UpdateFailed
)

// UpdateStatus is a snapshot of the in-app update.
type UpdateStatus struct {
	Phase    UpdatePhase
	Version  string  // the newer version, once found
	Notes    string  // its release notes, Markdown
	Progress float64 // 0–1 while downloading
	Err      string  // why the last check or download failed
}

// Updates is the launcher's self-update service. Checks, downloads and
// signature verification belong to it; the shell only shows its state.
type Updates interface {
	// Enabled reports whether this build can replace itself: development
	// builds and read-only installs cannot.
	Enabled() bool
	// Check checks now, including versions the user skipped.
	Check()
	// Download fetches and verifies the version that Check found.
	Download()
	// Restart installs a downloaded update and relaunches into it.
	Restart()
	Status() UpdateStatus
	AutomaticChecks() bool
	SetAutomaticChecks(bool)
	AutomaticDownloads() bool
	SetAutomaticDownloads(bool)
	LastCheck() time.Time
}

// BindUpdates connects the about page to the running version and its updater.
func (s *Shell) BindUpdates(version string, updates Updates) {
	s.version, s.updates = version, updates
}

func (s *Shell) aboutView(c *ui.Context) {
	t := s.theme
	p := colors(t)
	u := s.updates
	enabled := u != nil && u.Enabled()
	var status UpdateStatus
	if u != nil {
		status = u.Status()
	}
	ui.Column(c.Key("about-settings")).Grow(1).MinHeight(0).FillWidth().Children(func() {
		ui.Scroll(c.Key("about-settings-scroll")).Grow(1).MinHeight(0).FillWidth().Padding(18, 24, 24).Gap(12).Children(func() {
			ui.Row(c.Key("about-product")).FillWidth().Padding(16, 16).Gap(14).AlignItems(ui.Center).Radius(10).Border(0.5, p.ListDivider).Children(func() {
				ui.Image(c.Key("about-logo"), brandLogo).Label(branding.FullName).Size(52, 52).Shrink(0).Fit(ui.Contain)
				ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
					ui.Text(c, branding.Name).FontSize(18).FontWeight(600).TextColor(t.Text)
					ui.Text(c, branding.FullName).FontSize(12).TextColor(p.Metadata).MaxLines(1).Ellipsis("…")
				})
				ui.Column(c).Shrink(1).MinWidth(0).MaxWidth(260).Gap(3).AlignItems(ui.End).Children(func() {
					version := s.version
					if version == "" {
						version = "未知"
					}
					ui.Text(c, "版本 "+version).FontSize(12).TextColor(t.Text)
					if line, accent := updateStatusLine(status); line != "" {
						color := p.Metadata
						if accent {
							color = p.SettingsNavAccent
						}
						ui.Text(c, line).FontSize(11).TextColor(color).MaxLines(2).Ellipsis("…").TextAlign(ui.End)
					}
				})
				label, action, busy := updateAction(s, status)
				ui.Button(c.Key("about-update-action"), label).Height(28).FontSize(12).Shrink(0).Disabled(u == nil || busy).OnClick(action)
			})
			if (status.Phase == UpdateAvailable || status.Phase == UpdateReady) && status.Notes != "" {
				ui.Column(c.Key("about-update-notes")).FillWidth().Padding(14, 16).Gap(8).Radius(10).Border(0.5, p.ListDivider).Children(func() {
					ui.Text(c, status.Version+" 更新内容").FontSize(12).FontWeight(600).TextColor(t.Text)
					ui.Text(c, status.Notes).FontSize(12).TextColor(p.Metadata)
				})
			}
			ui.Column(c.Key("about-update-settings")).FillWidth().Radius(10).Border(0.5, p.ListDivider).Clip().Children(func() {
				gameSettingRow(c, "about-auto-check", "自动检查", func() {
					ui.Text(c, "每天在后台检查一次新版本").FontSize(11).TextColor(p.Metadata).MinWidth(0).MaxLines(1).Ellipsis("…")
					ui.Spacer(c)
					on := enabled && u.AutomaticChecks()
					ui.Switch(c.Key("about-auto-check-switch"), &on).Label("自动检查更新").Size(30, 18).Disabled(!enabled).OnClick(func() {
						if s.updates != nil {
							s.updates.SetAutomaticChecks(on)
						}
					})
				})
				gameSettingsDivider(c)
				gameSettingRow(c, "about-auto-install", "自动安装", func() {
					ui.Text(c, "下载后在下次启动时生效").FontSize(11).TextColor(p.Metadata).MinWidth(0).MaxLines(1).Ellipsis("…")
					ui.Spacer(c)
					on := enabled && u.AutomaticDownloads()
					ui.Switch(c.Key("about-auto-install-switch"), &on).Label("自动下载并安装更新").Size(30, 18).Disabled(!enabled || !u.AutomaticChecks()).OnClick(func() {
						if s.updates != nil {
							s.updates.SetAutomaticDownloads(on)
						}
					})
				})
				gameSettingsDivider(c)
				gameSettingRow(c, "about-last-check", "上次检查", func() {
					label := "尚未检查"
					if u != nil {
						if last := u.LastCheck(); !last.IsZero() {
							label = last.Local().Format("2006-01-02 15:04")
						}
					}
					ui.Text(c, label).FontSize(12).TextColor(p.Metadata)
				})
			})
			if u != nil && !enabled {
				ui.Text(c, "当前构建不能自动更新：开发版本，或所在文件夹不可写。请从发布页下载新版本。").FontSize(11).TextColor(p.Metadata).Margin(0, 2)
			}
		})
	})
}

// updateStatusLine is the line under the version, and whether it is news.
func updateStatusLine(status UpdateStatus) (string, bool) {
	switch status.Phase {
	case UpdateLatest:
		return "已是最新版本", false
	case UpdateAvailable:
		return "新版本 " + status.Version + " 可用", true
	case UpdateDownloading:
		return "正在下载 " + status.Version, false
	case UpdateReady:
		return "新版本 " + status.Version + " 已下载，重启后生效", true
	case UpdateFailed:
		return status.Err, false
	case UpdateIdle:
		if status.Version != "" {
			return "新版本 " + status.Version + " 可用", true
		}
	}
	return "", false
}

// updateAction is the card's button for the phase: its label, what it does
// and whether it is waiting on work in progress.
func updateAction(s *Shell, status UpdateStatus) (string, func(), bool) {
	call := func(f func(Updates)) func() {
		return func() {
			if s.updates != nil {
				f(s.updates)
			}
		}
	}
	switch status.Phase {
	case UpdateChecking:
		return "正在检查…", func() {}, true
	case UpdateAvailable:
		return "下载更新", call(Updates.Download), false
	case UpdateDownloading:
		return fmt.Sprintf("下载中 %d%%", int(status.Progress*100)), func() {}, true
	case UpdateReady:
		return "重启并安装", call(Updates.Restart), false
	}
	return "检查更新", call(Updates.Check), false
}
