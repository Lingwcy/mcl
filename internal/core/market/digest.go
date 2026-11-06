package market

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"io"
)

// Digest reads bounded chunks, never an entire archive into memory. The
// CurseForge protocol uses MurmurHash2 with seed 1 after excluding four ASCII
// whitespace bytes. Its length prefix requires a counting pass before hashing.
func Digest(ctx context.Context, reader io.ReadSeeker, size int64, withFingerprint bool, progress func(int64)) (FileDigest, error) {
	if size < 0 || size > 2<<30 || (size == 0 && withFingerprint) {
		return FileDigest{}, errors.New("资产文件大小超出识别范围")
	}
	if _, err := reader.Seek(0, io.SeekStart); err != nil {
		return FileDigest{}, err
	}
	var filtered uint32
	if withFingerprint {
		if err := readDigestBytes(ctx, reader, size, func(data []byte) {
			for _, value := range data {
				if !fingerprintSpace(value) {
					filtered++
				}
			}
		}, progress); err != nil {
			return FileDigest{}, err
		}
		if _, err := reader.Seek(0, io.SeekStart); err != nil {
			return FileDigest{}, err
		}
	}
	hash := sha1.New()
	fingerprint := fingerprintState{value: 1 ^ filtered}
	base := int64(0)
	if withFingerprint {
		base = size
	}
	err := readDigestBytes(ctx, reader, size, func(data []byte) {
		hash.Write(data)
		if withFingerprint {
			for _, value := range data {
				if !fingerprintSpace(value) {
					fingerprint.add(value)
				}
			}
		}
	}, func(read int64) {
		if progress != nil {
			progress(base + read)
		}
	})
	if err != nil {
		return FileDigest{}, err
	}
	result := FileDigest{SHA1: hex.EncodeToString(hash.Sum(nil)), Size: size}
	if withFingerprint {
		if fingerprint.count != filtered {
			return FileDigest{}, errors.New("指纹计算期间文件发生变化")
		}
		result.Fingerprint = fingerprint.finish()
	}
	return result, ctx.Err()
}

func readDigestBytes(ctx context.Context, reader io.Reader, size int64, consume func([]byte), progress func(int64)) error {
	buffer := make([]byte, 64<<10)
	limited := io.LimitReader(reader, size+1)
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		n, err := limited.Read(buffer)
		if n > 0 {
			total += int64(n)
			if total > size {
				return errors.New("识别期间文件大小发生变化")
			}
			consume(buffer[:n])
			if progress != nil {
				progress(total)
			}
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrNoProgress
		}
	}
	if total != size {
		return errors.New("识别期间文件长度发生变化")
	}
	return ctx.Err()
}

func fingerprintSpace(value byte) bool {
	return value == 9 || value == 10 || value == 13 || value == 32
}

const fingerprintMultiplier uint32 = 0x5bd1e995

type fingerprintState struct {
	value, tail, count uint32
	used               uint8
}

func (s *fingerprintState) add(value byte) {
	s.tail |= uint32(value) << (8 * s.used)
	s.count++
	s.used++
	if s.used == 4 {
		block := s.tail * fingerprintMultiplier
		block ^= block >> 24
		block *= fingerprintMultiplier
		s.value = s.value*fingerprintMultiplier ^ block
		s.tail, s.used = 0, 0
	}
}

func (s fingerprintState) finish() uint32 {
	if s.used > 0 {
		s.value ^= s.tail
		s.value *= fingerprintMultiplier
	}
	s.value ^= s.value >> 13
	s.value *= fingerprintMultiplier
	s.value ^= s.value >> 15
	return s.value
}
