// Package thumbnail decodes bounded images used by catalog and local assets.
package thumbnail

import (
	"bytes"
	"errors"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

const MaxBytes = 2 << 20
const Size = 80

func Decode(data []byte) (image.Image, error) {
	if len(data) == 0 || len(data) > MaxBytes {
		return nil, errors.New("图标文件大小无效")
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || config.Width < 1 || config.Height < 1 || config.Width > 2048 || config.Height > 2048 {
		return nil, errors.New("图标尺寸超出限制")
	}
	decoded, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	output := image.NewRGBA(image.Rect(0, 0, Size, Size))
	width, height := Size, Size
	if config.Width > config.Height {
		height = max(1, Size*config.Height/config.Width)
	} else {
		width = max(1, Size*config.Width/config.Height)
	}
	target := image.Rect((Size-width)/2, (Size-height)/2, (Size+width)/2, (Size+height)/2)
	draw.CatmullRom.Scale(output, target, decoded, decoded.Bounds(), draw.Over, nil)
	return output, nil
}

func Read(reader io.Reader) (image.Image, error) {
	data, err := io.ReadAll(io.LimitReader(reader, MaxBytes+1))
	if err != nil {
		return nil, err
	}
	return Decode(data)
}
