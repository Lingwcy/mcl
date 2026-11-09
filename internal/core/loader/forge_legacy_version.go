package loader

import (
	"bytes"
	"encoding/json"
	"errors"

	"mcl/internal/core/meta"
)

// Older producers omit inheritsFrom. The official fixed installer coordinate,
// install.minecraft and checked profile ID/core bind the game independently;
// explicit unknown/null/empty parents never turn into an implicit fallback.
func parseLegacyForgeVersion(raw []byte, baseID, installedGame string) (meta.Version, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return meta.Version{}, err
	}
	if fields == nil {
		return meta.Version{}, errors.New("旧Forge versionInfo不是对象")
	}
	for _, key := range []string{"inheritsFrom", "jar", "javaVersion"} {
		if value, ok := fields[key]; ok && bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return meta.Version{}, errors.New("旧Forge客户端身份不能为null")
		}
	}
	if _, present := fields["inheritsFrom"]; !present {
		if installedGame != baseID {
			return meta.Version{}, errors.New("旧Forge缺省父版本未由安装器绑定")
		}
		var jar string
		if value, ok := fields["jar"]; ok {
			if err := json.Unmarshal(value, &jar); err != nil {
				return meta.Version{}, err
			}
		}
		if jar != "" && jar != baseID {
			return meta.Version{}, errors.New("旧Forge客户端JAR与固定基底不符")
		}
		fields["inheritsFrom"], _ = json.Marshal(baseID)
		normalized, err := json.Marshal(fields)
		if err != nil {
			return meta.Version{}, err
		}
		raw = normalized
	}
	return meta.ParseVersion(raw)
}
