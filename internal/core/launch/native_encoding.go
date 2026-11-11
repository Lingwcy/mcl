package launch

import "strings"

func reservedNativeEncodingOption(option string) bool {
	key, _, _ := strings.Cut(option, "=")
	return key == "-Dsun.jnu.encoding" || key == "-Dnative.encoding"
}
