package ui

import (
	"image"
	"slices"

	"mcl/internal/app"

	"github.com/egoist/mygo/ui"
)

type assetArtwork struct {
	instanceID string
	queue      []app.AssetFile
	next       int
	active     int
	cancel     map[int]func()
	images     map[string]*ui.Bitmap
}

func (s *Shell) cancelAssetArtwork() {
	if state := s.assets.artwork; state != nil {
		for _, cancel := range state.cancel {
			cancel()
		}
	}
	s.assets.artwork = nil
}

func (s *Shell) loadAssetArtwork(snapshot app.AssetSnapshot) {
	if s.assets.actions.Icon == nil {
		return
	}
	state := &assetArtwork{
		instanceID: snapshot.InstanceID, queue: slices.Clone(snapshot.Files),
		cancel: make(map[int]func()), images: make(map[string]*ui.Bitmap),
	}
	s.assets.artwork = state
	s.nextAssetArtwork(state)
}

func (s *Shell) nextAssetArtwork(state *assetArtwork) {
	for state.active < 4 && state.next < len(state.queue) {
		index := state.next
		file := state.queue[index]
		state.next++
		state.active++
		cancel := s.assets.actions.Icon(state.instanceID, file, func(img image.Image, err error) {
			if s.assets.artwork != state {
				return
			}
			delete(state.cancel, index)
			state.active--
			if err == nil && img != nil {
				state.images[file.Path] = ui.NewBitmap(img)
			}
			s.nextAssetArtwork(state)
		})
		state.cancel[index] = cancel
	}
}

func (s *Shell) assetArtworkIcon(c *ui.Context, file app.AssetFile) {
	const size = 30
	ui.Box(c.Key("asset-artwork")).Size(size, size).Shrink(0).Radius(5).Clip().Center().Children(func() {
		if state := s.assets.artwork; state != nil && state.instanceID == s.assets.snapshot.InstanceID {
			if bitmap := state.images[file.Path]; bitmap != nil {
				ui.Image(c.Key("asset-image"), bitmap).Size(size, size).Fit(ui.Contain).Transition(motionArtwork)
				return
			}
		}
		ui.Icon(c.Key("asset-fallback"), category(file.Kind).icon).Size(20, 20).TextColor(colors(s.theme).Icon)
	})
}
