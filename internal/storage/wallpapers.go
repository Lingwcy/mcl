package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"io"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"

	"mcl/internal/app"
	"mcl/internal/core/instance"
	"mcl/internal/fileio"
)

const (
	wallpapersSchemaVersion = 1
	maxWallpapers           = 48
	maxWallpaperIndexBytes  = 1 << 20
	maxWallpaperSourceBytes = 128 << 20
	maxWallpaperStoredBytes = 64 << 20
	maxWallpaperSourceDim   = 16384
	maxWallpaperWidth       = 3840
	maxWallpaperHeight      = 2160
	wallpaperThumbWidth     = 320
	wallpaperThumbHeight    = 180
)

type wallpapersDocument struct {
	Version    int                  `json:"version"`
	ActiveID   string               `json:"active_id,omitempty"`
	Options    app.WallpaperOptions `json:"options,omitempty"`
	Wallpapers []app.Wallpaper      `json:"wallpapers,omitempty"`
}

// WallpaperStore persists imported home-page wallpapers under <root>/wallpapers.
type WallpaperStore struct {
	root string
	mu   sync.Mutex
}

func NewWallpaperStore(root string) *WallpaperStore {
	return &WallpaperStore{root: root}
}

func (s *WallpaperStore) dir() string {
	return filepath.Join(s.root, "wallpapers")
}

func (s *WallpaperStore) indexPath() string {
	return filepath.Join(s.dir(), "index.json")
}

func (s *WallpaperStore) imagePath(id string) string {
	return filepath.Join(s.dir(), id+".png")
}

func (s *WallpaperStore) thumbPath(id string) string {
	return filepath.Join(s.dir(), id+".thumb.png")
}

func (s *WallpaperStore) Load(ctx context.Context) (app.WallpaperSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, err := s.read(ctx)
	if err != nil {
		return app.WallpaperSnapshot{}, err
	}
	doc.Options = doc.Options.Normalized()
	if doc.Options.Shuffle && len(doc.Wallpapers) > 0 {
		nextID := pickRandomWallpaperID(doc.Wallpapers, doc.ActiveID)
		if nextID != "" && nextID != doc.ActiveID {
			doc.ActiveID = nextID
			_ = s.write(ctx, doc)
		}
	}
	return s.snapshot(ctx, doc)
}

type preparedWallpaper struct {
	entry    app.Wallpaper
	fullPNG  []byte
	thumbPNG []byte
}

