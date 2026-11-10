package loader

import (
	"bytes"
	"net/url"
	"path"
	"strings"

	"mcl/internal/core/meta"
)

// Accept raw hex, GNU sha1sum and BSD/OpenSSL only for the requested artifact.
// A sidecar is a single identity declaration, never an arbitrary text search.
func parseMavenSHA1(data []byte, relative string) (string, error) {
	if len(data) > 1024 || !meta.ValidPath(relative) {
		return "", identityError("Maven摘要响应或固定路径无效")
	}
	text := strings.TrimSpace(string(bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf})))
	if strings.ContainsRune(text, 10) || strings.ContainsRune(text, 13) {
		return "", identityError("Maven摘要包含多个或矛盾声明")
	}
	if meta.ValidSHA1(text) {
		return strings.ToLower(text), nil
	}
	name, digest := "", ""
	for _, prefix := range []string{"SHA1 (", "SHA-1 ("} {
		if strings.HasPrefix(text, prefix) {
			remainder := strings.TrimPrefix(text, prefix)
			var ok bool
			name, digest, ok = strings.Cut(remainder, ") = ")
			if !ok {
				return "", identityError("Maven摘要格式无效")
			}
			break
		}
	}
	if name == "" {
		fields := strings.Fields(text)
		if len(fields) != 2 {
			return "", identityError("Maven SHA-1格式无效")
		}
		digest, name = fields[0], strings.TrimPrefix(fields[1], "*")
	}
	name = strings.TrimPrefix(name, "./")
	if !meta.ValidSHA1(digest) || name != path.Base(relative) && name != relative {
		return "", identityError("Maven摘要与固定制品文件名不匹配")
	}
	return strings.ToLower(digest), nil
}
func validMavenAddress(address string) bool {
	if !AllowedLibraryURL(address) {
		return false
	}
	u, err := url.Parse(address)
	return err == nil && !u.ForceQuery && u.RawPath == "" && meta.ValidPath(strings.TrimPrefix(u.Path, "/"))
}

func validateMavenLocation(address, relative string) error {
	if !validMavenAddress(address) || !meta.ValidPath(relative) {
		return identityError("Maven制品来源或路径不受支持")
	}
	u, err := url.Parse(address)
	if err != nil || u.ForceQuery || u.RawPath != "" || !strings.HasSuffix(u.Path, "/"+relative) {
		return identityError("Maven地址与固定制品路径不匹配")
	}
	return nil
}
