package process

import (
	"strings"
	"syscall"
	"testing"
	"unicode/utf16"
)

func TestWindowsQuotedLengthMatchesExec(t *testing.T) {
	for _, arg := range []string{"", "simple", "a b", "a\tb", `a"b`, `a\"b`, `a b\`, `a\\" b\\`, "中文 😀", `\\server\share\space dir\`} {
		remaining := 1 << 20
		before := remaining
		if err := consumeWindowsArgument(arg, &remaining); err != nil {
			t.Fatal(err)
		}
		if before-remaining != len(utf16.Encode([]rune(syscall.EscapeArg(arg)))) {
			t.Fatalf("quoting mismatch: %q", arg)
		}
	}
}

func TestCommandSizeCountsUTF16AndQuoting(t *testing.T) {
	if err := validateCommandSize("C:/Java Home/bin/java.exe", []string{"-cp", "C:/中文 空格/classpath.jar", "Main", strings.Repeat("x", 1000)}); err != nil {
		t.Fatal(err)
	}
	for _, arg := range []string{strings.Repeat("x", 32767), strings.Repeat("😀", 16384), strings.Repeat(string(rune(34)), 16384)} {
		if err := validateCommandSize("C:/java.exe", []string{arg}); err == nil {
			t.Fatal("oversized or quote-expanded command accepted")
		}
	}
}
