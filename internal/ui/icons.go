package ui

import (
	"embed"
	"strings"

	"github.com/egoist/mygo/ui"
)

// SVG assets and applicable upstream licenses are bundled, never fetched at runtime.
// See icons/README.md for the pinned sources and attribution.
//
//go:embed icons/*.svg icons/LICENSE
var iconAssets embed.FS

func bundledIcon(name string) *ui.SVG {
	data, err := iconAssets.ReadFile("icons/" + name + ".svg")
	if err != nil {
		panic(err)
	}
	return ui.MustParseSVG([]byte(strings.ReplaceAll(string(data), `stroke-width="2"`, `stroke-width="1.75"`)))
}

var (
	iconHome             = bundledIcon("house")
	iconMoon             = bundledIcon("moon")
	iconSun              = bundledIcon("sun")
	iconSidebar          = bundledIcon("panel-left")
	iconSearch           = bundledIcon("search")
	iconBack             = bundledIcon("chevron-left")
	iconMarket           = bundledIcon("shopping-bag")
	iconPlay             = bundledIcon("play")
	iconLog              = bundledIcon("terminal")
	iconPuzzle           = bundledIcon("puzzle")
	iconSparkles         = bundledIcon("sparkles")
	iconPalette          = bundledIcon("palette")
	iconFile             = bundledIcon("file-text")
	iconArchive          = bundledIcon("archive")
	iconChevron          = bundledIcon("chevron-right")
	iconChevronDown      = bundledIcon("chevron-down")
	iconUser             = bundledIcon("user-round")
	iconMicrosoft        = bundledIcon("microsoft")
	iconPlus             = bundledIcon("plus")
	iconMinus            = bundledIcon("minus")
	iconSquare           = bundledIcon("square")
	iconRestoreWindow    = bundledIcon("copy")
	iconClose            = bundledIcon("x")
	iconFolder           = bundledIcon("folder")
	iconDownload         = bundledIcon("download")
	iconSettings         = bundledIcon("settings")
	iconCube             = bundledIcon("box")
	iconRefresh          = bundledIcon("rotate-cw")
	iconSort             = bundledIcon("arrow-down-wide-narrow")
	iconInfo             = bundledIcon("info")
	iconError            = bundledIcon("circle-x")
	iconWarning          = bundledIcon("triangle-alert")
	iconCheck            = bundledIcon("circle-check")
	iconTrash            = bundledIcon("trash-2")
	iconUndo             = bundledIcon("undo-2")
	iconScan             = bundledIcon("scan-search")
	iconLayers           = bundledIcon("layers")
	iconDatabase         = bundledIcon("database")
	iconMap              = bundledIcon("map")
	iconMonitor          = bundledIcon("monitor")
	iconServer           = bundledIcon("server")
	iconCode             = bundledIcon("code-xml")
	iconTags             = bundledIcon("tags")
	iconComponent        = bundledIcon("component")
	iconLibrary          = bundledIcon("library")
	iconCompass          = bundledIcon("compass")
	iconCPU              = bundledIcon("cpu")
	iconMagic            = bundledIcon("wand-sparkles")
	iconBlocks           = bundledIcon("blocks")
	iconGauge            = bundledIcon("gauge")
	iconLeaf             = bundledIcon("leaf")
	iconImage            = bundledIcon("image")
	iconHammer           = bundledIcon("hammer")
	iconGrid             = bundledIcon("grid-2x2")
	iconBook             = bundledIcon("book-open")
	iconSliders          = bundledIcon("sliders-horizontal")
	iconAddon            = bundledIcon("circle-plus")
	iconFilter           = bundledIcon("filter")
	iconSettingsRuntime  = bundledIcon("settings-runtime-solid")
	iconSettingsGame     = bundledIcon("settings-game-solid")
	iconSettingsAccount  = bundledIcon("settings-account-solid")
	iconSettingsDownload = bundledIcon("settings-download-solid")
)
