package ui

import (
	"mcl/internal/branding"

	"github.com/egoist/mygo/ui"
)

var brandLogo = decodeBrandLogo()

func decodeBrandLogo() *ui.Bitmap {
	logo, err := ui.DecodeBitmap(branding.LogoPNG)
	if err != nil {
		panic("invalid embedded mcl logo: " + err.Error())
	}
	return logo
}
