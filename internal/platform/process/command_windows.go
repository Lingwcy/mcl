package process

import (
	"errors"
	"strings"
	"syscall"
	"unicode/utf16"
)

func validateCommandSize(executable string, args []string) error {
	// Match os/exec's default CreateProcess quoting, not shell quoting. The
	// 32767 UTF-16 limit includes the terminator and still applies to Java 8's
	// non-classpath options and game arguments after classpath compression.
	var command strings.Builder
	command.WriteString(syscall.EscapeArg(executable))
	for _, arg := range args {
		command.WriteByte(' ')
		command.WriteString(syscall.EscapeArg(arg))
	}
	if len(utf16.Encode([]rune(command.String())))+1 > 32767 {
		return errors.New("启动参数超过 Windows 命令行长度限制；请缩短路径或减少自定义参数")
	}
	return nil
}
