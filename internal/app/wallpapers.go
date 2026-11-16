package app

import (
	"context"
	"strings"
	"time"
)

// Wallpaper describes an imported background image stored in the launcher's
// application data directory.
type Wallpaper struct {
	ID      string    `json:"id"`
	Name    string    `json:"name"`
	Width   int       `json:"width"`
	Height  int       `json:"height"`
	AddedAt time.Time `json:"added_at"`
}

// WallpaperOptions stores home-page personalization preferences.
type WallpaperOptions struct {
	Shuffle bool `json:"shuffle,omitempty"`
	Opacity int  `json:"opacity,omitempty"` // 15..100, default 100
	Blur    int  `json:"blur,omitempty"`    // 0..24, default 0
	Dim     int  `json:"dim,omitempty"`     // 0..70, default 0
}

// Normalized clamps personalization options to valid UI bounds and applies defaults.
func (o WallpaperOptions) Normalized() WallpaperOptions {
	if o.Opacity == 0 {
		o.Opacity = 100
	} else {
		o.Opacity = max(15, min(o.Opacity, 100))
	}
	o.Blur = max(0, min(o.Blur, 24))
	o.Dim = max(0, min(o.Dim, 70))
	return o
}

// WallpaperSnapshot captures the configured wallpapers, active selection,
// personalization options, active full-resolution PNG bytes, and thumbnails.
type WallpaperSnapshot struct {
	ActiveID   string            `json:"active_id,omitempty"`
	Options    WallpaperOptions  `json:"options,omitempty"`
	Wallpapers []Wallpaper       `json:"wallpapers"`
	ActiveData []byte            `json:"-"`
	Thumbnails map[string][]byte `json:"-"`
}

// WallpaperStore persists imported wallpapers, active background selection,
// and personalization effect options.
type WallpaperStore interface {
	Load(ctx context.Context) (WallpaperSnapshot, error)
	Add(ctx context.Context, sourcePaths []string) (WallpaperSnapshot, error)
	Select(ctx context.Context, id string) (WallpaperSnapshot, error)
	Shuffle(ctx context.Context) (WallpaperSnapshot, error)
	SaveOptions(ctx context.Context, options WallpaperOptions) (WallpaperSnapshot, error)
	Remove(ctx context.Context, id string) (WallpaperSnapshot, error)
}

// WallpaperService coordinates wallpaper loading, import, selection, and personalization.
type WallpaperService struct {
	store WallpaperStore
}

func NewWallpaperService(store WallpaperStore) *WallpaperService {
	return &WallpaperService{store: store}
}

func (s *WallpaperService) Load(ctx context.Context) (WallpaperSnapshot, error) {
	return s.store.Load(ctx)
}

func (s *WallpaperService) Add(ctx context.Context, sourcePaths []string) (WallpaperSnapshot, error) {
	cleaned := make([]string, 0, len(sourcePaths))
	for _, p := range sourcePaths {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			cleaned = append(cleaned, trimmed)
		}
	}
	return s.store.Add(ctx, cleaned)
}

func (s *WallpaperService) Select(ctx context.Context, id string) (WallpaperSnapshot, error) {
	return s.store.Select(ctx, strings.TrimSpace(id))
}

func (s *WallpaperService) Shuffle(ctx context.Context) (WallpaperSnapshot, error) {
	return s.store.Shuffle(ctx)
}

func (s *WallpaperService) SaveOptions(ctx context.Context, options WallpaperOptions) (WallpaperSnapshot, error) {
	return s.store.SaveOptions(ctx, options.Normalized())
}

func (s *WallpaperService) Remove(ctx context.Context, id string) (WallpaperSnapshot, error) {
	return s.store.Remove(ctx, strings.TrimSpace(id))
}
