package loader

import (
	"bytes"
	"encoding/json"

	"mcl/internal/core/meta"
)

// Official loader profiles may omit downloads/artifact and rely on a Maven
// repository root. Explicit null/empty identities never authorize that fallback.
func decodeLoaderProfile(data []byte) (rawProfile, error) {
	var raw rawProfile
	if err := json.Unmarshal(data, &raw); err != nil {
		return raw, identityError("加载器启动配置格式无效")
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal(data, &document); err != nil || document == nil {
		return raw, identityError("加载器启动配置不是对象")
	}
	for _, key := range []string{"javaVersion", "arguments"} {
		if value, present := document[key]; present && bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return raw, identityError("加载器显式身份字段不能为null")
		}
	}
	if _, present := document["javaVersion"]; present && (raw.Java.Major < 8 || raw.Java.Major > 100) {
		return raw, identityError("加载器显式Java主版本无效")
	}
	if _, present := document["compatibleJavaMajors"]; present {
		return raw, identityError("加载器compatibleJavaMajors方言尚未绑定，不能静默忽略")
	}
	var libraries []json.RawMessage
	if err := json.Unmarshal(document["libraries"], &libraries); err != nil || len(libraries) == 0 || len(libraries) > 128 {
		return raw, identityError("加载器依赖列表无效")
	}
	if len(libraries) != len(raw.Libraries) {
		return raw, identityError("加载器依赖列表不一致")
	}
	for i, encoded := range libraries {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(encoded, &fields); err != nil || fields == nil {
			return raw, identityError("加载器库不是对象")
		}
		if value, present := fields["url"]; present {
			var root string
			if err := json.Unmarshal(value, &root); err != nil || root == "" {
				return raw, identityError("加载器仓库根显式为空或无效")
			}
			if _, err := meta.MavenRepositoryRoot(root); err != nil {
				return raw, identityError("加载器仓库根不受支持")
			}
		}
		downloads, present := fields["downloads"]
		if !present {
			continue
		}
		var declarations map[string]json.RawMessage
		if err := json.Unmarshal(downloads, &declarations); err != nil || declarations == nil {
			return raw, identityError("加载器downloads显式为null或不是对象")
		}
		artifact, present := declarations["artifact"]
		if !present {
			continue
		}
		var identity map[string]json.RawMessage
		if err := json.Unmarshal(artifact, &identity); err != nil || identity == nil {
			return raw, identityError("加载器artifact显式为null或不是对象")
		}
		if path, present := identity["path"]; present {
			var value string
			if err := json.Unmarshal(path, &value); err != nil || value == "" {
				return raw, identityError("加载器制品路径显式为空或无效")
			}
		}
		for _, key := range []string{"url", "sha1", "size"} {
			if value, ok := identity[key]; !ok || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
				return raw, identityError("加载器显式制品身份缺少必要字段")
			}
		}
		file := raw.Libraries[i].Downloads.Artifact
		if file == nil || file.Validate() != nil || file.Size > maxMavenArtifactSize {
			return raw, identityError("加载器显式制品缺少有效摘要大小或地址")
		}
	}
	return raw, nil
}
