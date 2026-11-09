package loader

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"strings"

	"mcl/internal/core/archive"
	"mcl/internal/core/download"
	"mcl/internal/core/meta"
)

const legacyForgeMain = "net.minecraft.launchwrapper.Launch"
const legacyForgeTweaker = "net.minecraftforge.fml.common.launcher.FMLTweaker"

type legacyForgeLibrary struct {
	meta.Library
	URL       string
	ClientReq *bool
	Checksums []string
}
type legacyForgeSpec struct {
	Install struct {
		Path, Version, FilePath, Minecraft string
		Target                             json.RawMessage
	}
	VersionInfo json.RawMessage
	version     meta.Version
	libraries   []legacyForgeLibrary
}

func legacyForgeID(request Request, id string) bool {
	if _, ok := legacyForgeTweakerFor(request); !ok {
		return false
	}
	build := strings.TrimPrefix(request.Version, request.GameVersion+"-")
	return id == request.GameVersion+"-Forge"+build || id == request.GameVersion+"-forge-"+build || id == request.GameVersion+"-forge"+request.GameVersion+"-"+build
}
func legacyForgeCore(request Request, name string) bool {
	coordinate, err := meta.ParseCoordinate(name)
	build := strings.TrimPrefix(request.Version, request.GameVersion+"-")
	_, supported := legacyForgeTweakerFor(request)
	return err == nil && supported && coordinate.Group == "net.minecraftforge" && coordinate.Artifact == "forge" && coordinate.Version == request.GameVersion+"-"+build && coordinate.Extension == "jar" && (coordinate.Classifier == "" || coordinate.Classifier == "universal")
}

func legacyForgeArguments(request Request, template string) (meta.Arguments, error) {
	values, err := meta.SplitLegacyArguments(template)
	if err != nil {
		return meta.Arguments{}, err
	}
	if err := validateLegacyForgeArguments(request, values); err != nil {
		return meta.Arguments{}, err
	}
	result := meta.Arguments{}
	for _, value := range values {
		result.Game = append(result.Game, meta.Argument{Values: []string{value}})
	}
	return result, nil
}
func validateLegacyForgeArguments(request Request, values []string) error {
	tweaker, supported := legacyForgeTweakerFor(request)
	if !supported {
		return errors.New("旧Forge游戏代际未接入")
	}
	if len(values) == 0 || len(values) > 128 || len(values)%2 != 0 {
		return errors.New("旧Forge游戏参数必须为有界选项/值对")
	}
	allowed := map[string]string{
		"--username": "auth_player_name", "--version": "version_name", "--gameDir": "game_directory",
		"--assetsDir": "assets_root", "--assetIndex": "assets_index_name", "--uuid": "auth_uuid",
		"--session": "auth_session", "--accessToken": "auth_access_token", "--userType": "user_type", "--versionType": "version_type", "--userProperties": "user_properties",
	}
	seen := make(map[string]bool)
	for i := 0; i < len(values); i += 2 {
		option, value := values[i], values[i+1]
		if seen[option] {
			return errors.New("旧Forge参数重复，不能追加另一套身份或入口")
		}
		seen[option] = true
		if option == "--tweakClass" {
			if value != tweaker {
				return errors.New("旧Forge使用未知Tweaker，未当作受支持的LaunchWrapper配置")
			}
			continue
		}
		key, ok := allowed[option]
		if option == "--userProperties" && value == "{}" {
			continue
		}
		if option == "--assetsDir" && value == "$"+"{game_assets}" {
			continue
		}
		if !ok || value != "$"+"{"+key+"}" {
			return errors.New("旧Forge包含不支持的游戏参数或占位符")
		}
	}
	requiredOptions := []string{"--username", "--version", "--gameDir", "--tweakClass"}
	switch request.GameVersion {
	case "1.6.1", "1.6.2", "1.6.4":
		requiredOptions = append(requiredOptions, "--session")
		if seen["--accessToken"] || seen["--uuid"] {
			return errors.New("旧会话参数不能混入另一套账户参数")
		}
	default:
		requiredOptions = append(requiredOptions, "--uuid", "--accessToken")
		if seen["--session"] {
			return errors.New("现代身份参数不能混入旧会话参数")
		}
	}
	for _, required := range requiredOptions {
		if !seen[required] {
			return errors.New("旧Forge缺少必需的身份或Tweaker参数")
		}
	}
	return nil
}

