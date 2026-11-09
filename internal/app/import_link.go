package app

import (
	"errors"
	"net/url"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"
)

// ImportLinkPath accepts only mcl's local-file import intent. It cannot trigger
// a remote download, game launch, credentials action or arbitrary file opening.
// The native adapter must still obtain confirmation before reading this path.
func ImportLinkPath(address string) (string, error) {
	invalid := errors.New("无效的整合包导入链接")
	if len(address) > 24<<10 {
		return "", invalid
	}
	u, err := url.Parse(address)
	if err != nil || u.Scheme != "mcl" || u.Host != "import-modpack" || (u.Path != "" && u.Path != "/") || u.User != nil || u.Fragment != "" || u.Opaque != "" {
		return "", invalid
	}
	values, err := url.ParseQuery(u.RawQuery)
	if err != nil || len(values) != 1 || len(values["path"]) != 1 {
		return "", invalid
	}
	name := values.Get("path")
	extension := strings.ToLower(filepath.Ext(name))
	if !utf8.ValidString(name) || len(name) > 4096 || !filepath.IsAbs(name) || strings.IndexFunc(name, unicode.IsControl) >= 0 || (extension != ".zip" && extension != ".mrpack") {
		return "", invalid
	}
	return name, nil
}
