package loader

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"mcl/internal/core/archive"
	"mcl/internal/core/meta"
)

type forgeProcessor struct {
	Jar       string
	Classpath []string
	Args      []string
	Outputs   map[string]string
	Sides     []string
}

type forgeSpec struct {
	Spec       int
	Minecraft  string
	Version    string
	JSON       string
	Libraries  []meta.Library
	Processors []forgeProcessor
	Data       map[string]struct{ Client string }
	LegacyGame bool `json:"-"`
}

type forgeLibrary struct {
	name string
	file meta.Download
}

func validGeneration(name string) bool {
	return meta.ValidID(name) && strings.HasPrefix(name, "ready-") && len(name) == len("ready-")+26
}

func forgeMainClass(name string) bool {
	switch name {
	case "cpw.mods.modlauncher.Launcher", "cpw.mods.bootstraplauncher.BootstrapLauncher",
		"net.minecraftforge.bootstrap.ForgeBootstrap", "net.neoforged.fml.startup.Client":
		return true
	}
	return false
}

func forgeInstaller(request Request) (meta.Coordinate, string, string, error) {
	if err := request.Validate(); err != nil {
		return meta.Coordinate{}, "", "", err
	}
	prefix := request.GameVersion + "-"
	artifact := meta.Coordinate{Classifier: "installer", Extension: "jar"}
	switch request.Type {
	case "forge":
		short := strings.TrimPrefix(request.Version, prefix)
		artifact.Group, artifact.Artifact, artifact.Version = "net.minecraftforge", "forge", prefix+short
		return artifact, "https://maven.minecraftforge.net", prefix + "forge-" + short, nil
	case "neoforge":
		if request.GameVersion == "1.20.1" {
			short := strings.TrimPrefix(request.Version, prefix)
			artifact.Group, artifact.Artifact, artifact.Version = "net.neoforged", "forge", prefix+short
			return artifact, "https://maven.neoforged.net/releases", prefix + "forge-" + short, nil
		}
		artifact.Group, artifact.Artifact, artifact.Version = "net.neoforged", "neoforge", request.Version
		return artifact, "https://maven.neoforged.net/releases", "neoforge-" + request.Version, nil
	default:
		return meta.Coordinate{}, "", "", errors.New("此加载器没有 Forge 系安装配置")
	}
}

func modernRuntimeCore(request Request, coordinate meta.Coordinate) bool {
	expected, _, _, err := forgeInstaller(request)
	if err != nil || coordinate.Group != expected.Group || coordinate.Version != expected.Version || coordinate.Extension != "jar" {
		return false
	}
	return coordinate.Artifact == expected.Artifact && (coordinate.Classifier == "" || coordinate.Classifier == "client" || coordinate.Classifier == "universal") || coordinate.Artifact == "fmlloader" && coordinate.Classifier == ""
}

func readForgeSpec(ctx context.Context, z *archive.Zip, request Request, versionID string) (forgeSpec, meta.Version, error) {
	var spec forgeSpec
	data, err := z.Read(ctx, "install_profile.json", 4<<20)
	if err != nil {
		return spec, meta.Version{}, err
	}
	if err := json.Unmarshal(data, &spec); err != nil {
		return spec, meta.Version{}, fmt.Errorf("读取安装配置: %w", err)
	}
	validID := spec.Version == versionID || legacyForgeID(request, spec.Version)
	if spec.Spec != 1 || spec.Minecraft != request.GameVersion || !validID ||
		len(spec.Libraries) > 256 || len(spec.Data) > 128 || len(spec.Processors) > 64 {
		return spec, meta.Version{}, errors.New("安装配置的版本、格式或数量不受支持")
	}
	if spec.JSON == "" {
		spec.JSON = "/version.json"
	}
	data, err = z.Read(ctx, strings.TrimPrefix(spec.JSON, "/"), 4<<20)
	if err != nil {
		return spec, meta.Version{}, err
	}
	version, err := meta.ParseVersion(data)
	if err != nil {
		return spec, version, err
	}
	if version.ID != spec.Version || version.InheritsFrom != request.GameVersion || len(version.Libraries) == 0 || len(version.Libraries) > 256 {
		return spec, version, errors.New("启动配置与所选加载器或 Minecraft 版本不匹配")
	}
	if version.Arguments == nil && version.LegacyArguments != "" {
		if err := normalizeProcessedLegacyVersion(request, &version); err != nil {
			return spec, version, err
		}
		spec.LegacyGame = true
	} else if version.Arguments == nil || version.LegacyArguments != "" || !forgeMainClass(version.MainClass) {
		return spec, version, errors.New("启动配置与所选加载器或 Minecraft 版本不匹配")
	}
	for _, processor := range spec.Processors {
		if len(processor.Sides) > 2 {
			return spec, version, errors.New("安装处理器sides数量无效")
		}
		for _, side := range processor.Sides {
			if side != "client" && side != "server" {
				return spec, version, errors.New("未知安装处理器用途，不能静默省略")
			}
		}
	}
	spec.Processors = slices.DeleteFunc(spec.Processors, func(p forgeProcessor) bool {
		return len(p.Sides) > 0 && !slices.Contains(p.Sides, "client")
	})
	return spec, version, nil
}

func forgeLibraries(groups ...[]meta.Library) (map[string]forgeLibrary, error) {
	result := make(map[string]forgeLibrary)
	keys := make(map[string]string)
	for _, group := range groups {
		for _, library := range group {
			coordinate, err := meta.ParseCoordinate(library.Name)
			if err != nil {
				return nil, err
			}
			relative := coordinate.Path()
			if len(library.Natives) != 0 || len(library.Downloads.Classifiers) != 0 {
				return nil, fmt.Errorf("不支持加载器库的本地分类 %s", library.Name)
			}
			file := meta.Download{Path: relative}
			if value := library.Downloads.Artifact; value != nil {
				file = *value
				if file.Path == "" {
					file.Path = relative
				}
			}
			if file.URL != "" {
				if _, err := meta.BindMavenURL(file.URL, coordinate); err != nil {
					return nil, identityError("Forge库仓库地址与声明坐标不一致")
				}
			}
			if file.Path != relative || (file.URL != "" && (!AllowedLibraryURL(file.URL) || file.Validate() != nil || file.Size > 512<<20)) {
				return nil, fmt.Errorf("加载器库来源、摘要或路径无效 %s", library.Name)
			}
			key := archive.Key(relative)
			if existing, ok := keys[key]; ok && existing != relative {
				return nil, errors.New("加载器库路径别名冲突")
			}
			keys[key] = relative
			if old, ok := result[relative]; ok {
				if old.file.SHA1 != "" && file.SHA1 != "" && (!strings.EqualFold(old.file.SHA1, file.SHA1) || old.file.Size != file.Size) {
					return nil, fmt.Errorf("加载器库声明冲突 %s", library.Name)
				}
				if file.URL == "" && (old.file.URL != "" || old.file.SHA1 != "") {
					continue
				}
			}
			result[relative] = forgeLibrary{name: library.Name, file: file}
		}
	}
	return result, nil
}
