package meta

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
)

// OfficialChainDigest binds an effective version to all of its authenticated
// documents, in parent-to-child order. Single-document cache keys stay unchanged.
// Sources are internal resolver evidence, never accepted from version JSON.
func OfficialChainDigest(version Version, leafSHA1 string) (string, error) {
	if len(version.Sources) == 0 {
		return "", nil
	}
	if len(version.Sources) > maxInheritanceDepth {
		return "", errors.New("官方来源链超过8层")
	}
	type source struct{ ID, Type, URL, SHA1 string }
	var identities []source
	seen := make(map[string]bool)
	for _, entry := range version.Sources {
		if !officialPackage(entry) || seen[entry.ID] {
			return "", errors.New("官方来源链含未知来源或循环")
		}
		seen[entry.ID] = true
		identities = append(identities, source{entry.ID, entry.Type, entry.URL, strings.ToLower(entry.SHA1)})
	}
	leaf := version.Sources[len(version.Sources)-1]
	if leaf.ID != version.ID || !strings.EqualFold(leaf.SHA1, leafSHA1) {
		return "", errors.New("官方来源链末端与固定实例版本不符")
	}
	if version.ClientVersion != "" && !seen[version.ClientVersion] {
		return "", errors.New("客户端JAR身份不在官方来源链")
	}
	if len(identities) == 1 {
		return "", nil
	}
	data, err := json.Marshal(identities)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}
