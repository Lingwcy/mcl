package launch

import (
	"errors"
	"strings"

	"mcl/internal/core/meta"
)

// Metadata may declare this property only with the managed placeholder. The
// launch plan is the authority for its resolved value, including older cached
// profiles and conditional JVM argument variants.
func libraryDirectoryArguments(jvm []string, version meta.Version) ([]string, error) {
	result := append([]string(nil), jvm...)
	found := false
	for _, arg := range result {
		if arg != "-DlibraryDirectory" && !strings.HasPrefix(arg, "-DlibraryDirectory=") {
			continue
		}
		if arg != "-DlibraryDirectory=${library_directory}" || found || !meta.ValidLibraryGeneration(version.LibraryDirectory) {
			return nil, errors.New("运行库目录参数必须唯一并指向受管代际")
		}
		found = true
	}
	if len(version.RuntimeLookups) > 0 && !found {
		result = append(result, "-DlibraryDirectory=${library_directory}")
	}
	return result, nil
}
