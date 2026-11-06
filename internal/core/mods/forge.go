package mods

import (
	"errors"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

func forge(data []byte, loader, jarVersion string) (parsedVariant, error) {
	result := parsedVariant{Variant: Variant{Loader: loader}}
	var raw struct {
		ModLoader, LoaderVersion string
		Mods                     []struct {
			ModID, DisplayName, Version string
			Features                    map[string]string
		}
		Dependencies map[string][]struct {
			ModID, VersionRange, Side, Type string
			Mandatory                       *bool
		}
	}
	if err := toml.Unmarshal(data, &raw); err != nil {
		return result, errors.New("Forge/NeoForge 声明不是有效 TOML")
	}
	if len(raw.Mods) == 0 || len(raw.Mods) > 128 {
		return result, errors.New("Forge/NeoForge 模组数量无效")
	}
	for _, entry := range raw.Mods {
		mod := Mod{ID: entry.ModID, Name: entry.DisplayName, Version: strings.ReplaceAll(entry.Version, "${file.jarVersion}", jarVersion)}
		if len(entry.Features) > 16 {
			return result, errors.New("模组运行特性过多")
		}
		for feature, constraint := range entry.Features {
			if feature == "javaVersion" {
				mod.Requires = append(mod.Requires, Requirement{ID: "java", Versions: Versions{Text: constraint}, Syntax: Maven})
			} else {
				result.Incomplete = true
				if len(result.Problems) < 8 {
					result.Problems = append(result.Problems, "运行特性尚未检查："+feature)
				}
			}
		}
		if raw.ModLoader != "" {
			mod.Requires = append(mod.Requires, Requirement{ID: raw.ModLoader, Versions: Versions{Text: raw.LoaderVersion}, Syntax: Maven})
		}
		dependencies := raw.Dependencies[entry.ModID]
		if len(dependencies) > 128 {
			return result, errors.New("Forge/NeoForge 依赖过多")
		}
		for _, dependency := range dependencies {
			if dependency.ModID == "neoforge" && loader == "forge" {
				result.Loader = "neoforge"
			}
			rule := Requirement{ID: dependency.ModID, Versions: Versions{Text: dependency.VersionRange}, Syntax: Maven}
			if dependency.Mandatory != nil {
				rule.Optional = !*dependency.Mandatory
			}
			switch strings.ToLower(dependency.Type) {
			case "", "required":
			case "optional":
				rule.Optional = true
			case "incompatible":
				rule.Incompatible = true
			case "discouraged":
				rule.Incompatible, rule.Warning = true, true
			default:
				return result, errors.New("未知 Forge/NeoForge 依赖类型")
			}
			switch strings.ToUpper(dependency.Side) {
			case "", "BOTH", "CLIENT":
			case "SERVER":
				rule.ServerOnly = true
			default:
				return result, errors.New("未知 Forge/NeoForge 依赖环境")
			}
			mod.Requires = append(mod.Requires, rule)
		}
		result.Mods = append(result.Mods, mod)
	}
	return result, validateParsed(result)
}

// A manifest continuation starts with one space. Only main attributes count;
// named sections cannot override the file's Implementation-Version.
func manifestVersion(data []byte) string {
	var lines []string
	for _, line := range strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n") {
		if line == "" {
			break
		}
		if strings.HasPrefix(line, " ") && len(lines) > 0 {
			lines[len(lines)-1] += line[1:]
		} else {
			lines = append(lines, line)
		}
	}
	for _, line := range lines {
		key, value, ok := strings.Cut(line, ":")
		if ok && strings.EqualFold(key, "Implementation-Version") {
			return strings.TrimSpace(value)
		}
	}
	return "${file.jarVersion}"
}
