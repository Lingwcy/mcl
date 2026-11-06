package platform

import (
	"bytes"
	"context"
	"errors"
	"image/png"
	"os"

	"mcl/internal/core/thumbnail"
)

// ReadInstanceIcon reads a user-selected image and returns a bounded PNG copy.
// The source path is never persisted into an instance document.
func ReadInstanceIcon(ctx context.Context, name string) ([]byte, error) {
	before, err := os.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !before.Mode().IsRegular() || before.Size() > thumbnail.MaxBytes {
		return nil, errors.New("请选择 2 MiB 以内的图片文件")
	}
	file, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(before, opened) {
		return nil, errors.New("所选图片已改变")
	}
	image, err := thumbnail.Read(file)
	if err != nil {
		return nil, err
	}
	after, err := file.Stat()
	if err != nil || after.Size() != before.Size() || !after.ModTime().Equal(before.ModTime()) {
		return nil, errors.New("图片读取期间发生改变")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var output bytes.Buffer
	if err := png.Encode(&output, image); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}
