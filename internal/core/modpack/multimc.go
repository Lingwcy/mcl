package modpack

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"strings"

	"mcl/internal/core/archive"
	"mcl/internal/core/meta"
)

// RuntimeComponent pins a component which must agree with the official client
// metadata. Arbitrary MultiMC patches, libraries or executable hooks are never
// treated as a vanilla instance merely because Minecraft is also listed.
type RuntimeComponent struct{ UID, Version string }

func multiMCCandidates(z *archive.Zip) []string {
	var candidates []string
	for _, entry := range z.Entries() {
		if entry.Directory || path.Base(entry.Name) != "mmc-pack.json" {
			continue
		}
		prefix := strings.TrimSuffix(entry.Name, "mmc-pack.json")
		if strings.Count(strings.TrimSuffix(prefix, "/"), "/") > 1 {
			continue
		}
		candidates = append(candidates, prefix)
	}
	return candidates
}

type multiMCComponent struct {
	UID, Version  string
	CachedVersion string `json:"cachedVersion"`
}

func readMultiMC(ctx context.Context, z *archive.Zip, prefix string) (Manifest, []string, error) {
	data, err := z.Read(ctx, prefix+"mmc-pack.json", 1<<20)
	if err != nil {
		return Manifest{}, nil, err
	}
	var raw struct {
		FormatVersion int                `json:"formatVersion"`
		Components    []multiMCComponent `json:"components"`
	}
	if err := json.Unmarshal(data, &raw); err != nil || raw.FormatVersion != 1 || len(raw.Components) == 0 || len(raw.Components) > 64 {
		return Manifest{}, nil, errors.New("不支持或无效的 MultiMC / Prism 清单")
	}
	cfg, err := z.Read(ctx, prefix+"instance.cfg", 256<<10)
	if err != nil {
		return Manifest{}, nil, errors.New("MultiMC / Prism 包缺少有效的 instance.cfg")
	}
	name, err := multiMCName(string(cfg))
	if err != nil {
		return Manifest{}, nil, err
	}
	m := Manifest{Format: "multimc", Name: name}
	seen := map[string]bool{}
	for _, component := range raw.Components {
		if seen[component.UID] {
			return m, nil, errors.New("MultiMC 包包含重复运行组件")
		}
		seen[component.UID] = true
		version := component.Version
		if version == "" {
			version = component.CachedVersion
		}
		if !meta.ValidID(version) {
			return m, nil, errors.New("MultiMC 组件没有有效的固定版本")
		}
		switch component.UID {
		case "net.minecraft":
			m.Minecraft = version
		case "net.fabricmc.fabric-loader":
			m.Loaders = append(m.Loaders, Loader{Type: "fabric", Version: version})
		case "org.quiltmc.quilt-loader":
			m.Loaders = append(m.Loaders, Loader{Type: "quilt", Version: version})
		case "net.minecraftforge":
			m.Loaders = append(m.Loaders, Loader{Type: "forge", Version: strings.TrimPrefix(version, multiMCMinecraft(raw.Components)+"-")})
		case "net.neoforged":
			m.Loaders = append(m.Loaders, Loader{Type: "neoforge", Version: version})
		case "org.lwjgl", "org.lwjgl3":
			m.RuntimeComponents = append(m.RuntimeComponents, RuntimeComponent{UID: component.UID, Version: version})
		default:
			return m, nil, fmt.Errorf("MultiMC 包要求尚未支持的组件 %s，未按原版导入", component.UID)
		}
	}
	if m.Minecraft == "" {
		return m, nil, errors.New("MultiMC 包缺少固定 Minecraft 版本")
	}
	for _, entry := range z.Entries() {
		if !entry.Directory && strings.HasPrefix(entry.Name, prefix+"patches/") {
			return m, nil, errors.New("此 MultiMC 包使用自定义运行补丁，暂不能安全导入")
		}
	}
	payload := prefix + ".minecraft/"
	for _, entry := range z.Entries() {
		if strings.HasPrefix(entry.Name, payload) {
			return m, []string{payload}, nil
		}
	}
	payload = prefix + "minecraft/"
	for _, entry := range z.Entries() {
		if strings.HasPrefix(entry.Name, payload) {
			return m, []string{payload}, nil
		}
	}
	// Empty vanilla exports still have a valid component manifest.
	return m, []string{prefix + ".minecraft/"}, nil
}

func multiMCMinecraft(components []multiMCComponent) string {
	for _, component := range components {
		if component.UID == "net.minecraft" {
			if component.Version != "" {
				return component.Version
			}
			return component.CachedVersion
		}
	}
	return ""
}
func multiMCName(cfg string) (string, error) {
	name, section := "", ""
	for line := range strings.SplitSeq(strings.TrimPrefix(cfg, "\ufeff"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.TrimSpace(line[1 : len(line)-1])
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(key) != "name" || (section != "" && section != "General") {
			continue
		}
		if name != "" {
			return "", errors.New("MultiMC instance.cfg 包含重复名称")
		}
		name = strings.TrimSpace(value)
		if len(name) >= 2 && name[0] == '"' && name[len(name)-1] == '"' {
			name = name[1 : len(name)-1]
		}
	}
	return name, nil
}

func (m Manifest) CheckRuntime(version meta.Version) error {
	for _, component := range m.RuntimeComponents {
		expectedGroup := "org.lwjgl:"
		if component.UID == "org.lwjgl" {
			expectedGroup = "org.lwjgl.lwjgl:"
		}
		found := false
		for _, library := range version.Libraries {
			parts := strings.Split(library.Name, ":")
			if strings.HasPrefix(library.Name, expectedGroup) && len(parts) >= 3 && parts[1] == "lwjgl" && parts[2] == component.Version {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("整合包的 %s %s 与官方游戏运行库不同，暂不导入自定义运行环境", component.UID, component.Version)
		}
	}
	return nil
}
