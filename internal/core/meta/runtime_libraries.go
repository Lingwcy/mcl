package meta

import (
	"encoding/hex"
	"errors"
	"path"
	"strings"
)

// ValidLibraryGeneration accepts only the launcher's non-overwriting Forge
// runtime layout, never an arbitrary relative directory supplied by metadata.
func ValidLibraryGeneration(directory string) bool {
	parts := strings.Split(directory, "/")
	if len(parts) != 5 || parts[0] != "cache" || parts[1] != "loader-runtimes" || len(parts[2]) != 64 || parts[4] != "libraries" {
		return false
	}
	if _, err := hex.DecodeString(parts[2]); err != nil {
		return false
	}
	return strings.HasPrefix(parts[3], "ready-") && len(parts[3]) == len("ready-")+26 && ValidID(parts[3]) && ValidPath(directory)
}

// RuntimeLookups are not classpath entries or remote downloads. They are fixed
// generated files a loader opens by Maven path under LibraryDirectory.
func (v Version) validateRuntimeLookups() error {
	if len(v.RuntimeLookups) > 16 {
		return errors.New("运行库查询产物超过边界")
	}
	if v.LibraryDirectory != "" && !ValidLibraryGeneration(v.LibraryDirectory) {
		return errors.New("运行库目录不是受管代际")
	}
	if len(v.RuntimeLookups) > 0 && v.LibraryDirectory == "" {
		return errors.New("运行库查询缺少固定代际目录")
	}
	seen := make(map[string]bool)
	for _, file := range v.RuntimeLookups {
		if file.ValidateIdentity() != nil || file.URL != "" || file.Size > 512<<20 {
			return errors.New("运行库查询产物身份无效")
		}
		coordinate, err := CoordinateFromPath(file.Path)
		if err != nil || coordinate.Extension != "jar" || seen[strings.ToLower(file.Path)] {
			return errors.New("运行库查询产物坐标或路径冲突")
		}
		seen[strings.ToLower(file.Path)] = true
		for _, lib := range v.Libraries {
			if lib.LocalRoot == "" || lib.Downloads.Artifact == nil {
				continue
			}
			artifact := lib.Downloads.Artifact
			if path.Join(lib.LocalRoot, artifact.Path) == path.Join(v.LibraryDirectory, file.Path) && *artifact != file {
				return errors.New("运行库查询与classpath文件身份冲突")
			}
		}
	}
	return nil
}
