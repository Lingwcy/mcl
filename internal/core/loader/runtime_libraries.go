package loader

import (
	"errors"
	"strings"

	"mcl/internal/core/meta"
)

// ForgeBootstrap's production client opens the generated forge:<build>:client
// via fmlloader LibraryFinder, not the JVM classpath. Official 49.0.3 sources
// and installer metadata establish this contract; it does not apply to legacy
// LaunchWrapper, other main classes or unrelated loaders.
func (p Profile) runtimeLookups() ([]meta.Download, error) {
	if p.Request.Type != "forge" || p.MainClass != "net.minecraftforge.bootstrap.ForgeBootstrap" {
		return nil, nil
	}
	if p.Legacy || p.LegacyGame {
		return nil, errors.New("ForgeBootstrap不能使用旧游戏模板")
	}
	installer, _, _, err := forgeInstaller(p.Request)
	if err != nil {
		return nil, err
	}
	expected := installer
	expected.Classifier = "client"
	boundLoader := false
	for _, library := range p.Libraries {
		coordinate, err := meta.ParseCoordinate(library.Name)
		if err != nil {
			return nil, err
		}
		if coordinate.Group == installer.Group && coordinate.Artifact == "fmlloader" && coordinate.Version == installer.Version && coordinate.Classifier == "" && coordinate.Extension == "jar" {
			boundLoader = true
		}
	}
	if !boundLoader {
		return nil, errors.New("ForgeBootstrap缺少同一构建的FML运行库")
	}
	var found *meta.Download
	for _, output := range p.Outputs {
		if output.Path != expected.Path() {
			continue
		}
		if found != nil {
			return nil, errors.New("Forge客户端查询产物重复")
		}
		value := output
		found = &value
	}
	// The exact file may instead be an explicit runtime library in an official
	// profile. It still must be the same fixed client identity, not universal.
	for _, library := range p.Libraries {
		if library.Name != expected.String() {
			continue
		}
		if library.Downloads.Artifact == nil {
			return nil, errors.New("Forge客户端查询产物缺少身份")
		}
		value := *library.Downloads.Artifact
		if found != nil && *found != value {
			return nil, errors.New("Forge客户端查询身份冲突")
		}
		found = &value
	}
	if found == nil || found.Path != expected.Path() || found.ValidateIdentity() != nil || found.URL != "" {
		return nil, errors.New("Forge客户端缺少固定的client查询产物，需重新准备运行环境")
	}
	return []meta.Download{*found}, nil
}

func validateLibraryDirectoryArguments(arguments []meta.Argument) error {
	for _, argument := range arguments {
		for _, value := range argument.Values {
			if value == "-DlibraryDirectory" || strings.HasPrefix(value, "-DlibraryDirectory=") {
				if value != "-DlibraryDirectory=${library_directory}" {
					return errors.New("加载器不能替换受管运行库目录")
				}
			}
		}
	}
	return nil
}
