// Package install plans and prepares official vanilla Minecraft installations.
package install

import (
	"errors"
	"fmt"
	"path"
	"slices"
	"strings"

	"mcl/internal/core/download"
	"mcl/internal/core/meta"
)

const AssetObjectURL = "https://resources.download.minecraft.net/"
const MaxAssetIndexSize = 16 << 20

type NativeArchive struct {
	File    download.File
	Exclude []string
}

// Plan paths are slash-separated, relative to the application data root. The
// classpath includes only the target platform's modern native JARs. Legacy
// classifier archives are downloaded separately for extraction.
type Plan struct {
	VersionID                               string
	Platform                                meta.Platform
	Files                                   []download.File
	Classpath                               []string
	ClientPath, AssetIndexPath, LoggingPath string
	Natives                                 []NativeArchive
	NeedsGameAssets                         bool
}

func Build(version meta.Version, platform meta.Platform) (Plan, error) {
	if !meta.ValidID(version.ID) {
		return Plan{}, errors.New("invalid version ID")
	}
	if err := version.ValidateVanilla(); err != nil {
		return Plan{}, err
	}
	platform.Arch = meta.NormalizeArch(platform.Arch)
	if (platform.OS != "osx" && platform.OS != "windows" && platform.OS != "linux") || (platform.Arch != "amd64" && platform.Arch != "arm64" && platform.Arch != "386") {
		return Plan{}, errors.New("unsupported installation platform")
	}
	p := Plan{VersionID: version.ID, Platform: platform}
	for _, argument := range version.Arguments.Game {
		for _, value := range argument.Values {
			p.NeedsGameAssets = p.NeedsGameAssets || strings.Contains(value, "${game_assets}")
		}
	}
	seen := make(map[string]download.File)
	nativeGroups := make(map[string]bool)
	nativeArchives := make(map[string]int)
	add := func(target string, data meta.Download) (download.File, error) {
		f := download.File{Path: target, Download: data}
		if err := f.Validate(); err != nil {
			return f, err
		}
		key := strings.ToLower(target)
		if previous, ok := seen[key]; ok {
			if previous.Path != target || previous.Size != f.Size || !strings.EqualFold(previous.SHA1, f.SHA1) {
				return f, fmt.Errorf("conflicting installation target %s", target)
			}
			return previous, nil
		}
		seen[key] = f
		p.Files = append(p.Files, f)
		return f, nil
	}
	clientVersion := version.ID
	if version.ClientVersion != "" {
		clientVersion = version.ClientVersion
	}
	p.ClientPath = path.Join("cache/versions", clientVersion, clientVersion+".jar")
	if _, err := add(p.ClientPath, version.Downloads["client"]); err != nil {
		return Plan{}, err
	}
	for _, lib := range version.Libraries {
		allow, err := meta.Allowed(lib.Rules, platform)
		if err != nil {
			return Plan{}, fmt.Errorf("library %s: %w", lib.Name, err)
		}
		if !allow {
			continue
		}
		coordinate, err := meta.ParseCoordinate(lib.Name)
		if err != nil {
			return Plan{}, err
		}
		if strings.HasPrefix(coordinate.Classifier, "natives-") {
			group := coordinate.Group + ":" + coordinate.Artifact + ":" + coordinate.Version
			if _, ok := nativeGroups[group]; !ok {
				nativeGroups[group] = false
			}
			matches, err := modernNativeMatches(coordinate.Classifier, platform)
			if err != nil {
				return Plan{}, err
			}
			if !matches {
				continue
			}
			nativeGroups[group] = true
		}
		if lib.Downloads.Artifact != nil {
			data := *lib.Downloads.Artifact
			relative := data.Path
			if relative == "" {
				relative = coordinate.Path()
			}
			if lib.LocalRoot != "" {
				p.Classpath = append(p.Classpath, path.Join(lib.LocalRoot, relative))
				continue
			}
			file, err := add("cache/libraries/"+relative, data)
			if err != nil {
				return Plan{}, err
			}
			if !slices.Contains(p.Classpath, file.Path) {
				p.Classpath = append(p.Classpath, file.Path)
			}
		} else if len(lib.Natives) == 0 {
			return Plan{}, fmt.Errorf("library %s has no verified artifact", lib.Name)
		}
		if len(lib.Natives) == 0 {
			continue
		}
		classifier, ok := lib.Natives[platform.OS]
		if !ok {
			return Plan{}, fmt.Errorf("library %s has no natives for %s", lib.Name, platform.OS)
		}
		// ${arch} in the legacy format means x86 bitness, not host pointer
		// width. Do not substitute 64 and accidentally select x86 on ARM.
		if platform.Arch == "arm64" && strings.Contains(classifier, "${arch}") {
			return Plan{}, fmt.Errorf("library %s has no explicit ARM64 native classifier", lib.Name)
		}
		bits := "64"
		if platform.Arch == "386" {
			bits = "32"
		}
		classifier = strings.ReplaceAll(classifier, "${arch}", bits)
		if strings.Contains(classifier, "${") {
			return Plan{}, errors.New("unknown native classifier placeholder")
		}
		if platform.Arch == "arm64" && !strings.HasSuffix(classifier, "-arm64") && !strings.HasSuffix(classifier, "-aarch64") {
			return Plan{}, fmt.Errorf("library %s lacks explicit ARM64 natives", lib.Name)
		}
		matched, err := legacyNativeMatches(classifier, platform)
		if err != nil {
			return Plan{}, fmt.Errorf("library %s: %w", lib.Name, err)
		}
		if !matched {
			return Plan{}, fmt.Errorf("library %s native classifier %s does not match %s/%s", lib.Name, classifier, platform.OS, platform.Arch)
		}
		if err := validateNativeExclusions(lib.Extract.Exclude); err != nil {
			return Plan{}, err
		}
		data, ok := lib.Downloads.Classifiers[classifier]
		if !ok {
			return Plan{}, fmt.Errorf("library %s is missing classifier %s", lib.Name, classifier)
		}
		relative := data.Path
		if relative == "" {
			coordinate.Classifier = classifier
			relative = coordinate.Path()
		}
		file, err := add("cache/libraries/"+relative, data)
		if err != nil {
			return Plan{}, err
		}
		if previous, ok := nativeArchives[file.Path]; ok {
			if !slices.Equal(p.Natives[previous].Exclude, lib.Extract.Exclude) {
				return Plan{}, errors.New("同一本地库归档存在不同的解压排除项")
			}
			continue
		}
		nativeArchives[file.Path] = len(p.Natives)
		p.Natives = append(p.Natives, NativeArchive{File: file, Exclude: slices.Clone(lib.Extract.Exclude)})
	}
	for group, matched := range nativeGroups {
		if !matched {
			return Plan{}, fmt.Errorf("library %s has no native artifact for %s/%s", group, platform.OS, platform.Arch)
		}
	}
	p.Classpath = append(p.Classpath, p.ClientPath)
	p.AssetIndexPath = "cache/assets/indexes/" + version.AssetIndex.ID + ".json"
	if version.AssetIndex.Size > MaxAssetIndexSize {
		return Plan{}, errors.New("asset index exceeds 16 MiB")
	}
	if _, err := add(p.AssetIndexPath, version.AssetIndex.Download); err != nil {
		return Plan{}, err
	}
	if logging := version.Logging.Client; logging != nil {
		p.LoggingPath = "cache/assets/log_configs/" + logging.File.ID
		if _, err := add(p.LoggingPath, logging.File.Download); err != nil {
			return Plan{}, err
		}
	}
	return p, nil
}

