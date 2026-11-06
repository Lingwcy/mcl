package platform

import (
	"fmt"
	"os"
	"path/filepath"
)

type Paths struct {
	Root       string
	ConfigFile string
	Instances  string
	Metadata   string
}

// DataPaths resolves paths without creating directories. Tests and portable
// development runs can supply an explicit root without touching user settings.
func DataPaths(override string) (Paths, error) {
	root := override
	if root == "" {
		base, err := os.UserConfigDir()
		if err != nil {
			return Paths{}, fmt.Errorf("resolve application data directory: %w", err)
		}
		root = filepath.Join(base, "mcl")
		if _, err := os.Lstat(root); os.IsNotExist(err) {
			legacy := filepath.Join(base, "grun")
			if info, legacyErr := os.Lstat(legacy); legacyErr == nil {
				if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
					return Paths{}, fmt.Errorf("旧版启动器的数据目录不是普通目录 %q", legacy)
				}
				root = legacy
			} else if !os.IsNotExist(legacyErr) {
				return Paths{}, fmt.Errorf("读取旧版启动器数据目录: %w", legacyErr)
			}
		} else if err != nil {
			return Paths{}, fmt.Errorf("读取 mcl 数据目录: %w", err)
		}
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return Paths{}, fmt.Errorf("resolve data directory %q: %w", root, err)
	}
	return Paths{
		Root: root, ConfigFile: filepath.Join(root, "settings.json"),
		Instances: filepath.Join(root, "instances"), Metadata: filepath.Join(root, "cache", "metadata"),
	}, nil
}
