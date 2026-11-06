package java

import (
	"errors"
	"strings"
)

// EncodeArgumentFile quotes arguments using the Java launcher file format.
func EncodeArgumentFile(arguments []string) ([]byte, error) {
	if len(arguments) == 0 {
		return nil, errors.New("empty Java argument list")
	}
	var out strings.Builder
	for _, arg := range arguments {
		if strings.ContainsRune(arg, '\x00') {
			return nil, errors.New("Java argument contains NUL")
		}
		out.WriteByte('"')
		for _, r := range arg {
			switch r {
			case '\\', '"':
				out.WriteByte('\\')
				out.WriteRune(r)
			case '\n':
				out.WriteString(`\n`)
			case '\r':
				out.WriteString(`\r`)
			case '\t':
				out.WriteString(`\t`)
			case '\f':
				out.WriteString(`\f`)
			default:
				out.WriteRune(r)
			}
		}
		out.WriteString("\"\n")
		if out.Len() > 4<<20 {
			return nil, errors.New("Java arguments exceed 4 MiB")
		}
	}
	return []byte(out.String()), nil
}