func (s *WallpaperStore) Add(ctx context.Context, sourcePaths []string) (app.WallpaperSnapshot, error) {
	if len(sourcePaths) == 0 {
		return app.WallpaperSnapshot{}, errors.New("未选择图片文件")
	}

	prepared := make([]preparedWallpaper, 0, len(sourcePaths))
	now := time.Now().UTC()
	for _, sourcePath := range sourcePaths {
		raw, err := readBoundedWallpaperSource(ctx, sourcePath)
		if err != nil {
			return app.WallpaperSnapshot{}, err
		}
		fullPNG, thumbPNG, width, height, err := encodeWallpaperImages(ctx, raw)
		if err != nil {
			return app.WallpaperSnapshot{}, err
		}
		prepared = append(prepared, preparedWallpaper{
			entry: app.Wallpaper{
				ID:      instance.NewID(),
				Name:    wallpaperDisplayName(sourcePath),
				Width:   width,
				Height:  height,
				AddedAt: now,
			},
			fullPNG:  fullPNG,
			thumbPNG: thumbPNG,
		})
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	doc, err := s.read(ctx)
	if err != nil {
		return app.WallpaperSnapshot{}, err
	}
	if len(doc.Wallpapers)+len(prepared) > maxWallpapers {
		return app.WallpaperSnapshot{}, errors.New("已达到墙纸数量上限，请先移除部分墙纸")
	}

	writtenIDs := make([]string, 0, len(prepared))
	cleanupWritten := func() {
		for _, id := range writtenIDs {
			_ = os.Remove(s.imagePath(id))
			_ = os.Remove(s.thumbPath(id))
		}
	}
	for _, item := range prepared {
		if err := fileio.Write(ctx, s.imagePath(item.entry.ID), item.fullPNG, 0o600); err != nil {
			cleanupWritten()
			return app.WallpaperSnapshot{}, err
		}
		if err := fileio.Write(ctx, s.thumbPath(item.entry.ID), item.thumbPNG, 0o600); err != nil {
			_ = os.Remove(s.imagePath(item.entry.ID))
			cleanupWritten()
			return app.WallpaperSnapshot{}, err
		}
		writtenIDs = append(writtenIDs, item.entry.ID)
	}

	addedEntries := make([]app.Wallpaper, 0, len(prepared))
	for i := len(prepared) - 1; i >= 0; i-- {
		addedEntries = append(addedEntries, prepared[i].entry)
	}
	doc.Wallpapers = append(addedEntries, doc.Wallpapers...)
	doc.Options = doc.Options.Normalized()
	doc.Options.Shuffle = false
	doc.ActiveID = addedEntries[0].ID

	if err := s.write(ctx, doc); err != nil {
		cleanupWritten()
		return app.WallpaperSnapshot{}, err
	}
	return s.snapshot(ctx, doc)
}

func (s *WallpaperStore) Select(ctx context.Context, id string) (app.WallpaperSnapshot, error) {
	if id != "" && !instance.ValidID(id) {
		return app.WallpaperSnapshot{}, errors.New("墙纸标识无效")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, err := s.read(ctx)
	if err != nil {
		return app.WallpaperSnapshot{}, err
	}
	if id != "" && !slices.ContainsFunc(doc.Wallpapers, func(w app.Wallpaper) bool { return w.ID == id }) {
		return app.WallpaperSnapshot{}, errors.New("所选墙纸不存在")
	}
	doc.Options = doc.Options.Normalized()
	doc.Options.Shuffle = false
	doc.ActiveID = id
	if err := s.write(ctx, doc); err != nil {
		return app.WallpaperSnapshot{}, err
	}
	return s.snapshot(ctx, doc)
}

func (s *WallpaperStore) Shuffle(ctx context.Context) (app.WallpaperSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, err := s.read(ctx)
	if err != nil {
		return app.WallpaperSnapshot{}, err
	}
	if len(doc.Wallpapers) == 0 {
		return app.WallpaperSnapshot{}, errors.New("请先添加至少一张墙纸后再开启随机切换")
	}
	doc.Options = doc.Options.Normalized()
	doc.Options.Shuffle = true
	doc.ActiveID = pickRandomWallpaperID(doc.Wallpapers, doc.ActiveID)
	if err := s.write(ctx, doc); err != nil {
		return app.WallpaperSnapshot{}, err
	}
	return s.snapshot(ctx, doc)
}

func (s *WallpaperStore) SaveOptions(ctx context.Context, options app.WallpaperOptions) (app.WallpaperSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, err := s.read(ctx)
	if err != nil {
		return app.WallpaperSnapshot{}, err
	}
	prevOptions := doc.Options.Normalized()
	nextOptions := options.Normalized()
	if len(doc.Wallpapers) == 0 {
		nextOptions.Shuffle = false
	} else if nextOptions.Shuffle && (!prevOptions.Shuffle || doc.ActiveID == "") {
		doc.ActiveID = pickRandomWallpaperID(doc.Wallpapers, doc.ActiveID)
	}
	doc.Options = nextOptions
	if err := s.write(ctx, doc); err != nil {
		return app.WallpaperSnapshot{}, err
	}
	return s.snapshot(ctx, doc)
}

func (s *WallpaperStore) Remove(ctx context.Context, id string) (app.WallpaperSnapshot, error) {
	if !instance.ValidID(id) {
		return app.WallpaperSnapshot{}, errors.New("墙纸标识无效")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, err := s.read(ctx)
	if err != nil {
		return app.WallpaperSnapshot{}, err
	}
	filtered := slices.DeleteFunc(slices.Clone(doc.Wallpapers), func(w app.Wallpaper) bool { return w.ID == id })
	doc.Wallpapers = filtered
	doc.Options = doc.Options.Normalized()
	if len(doc.Wallpapers) == 0 {
		doc.Options.Shuffle = false
		doc.ActiveID = ""
	} else if doc.ActiveID == id {
		if doc.Options.Shuffle {
			doc.ActiveID = pickRandomWallpaperID(doc.Wallpapers, "")
		} else {
			doc.ActiveID = ""
		}
	}
	if err := s.write(ctx, doc); err != nil {
		return app.WallpaperSnapshot{}, err
	}
	_ = os.Remove(s.imagePath(id))
	_ = os.Remove(s.thumbPath(id))
	return s.snapshot(ctx, doc)
}

func pickRandomWallpaperID(wallpapers []app.Wallpaper, excludeID string) string {
	if len(wallpapers) == 0 {
		return ""
	}
	if len(wallpapers) == 1 || excludeID == "" {
		return wallpapers[rand.IntN(len(wallpapers))].ID
	}
	candidates := make([]string, 0, len(wallpapers)-1)
	for _, w := range wallpapers {
		if w.ID != excludeID {
			candidates = append(candidates, w.ID)
		}
	}
	if len(candidates) == 0 {
		return wallpapers[rand.IntN(len(wallpapers))].ID
	}
	return candidates[rand.IntN(len(candidates))]
}

func (s *WallpaperStore) read(ctx context.Context) (wallpapersDocument, error) {
	data, err := fileio.Read(ctx, s.indexPath(), maxWallpaperIndexBytes)
	if errors.Is(err, os.ErrNotExist) {
		return wallpapersDocument{Version: wallpapersSchemaVersion, Options: app.WallpaperOptions{Opacity: 100}}, nil
	}
	if err != nil {
		return wallpapersDocument{}, err
	}
	var doc wallpapersDocument
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if dec.Decode(&doc) != nil || !errors.Is(dec.Decode(new(any)), io.EOF) || doc.Version != wallpapersSchemaVersion {
		return wallpapersDocument{}, errors.New("无法读取墙纸配置，已保留原文件")
	}
	if doc.ActiveID != "" && !instance.ValidID(doc.ActiveID) {
		doc.ActiveID = ""
	}
	for _, w := range doc.Wallpapers {
		if !instance.ValidID(w.ID) {
			return wallpapersDocument{}, errors.New("墙纸配置包含无效条目")
		}
	}
	return doc, nil
}

func (s *WallpaperStore) write(ctx context.Context, doc wallpapersDocument) error {
	doc.Version = wallpapersSchemaVersion
	doc.Options = doc.Options.Normalized()
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	return fileio.Write(ctx, s.indexPath(), append(data, '\n'), 0o600)
}

func (s *WallpaperStore) snapshot(ctx context.Context, doc wallpapersDocument) (app.WallpaperSnapshot, error) {
	out := app.WallpaperSnapshot{
		ActiveID:   doc.ActiveID,
		Options:    doc.Options.Normalized(),
		Wallpapers: slices.Clone(doc.Wallpapers),
		Thumbnails: make(map[string][]byte, len(doc.Wallpapers)),
	}
	if doc.ActiveID != "" {
		data, err := fileio.Read(ctx, s.imagePath(doc.ActiveID), maxWallpaperStoredBytes)
		if err == nil {
			out.ActiveData = data
		} else if !errors.Is(err, os.ErrNotExist) {
			return app.WallpaperSnapshot{}, err
		} else {
			out.ActiveID = ""
		}
	}
	for _, w := range doc.Wallpapers {
		if thumb, err := fileio.Read(ctx, s.thumbPath(w.ID), maxWallpaperStoredBytes); err == nil {
			out.Thumbnails[w.ID] = thumb
		}
	}
	return out, ctx.Err()
}

func readBoundedWallpaperSource(ctx context.Context, sourcePath string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	before, err := os.Lstat(sourcePath)
	if err != nil {
		return nil, err
	}
	if !before.Mode().IsRegular() {
		return nil, errors.New("请选择常规图片文件")
	}
	if before.Size() <= 0 || before.Size() > maxWallpaperSourceBytes {
		return nil, errors.New("请选择 128 MiB 以内的图片文件")
	}
	file, err := os.Open(sourcePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(before, opened) {
		return nil, errors.New("所选图片已改变")
	}
	raw, err := io.ReadAll(io.LimitReader(file, maxWallpaperSourceBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > maxWallpaperSourceBytes {
		return nil, errors.New("请选择 128 MiB 以内的图片文件")
	}
	after, err := file.Stat()
	if err != nil || after.Size() != before.Size() || !after.ModTime().Equal(before.ModTime()) {
		return nil, errors.New("图片读取期间发生改变")
	}
	return raw, ctx.Err()
}

func encodeWallpaperImages(ctx context.Context, raw []byte) (fullPNG, thumbPNG []byte, width, height int, err error) {
	config, _, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		return nil, nil, 0, 0, errors.New("无法解析图片，请选择 PNG、JPEG、GIF 或 WebP 格式")
	}
	if config.Width < 1 || config.Height < 1 || config.Width > maxWallpaperSourceDim || config.Height > maxWallpaperSourceDim {
		return nil, nil, 0, 0, errors.New("图片尺寸应在 1×1 到 16384×16384 之间")
	}
	decoded, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, nil, 0, 0, errors.New("图片解码失败")
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, 0, 0, err
	}

	outW, outH := fitWallpaperDimensions(config.Width, config.Height, maxWallpaperWidth, maxWallpaperHeight)
	fullImg := image.NewRGBA(image.Rect(0, 0, outW, outH))
	scaler := draw.CatmullRom
	if config.Width > 4096 || config.Height > 4096 {
		scaler = draw.BiLinear
	}
	scaler.Scale(fullImg, fullImg.Bounds(), decoded, decoded.Bounds(), draw.Src, nil)

	var fullBuf bytes.Buffer
	encoder := &png.Encoder{CompressionLevel: png.BestSpeed}
	if err := encoder.Encode(&fullBuf, fullImg); err != nil {
		return nil, nil, 0, 0, err
	}

	if err := ctx.Err(); err != nil {
		return nil, nil, 0, 0, err
	}

	crop := centerCoverRect(fullImg.Bounds(), wallpaperThumbWidth, wallpaperThumbHeight)
	thumbImg := image.NewRGBA(image.Rect(0, 0, wallpaperThumbWidth, wallpaperThumbHeight))
	draw.CatmullRom.Scale(thumbImg, thumbImg.Bounds(), fullImg, crop, draw.Src, nil)

	var thumbBuf bytes.Buffer
	if err := png.Encode(&thumbBuf, thumbImg); err != nil {
		return nil, nil, 0, 0, err
	}
	return fullBuf.Bytes(), thumbBuf.Bytes(), config.Width, config.Height, nil
}

func fitWallpaperDimensions(srcW, srcH, maxW, maxH int) (int, int) {
	if srcW <= maxW && srcH <= maxH {
		return srcW, srcH
	}
	scale := min(float64(maxW)/float64(srcW), float64(maxH)/float64(srcH))
	return max(1, int(math.Round(float64(srcW)*scale))), max(1, int(math.Round(float64(srcH)*scale)))
}

func centerCoverRect(bounds image.Rectangle, targetW, targetH int) image.Rectangle {
	srcW, srcH := bounds.Dx(), bounds.Dy()
	if srcW <= 0 || srcH <= 0 {
		return bounds
	}
	targetAspect := float64(targetW) / float64(targetH)
	srcAspect := float64(srcW) / float64(srcH)
	if srcAspect > targetAspect {
		cropW := max(1, int(math.Round(float64(srcH)*targetAspect)))
		x0 := bounds.Min.X + (srcW-cropW)/2
		return image.Rect(x0, bounds.Min.Y, x0+cropW, bounds.Max.Y)
	}
	if srcAspect < targetAspect {
		cropH := max(1, int(math.Round(float64(srcW)/targetAspect)))
		y0 := bounds.Min.Y + (srcH-cropH)/2
		return image.Rect(bounds.Min.X, y0, bounds.Max.X, y0+cropH)
	}
	return bounds
}

func wallpaperDisplayName(sourcePath string) string {
	base := filepath.Base(sourcePath)
	name := strings.TrimSpace(strings.TrimSuffix(base, filepath.Ext(base)))
	if name == "" {
		return "自定义墙纸"
	}
	if utf8.RuneCountInString(name) > 80 {
		runes := []rune(name)
		return string(runes[:80])
	}
	return name
}