func validateLegacyForgeProfile(profile Profile) error {
	if !legacyForgeID(profile.Request, profile.ID) || profile.MainClass != legacyForgeMain || profile.JavaMajor != 8 || len(profile.Arguments.JVM) != 0 || len(profile.Outputs) != 0 {
		return errors.New("旧Forge配置的代际、主类或Java要求无效")
	}
	var values []string
	for _, arg := range profile.Arguments.Game {
		if len(arg.Rules) != 0 || len(arg.Values) != 1 {
			return errors.New("旧Forge完整游戏模板不能包含条件参数")
		}
		values = append(values, arg.Values[0])
	}
	if err := validateLegacyForgeArguments(profile.Request, values); err != nil {
		return err
	}
	core, wrapper := 0, 0
	for _, lib := range profile.Libraries {
		if err := validateLegacyRuntimeLibrary(profile.Request, lib, false); err != nil {
			return err
		}
		if legacyForgeWrapper(profile.Request, lib.Name) {
			wrapper++
		}
		if legacyForgeCore(profile.Request, lib.Name) {
			core++
		}
	}
	if core != 1 || wrapper != 1 {
		return errors.New("旧Forge没有唯一匹配版本的Universal或已支持LaunchWrapper运行库")
	}
	return nil
}

func readLegacyForgeSpec(ctx context.Context, z *archive.Zip, request Request, base meta.Version) (legacyForgeSpec, bool, error) {
	var spec legacyForgeSpec
	data, err := z.Read(ctx, "install_profile.json", 4<<20)
	if err != nil {
		return spec, false, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return spec, false, errors.New("Forge安装清单JSON无效")
	}
	_, hasVersion := fields["versionInfo"]
	_, hasInstall := fields["install"]
	if !hasVersion && !hasInstall {
		return spec, false, nil
	}
	if !hasVersion || !hasInstall || z.Has("version.json") {
		return spec, true, errors.New("Forge新旧安装格式混合或不完整")
	}
	if raw, ok := fields["spec"]; ok {
		var generation int
		if err := json.Unmarshal(raw, &generation); err != nil || generation != 0 || strings.TrimSpace(string(raw)) == "null" {
			return spec, true, errors.New("旧Forge仅接受明确的spec0")
		}
	}
	for _, key := range []string{"processors", "json", "data"} {
		if _, ok := fields[key]; ok {
			return spec, true, errors.New("旧Forge清单混入现代处理器字段")
		}
	}
	if _, supported := legacyForgeTweakerFor(request); !supported {
		return spec, true, errors.New("已识别旧Forge安装器；此版本不在已列明1.6–1.12正式版兼容范围，不能执行未知组件")
	}
	if err := request.ValidateBase(base); err != nil {
		return spec, true, err
	}
	if base.ID != request.GameVersion || base.Java.Major != 8 {
		return spec, true, errors.New("旧Forge要求已固定来源的官方Java8客户端")
	}
	if err := base.ValidateVanilla(); err != nil {
		return spec, true, err
	}
	if err := json.Unmarshal(data, &spec); err != nil {
		return spec, true, err
	}
	version, err := parseLegacyForgeVersion(spec.VersionInfo, base.ID, spec.Install.Minecraft)
	if err != nil {
		return spec, true, fmt.Errorf("旧Forge versionInfo无效: %w", err)
	}

	// install.version is a human-readable installer label in older releases.
	// install.target binds versionInfo.id; only producers without target use
	// version itself as the exact launch ID. An explicit malformed/null target
	// must not fall back to that label.
	target := spec.Install.Version
	if len(spec.Install.Target) > 0 {
		target = ""
		if err := json.Unmarshal(spec.Install.Target, &target); err != nil || target == "" {
			return spec, true, errors.New("旧Forge安装目标ID无效")
		}
	}
	if !legacyForgeID(request, version.ID) {
		return spec, true, errors.New("旧Forge客户端版本ID与固定构建不匹配")
	}
	if target != version.ID {
		return spec, true, errors.New("旧Forge安装目标与客户端版本ID不匹配")
	}
	if version.InheritsFrom != base.ID {
		return spec, true, errors.New("旧Forge客户端父版本与官方基底不匹配")
	}
	if version.MainClass != legacyForgeMain {
		return spec, true, errors.New("旧Forge客户端主类不受支持")
	}
	if version.Arguments != nil || version.LegacyArguments == "" || len(version.Patches) != 0 || len(version.Downloads) != 0 || version.AssetIndex.ID != "" || version.Java.Major != 0 && version.Java.Major != 8 {
		return spec, true, errors.New("旧Forge客户端参数或运行字段不受支持")
	}
	if version.Type != "" && version.Type != "release" || version.Jar != "" && version.Jar != base.ID || version.Assets != "" && version.Assets != base.Assets || spec.Install.Minecraft != "" && spec.Install.Minecraft != base.ID {
		return spec, true, errors.New("旧Forge试图替换官方客户端或资源身份")
	}
	if !legacyForgeCore(request, spec.Install.Path) || !archive.SafePath(spec.Install.FilePath) || !z.Has(spec.Install.FilePath) {
		return spec, true, errors.New("旧Forge内嵌Universal路径或版本坐标无效")
	}
	var details struct{ Libraries []legacyForgeLibrary }
	if err := json.Unmarshal(spec.VersionInfo, &details); err != nil {
		return spec, true, err
	}
	if len(details.Libraries) == 0 || len(details.Libraries) > 128 {
		return spec, true, errors.New("旧Forge运行库数量无效")
	}
	core, wrapper := 0, 0
	for _, lib := range details.Libraries {
		if len(lib.Checksums) > 16 {
			return spec, true, errors.New("旧Forge依赖checksum数量超过边界")
		}
		for _, checksum := range lib.Checksums {
			if !meta.ValidSHA1(checksum) {
				return spec, true, errors.New("旧Forge依赖checksum声明无效")
			}
		}
		if err := validateLegacyRuntimeLibrary(request, lib.Library, false); err != nil {
			return spec, true, err
		}
		if legacyForgeWrapper(request, lib.Name) {
			if len(lib.Rules) != 0 || lib.ClientReq != nil && !*lib.ClientReq {
				return spec, true, errors.New("旧Forge LaunchWrapper入口不能省略")
			}
			wrapper++
		}
		if lib.Name == spec.Install.Path {
			if len(lib.Rules) != 0 || lib.ClientReq != nil && !*lib.ClientReq {
				return spec, true, errors.New("旧Forge Universal不能按平台条件省略")
			}
			core++
		}
		if len(lib.Natives) != 0 || len(lib.Downloads.Classifiers) != 0 {
			return spec, true, errors.New("旧Forge加载器库不能替换官方native分类")
		}
	}
	if core != 1 || wrapper != 1 {
		return spec, true, errors.New("旧Forge运行库未唯一引用内嵌Universal和已支持LaunchWrapper")
	}
	args, err := legacyForgeArguments(request, version.LegacyArguments)
	if err != nil {
		return spec, true, err
	}
	version.Arguments = &args
	version.LegacyArguments = ""
	version.Java.Major = 8
	spec.version, spec.libraries = version, details.Libraries
	return spec, true, nil
}

