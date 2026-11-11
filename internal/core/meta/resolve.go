package meta

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/url"
	"slices"
	"strings"
)

const maxInheritanceDepth = 8
const maxComposedLibraries = 4096
const maxComposedArguments = 2048

// Version preserves the raw SHA-keyed cache. Normalization/composition is only
// in memory, after identity verification; a transformed document must never be
// cached under the digest of its original bytes.
func (c *Client) Version(ctx context.Context, entry ManifestEntry) (Version, error) {
	return c.resolve(ctx, entry, nil, 0, nil)
}

func officialPackage(entry ManifestEntry) bool {
	u, err := url.Parse(entry.URL)
	if err != nil || u.Scheme != "https" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return false
	}
	if u.Host != "piston-meta.mojang.com" && u.Host != "launchermeta.mojang.com" {
		return false
	}
	return ValidID(entry.ID) && ValidSHA1(entry.SHA1) && u.EscapedPath() == "/v1/packages/"+strings.ToLower(entry.SHA1)+"/"+entry.ID+".json"
}

func (c *Client) resolve(ctx context.Context, entry ManifestEntry, chain []string, depth int, manifest *Manifest) (Version, error) {
	if err := ctx.Err(); err != nil {
		return Version{}, err
	}
	if depth >= maxInheritanceDepth {
		return Version{}, errors.New("版本继承超过8层")
	}
	if slices.Contains(chain, entry.ID) {
		return Version{}, errors.New("版本继承存在循环")
	}
	v, err := c.versionDocument(ctx, entry)
	if err != nil {
		return Version{}, err
	}
	if !officialPackage(entry) {
		if v.InheritsFrom != "" || v.LegacyArguments != "" || v.Java.Major == 0 {
			return Version{}, errors.New("旧版或继承元数据必须来自可核验的 Mojang 官方固定摘要文档")
		}
		return v, nil
	}
	if len(v.Patches) != 0 {
		return Version{}, errors.New("不接受第三方 patches；加载器必须经各自已支持的官方配置链路")
	}
	if v.Type != "" && v.Type != entry.Type {
		return Version{}, errors.New("版本类型与官方清单不一致")
	}
	v.Type = entry.Type
	if v.InheritsFrom != "" {
		if manifest == nil {
			catalog, err := c.Manifest(ctx, false)
			if err != nil {
				return Version{}, fmt.Errorf("读取继承父版本清单: %w", err)
			}
			manifest = &catalog.Manifest
		}
		parentEntry, ok := manifest.Find(v.InheritsFrom)
		if !ok || !officialPackage(parentEntry) {
			return Version{}, errors.New("继承父版本不在可核验的官方清单中")
		}
		parent, err := c.resolve(ctx, parentEntry, append(slices.Clone(chain), entry.ID), depth+1, manifest)
		if err != nil {
			return Version{}, fmt.Errorf("读取继承父版本 %s: %w", parentEntry.ID, err)
		}
		v, err = mergeOfficial(parent, v)
		if err != nil {
			return Version{}, err
		}
	}
	v, err = normalizeOfficial(v)
	if err != nil {
		return Version{}, err
	}
	v.Sources = append(slices.Clone(v.Sources), entry)
	return v, nil
}

