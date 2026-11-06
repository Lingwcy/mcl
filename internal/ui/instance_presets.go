package ui

import (
	"image"
	"image/color"

	"github.com/egoist/mygo/ui"
)

// Original 16-pixel artwork, kept independent of network loader metadata.
// The bitmap sampler preserves the pixel grid at sidebar and preview sizes.
var instancePresets = map[string]*ui.Bitmap{
	"grass": pixelPreset([16]string{
		"................", "................", "..hHHHHHHHHHHh..", "..HGGgGGGGgGGH..",
		"..HGGGGhhGGGGH..", "..GggGGGGGGggG..", "..bgGggGGggGgb..", "..bbggbggbggbb..",
		"..bDbDbbbbbbDb..", "..bbbbbbbDbbbb..", "..bbdbbbbDDbbb..", "..bbbbDbbbbdbb..",
		"..bdbbbbdbbbbb..", "..dddddddddddd..", "................", "................",
	}),
	"fabric": pixelPreset([16]string{
		"................", "................", "....TTTTTTTT....", "...TCCYYYYYCT...",
		"...TCtttttCCT...", "...TTttttttTT...", "....ttkttktt....", "....tkkttkkt....",
		"....ttkttktt....", "....tkkttkkt....", "...TTttttttTT...", "...TCtttttCCT...",
		"...TCCYYYYYCT...", "....TTTTTTTT....", "................", "................",
	}),
	"forge": pixelPreset([16]string{
		"................", "................", "................", ".SSSSSSSSSSSSS..",
		".SsssssssssssS..", "..SSssssssSSS...", "....SssssS......", ".....SssS.......",
		".....SssS.......", ".....sffs.......", "....SssssS......", "...SssssssS.....",
		"..SSSSSSSSSS....", "..fffffffffff...", "................", "................",
	}),
	"neoforge": pixelPreset([16]string{
		"................", "..oO........Oo..", "..oOO......OOo..", "..oMOO....OOMo..",
		"..oOOOo..oOOOo..", "...oOOOOOOOOo...", "..oOOOOOOOOOOo..", "..oOoOOOOOOoOo..",
		"..oOXCOOOCXOOo..", "...oCCOOOCCOo...", "...oCCCCCCCCo...", "....oCCXXCCo....",
		".....oCCCCo.....", "......oooo......", "................", "................",
	}),
	"quilt": pixelPreset([16]string{
		"................", "................", "...vvvvvvvvvv...", "..vPPCvvYYCCv...",
		"..vPPPCvYYYvCv..", "..vPPPPvYYYYCv..", "..vCPPCvCYYCCv..", "..vvvvvvvvvvvv..",
		"..vttCvvLLCvvv..", "..vtttCvLLLCvv..", "..vttttvLLLLCv..", "..vCttCvCLLCCv..",
		"...vvvvvvvvvv...", "................", "................", "................",
	}),
}

func pixelPreset(rows [16]string) *ui.Bitmap {
	palette := map[byte]color.NRGBA{
		'h': {R: 175, G: 212, B: 125, A: 255}, 'H': {R: 101, G: 153, B: 73, A: 255},
		'G': {R: 128, G: 184, B: 84, A: 255}, 'g': {R: 76, G: 131, B: 58, A: 255},
		'b': {R: 145, G: 98, B: 65, A: 255}, 'D': {R: 185, G: 142, B: 94, A: 255}, 'd': {R: 112, G: 75, B: 52, A: 255},
		'T': {R: 147, G: 113, B: 65, A: 255}, 'Y': {R: 230, G: 194, B: 112, A: 255},
		'C': {R: 255, G: 239, B: 198, A: 255}, 't': {R: 71, G: 166, B: 170, A: 255}, 'k': {R: 44, G: 127, B: 143, A: 255},
		'S': {R: 139, G: 157, B: 175, A: 255}, 's': {R: 83, G: 104, B: 125, A: 255}, 'f': {R: 53, G: 69, B: 90, A: 255},
		'O': {R: 235, G: 137, B: 57, A: 255}, 'o': {R: 172, G: 86, B: 39, A: 255},
		'M': {R: 118, G: 66, B: 52, A: 255}, 'X': {R: 49, G: 46, B: 48, A: 255},
		'v': {R: 113, G: 91, B: 151, A: 255}, 'P': {R: 217, G: 145, B: 183, A: 255}, 'L': {R: 177, G: 162, B: 219, A: 255},
	}
	bitmap := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	for y, row := range rows {
		if len(row) != 16 {
			panic("invalid instance pixel preset width")
		}
		for x := range 16 {
			if row[x] == '.' {
				continue
			}
			paint, ok := palette[row[x]]
			if !ok {
				panic("invalid instance pixel preset color")
			}
			bitmap.SetNRGBA(x, y, paint)
		}
	}
	return ui.NewBitmap(bitmap).Pixelated()
}

func instancePresetLabel(style string) string {
	switch style {
	case "grass":
		return "Vanilla · 草方块"
	case "fabric":
		return "Fabric · 布卷"
	case "forge":
		return "Forge · 铁砧"
	case "neoforge":
		return "NeoForge · 狐狸"
	case "quilt":
		return "Quilt · 拼布"
	case "cat":
		return "mcl · 猫咪"
	case "leaf":
		return "树叶"
	case "spark":
		return "星光"
	default:
		return "工作台"
	}
}

func (s *Shell) creationLoaderStyle(kind string) string {
	if kind == "" {
		return "grass"
	}
	return kind
}

func (s *Shell) creationArtworkStyle() string {
	if s.creation.iconStyle != "" {
		return s.creation.iconStyle
	}
	return s.creationLoaderStyle(s.creation.kind)
}
