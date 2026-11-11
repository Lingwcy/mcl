package java

import (
	"errors"
	"strings"
	"unicode/utf8"
)

const MaxArgumentFileSize = 4 << 20

// EncodeArgumentFile quotes UTF-8 arguments using the Java launcher file
// syntax. Native Windows launchers need EncodeRuntimeArgumentFile instead.
func EncodeArgumentFile(arguments []string) ([]byte, error) {
	return encodeArgumentFile(arguments, "utf8")
}

// EncodeRuntimeArgumentFile uses the encoding reported by the selected JVM,
// never the launcher's system locale or file.encoding JVM override. Absent
// Windows encoding evidence permits ASCII only; unsupported names fail.
func EncodeRuntimeArgumentFile(arguments []string, targetOS, nativeEncoding string) ([]byte, error) {
	encoding := "utf8"
	if targetOS == "windows" {
		encoding = nativeEncoding
	}
	return encodeArgumentFile(arguments, encoding)
}
func encodeArgumentFile(arguments []string, encoding string) ([]byte, error) {
	if len(arguments) == 0 {
		return nil, errors.New("empty Java argument list")
	}
	var out strings.Builder
	for _, arg := range arguments {
		if !utf8.ValidString(arg) || strings.ContainsRune(arg, 0) {
			return nil, errors.New("Java argument is not valid UTF-8 or contains NUL")
		}
		if len(arg) > MaxArgumentFileSize {
			return nil, errors.New("Java arguments exceed 4 MiB")
		}
		encoded, err := nativeArgumentBytes(arg, encoding)
		if err != nil {
			return nil, err
		}
		// Escape AFTER native encoding: Shift-JIS can use 0x5c as a trailing
		// multibyte byte; the launcher's byte lexer still treats it as backslash.
		if out.Len() > MaxArgumentFileSize-3 {
			return nil, errors.New("Java arguments exceed 4 MiB")
		}
		out.WriteByte('"')
		for _, value := range encoded {
			extra := 1
			switch value {
			case '\\', '"', '\n', '\r', '\t', '\f':
				extra = 2
			}
			if out.Len() > MaxArgumentFileSize-extra-2 {
				return nil, errors.New("Java arguments exceed 4 MiB")
			}
			switch value {
			case '\\', '"':
				out.WriteByte('\\')
				out.WriteByte(value)
			case '\n':
				out.WriteString("\\n")
			case '\r':
				out.WriteString("\\r")
			case '\t':
				out.WriteString("\\t")
			case '\f':
				out.WriteString("\\f")
			default:
				out.WriteByte(value)
			}
		}
		out.WriteString("\"\n")
	}
	return []byte(out.String()), nil
}