func normalizeOfficial(v Version) (Version, error) {
	if err := validTemplatePresence(v); err != nil {
		return Version{}, err
	}
	if v.Java.Major == 0 {
		if v.has("javaVersion") || v.Type != "release" || !historicalJava8Release(v.ID) {
			return Version{}, errors.New("此官方版本缺少可追溯的 Java 要求，当前历史兼容表未覆盖")
		}
		v.Java = JavaVersion{Major: 8, Component: "jre-legacy"}
	}
	if v.Jar != "" && v.Jar != v.ID && v.Jar != v.ClientVersion {
		return Version{}, errors.New("客户端 JAR 引用不属于已核验的继承链")
	}
	v.Jar = ""
	if v.Arguments != nil && v.LegacyArguments != "" {
		return Version{}, errors.New("版本同时包含新旧参数模板，不能猜测优先级")
	}
	if v.LegacyArguments != "" {
		game, err := SplitLegacyArguments(v.LegacyArguments)
		if err != nil {
			return Version{}, err
		}
		args := &Arguments{JVM: legacyJVMArguments()}
		for _, value := range game {
			args.Game = append(args.Game, Argument{Values: []string{value}})
		}
		v.Arguments, v.LegacyArguments = args, ""
	}
	if v.Arguments == nil {
		return Version{}, errors.New("官方版本缺少可用启动参数")
	}
	if err := compositionBounds(v); err != nil {
		return Version{}, err
	}
	v.InheritsFrom = ""
	if err := v.ValidateVanilla(); err != nil {
		return Version{}, err
	}
	return v, nil
}

// Child module groups replace parent groups, preserving ALL conditional
// variants and their ordered rules inside each chosen group. Arguments append
// in parent->child order; a legacy child replaces only the game template.
func mergeOfficial(parent, child Version) (Version, error) {
	if err := validTemplatePresence(child); err != nil {
		return Version{}, err
	}
	if len(child.Libraries) > maxComposedLibraries {
		return Version{}, errors.New("继承文档运行库超过4096项")
	}
	if child.InheritsFrom != parent.ID {
		return Version{}, errors.New("继承父文档身份不匹配")
	}
	if child.Arguments != nil && child.LegacyArguments != "" {
		return Version{}, errors.New("继承文档包含冲突的新旧参数模板")
	}
	if len(child.Patches) != 0 {
		return Version{}, errors.New("继承文档包含未支持的第三方补丁")
	}
	result := cloneVersion(parent)
	result.ID, result.Type, result.InheritsFrom = child.ID, child.Type, ""
	result.fields = maps.Clone(child.fields)
	result.Sources = slices.Clone(parent.Sources)
	if child.has("mainClass") {
		result.MainClass = child.MainClass
	}
	if child.has("assetIndex") {
		result.AssetIndex = child.AssetIndex
	}
	if child.has("assets") {
		result.Assets = child.Assets
	}
	if child.has("logging") {
		result.Logging = cloneVersion(child).Logging
	}
	result.MinimumLauncher = max(parent.MinimumLauncher, child.MinimumLauncher)
	result.ComplianceLevel = max(parent.ComplianceLevel, child.ComplianceLevel)
	if child.has("javaVersion") {
		if child.Java.Major <= 0 {
			return Version{}, errors.New("继承文档的 Java 要求无效")
		}
		if child.Java.Major >= parent.Java.Major {
			result.Java = child.Java
		}
	}
	result.ClientVersion = parent.ClientVersion
	if result.ClientVersion == "" {
		result.ClientVersion = parent.ID
	}
	if result.Downloads == nil {
		result.Downloads = make(map[string]Download)
	}
	for name, file := range child.Downloads {
		result.Downloads[name] = file
		if name == "client" {
			result.ClientVersion = child.ID
		}
	}
	result.Jar = child.Jar
	if result.Jar != "" && result.Jar != result.ClientVersion {
		return Version{}, errors.New("继承文档引用了其他客户端 JAR")
	}
	child = cloneVersion(child)
	overrides := make(map[string]bool)
	for _, library := range child.Libraries {
		key, err := libraryModule(library.Name)
		if err != nil {
			return Version{}, err
		}
		overrides[key] = true
	}
	result.Libraries = child.Libraries
	for _, library := range cloneVersion(parent).Libraries {
		key, err := libraryModule(library.Name)
		if err != nil {
			return Version{}, err
		}
		if !overrides[key] {
			result.Libraries = append(result.Libraries, library)
		}
	}
	if child.LegacyArguments != "" {
		game, err := SplitLegacyArguments(child.LegacyArguments)
		if err != nil {
			return Version{}, err
		}
		result.Arguments.Game = nil
		for _, value := range game {
			result.Arguments.Game = append(result.Arguments.Game, Argument{Values: []string{value}})
		}
	} else if child.Arguments != nil {
		result.Arguments.Game = append(result.Arguments.Game, child.Arguments.Game...)
		result.Arguments.JVM = append(result.Arguments.JVM, child.Arguments.JVM...)
	}
	result.LegacyArguments = ""
	if err := compositionBounds(result); err != nil {
		return Version{}, err
	}
	return result, nil
}

