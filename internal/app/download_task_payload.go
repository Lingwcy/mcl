package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
)

func safeTaskPayload(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) > 4<<20 {
		return nil, errors.New("任务固定计划超过4MiB")
	}
	var value any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	var clean func(any, int) (any, error)
	nodes := 0
	clean = func(v any, depth int) (any, error) {
		nodes++
		if depth > 64 || nodes > 120000 {
			return nil, errors.New("任务计划结构超出边界")
		}
		switch data := v.(type) {
		case map[string]any:
			for key, child := range data {
				normalized := strings.ToLower(strings.ReplaceAll(key, "_", ""))
				if strings.Contains(normalized, "token") || normalized == "authorization" || normalized == "password" || normalized == "clientsecret" || normalized == "apikey" || normalized == "oauthcode" {
					return nil, errors.New("任务计划不能包含认证秘密")
				}
				value, err := clean(child, depth+1)
				if err != nil {
					return nil, err
				}
				data[key] = value
			}
		case []any:
			for index, child := range data {
				value, err := clean(child, depth+1)
				if err != nil {
					return nil, err
				}
				data[index] = value
			}
		case string:
			if strings.Contains(data, "://") {
				u, err := url.Parse(data)
				if err != nil || u.Scheme != "https" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || !publicTaskHost(u.Hostname()) {
					return "", nil
				}
			}
		}
		return v, nil
	}
	value, err := clean(value, 0)
	if err != nil {
		return nil, err
	}
	return json.Marshal(value)
}
func publicTaskHost(host string) bool {
	host = strings.ToLower(host)
	for _, allowed := range []string{"mojang.com", "minecraft.net", "minecraftforge.net", "neoforged.net", "fabricmc.net", "quiltmc.org", "modrinth.com", "forgecdn.net", "github.com", "cdn.azul.com", "maven.apache.org", "maven.org"} {
		if host == allowed || strings.HasSuffix(host, "."+allowed) {
			return true
		}
	}
	return false
}
