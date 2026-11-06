package ui

import (
	"mcl/internal/core/instance"

	"github.com/egoist/mygo/ui"
)

func (s *Shell) instanceAvatar(c *ui.Context, value instance.Instance, size float32) {
	style := value.IconStyle
	if style == "" {
		if required, ok := value.RequiredLoader(); ok {
			style = required.Type
		}
	}
	ui.Box(c).Size(size, size).Shrink(0).Children(func() { drawInstanceArtwork(c, style, s.library.icons[value.ID], size) })
}

func drawInstanceArtwork(c *ui.Context, style string, bitmap *ui.Bitmap, size float32) {
	ui.Box(c.Key("instance-artwork")).Size(size, size).Shrink(0).Radius(size / 4).Clip().Center().Children(func() {
		if bitmap != nil {
			ui.Image(c, bitmap).Size(size, size).Fit(ui.Contain).Transition(motionArtwork)
			return
		}
		if style == "cat" {
			ui.Image(c, brandLogo).Size(size, size).Fit(ui.Contain)
			return
		}
		if style == "" {
			style = "grass"
		}
		if bitmap := instancePresets[style]; bitmap != nil {
			ui.Image(c.Key("preset-"+style), bitmap).Size(size, size).Fit(ui.Contain).Transition(motionArtwork)
			return
		}
		icon, color := iconHammer, ui.Hex("#b27c4f")
		switch style {
		case "leaf":
			icon, color = iconLeaf, ui.Hex("#529a70")
		case "spark":
			icon, color = iconSparkles, ui.Hex("#8876bd")
		}
		ui.Box(c).Fill().Background(color.Alpha(.14)).Center().Children(func() { ui.Icon(c, icon).Size(size*.52, size*.52).TextColor(color) })
	})
}
