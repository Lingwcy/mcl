package process

import (
	"strings"
	"testing"
)

func TestWindowsCommandUTF16BoundOffline(t *testing.T) {
	// "java" + space + one quoted/escaped argument + trailing NUL = 32767.
	for _, tc := range []struct {
		arg  string
		fits bool
	}{
		{strings.Repeat("a", 32761), true}, {strings.Repeat("a", 32762), false},
		{strings.Repeat("😀", 16380) + "a", true}, {strings.Repeat("😀", 16381), false},
		{strings.Repeat("中", 32761), true},
		{" " + strings.Repeat("\\", 16379), true}, {" " + strings.Repeat("\\", 16380), false},
		{strings.Repeat("\"", 16380) + "a", true}, {strings.Repeat("\"", 16381), false},
		{"bad\x00value", false}, {string([]byte{0xff}), false},
	} {
		err := validateWindowsCommandSize("java", []string{tc.arg})
		if (err == nil) != tc.fits {
			t.Fatalf("UTF16/quoting boundary len=%d: %v", len(tc.arg), err)
		}
	}
	if err := validateWindowsCommandSize("C:\\Java Home\\bin\\java.exe", []string{"-cp", "C:\\中文 空格\\library.jar", "Main", ""}); err != nil {
		t.Fatal(err)
	}
	if err := validateWindowsCommandSize("java", []string{strings.Repeat("a", 16000), strings.Repeat("b", 17000)}); err == nil {
		t.Fatal("combined argv bound ignored")
	}
}
