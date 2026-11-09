// Package loader resolves explicit mod-loader requirements into verified
// libraries and a native launcher profile.
package loader

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"slices"
	"strings"

	"mcl/internal/core/meta"
)

type Request struct {
	Type, Version, GameVersion, BaseSHA1 string
	BaseChainSHA256                      string `json:",omitempty"`
	TargetOS                             string `json:",omitempty"`
	TargetArch                           string `json:",omitempty"`
}

func (r Request) WithPlatform(target meta.Platform) (Request, error) {
	if UsesProcessors(r.Type) {
		r.TargetOS = target.OS
		r.TargetArch = meta.NormalizeArch(target.Arch)
	}
	return r, r.Validate()
}

func (r Request) WithBase(base meta.Version) (Request, error) {
	if base.ID != r.GameVersion {
		return r, errors.New("加载器基底版本身份不匹配")
	}
	digest, err := meta.OfficialChainDigest(base, r.BaseSHA1)
	if err != nil {
		return r, err
	}
	r.BaseChainSHA256 = digest
	return r, r.Validate()
}
func (r Request) ValidateBase(base meta.Version) error {
	expected, err := r.WithBase(base)
	if err != nil {
		return err
	}
	if expected != r {
		return errors.New("加载器缓存未绑定完整官方继承链")
	}
	return nil
}

func (r Request) Validate() error {
	if !Supported(r.Type) {
		return fmt.Errorf("%s 加载器安装处理器尚未接入", r.Type)
	}
	if !meta.ValidID(r.Version) || strings.ContainsAny(r.Version, " /\\:\r\n") || !meta.ValidID(r.GameVersion) || !meta.ValidSHA1(r.BaseSHA1) {
		return errors.New("加载器或游戏版本身份无效")
	}
	if r.TargetOS != "" || r.TargetArch != "" {
		if (r.TargetOS != "osx" && r.TargetOS != "windows" && r.TargetOS != "linux") || (r.TargetArch != "amd64" && r.TargetArch != "arm64" && r.TargetArch != "386") {
			return errors.New("加载器目标平台身份无效")
		}
	}
	if r.BaseChainSHA256 != "" {
		decoded, err := hex.DecodeString(r.BaseChainSHA256)
		if err != nil || len(decoded) != 32 {
			return errors.New("加载器官方继承链摘要无效")
		}
	}
	return nil
}

func Supported(kind string) bool {
	return kind == "fabric" || kind == "quilt" || UsesProcessors(kind)
}

