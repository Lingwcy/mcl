package ui

import (
	"embed"
	"image"
	"strings"

	"mcl/internal/core/market"

	"github.com/egoist/mygo/ui"
)

//go:embed market-icons/*.svg
var marketIconAssets embed.FS

func sourceIcon(name string) *ui.SVG {
	data, err := marketIconAssets.ReadFile("market-icons/" + name + ".svg")
	if err != nil {
		panic(err)
	}
	return ui.MustParseSVG(data)
}

var (
	iconModrinth   = sourceIcon("modrinth")
	iconCurseForge = sourceIcon("curseforge")
)

type marketSymbol struct {
	svg     *ui.SVG
	bitmap  *ui.Bitmap
	colored bool
}

func (symbol marketSymbol) draw(c *ui.Context, size float32, color ui.Color) {
	switch {
	case symbol.bitmap != nil:
		ui.Image(c, symbol.bitmap).Size(size, size).Fit(ui.Contain).Shrink(0)
	case symbol.svg != nil && symbol.colored:
		ui.Image(c, symbol.svg).Size(size, size).Fit(ui.Contain).TextColor(color).Shrink(0)
	case symbol.svg != nil:
		ui.Icon(c, symbol.svg).Size(size, size).TextColor(color).Shrink(0)
	default:
		ui.Icon(c, iconTags).Size(size, size).TextColor(color).Shrink(0)
	}
}

func (s *Shell) loadMarketLoaderIcons() {
	m := &s.market
	if m.loaderIcons != nil || m.loadingLoaderIcons || m.actions.LoaderIcons == nil {
		return
	}
	m.loadingLoaderIcons = true
	m.actions.LoaderIcons(func(icons []market.LoaderIcon, err error) {
		m.loadingLoaderIcons = false
		if err != nil {
			return
		}
		m.loaderIcons = make(map[string]marketSymbol)
		for _, entry := range icons {
			if icon, err := ui.ParseSVG([]byte(entry.SVG)); err == nil {
				m.loaderIcons[entry.Name] = marketSymbol{svg: icon, colored: true}
			}
		}
	})
}

func (s *Shell) loadCategoryIcons(categories []market.Category, generation uint64) {
	m := &s.market
	requests := make(map[string][]string)
	for _, entry := range categories {
		if entry.IconSVG != "" {
			if icon, err := ui.ParseSVG([]byte(entry.IconSVG)); err == nil {
				m.categoryIcons[entry.ID] = marketSymbol{svg: icon}
				continue
			}
		}
		if entry.IconURL != "" && m.actions.Icon != nil {
			if len(requests) < 128 || requests[entry.IconURL] != nil {
				requests[entry.IconURL] = append(requests[entry.IconURL], entry.ID)
			}
		}
	}
	for address, ids := range requests {
		cancel := m.actions.Icon(address, func(img image.Image, err error) {
			if generation != m.categoryGeneration || err != nil || img == nil {
				return
			}
			symbol := marketSymbol{bitmap: ui.NewBitmap(img)}
			for _, id := range ids {
				m.categoryIcons[id] = symbol
			}
		})
		m.categoryIconCancels = append(m.categoryIconCancels, cancel)
	}
}

func (s *Shell) loaderSymbol(name string) marketSymbol {
	key := strings.ToLower(name)
	if icon, found := s.market.loaderIcons[key]; found {
		return icon
	}
	fallback := iconComponent
	switch key {
	case "forge", "neoforge":
		fallback = iconHammer
	case "fabric", "legacy-fabric", "babric", "quilt":
		fallback = iconGrid
	case "iris", "optifine", "canvas":
		fallback = iconSparkles
	case "vanilla", "minecraft":
		fallback = iconCube
	}
	return marketSymbol{svg: fallback}
}

func (s *Shell) categorySymbol(id, name string) marketSymbol {
	if icon, found := s.market.categoryIcons[id]; found {
		return icon
	}
	for _, entry := range s.market.categories {
		if strings.EqualFold(entry.Name, name) {
			if icon, found := s.market.categoryIcons[entry.ID]; found {
				return icon
			}
			break
		}
	}
	return marketSymbol{svg: categoryFallback(name)}
}

func (s *Shell) marketLoaderTag(c *ui.Context, name string) {
	color := colors(c.Theme()).Secondary
	ui.Row(c).Gap(4).Padding(2, 5).Radius(4).Background(colors(c.Theme()).Tag).Children(func() {
		s.loaderSymbol(name).draw(c, 11, color)
		ui.Text(c, marketLoaderLabel(name)).FontSize(10).TextColor(color).MaxLines(1).Ellipsis("…")
	})
}

func categoryFallback(name string) *ui.SVG {
	name = strings.ToLower(strings.TrimSpace(name))
	switch name {
	case "library", "api and library", "api and library mods":
		return iconLibrary
	case "adventure", "adventure and rpg":
		return iconCompass
	case "technology", "tech":
		return iconCPU
	case "optimization", "performance", "lightweight":
		return iconGauge
	case "magic", "fantasy":
		return iconMagic
	case "worldgen", "world gen", "world generation", "dimensions", "biomes":
		return iconMap
	case "decoration", "cosmetic", "decorative blocks", "building":
		return iconBlocks
	case "utility", "management", "game-mechanics", "tweaks":
		return iconSliders
	case "storage":
		return iconArchive
	case "social", "multiplayer":
		return iconUser
	case "realistic", "environmental", "nature":
		return iconLeaf
	case "themed", "textures", "simplistic":
		return iconImage
	case "addons", "add-ons":
		return iconAddon
	case "mobs", "equipment", "transportation", "food":
		return iconPuzzle
	case "quests", "guidebook":
		return iconBook
	default:
		if strings.HasSuffix(name, "x") || strings.Contains(name, "resolution") {
			return iconGrid
		}
		return iconTags
	}
}
