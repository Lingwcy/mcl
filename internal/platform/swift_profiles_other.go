//go:build !darwin

package platform

import (
	"context"
	"errors"
	"mcl/internal/core/discovery"
)

func ReadSwiftProfiles(ctx context.Context, snapshot []byte, selected string) ([]discovery.SwiftProfile, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return nil, errors.New("Swift Craft Launcher的实例索引目前仅支持macOS；标准versions目录不受影响")
}
