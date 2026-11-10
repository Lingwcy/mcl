package loader

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"mcl/internal/core/diagnostics"
	"mcl/internal/core/download"
	"mcl/internal/core/meta"
)

// No URL or transport error is retained in the status used for fallback.
type metadataStatusError struct{ status int }

func (e *metadataStatusError) DiagnosticCode() string { return diagnostics.HTTPCode(e.status) }

func (e *metadataStatusError) Error() string {
	return fmt.Sprintf("加载器服务返回 HTTP %d", e.status)
}

// Legacy installers may pin several historical variants of one artifact while
// their repository serves the JAR without a SHA-1 sidecar. Never infer identity
// from HEAD alone, a filename, a different coordinate or freshly downloaded bytes.
func (s *ForgeInstaller) legacyForgePinnedMetadata(ctx context.Context, address, relative string, checksums []string, maxSize int64) (meta.Download, error) {
	ctx = WithResolutionBudget(ctx)
	if maxSize <= 0 {
		return meta.Download{}, identityError("旧Forge依赖没有剩余安装预算")
	}
	if err := validateMavenLocation(address, relative); err != nil {
		return meta.Download{}, err
	}
	ctx, evidence := diagnostics.Start(ctx, "metadata", relative)
	defer evidence.End()
	if len(checksums) > 16 {
		return meta.Download{}, identityError("旧Forge依赖checksum数量超过边界")
	}
	pins := make([]string, 0, len(checksums))
	seen := make(map[string]bool)
	for _, pin := range checksums {
		if !meta.ValidSHA1(pin) {
			return meta.Download{}, identityError("旧Forge依赖checksum声明无效")
		}
		pin = strings.ToLower(pin)
		if !seen[pin] {
			pins = append(pins, pin)
			seen[pin] = true
		}
	}
	fixed, err := s.catalog.mavenMetadata(ctx, address, relative)
	if ctx.Err() != nil {
		return meta.Download{}, ctx.Err()
	}
	if err == nil {
		if fixed.Size > maxSize {
			return meta.Download{}, identityError("旧Forge依赖超过剩余安装资源边界")
		}
		if len(pins) > 0 && !seen[strings.ToLower(fixed.SHA1)] {
			return meta.Download{}, identityError("旧Forge依赖与安装清单checksum不一致")
		}
		return fixed, nil
	}
	var checksum *mavenChecksumError
	var status *metadataStatusError
	if len(pins) == 0 || !errors.As(err, &checksum) || !errors.As(checksum.cause, &status) || (status.status != http.StatusNotFound && status.status != http.StatusGone) {
		return meta.Download{}, err
	}
	size, err := s.catalog.mavenSize(ctx, address)
	if err != nil {
		return meta.Download{}, err
	}
	if size > maxSize {
		return meta.Download{}, identityError("旧Forge依赖超过安装资源边界")
	}
	// Try only installer-pinned digests. Each transfer uses the shared client,
	// original concurrency/rate/proxy/cache/partial/lease and full-hash protection.
	// A filesystem/network/publication failure must not be treated as another hash.
	for i, pin := range pins {
		fixed = meta.Download{Path: relative, URL: address, SHA1: pin, Size: size}
		file := forgeInput(fixed)
		err = s.downloads.Run(ctx, []download.File{file}, nil)
		if ctx.Err() != nil {
			return meta.Download{}, ctx.Err()
		}
		if err == nil {
			return fixed, nil
		}
		if !errors.Is(err, download.ErrContentMismatch) || i+1 == len(pins) {
			return meta.Download{}, err
		}
		if err := download.Observe(ctx, download.Event{Key: download.FileKey(file), Path: file.Path, State: "skipped", Total: file.Size}); err != nil {
			return meta.Download{}, err
		}
	}
	return meta.Download{}, errors.New("旧Forge依赖未匹配安装清单checksum")
}
