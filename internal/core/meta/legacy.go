package meta

import (
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"
)

const maxLegacyArgumentBytes = 16 << 10

// SplitLegacyArguments is a bounded token lexer, not a shell. Quotes group
// template text; backslashes are literal except before a quote/backslash.
// Placeholder substitution happens later, once each complete token exists, so
// spaces/quotes in a player name or path never introduce extra arguments.
func SplitLegacyArguments(text string) ([]string, error) {
	if len(text) > maxLegacyArgumentBytes || !utf8.ValidString(text) {
		return nil, errors.New("旧版启动参数超过16KiB或不是有效文本")
	}
	var out []string
	var token strings.Builder
	var quote rune
	started := false
	chars := []rune(text)
	for i := 0; i < len(chars); i++ {
		char := chars[i]
		if unicode.IsControl(char) && char != 9 {
			return nil, errors.New("旧版启动参数不能包含换行或控制字符")
		}
		if char == 92 && i+1 < len(chars) {
			next := chars[i+1]
			if next == 92 || next == quote && quote != 0 || quote == 0 && (next == 34 || next == 39) {
				token.WriteRune(next)
				started = true
				i++
				continue
			}
		}
		switch {
		case quote != 0:
			if char == quote {
				quote = 0
			} else {
				token.WriteRune(char)
			}
		case char == 34 || char == 39:
			quote = char
			started = true
		case char == ' ' || char == 9:
			if started {
				out = append(out, token.String())
				token.Reset()
				started = false
			}
		default:
			token.WriteRune(char)
			started = true
		}
		if len(out) > 256 {
			return nil, errors.New("旧版启动参数超过256项")
		}
	}
	if quote != 0 {
		return nil, errors.New("旧版启动参数引号未闭合")
	}
	if started {
		out = append(out, token.String())
	}
	if len(out) == 0 || len(out) > 256 {
		return nil, errors.New("旧版启动参数为空或超过256项")
	}
	for _, value := range out {
		if strings.ContainsFunc(value, unicode.IsControl) {
			return nil, errors.New("旧版启动参数包含控制字符")
		}
	}
	return out, nil
}

// This table is only consulted AFTER the client's exact official package URL,
// SHA-1 and document ID have been verified. It is not a general version-name
// heuristic and deliberately excludes snapshots and unlisted alpha/beta IDs.
// A release still needs complete pinned client/assets/libraries and a direct
// metadata main entry; no applet wrapper or client rewrite is introduced. These
// older releases use stable Java 8 in mcl, not an unbounded newer-major fallback.
func historicalJava8Release(id string) bool {
	switch id {
	case "1.0", "1.1", "1.2.1", "1.2.2", "1.2.3", "1.2.4", "1.2.5",
		"1.3.1", "1.3.2", "1.4.2", "1.4.4", "1.4.5", "1.4.6", "1.4.7", "1.5", "1.5.1", "1.5.2",
		"1.6.1", "1.6.2", "1.6.4",
		"1.7.2", "1.7.4", "1.7.5", "1.7.6", "1.7.7", "1.7.8", "1.7.9", "1.7.10",
		"1.8", "1.8.1", "1.8.2", "1.8.3", "1.8.4", "1.8.5", "1.8.6", "1.8.7", "1.8.8", "1.8.9",
		"1.9", "1.9.1", "1.9.2", "1.9.3", "1.9.4", "1.10", "1.10.1", "1.10.2",
		"1.11", "1.11.1", "1.11.2", "1.12", "1.12.1", "1.12.2",
		"1.13", "1.13.1", "1.13.2", "1.14", "1.14.1", "1.14.2", "1.14.3", "1.14.4",
		"1.15", "1.15.1", "1.15.2", "1.16", "1.16.1", "1.16.2", "1.16.3", "1.16.4", "1.16.5":
		return true
	}
	return false
}

// Legacy minecraftArguments documents do not supply JVM arguments. In
// particular, LWJGL2 uses its AWT/Cocoa integration rather than LWJGL3/GLFW's
// first-thread entry. Do not synthesize -XstartOnFirstThread here; modern
// documents keep their own explicit, OS-scoped JVM argument rules unchanged.
func legacyJVMArguments() []Argument {
	return []Argument{
		{Values: []string{"-Djava.library.path=${natives_directory}"}},
		{Values: []string{"-Dminecraft.launcher.brand=${launcher_name}"}},
		{Values: []string{"-Dminecraft.launcher.version=${launcher_version}"}},
		{Values: []string{"-cp"}}, {Values: []string{"${classpath}"}},
	}
}