func libraryModule(name string) (string, error) {
	c, err := ParseCoordinate(name)
	if err != nil {
		return "", err
	}
	return c.Module(), nil
}

func (v Version) has(field string) bool { _, ok := v.fields[field]; return ok }

func cloneRules(rules []Rule) []Rule {
	out := slices.Clone(rules)
	for i := range out {
		out[i].Features = maps.Clone(out[i].Features)
		if out[i].OS != nil {
			value := *out[i].OS
			out[i].OS = &value
		}
	}
	return out
}
func cloneArguments(arguments []Argument) []Argument {
	out := slices.Clone(arguments)
	for i := range out {
		out[i].Values = slices.Clone(out[i].Values)
		out[i].Rules = cloneRules(out[i].Rules)
	}
	return out
}

// Clone returns an independent snapshot, including nested arguments, rules,
// download identities and native maps. Derived runtime fields remain in memory.
func (v Version) Clone() Version { return cloneVersion(v) }

// Clone snapshots argument values and nested rule conditions.
func (a Arguments) Clone() Arguments {
	return Arguments{Game: cloneArguments(a.Game), JVM: cloneArguments(a.JVM)}
}

// Clone snapshots fixed file identities, native maps and conditional rules.
func (lib Library) Clone() Library {
	lib.Natives = maps.Clone(lib.Natives)
	lib.Rules = cloneRules(lib.Rules)
	lib.Extract.Exclude = slices.Clone(lib.Extract.Exclude)
	lib.Downloads.Classifiers = maps.Clone(lib.Downloads.Classifiers)
	if lib.Downloads.Artifact != nil {
		value := *lib.Downloads.Artifact
		lib.Downloads.Artifact = &value
	}
	return lib
}

func cloneVersion(v Version) Version {
	v.fields = maps.Clone(v.fields)
	for key, raw := range v.fields {
		v.fields[key] = slices.Clone(raw)
	}
	v.Patches = slices.Clone(v.Patches)
	for i := range v.Patches {
		v.Patches[i] = slices.Clone(v.Patches[i])
	}
	v.Sources = slices.Clone(v.Sources)
	v.RuntimeLookups = slices.Clone(v.RuntimeLookups)
	v.Downloads = maps.Clone(v.Downloads)
	if v.Arguments != nil {
		args := v.Arguments.Clone()
		v.Arguments = &args
	}
	if v.Logging.Client != nil {
		logging := *v.Logging.Client
		v.Logging.Client = &logging
	}
	v.Libraries = slices.Clone(v.Libraries)
	for i := range v.Libraries {
		v.Libraries[i] = v.Libraries[i].Clone()
	}
	return v
}

func validTemplatePresence(v Version) error {
	if v.has("arguments") && v.Arguments == nil {
		return errors.New("版本显式 arguments 为空，不能悄悄沿用父模板")
	}
	if v.has("minecraftArguments") && v.LegacyArguments == "" && v.Arguments == nil {
		return errors.New("版本显式旧参数模板为空")
	}
	return nil
}

func compositionBounds(v Version) error {
	if v.Arguments == nil || len(v.Libraries) > maxComposedLibraries {
		return errors.New("合并后的运行库或参数超过边界")
	}
	count, size := 0, 0
	for _, group := range [][]Argument{v.Arguments.JVM, v.Arguments.Game} {
		for _, argument := range group {
			count += len(argument.Values)
			for _, value := range argument.Values {
				size += len(value)
			}
		}
	}
	if count > maxComposedArguments || size > 256<<10 {
		return errors.New("合并后的参数超过2048项或256KiB")
	}
	return nil
}
