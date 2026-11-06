package ui

import "github.com/egoist/mygo/ui"

// Semantic colors shared by custom views and the MyGo theme adapter.
// Keep the light/dark definitions paired; new screens should choose a role,
// rather than invent a hex value or an accent alpha for their own background.
// Product artwork and third-party resource images retain their own colors.
type interfaceColors struct {
	Chrome, Sidebar, Canvas, Control, Hover, Pressed, Raised ui.Color
	Text, Secondary, Metadata, Icon, IconMuted               ui.Color
	Border, Divider, ListDivider                             ui.Color
	Accent, AccentHover, AccentPressed, OnAccent             ui.Color
	AccentInk, Selected, SelectedHover, Tag                  ui.Color
	SettingsNavAccent                                        ui.Color
	LaunchStart, LaunchMiddle, LaunchEnd                     ui.Color
	LaunchReadyMint, LaunchReadyAqua                         ui.Color
	LaunchGlint, LaunchParticle                              ui.Color
	Focus, TextSelection                                     ui.Color
	Success, Warning, Danger                                 ui.Color
	Inverse, InverseText, Scrollbar, Shadow, Skeleton        ui.Color
}

var lightColors = interfaceColors{
	Chrome: ui.Hex("#E6E6E6"), Sidebar: ui.Hex("#F1F1F1"), Canvas: ui.Hex("#FDFDFD"),
	Control: ui.Hex("#F6F6F6"), Hover: ui.Hex("#E2E2E2"), Pressed: ui.Hex("#D8D8D8"), Raised: ui.Hex("#FFFFFF"),
	Text: ui.Hex("#242424"), Secondary: ui.Hex("#626262"), Metadata: ui.Hex("#717171"),
	Icon: ui.Hex("#424242"), IconMuted: ui.Hex("#A2A2A2"),
	Border: ui.Hex("#D9D9D9"), Divider: ui.Hex("#E5E5E5"), ListDivider: ui.Hex("#EAEAEA"),
	Accent: ui.Hex("#356DA5"), AccentHover: ui.Hex("#2E6296"), AccentPressed: ui.Hex("#275582"), OnAccent: ui.Hex("#FFFFFF"),
	AccentInk: ui.Hex("#405F7B"), Selected: ui.Hex("#DCDEE1"), SelectedHover: ui.Hex("#D3D6DA"), Tag: ui.Hex("#ECECEC"),
	SettingsNavAccent: ui.Hex("#356DA5"),
	LaunchStart:       ui.Hex("#5278ED"), LaunchMiddle: ui.Hex("#16AB99"), LaunchEnd: ui.Hex("#9259DC"),
	LaunchReadyMint: ui.Hex("#43C685"), LaunchReadyAqua: ui.Hex("#1B9E8D"),
	LaunchGlint: ui.Hex("#E1FFF7"), LaunchParticle: ui.Hex("#FFF9E1"),
	Focus: ui.RGBA(53, 109, 165, .70), TextSelection: ui.RGBA(53, 109, 165, .19),
	Success: ui.Hex("#3D7A5C"), Warning: ui.Hex("#94651E"), Danger: ui.Hex("#B24E56"),
	Inverse: ui.Hex("#292929"), InverseText: ui.Hex("#FAFAFA"),
	Scrollbar: ui.RGBA(86, 96, 108, .34), Shadow: ui.RGBA(0, 0, 0, .12), Skeleton: ui.Hex("#E8E8E8"),
}

var darkColors = interfaceColors{
	Chrome: ui.Hex("#15181D"), Sidebar: ui.Hex("#1B1F25"), Canvas: ui.Hex("#20252C"),
	Control: ui.Hex("#292F38"), Hover: ui.Hex("#333C47"), Pressed: ui.Hex("#3C4754"), Raised: ui.Hex("#2C333D"),
	Text: ui.Hex("#EBF0F6"), Secondary: ui.Hex("#AFBBC9"), Metadata: ui.Hex("#8B9AAC"),
	Icon: ui.Hex("#A3B0C0"), IconMuted: ui.Hex("#718196"),
	Border: ui.Hex("#465261"), Divider: ui.Hex("#323B46"), ListDivider: ui.Hex("#2C333C"),
	Accent: ui.Hex("#8BB9CC"), AccentHover: ui.Hex("#A1C9D9"), AccentPressed: ui.Hex("#78A6BA"), OnAccent: ui.Hex("#172C37"),
	AccentInk: ui.Hex("#9CCBDD"), Selected: ui.Hex("#2A414E"), SelectedHover: ui.Hex("#324E5D"), Tag: ui.Hex("#2C343E"),
	SettingsNavAccent: ui.Hex("#759BFA"),
	LaunchStart:       ui.Hex("#728EFF"), LaunchMiddle: ui.Hex("#51DFCF"), LaunchEnd: ui.Hex("#BD82FF"),
	LaunchReadyMint: ui.Hex("#79EFA2"), LaunchReadyAqua: ui.Hex("#42D6BF"),
	LaunchGlint: ui.Hex("#EEFFF8"), LaunchParticle: ui.Hex("#EBFFF7"),
	Focus: ui.RGBA(131, 187, 209, .65), TextSelection: ui.RGBA(131, 187, 209, .28),
	Success: ui.Hex("#89B89D"), Warning: ui.Hex("#D3B078"), Danger: ui.Hex("#DD969D"),
	Inverse: ui.Hex("#E3EAF2"), InverseText: ui.Hex("#202A35"),
	Scrollbar: ui.RGBA(156, 169, 184, .36), Shadow: ui.RGBA(0, 0, 0, .30), Skeleton: ui.Hex("#3A4654"),
}

func colors(t *ui.Theme) interfaceColors {
	if t.Dark {
		return darkColors
	}
	return lightColors
}

// Custom ButtonBase controls follow the same state hierarchy as native
// buttons. Keep a selected row selected while it is hovered or pressed.
func controlFill(t *ui.Theme, button ui.Element, fill ui.Color) ui.Color {
	if button.IsDisabled() {
		return fill
	}
	p := colors(t)
	if fill == p.Selected || fill == p.SelectedHover {
		if button.Hovered() || button.Pressed() {
			return p.SelectedHover
		}
		return p.Selected
	}
	if fill == p.Accent || fill == p.AccentHover || fill == p.AccentPressed {
		if button.Pressed() {
			return p.AccentPressed
		}
		if button.Hovered() {
			return p.AccentHover
		}
		return p.Accent
	}
	if button.Pressed() {
		return p.Pressed
	}
	if button.Hovered() {
		return p.Hover
	}
	return fill
}
