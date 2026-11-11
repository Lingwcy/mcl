package process

import (
	"errors"
	"strings"
	"unicode/utf8"
)

const windowsCommandLimit = 32767

// Count default os/exec CreateProcess quoting, not shell syntax. Do not build
// another copy of the sensitive command or allocate an unbounded UTF-16 slice.
func validateWindowsCommandSize(executable string, args []string) error {
	remaining := windowsCommandLimit - 1 // NUL terminator
	if err := consumeWindowsArgument(executable, &remaining); err != nil {
		return err
	}
	for _, arg := range args {
		remaining-- // separating space
		if err := consumeWindowsArgument(arg, &remaining); err != nil {
			return err
		}
	}
	return nil
}
func consumeWindowsArgument(value string, remaining *int) error {
	invalid := func() error {
		return errors.New("启动参数超过 Windows 命令行长度限制或包含无效文本；请缩短路径或减少自定义参数")
	}
	if !utf8.ValidString(value) || strings.ContainsRune(value, 0) {
		return invalid()
	}
	quoted := len(value) == 0 || strings.ContainsAny(value, " \t")
	if quoted {
		*remaining -= 2
	}
	backslashes := 0
	for _, r := range value {
		*remaining -= 1
		if r > 0xffff {
			*remaining -= 1
		}
		if r == '"' {
			*remaining -= backslashes + 1
		}
		if r == '\\' {
			backslashes++
		} else {
			backslashes = 0
		}
		if *remaining < 0 {
			return invalid()
		}
	}
	if quoted {
		*remaining -= backslashes
	}
	if *remaining < 0 {
		return invalid()
	}
	return nil
}
