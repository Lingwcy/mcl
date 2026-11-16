package ui

import (
	"mcl/internal/branding"

	"github.com/egoist/mygo/ui"
)

type creditEntry struct {
	name string
	head *ui.Bitmap
}

var creditEntries = decodeCreditEntries()

func decodeCreditEntries() []creditEntry {
	entries := make([]creditEntry, 0, len(branding.Credits))
	for _, credit := range branding.Credits {
		head, err := ui.DecodeBitmap(credit.HeadPNG)
		if err != nil {
			panic("invalid embedded credit head for " + credit.Name + ": " + err.Error())
		}
		entries = append(entries, creditEntry{
			name: credit.Name,
			head: head.Pixelated(),
		})
	}
	return entries
}

func (s *Shell) creditsView(c *ui.Context) {
	t := s.theme
	p := colors(t)
	columns := s.creditsColumnCount(c)
	ui.Column(c.Key("credits-settings")).Grow(1).MinHeight(0).FillWidth().Children(func() {
		ui.Scroll(c.Key("credits-settings-scroll")).Grow(1).MinHeight(0).FillWidth().Padding(18, 24, 24).Gap(12).Children(func() {
			ui.Column(c.Key("credits-section")).FillWidth().Shrink(0).Gap(8).Children(func() {
				settingsGroupLabel(c, "鸣谢")
				ui.Text(c, s.tr("启动器的开发和发展离不开他们的辛苦付出。")).FontSize(12).TextColor(p.Metadata).Margin(0, 2)
				ui.Grid(c.Key("credits-grid")).FillWidth().Columns(columns).Gap(10).Children(func() {
					for _, entry := range creditEntries {
						creditCard(c, t, entry)
					}
				})
			})
		})
	})
}

func (s *Shell) creditsColumnCount(c *ui.Context) int {
	width, _ := c.Size()
	panelWidth := width - railWidth - chromeEdge - 48
	if !s.sidebarCollapsed {
		panelWidth -= secondarySidebarWidth(c)
	}
	if panelWidth < 360 {
		return 1
	}
	if panelWidth < 560 {
		return 2
	}
	return 3
}

func creditCard(c *ui.Context, t *ui.Theme, entry creditEntry) {
	p := colors(t)
	ui.Row(c.Key("credit-"+entry.name)).
		FillWidth().
		Padding(12, 14).
		Gap(12).
		AlignItems(ui.Center).
		Radius(10).
		Border(0.5, p.Divider).
		Background(t.Surface).
		Children(func() {
			ui.Image(c, entry.head).
				Label(entry.name).
				Size(40, 40).
				Shrink(0).
				Radius(6).
				Fit(ui.Contain)
			ui.Text(c, entry.name).
				FontSize(14).
				FontWeight(600).
				TextColor(t.Text).
				Grow(1).
				MinWidth(0).
				MaxLines(1).
				Ellipsis("…")
		})
}
