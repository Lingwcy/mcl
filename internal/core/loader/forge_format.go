package loader

import (
	"bytes"
	"encoding/json"
	"strings"

	"mcl/internal/core/meta"
)

const (
	ForgeFormatLegacy     = "install-version-info"
	ForgeFormatEmbedded   = "embedded-maven-spec0"
	ForgeFormatProcessors = "client-processors-spec1"
	ForgeAdapterPolicy    = "official-installer-v2"
)

func installerFormat(data []byte) (string, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil || fields == nil {
		return "", identityError("Forge安装清单不是有效对象")
	}
	_, install := fields["install"]
	_, versionInfo := fields["versionInfo"]
	if install || versionInfo {
		if !install || !versionInfo {
			return "", identityError("Forge旧式安装清单字段不完整")
		}
		for _, key := range []string{"install", "versionInfo"} {
			var value map[string]json.RawMessage
			if err := json.Unmarshal(fields[key], &value); err != nil || value == nil {
				return "", identityError("Forge旧式安装字段不能为null或非对象")
			}
		}
		for _, key := range []string{"json", "processors", "data"} {
			if _, ok := fields[key]; ok {
				return "", identityError("Forge安装清单混入不同代际字段")
			}
		}
		if raw, ok := fields["spec"]; ok {
			var spec *int
			if json.Unmarshal(raw, &spec) != nil || spec == nil || *spec != 0 {
				return "", identityError("Forge旧式spec字段无效")
			}
		}
		return ForgeFormatLegacy, nil
	}
	raw, ok := fields["spec"]
	if !ok {
		return "", identityError("Forge安装清单缺少明确格式")
	}
	var spec *int
	if json.Unmarshal(raw, &spec) != nil || spec == nil {
		return "", identityError("Forge安装spec不能为null或非整数")
	}
	if *spec == 0 {
		return ForgeFormatEmbedded, nil
	}
	if *spec == 1 {
		return ForgeFormatProcessors, nil
	}
	return "", identityError("Forge安装spec尚未支持")
}
func forgeProfileID(request Request, id string) bool {
	_, _, expected, err := forgeInstaller(request)
	if err != nil {
		return false
	}
	if id == expected {
		return true
	}
	if request.Type == "forge" {
		build := strings.TrimPrefix(request.Version, request.GameVersion+"-")
		return id == request.GameVersion+"-forge"+build || legacyForgeID(request, id)
	}
	return false
}

// Spec0/version.json is a passive embedded runtime format. It is not spec1
// with a zero substituted: no data expansion or Java processors are admitted.
func validateEmbeddedForgeSpec(spec forgeSpec, version meta.Version, zHas func(string) bool, request Request) error {
	if request.Type != "forge" || !spec.LegacyGame {
		return identityError("此spec0格式只接受已绑定的旧Forge客户端")
	}
	if len(spec.Data) != 0 || len(spec.Processors) != 0 {
		return identityError("spec0内嵌格式不能声明处理器或数据任务")
	}
	coordinate, err := meta.ParseCoordinate(spec.Path)
	if err != nil || !legacyForgeCore(request, spec.Path) {
		return identityError("spec0核心坐标与固定构建不匹配")
	}
	if !zHas("maven/" + coordinate.Path()) {
		return identityError("spec0缺少固定的内嵌核心库")
	}
	found := false
	for _, library := range version.Libraries {
		if library.Name != spec.Path {
			continue
		}
		file := library.Downloads.Artifact
		if file == nil || file.ValidateIdentity() != nil || file.Path != coordinate.Path() || file.URL != "" {
			return identityError("spec0内嵌核心库缺少固定摘要或路径")
		}
		found = true
	}
	if !found {
		return identityError("spec0运行配置未引用同一内嵌核心库")
	}
	return nil
}

func validateInstallerJSONFields(data []byte) error {
	var fields map[string]json.RawMessage
	if json.Unmarshal(data, &fields) != nil || fields == nil {
		return identityError("Forge客户端配置不是对象")
	}
	for _, key := range []string{"inheritsFrom", "jar", "javaVersion", "arguments", "minecraftArguments"} {
		if value, ok := fields[key]; ok && bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return identityError("Forge客户端显式身份字段不能为null")
		}
	}
	if raw, ok := fields["javaVersion"]; ok {
		var value meta.JavaVersion
		if json.Unmarshal(raw, &value) != nil || value.Major < 8 || value.Major > 100 {
			return identityError("Forge客户端显式Java需求无效")
		}
	}
	if _, ok := fields["compatibleJavaMajors"]; ok {
		return identityError("Forge客户端Java允许集合方言尚未绑定")
	}
	return nil
}
