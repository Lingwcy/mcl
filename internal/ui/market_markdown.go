package ui

import (
	"errors"
	"strings"
	"sync"

	"mcl/internal/core/market"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
	"github.com/egoist/mygo/ui/markdown/parser"
)

type marketMediaEntry struct {
	mu   sync.Mutex
	src  ui.ImageSource
	err  error
	done bool
}

type marketMediaCache struct {
	entries sync.Map // url -> *marketMediaEntry
}

func newMarketMediaCache() *marketMediaCache {
	return &marketMediaCache{}
}

type linkedImageItem struct {
	src, alt, title, href string
}

func (s *Shell) configureMarketMarkdown(c *ui.Context, opts *markdown.Options, cache *marketMediaCache) {
	services := c.Services()
	opts.OnLink = func(address string) {
		if market.SafeLinkURL(address) {
			services.OpenURL(address)
		}
	}
	opts.LoadImage = func(address string) (ui.ImageSource, error) {
		return s.fetchMarketMedia(address)
	}
	opts.HTML = func(c *ui.Context, raw string) {
		text := market.DescriptionPlainText(raw)
		if text == "" {
			return
		}
		ui.Text(c, text).FontSize(12).TextColor(s.theme.TextMuted).FillWidth()
	}
	opts.Block = func(c *ui.Context, d *parser.Document, i int) bool {
		items, ok := linkedImageParagraph(d, i)
		if !ok {
			return false
		}
		s.renderLinkedImages(c, cache, items, services)
		return true
	}
}

func (s *Shell) fetchMarketMedia(address string) (ui.ImageSource, error) {
	if s.market.actions.Media == nil || !market.SafeLinkURL(address) {
		return nil, errors.New("image loader unavailable")
	}
	data, isSVG, err := s.market.actions.Media(address)
	if err != nil {
		return nil, err
	}
	if isSVG {
		return ui.ParseSVG(data)
	}
	return ui.DecodeBitmap(data)
}

func (s *Shell) cachedMarketMedia(c *ui.Context, cache *marketMediaCache, address string) ui.ImageSource {
	if cache == nil || address == "" || s.market.actions.Media == nil {
		return nil
	}
	v, loaded := cache.entries.LoadOrStore(address, &marketMediaEntry{})
	entry := v.(*marketMediaEntry)
	if !loaded {
		services := c.Services()
		go func() {
			src, err := s.fetchMarketMedia(address)
			entry.mu.Lock()
			entry.src, entry.err, entry.done = src, err, true
			entry.mu.Unlock()
			services.Invalidate()
		}()
	}
	entry.mu.Lock()
	defer entry.mu.Unlock()
	return entry.src
}

func (s *Shell) renderLinkedImages(c *ui.Context, cache *marketMediaCache, items []linkedImageItem, services ui.Services) {
	t := s.theme
	ui.Row(c).Wrap().Gap(6).AlignSelf(ui.Stretch).Children(func() {
		for index, item := range items {
			src := s.cachedMarketMedia(c, cache, item.src)
			label := item.alt
			if label == "" {
				label = "图片"
			}
			draw := func() {
				if src == nil {
					ui.Row(c).Padding(6, 10).Radius(4).Background(t.Surface).Children(func() {
						ui.Text(c, label).FontSize(11).TextColor(t.TextMuted)
					})
					return
				}
				img := ui.Image(c, src).Radius(4).MaxWidthPercent(100).Label(item.alt)
				var w, h float32
				switch typed := src.(type) {
				case *ui.Bitmap:
					iw, ih := typed.Size()
					w, h = float32(iw), float32(ih)
				case *ui.SVG:
					w, h = typed.Size()
				}
				if w > 0 && h > 0 {
					img.AspectRatio(w / h)
				}
				if item.title != "" && item.href == "" {
					img.Tooltip(item.title)
				}
			}
			if market.SafeLinkURL(item.href) {
				href := item.href
				tip := item.title
				if tip == "" {
					tip = href
				}
				ui.ButtonBase(c.Key(index)).Label(label).Tooltip(tip).MaxWidthPercent(100).Radius(4).
					OnClick(func() { services.OpenURL(href) }).Children(draw)
				continue
			}
			draw()
		}
	})
}

func linkedImageParagraph(d *parser.Document, blockIndex int) ([]linkedImageItem, bool) {
	if d.Block(blockIndex).Kind != parser.Paragraph {
		return nil, false
	}
	in := d.Inlines(blockIndex)
	var items []linkedImageItem
	hasLinked := false
	for k := 0; k < len(in); k = in[k].Next {
		n := &in[k]
		switch n.Kind {
		case parser.Image:
			if n.Pending {
				return nil, false
			}
			items = append(items, linkedImageItem{
				src:   n.Dest,
				alt:   parser.PlainText(in[k+1 : n.Next]),
				title: n.Title,
			})
		case parser.Link:
			if n.Pending {
				return nil, false
			}
			nested, ok := imagesInsideLink(in, k+1, n.Next)
			if !ok {
				return nil, false
			}
			hasLinked = true
			for _, img := range nested {
				if img.title == "" {
					img.title = n.Title
				}
				img.href = n.Dest
				items = append(items, img)
			}
		case parser.SoftBreak, parser.LineBreak:
		case parser.Text:
			if strings.TrimSpace(n.Text) != "" {
				return nil, false
			}
		default:
			return nil, false
		}
	}
	if !hasLinked || len(items) == 0 {
		return nil, false
	}
	return items, true
}

func imagesInsideLink(in []parser.Inline, from, to int) ([]linkedImageItem, bool) {
	var items []linkedImageItem
	for k := from; k < to; k = in[k].Next {
		n := &in[k]
		switch n.Kind {
		case parser.Image:
			if n.Pending {
				return nil, false
			}
			items = append(items, linkedImageItem{
				src:   n.Dest,
				alt:   parser.PlainText(in[k+1 : n.Next]),
				title: n.Title,
			})
		case parser.SoftBreak, parser.LineBreak:
		case parser.Text:
			if strings.TrimSpace(n.Text) != "" {
				return nil, false
			}
		default:
			return nil, false
		}
	}
	return items, len(items) > 0
}
