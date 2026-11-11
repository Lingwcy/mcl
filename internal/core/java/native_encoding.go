package java

import (
	"bytes"
	"errors"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/traditionalchinese"
)

var errNativeEncoding = errors.New("当前Java的原生字符编码不能无损传输启动参数；请使用可表示的路径和参数，或更换兼容的Java环境")

func nativeArgumentBytes(value, name string) ([]byte, error) {
	if len(name) > 64 {
		return nil, errNativeEncoding
	}
	if !utf8.ValidString(value) || strings.ContainsRune(value, 0) {
		return nil, errors.New("启动参数包含无效文本")
	}
	normalized := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(name), "-", ""), "_", ""))
	switch normalized {
	case "utf8", "cp65001", "ms65001", "windows65001":
		return []byte(value), nil
	}
	if normalized == "" || normalized == "ascii" || normalized == "usascii" {
		for i := range len(value) {
			if value[i] > 127 {
				return nil, errNativeEncoding
			}
		}
		return []byte(value), nil
	}
	var codec encoding.Encoding
	switch normalized {
	case "gbk", "ms936", "cp936", "windows936":
		codec = simplifiedchinese.GBK
	case "gb18030":
		codec = simplifiedchinese.GB18030
	case "big5", "ms950", "cp950", "windows950":
		codec = traditionalchinese.Big5
	case "ms932", "cp932", "windows932", "shiftjis", "sjis":
		codec = japanese.ShiftJIS
	case "ms949", "cp949", "windows949", "euckr":
		codec = korean.EUCKR
	case "cp1250", "ms1250", "windows1250":
		codec = charmap.Windows1250
	case "cp1251", "ms1251", "windows1251":
		codec = charmap.Windows1251
	case "cp1252", "ms1252", "windows1252":
		codec = charmap.Windows1252
	case "cp1253", "ms1253", "windows1253":
		codec = charmap.Windows1253
	case "cp1254", "ms1254", "windows1254":
		codec = charmap.Windows1254
	case "cp1255", "ms1255", "windows1255":
		codec = charmap.Windows1255
	case "cp1256", "ms1256", "windows1256":
		codec = charmap.Windows1256
	case "cp1257", "ms1257", "windows1257":
		codec = charmap.Windows1257
	case "cp1258", "ms1258", "windows1258":
		codec = charmap.Windows1258
	default:
		return nil, errNativeEncoding
	}
	encoded, err := codec.NewEncoder().Bytes([]byte(value))
	if err != nil {
		return nil, errNativeEncoding
	}
	decoded, err := codec.NewDecoder().Bytes(encoded)
	if err != nil || !bytes.Equal(decoded, []byte(value)) {
		return nil, errNativeEncoding
	}
	return encoded, nil
}

// ValidateNativeArguments checks only representability. It does not encode the
// exec argv: os/exec still receives the original []string and CreateProcessW
// quoting; the JVM performs its own conversion to the reported native encoding.
func ValidateNativeArguments(arguments []string, targetOS, nativeEncoding string) error {
	for _, arg := range arguments {
		if !utf8.ValidString(arg) || strings.ContainsRune(arg, 0) {
			return errors.New("启动参数包含无效文本")
		}
		if targetOS != "windows" {
			continue
		}
		if len(arg) > MaxArgumentFileSize {
			return errors.New("Java启动参数超过边界")
		}
		if _, err := nativeArgumentBytes(arg, nativeEncoding); err != nil {
			return err
		}
	}
	return nil
}