func UsesProcessors(kind string) bool { return kind == "forge" || kind == "neoforge" }
func (r Request) Key() string {
	data, _ := json.Marshal(r)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

type Profile struct {
	Request       Request
	ID, MainClass string
	JavaMajor     int
	Libraries     []meta.Library
	Arguments     meta.Arguments
	Generation    string          `json:",omitempty"`
	InstallerSHA1 string          `json:",omitempty"`
	Legacy        bool            `json:",omitempty"`
	LegacyGame    bool            `json:",omitempty"`
	Outputs       []meta.Download `json:",omitempty"`
}

func (p Profile) LibraryDirectory() string {
	return path.Join("cache/loader-runtimes", p.Request.Key(), p.Generation, "libraries")
}

func (p Profile) Fingerprint() string {
	data, _ := json.Marshal(p)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
func (p Profile) Validate() error {
	if err := p.Request.Validate(); err != nil {
		return err
	}
	prepared := UsesProcessors(p.Request.Type)
	limit := 128
	if prepared {
		limit = 256
	}
	if !meta.ValidID(p.ID) || p.JavaMajor < 0 || p.JavaMajor > 100 || len(p.Libraries) == 0 || len(p.Libraries) > limit {
		return errors.New("加载器配置字段或依赖数量无效")
	}
	if p.Request.Type == "fabric" && p.MainClass != "net.fabricmc.loader.impl.launch.knot.KnotClient" && p.MainClass != "net.fabricmc.loader.launch.knot.KnotClient" {
		return errors.New("Fabric 客户端主类不受支持")
	}
	if p.Request.Type == "quilt" && p.MainClass != "org.quiltmc.loader.impl.launch.knot.KnotClient" {
		return errors.New("Quilt 客户端主类不受支持")
	}
	if p.Legacy {
		if err := validateLegacyForgeProfile(p); err != nil {
			return err
		}
	}
	if p.LegacyGame {
		if err := validateProcessedLegacyGame(p); err != nil {
			return err
		}
	}
	if prepared {
		_, _, versionID, err := forgeInstaller(p.Request)
		if err != nil {
			return err
		}
		validIdentity := p.ID == versionID && forgeMainClass(p.MainClass)
		if p.Legacy {
			validIdentity = legacyForgeID(p.Request, p.ID) && p.MainClass == legacyForgeMain
		}
		if p.LegacyGame {
			validIdentity = legacyForgeID(p.Request, p.ID) && p.MainClass == legacyForgeMain
		}
		if !validIdentity || !validGeneration(p.Generation) || !meta.ValidSHA1(p.InstallerSHA1) || len(p.Outputs) > 4096 {
			return errors.New("Forge 系加载器运行环境信息无效")
		}
	} else if p.Generation != "" || p.InstallerSHA1 != "" || len(p.Outputs) != 0 || p.Legacy || p.LegacyGame {
		return errors.New("加载器包含非预期的生成文件")
	}
	found := p.Legacy || p.LegacyGame
	seen := make(map[string]meta.Library)
	for _, library := range p.Libraries {
		coordinate, err := meta.ParseCoordinate(library.Name)
		if err != nil {
			return err
		}
		if previous, duplicate := seen[library.Name]; duplicate {
			a, b := previous.Downloads.Artifact, library.Downloads.Artifact
			if len(previous.Rules) == 0 || len(library.Rules) == 0 || a == nil || b == nil || *a != *b {
				return errors.New("加载器重复库未绑定同一文件或缺少条件规则")
			}
		}
		if len(library.Natives) > 0 || len(library.Downloads.Classifiers) > 0 || coordinate.Extension != "jar" {
			return errors.New("加载器库包含不支持的本地分类")
		}
		seen[library.Name] = library
		if err := meta.ValidateRules(library.Rules); err != nil {
			return err
		}
		artifact := library.Downloads.Artifact
		if artifact == nil {
			return errors.New("加载器库缺少摘要或大小")
		}
		if err := artifact.ValidateIdentity(); err != nil {
			return err
		}
		if artifact.Path != coordinate.Path() || (prepared && artifact.URL != "") || (!prepared && !AllowedLibraryURL(artifact.URL)) {
			return errors.New("加载器库路径或来源无效")
		}
		if prepared && modernRuntimeCore(p.Request, coordinate) {
			found = true
		}
		if coordinate.Artifact == p.Request.Type+"-loader" && coordinate.Version == p.Request.Version {
			expected := "net.fabricmc"
			if p.Request.Type == "quilt" {
				expected = "org.quiltmc"
			}
			found = found || coordinate.Group == expected
		}
	}
	for _, output := range p.Outputs {
		if err := output.ValidateIdentity(); err != nil {
			return err
		}
		if output.Path == "" || output.URL != "" {
			return errors.New("生成文件身份无效")
		}
	}
	if !found {
		return errors.New("加载器配置没有包含所选版本的核心库")
	}
	for _, arguments := range [][]meta.Argument{p.Arguments.JVM, p.Arguments.Game} {
		if len(arguments) > 128 {
			return errors.New("加载器启动参数过多")
		}
		for _, argument := range arguments {
			if err := meta.ValidateRules(argument.Rules); err != nil {
				return err
			}
			for _, value := range argument.Values {
				if len(value) > 8192 || strings.ContainsAny(value, "\x00\r\n") || strings.HasPrefix(value, "@") || strings.Contains(value, "-javaagent:") || strings.Contains(value, "-agentlib:") || strings.Contains(value, "-agentpath:") {
					return errors.New("加载器配置包含不支持的注入或参数文件")
				}
			}
		}
	}
	return nil
}

// Merge keeps official client/assets/natives and JVM defaults. Loader libraries
// take precedence for an identical Maven module (including classifier), so the
// classpath never contains two versions of one overridden module.
func (p Profile) Merge(base meta.Version) (meta.Version, error) {
	if err := p.Validate(); err != nil {
		return meta.Version{}, err
	}
	if err := p.Request.ValidateBase(base); err != nil {
		return meta.Version{}, err
	}
	if err := base.ValidateVanilla(); err != nil {
		return meta.Version{}, err
	}
	effective := base
	effective.MainClass = p.MainClass
	if UsesProcessors(p.Request.Type) {
		effective.LibraryDirectory, effective.LaunchName = p.LibraryDirectory(), p.ID
	}
	effective.Java.Major = max(base.Java.Major, p.JavaMajor)
	args := *base.Arguments
	args.Game = append(slices.Clone(args.Game), p.Arguments.Game...)
	if p.Legacy || p.LegacyGame {
		if base.Java.Major != 8 {
			return meta.Version{}, errors.New("旧 Forge 配置要求稳定 Java 8 的官方客户端")
		}
		// The old profile owns a complete game template, not incremental flags.
		args.Game = slices.Clone(p.Arguments.Game)
	}
	args.JVM = append(slices.Clone(args.JVM), p.Arguments.JVM...)
	if UsesProcessors(p.Request.Type) {
		for index, argument := range args.JVM {
			for _, value := range argument.Values {
				if strings.HasPrefix(value, "-DignoreList=") && strings.Contains(value, "${version_name}.jar") {
					args.JVM[index].Values = slices.Clone(argument.Values)
					for n, text := range argument.Values {
						args.JVM[index].Values[n] = strings.ReplaceAll(text, "${version_name}.jar", "${primary_jar_name}")
					}
					break
				}
			}
		}
	}
	effective.Arguments = &args
	module := func(name string) string {
		coordinate, _ := meta.ParseCoordinate(name)
		return coordinate.Group + ":" + coordinate.Artifact + ":" + coordinate.Classifier + "@" + coordinate.Extension
	}
	seen := make(map[string]string)
	effective.Libraries = nil
	for _, library := range p.Libraries {
		key := module(library.Name)
		if previous, exists := seen[key]; exists && previous != library.Name {
			return meta.Version{}, errors.New("加载器包含同一模块的多个版本")
		}
		seen[key] = library.Name
		library.LocalRoot = effective.LibraryDirectory
		effective.Libraries = append(effective.Libraries, library)
	}
	for _, library := range base.Libraries {
		if _, overridden := seen[module(library.Name)]; !overridden {
			effective.Libraries = append(effective.Libraries, library)
		}
	}
	if err := effective.ValidateVanilla(); err != nil {
		return meta.Version{}, err
	}
	return effective, nil
}
