package ui

import "github.com/egoist/mygo/ui"

func compactTheme(dark bool) *ui.Theme {
	t := ui.LightTheme()
	if dark {
		t = ui.DarkTheme()
	}
	p := colors(t)
	t.Background, t.Surface = p.Canvas, p.Control
	t.SurfaceHover, t.SurfacePressed = p.Hover, p.Pressed
	t.Text, t.TextMuted, t.Border = p.Text, p.Secondary, p.Border
	t.FontSize, t.Spacing, t.Radius = 13, 4, 6
	t.Accent, t.AccentHover, t.AccentPressed, t.AccentText = p.Accent, p.AccentHover, p.AccentPressed, p.OnAccent
	t.Selection, t.Focus = p.TextSelection, p.Focus
	t.Success, t.Warning, t.Danger = p.Success, p.Warning, p.Danger
	t.Inverse, t.InverseText, t.Scrollbar = p.Inverse, p.InverseText, p.Scrollbar
	t.ScrollbarWidth = 4
	return t
}

func sidebarColor(t *ui.Theme) ui.Color {
	return colors(t).Sidebar
}

func chromeColor(t *ui.Theme) ui.Color {
	return colors(t).Chrome
}

func section(c *ui.Context, title string, body func()) {
	ui.Column(c).Gap(8).Children(func() {
		ui.Text(c, title).FontSize(11).TextColor(c.Theme().TextMuted)
		ui.Column(c).FillWidth().Padding(14).Gap(12).Radius(10).Border(1, colors(c.Theme()).Divider).Background(c.Theme().Surface).Children(body)
	})
}

func iconButton(c *ui.Context, key, label string, icon *ui.SVG, action func()) ui.Element {
	return sizedIconButton(c, key, label, icon, 28, 16, 5, action)
}

func titlebarIconButton(c *ui.Context, key, label string, icon *ui.SVG, action func()) ui.Element {
	return sizedIconButton(c, key, label, icon, 24, 14, 5, action)
}

func sizedIconButton(c *ui.Context, key, label string, icon *ui.SVG, buttonSize, iconSize, radius float32, action func()) ui.Element {
	button := ui.ButtonBase(c.Key(key)).Label(label).Tooltip(label).Size(buttonSize, buttonSize).Radius(radius).Center().Disabled(action == nil)
	fill := ui.Transparent
	if action != nil && button.Hovered() {
		fill = c.Theme().SurfaceHover
	}
	return controlMotion(c, button, fill, action != nil).OnClick(action).Children(func() {
		ui.Icon(c, icon).Size(iconSize, iconSize).TextColor(colors(c.Theme()).Icon).ContentTransition(ui.ContentTransition{Animation: &motionFade})
	})
}
