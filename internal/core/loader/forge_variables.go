package loader

import (
	"errors"
	"strings"
)

// Scalar substitutions are textual only. Typed file variables stay anchored at
// the start and are resolved by forgeArg; a scalar cannot turn into an external
// pathname, URL, JVM argument file, or a second interpolation pass.
func expandForgeScalars(text string, values map[string]forgeValue) (string, error) {
	var out strings.Builder
	for pos := 0; pos < len(text); {
		open := strings.IndexByte(text[pos:], '{')
		if open < 0 {
			out.WriteString(text[pos:])
			break
		}
		open += pos
		out.WriteString(text[pos:open])
		end := strings.IndexByte(text[open+1:], '}')
		if end < 0 {
			return "", errors.New("安装变量未闭合")
		}
		end += open + 1
		key := text[open+1 : end]
		value, ok := values[key]
		if !ok {
			return "", errors.New("未知安装变量")
		}
		if value.file {
			if open != 0 {
				return "", errors.New("安装路径变量只能作为路径起点")
			}
			out.WriteString(text[open : end+1])
		} else {
			if !safeForgeLiteral(value.text) || strings.ContainsAny(value.text, "{}") {
				return "", errors.New("安装标量包含不安全路径或递归变量")
			}
			out.WriteString(value.text)
		}
		pos = end + 1
		if out.Len() > 4096 {
			return "", errors.New("安装变量展开超过边界")
		}
	}
	result := out.String()
	if len(result) > 4096 {
		return "", errors.New("安装变量展开超过边界")
	}
	return result, nil
}