// Exact historical public repository roots may upgrade to canonical HTTPS.
// Coordinates still require fixed digests and sizes before downloading. Never
// rewrite arbitrary artifact URLs, paths, credentials, queries or private hosts.
func legacyForgeLibraryRoot(value string) (string, error) {
	switch strings.TrimRight(value, "/") {
	case "", "http://libraries.minecraft.net", "https://libraries.minecraft.net":
		return "https://libraries.minecraft.net", nil
	case "http://files.minecraftforge.net/maven", "https://files.minecraftforge.net/maven", "https://maven.minecraftforge.net":
		return "https://maven.minecraftforge.net", nil
	case "http://repo1.maven.org/maven2", "https://repo1.maven.org/maven2", "http://repo.maven.apache.org/maven2", "https://repo.maven.apache.org/maven2":
		return "https://repo.maven.apache.org/maven2", nil
	default:
		return "", errors.New("旧Forge依赖必须使用明确的官方或公共Maven根")
	}
}

func (s *ForgeInstaller) prepareLegacyForge(ctx context.Context, z *archive.Zip, request Request, base meta.Version, spec legacyForgeSpec, stage *os.Root, generation, installerSHA1 string, report func(download.Progress)) (Profile, error) {
	profile := Profile{Request: request, ID: spec.version.ID, MainClass: legacyForgeMain, JavaMajor: 8, Arguments: *spec.version.Arguments, Generation: generation, InstallerSHA1: installerSHA1, Legacy: true}
	cache, err := os.OpenRoot(s.downloads.Root)
	if err != nil {
		return Profile{}, err
	}
	defer cache.Close()
	seen := make(map[string]bool)
	pins := make(map[string][]string)
	var inputs []download.File
	var mappings []archive.Mapping
	var total int64
	for _, lib := range spec.libraries {
		if err := ctx.Err(); err != nil {
			return Profile{}, err
		}
		if lib.ClientReq != nil && !*lib.ClientReq {
			if lib.Name == spec.Install.Path {
				return Profile{}, errors.New("旧Forge Universal被声明为服务端专用")
			}
			continue
		}
		coordinate, err := meta.ParseCoordinate(lib.Name)
		if err != nil {
			return Profile{}, err
		}
		if coordinate.Extension != "jar" {
			return Profile{}, errors.New("旧Forge依赖不是JAR")
		}
		relative := coordinate.Path()
		key := archive.Key(relative)
		if seen[key] {
			return Profile{}, errors.New("旧Forge依赖重复或路径别名冲突")
		}
		seen[key] = true
		pins[lib.Name] = lib.Checksums
		if _, err := legacyForgeLibraryRoot(lib.URL); err != nil {
			return Profile{}, err
		}
		library := meta.Library{Name: lib.Name, Rules: lib.Rules}
		if lib.Name == spec.Install.Path {
			mappings = append(mappings, archive.Mapping{Source: spec.Install.FilePath, Target: path.Join("libraries", relative)})
			identity := meta.Download{Path: relative}
			if declared := lib.Downloads.Artifact; declared != nil {
				if err := declared.ValidateIdentity(); err != nil {
					return Profile{}, err
				}
				if declared.Path != "" && declared.Path != relative {
					return Profile{}, errors.New("内嵌Universal声明路径不匹配")
				}
				identity.SHA1, identity.Size = declared.SHA1, declared.Size
			}
			library.Downloads.Artifact = &identity
		} else {
			file, err := s.legacyForgeLibraryIdentity(ctx, lib, base)
			if err != nil {
				return Profile{}, fmt.Errorf("固定旧Forge依赖 %s: %w", lib.Name, err)
			}
			if file.Size > 512<<20 {
				return Profile{}, errors.New("旧Forge单项依赖超过512MiB")
			}
			total += file.Size
			if total > 4<<30 {
				return Profile{}, errors.New("旧Forge依赖超过4GiB")
			}
			library.Downloads.Artifact = &file
			inputs = append(inputs, forgeInput(file))
		}
		profile.Libraries = append(profile.Libraries, library)
	}
	if err := s.downloads.Run(ctx, inputs, report); err != nil {
		return Profile{}, err
	}
	if err := z.Extract(ctx, stage, mappings); err != nil {
		return Profile{}, err
	}
	for i := range profile.Libraries {
		lib := &profile.Libraries[i]
		fixed := *lib.Downloads.Artifact
		target := path.Join("libraries", fixed.Path)
		if lib.Name != spec.Install.Path {
			if err := copyForgeInput(ctx, cache, forgeInput(fixed), stage, target); err != nil {
				return Profile{}, err
			}
		}
		identity, err := hashForgeFile(ctx, stage, target, fixed.SHA1, fixed.Size)
		if err != nil {
			return Profile{}, err
		}
		if declared := pins[lib.Name]; len(declared) > 0 {
			matched := false
			for _, pin := range declared {
				matched = matched || strings.EqualFold(pin, identity.SHA1)
			}
			if !matched {
				return Profile{}, errors.New("旧Forge依赖与安装清单checksum不一致")
			}
		}
		identity.Path = fixed.Path
		lib.Downloads.Artifact = &identity
		if lib.Name == spec.Install.Path || legacyForgeWrapper(request, lib.Name) {
			tweaker, _ := legacyForgeTweakerFor(request)
			entry := strings.ReplaceAll(tweaker, ".", "/") + ".class"
			if legacyForgeWrapper(request, lib.Name) {
				entry = "net/minecraft/launchwrapper/Launch.class"
			}
			f, err := stage.Open(target)
			if err != nil {
				return Profile{}, err
			}
			contents, err := archive.Open(f, identity.Size)
			if err == nil && !contents.Has(entry) {
				err = errors.New("旧Forge运行库不包含已固定的客户端入口")
			}
			f.Close()
			if err != nil {
				return Profile{}, err
			}
		}
	}
	if err := profile.Validate(); err != nil {
		return Profile{}, err
	}
	if _, err := profile.Merge(base); err != nil {
		return Profile{}, err
	}
	return profile, ctx.Err()
}

