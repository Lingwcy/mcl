package platform

import (
	"errors"
	"fmt"
	"strings"
	"unicode"

	"mcl/internal/core/meta"
)

// HostPlatform is collected once during dependency assembly, not while building
// UI or resolving metadata rules. Core CurrentPlatform provides only OS/CPU;
// rule evaluation additionally needs the native OS version (not a Java probe).
func HostPlatform() (meta.Platform, error) {
	return hostPlatform(meta.CurrentPlatform(), operatingSystemVersion)
}

func hostPlatform(identity meta.Platform, version func() (string, error)) (meta.Platform, error) {
	value, err := version()
	if err != nil {
		return identity, fmt.Errorf("读取操作系统版本: %w", err)
	}
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 256 || strings.ContainsFunc(value, unicode.IsControl) {
		return identity, errors.New("操作系统版本为空或格式无效")
	}
	identity.Version = value
	return identity, nil
}
