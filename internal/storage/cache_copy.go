package storage

import (
	"context"
	"crypto/sha1"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path"
	"strings"

	"mcl/internal/core/meta"
)

// copyVerifiedCache creates a new private copy. It never links mutable game
// files directly to the shared cache and verifies both hashes when supplied.
func copyVerifiedCache(ctx context.Context, cache, target *os.Root, name, sha1Digest, sha512Digest string, size int64) error {
	return copyVerifiedAt(ctx, cache, path.Join("cache", "market", sha1Digest), target, name, sha1Digest, sha512Digest, size)
}

func copyVerifiedAt(ctx context.Context, from *os.Root, sourceName string, target *os.Root, name, sha1Digest, sha512Digest string, size int64) error {
	if !meta.ValidSHA1(sha1Digest) || size < 0 || size > 2<<30 {
		return errors.New("缓存文件身份无效")
	}
	if sha512Digest != "" {
		decoded, err := hex.DecodeString(sha512Digest)
		if err != nil || len(decoded) != 64 {
			return errors.New("SHA-512 无效")
		}
	}
	if size == 0 {
		empty1, empty512 := sha1.Sum(nil), sha512.Sum512(nil)
		if !strings.EqualFold(sha1Digest, hex.EncodeToString(empty1[:])) || (sha512Digest != "" && !strings.EqualFold(sha512Digest, hex.EncodeToString(empty512[:]))) {
			return errors.New("空文件摘要无效")
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		f, err := target.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return err
		}
		if err := f.Sync(); err != nil {
			f.Close()
			target.Remove(name)
			return err
		}
		return f.Close()
	}
	source, err := from.Open(sourceName)
	if err != nil {
		return err
	}
	defer source.Close()
	info, err := source.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() != size {
		return errors.New("缓存文件类型或大小无效")
	}
	destination, err := target.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	defer destination.Close()
	ready := false
	defer func() {
		if !ready {
			target.Remove(name)
		}
	}()
	h1, h512 := sha1.New(), sha512.New()
	writer := io.MultiWriter(destination, h1, h512)
	buffer := make([]byte, 64<<10)
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		n, err := source.Read(buffer)
		if n > 0 {
			total += int64(n)
			if total > size {
				return errors.New("缓存文件过大")
			}
			if _, err := writer.Write(buffer[:n]); err != nil {
				return err
			}
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
	}
	if total != size || !strings.EqualFold(hex.EncodeToString(h1.Sum(nil)), sha1Digest) || (sha512Digest != "" && !strings.EqualFold(hex.EncodeToString(h512.Sum(nil)), sha512Digest)) {
		return errors.New("缓存文件摘要不匹配")
	}
	if err := destination.Sync(); err != nil {
		return err
	}
	if err := destination.Close(); err != nil {
		return err
	}
	ready = true
	return nil
}