func modernNativeMatches(classifier string, p meta.Platform) (bool, error) {
	base := strings.TrimPrefix(classifier, "natives-")
	// Mojang's 1.21.1 FreeType macOS patch JAR contains macos/x64/
	// resources (verified against its official SHA-1), not a new CPU type.
	if base == "macos-patch" {
		base = "macos"
	}
	arch := "amd64"
	for _, suffix := range []struct{ suffix, arch string }{{"-x86_64", "amd64"}, {"-aarch64", "arm64"}, {"-arm64", "arm64"}, {"-amd64", "amd64"}, {"-x64", "amd64"}, {"-x86", "386"}, {"-arm32", "arm"}, {"-64", "amd64"}, {"-32", "386"}} {
		if strings.HasSuffix(base, suffix.suffix) {
			base = strings.TrimSuffix(base, suffix.suffix)
			arch = suffix.arch
			break
		}
	}
	if base == "macos" {
		base = "osx"
	}
	if base != "osx" && base != "windows" && base != "linux" {
		return false, fmt.Errorf("unsupported native classifier %q", classifier)
	}
	return base == p.OS && arch == p.Arch, nil
}

type AssetObject struct {
	Hash string `json:"hash"`
	Size int64  `json:"size"`
}
type AssetIndex struct {
	Objects        map[string]AssetObject `json:"objects"`
	Virtual        bool                   `json:"virtual"`
	MapToResources bool                   `json:"map_to_resources"`
}

// PlanAssets returns the shared, hash-addressed downloads for every supported
// layout. Named copies are prepared separately under the instance reservation.
func PlanAssets(data []byte) ([]download.File, error) {
	plan, err := ParseAssetPlan(data)
	return plan.Files, err
}
