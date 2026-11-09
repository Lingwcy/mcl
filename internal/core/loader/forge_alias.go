package loader

import (
	"context"
	"errors"
	"strings"
)

// ValidateForgeAlias only permits the same numeric build qualified with its
// own game branch. Base-chain and platform identity must remain byte-for-byte
// equal. A different release, foreign branch or prerelease is never an alias.
func ValidateForgeAlias(requested, canonical Request) error {
	if err := requested.Validate(); err != nil {
		return err
	}
	if err := canonical.Validate(); err != nil {
		return err
	}
	if requested == canonical {
		return nil
	}
	same := canonical
	same.Version = requested.Version
	if requested.Type != "forge" || same != requested {
		return errors.New("Forge 版本映射改变了固定构建或平台身份")
	}
	prefix := requested.GameVersion + "-"
	original := strings.TrimPrefix(requested.Version, prefix)
	resolved := strings.TrimPrefix(canonical.Version, prefix)
	if !stableForgeBuild(original, requested.GameVersion) || strings.Contains(original, "-") || (resolved != original && resolved != original+"-"+requested.GameVersion) {
		return errors.New("Forge 版本映射不是同一构建的官方游戏分支")
	}
	return nil
}

// CanonicalForgeRequest resolves short pack build IDs before looking up an
// installer. Fully-qualified requests and modern builds retain exact identity.
func (c *Client) CanonicalForgeRequest(ctx context.Context, request Request) (Request, error) {
	if err := request.Validate(); err != nil {
		return request, err
	}
	if request.Type != "forge" {
		return request, ctx.Err()
	}
	if _, historical := legacyForgeTweakerFor(request); !historical {
		return request, ctx.Err()
	}
	short := strings.TrimPrefix(request.Version, request.GameVersion+"-")
	if strings.Contains(short, "-") {
		return request, ctx.Err()
	}
	if !stableForgeBuild(short, request.GameVersion) {
		return request, errors.New("Forge 短版本不是有效的固定构建")
	}
	versions, err := c.forgeVersions(ctx, request.GameVersion)
	if err != nil {
		return request, err
	}
	exact, qualified := false, ""
	for _, version := range versions {
		candidate := strings.TrimPrefix(version, request.GameVersion+"-")
		if candidate == short {
			exact = true
			continue
		}
		if candidate == short+"-"+request.GameVersion {
			qualified = candidate
		}
	}
	if exact {
		return request, ctx.Err()
	}
	if qualified == "" {
		return request, errors.New("官方 Forge 目录中未找到该固定构建及对应游戏分支")
	}
	canonical := request
	canonical.Version = qualified
	if err := ValidateForgeAlias(request, canonical); err != nil {
		return request, err
	}
	return canonical, ctx.Err()
}
