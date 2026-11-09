package ui

import (
	"time"

	"mcl/internal/branding"

	"github.com/egoist/mygo/ui"
)

// Updates is the launcher's self-update service. The shell only reads and
// changes preferences; the platform updater owns checks, downloads,
// signature verification and the update window.
type Updates interface {
	// Enabled reports whether this build can replace itself: development
	// builds and read-only installs cannot.
	Enabled() bool
	// Check opens the update window and checks now, including skipped versions.
	Check()
	AutomaticChecks() bool
	SetAutomaticChecks(bool)
	AutomaticDownloads() bool
	SetAutomaticDownloads(bool)
	LastCheck() time.Time
	// AvailableVersion is the newer version found by the last check, or "".
	AvailableVersion() string
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
	ui.Column(c.Key("about-settings")).Grow(1).MinHeight(0).FillWidth().Children(func() {
		ui.Scroll(c.Key("about-settings-scroll")).Grow(1).MinHeight(0).FillWidth().Padding(18, 24, 24).Gap(12).Children(func() {
			ui.Row(c.Key("about-product")).FillWidth().Padding(16, 16).Gap(14).AlignItems(ui.Center).Radius(10).Border(0.5, p.ListDivider).Children(func() {
				ui.Image(c.Key("about-logo"), brandLogo).Label(branding.FullName).Size(52, 52).Shrink(0).Fit(ui.Contain)
				ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
					ui.Text(c, branding.Name).FontSize(18).FontWeight(600).TextColor(t.Text)
					ui.Text(c, branding.FullName).FontSize(12).TextColor(p.Metadata).MaxLines(1).Ellipsis("…")
				})
				ui.Column(c).Shrink(0).Gap(3).AlignItems(ui.End).Children(func() {
					version := s.version
					if version == "" {
						version = "未知"
					}
					ui.Text(c, "版本 "+version).FontSize(12).TextColor(t.Text)
					if u != nil {
						if available := u.AvailableVersion(); available != "" {
							ui.Text(c, "新版本 "+available+" 可用").FontSize(11).TextColor(p.SettingsNavAccent)
						}
					}
				})
				ui.Button(c.Key("about-check-updates"), "检查更新").Height(28).FontSize(12).Shrink(0).Disabled(u == nil).OnClick(func() {
					if s.updates != nil {
						s.updates.Check()
					}
				})
			})
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
				ui.Text(c, "当前构建不能自动更新：开发版本，或安装位置不可写。请从发布页下载安装包更新。").FontSize(11).TextColor(p.Metadata).Margin(0, 2)
			}
		})
	})
}
