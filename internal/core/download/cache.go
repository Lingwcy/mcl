package download

import (
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
)

// Object identifies bytes, not a URL, local path or distribution permission.
type Object struct {
	SHA1, SHA256, SHA512 string
	Size                 int64
}

func (o Object) Key() (string, error) {
	if o.Size < 0 || o.Size > MaxFileSize {
		return "", errors.New("缓存大小无效")
	}
	valid := func(value string, size int) bool {
		data, err := hex.DecodeString(value)
		return err == nil && len(data) == size
	}
	var primary string
	switch {
	case o.SHA1 != "" && o.SHA256 == "" && valid(o.SHA1, 20):
		primary = "sha1-" + o.SHA1
	case o.SHA256 != "" && o.SHA1 == "" && valid(o.SHA256, 32):
		primary = "sha256-" + o.SHA256
	default:
		return "", errors.New("缓存摘要无效")
	}
	if o.SHA512 != "" {
		if !valid(o.SHA512, 64) {
			return "", errors.New("缓存 SHA-512 无效")
		}
		primary += "-" + o.SHA512
	}
	return fmt.Sprintf("object-%s-%d.blob", strings.ToLower(primary), o.Size), nil
}

// Copy verifies while copying. Callers must not publish destination on failure.
func (o Object) Copy(ctx context.Context, destination io.Writer, source io.Reader) error {
	if _, err := o.Key(); err != nil {
		return err
	}
	h1, h256, h512 := sha1.New(), sha256.New(), sha512.New()
	writers := []io.Writer{destination}
	if o.SHA1 != "" {
		writers = append(writers, h1)
	}
	if o.SHA256 != "" {
		writers = append(writers, h256)
	}
	if o.SHA512 != "" {
		writers = append(writers, h512)
	}
	writer := io.MultiWriter(writers...)
	count, err := io.CopyBuffer(writer, io.LimitReader(&contextReader{ctx: ctx, reader: source}, o.Size+1), make([]byte, 64<<10))
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	equal := func(hash []byte, expected string) bool {
		return expected == "" || strings.EqualFold(hex.EncodeToString(hash), expected)
	}
	if count != o.Size || !equal(h1.Sum(nil), o.SHA1) || !equal(h256.Sum(nil), o.SHA256) || !equal(h512.Sum(nil), o.SHA512) {
		return errors.New("下载副本摘要或大小不匹配")
	}
	return nil
}

type ObjectCache interface {
	Acquire(context.Context) (CacheLease, error)
}

// A lease pins its directory and holds a shared system lock for the entire job.
type CacheLease interface {
	io.Closer
	Restore(context.Context, Object, io.Writer) (bool, error)
	Store(context.Context, Object, io.Reader) error
}
type CacheInfo struct {
	Directory                      string
	Files, TemporaryFiles, Skipped int
	Bytes, TemporaryBytes          int64
}