func (s *ForgeInstaller) legacyForgeLibraryIdentity(ctx context.Context, lib legacyForgeLibrary, base meta.Version) (meta.Download, error) {
	coordinate, err := meta.ParseCoordinate(lib.Name)
	if err != nil {
		return meta.Download{}, err
	}
	relative := coordinate.Path()
	if declared := lib.Downloads.Artifact; declared != nil {
		fixed := *declared
		if fixed.Path == "" {
			fixed.Path = relative
		}
		if fixed.Path != relative || !AllowedLibraryURL(fixed.URL) {
			return meta.Download{}, errors.New("旧Forge固定依赖来源或路径无效")
		}
		return fixed, fixed.Validate()
	}
	var candidates []meta.Download
	for _, library := range base.Libraries {
		if library.Name == lib.Name && library.Downloads.Artifact != nil {
			candidates = append(candidates, *library.Downloads.Artifact)
		}
	}
	if len(candidates) > 0 {
		fixed := candidates[0]
		if fixed.Path == "" {
			fixed.Path = relative
		}
		for _, candidate := range candidates[1:] {
			if candidate.SHA1 != fixed.SHA1 || candidate.Size != fixed.Size {
				return meta.Download{}, errors.New("官方客户端同名依赖有平台差异，不能猜测")
			}
		}
		if fixed.Path != relative || !AllowedLibraryURL(fixed.URL) {
			return meta.Download{}, errors.New("官方依赖路径或来源不匹配")
		}
		return fixed, fixed.Validate()
	}
	root, err := legacyForgeLibraryRoot(lib.URL)
	if err != nil {
		return meta.Download{}, err
	}
	return s.catalog.mavenMetadata(ctx, root+"/"+relative, relative)
}
