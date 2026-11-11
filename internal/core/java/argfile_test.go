package java

import (
	"bytes"
	"strings"
	"testing"
)

func TestArgumentFileLiteralSyntax(t *testing.T) {
	for _, tc := range []struct{ arg, want string }{
		{"", "\"\"\n"},
		{"#not-a-comment", "\"#not-a-comment\"\n"},
		{"@not-another-file", "\"@not-another-file\"\n"},
		{"a\n\r\t\f", "\"a\\n\\r\\t\\f\"\n"},
		{"a\"'b", "\"a\\\"'b\"\n"},
		{"C:\\中文 空格\\😀\\", "\"C:\\\\中文 空格\\\\😀\\\\\"\n"},
	} {
		got, err := EncodeArgumentFile([]string{tc.arg})
		if err != nil || string(got) != tc.want {
			t.Fatalf("quoted bytes: got %q, want %q, error %v", got, tc.want, err)
		}
	}
}

func TestRuntimeArgumentFileNativeEncoding(t *testing.T) {
	// These are fixed encoded bytes, not JVM execution evidence.
	for _, tc := range []struct {
		encoding, arg string
		want          []byte
	}{
		{"MS936", "-Dname=中文", []byte{'"', '-', 'D', 'n', 'a', 'm', 'e', '=', 0xd6, 0xd0, 0xce, 0xc4, '"', '\n'}},
		{"Cp1252", "-Dname=café", []byte{'"', '-', 'D', 'n', 'a', 'm', 'e', '=', 'c', 'a', 'f', 0xe9, '"', '\n'}},
		// ソ encodes as 83 5c: encoding AFTER quoting would corrupt this token.
		{"MS932", "-Dname=ソ", []byte{'"', '-', 'D', 'n', 'a', 'm', 'e', '=', 0x83, 0x5c, 0x5c, '"', '\n'}},
		{"UTF-8", "-Dname=中文😀", []byte("\"-Dname=中文😀\"\n")},
		{"", "-Dname=ascii", []byte("\"-Dname=ascii\"\n")},
	} {
		t.Run(tc.encoding, func(t *testing.T) {
			got, err := EncodeRuntimeArgumentFile([]string{tc.arg}, "windows", tc.encoding)
			if err != nil || !bytes.Equal(got, tc.want) {
				t.Fatalf("native bytes: got %x, want %x, error %v", got, tc.want, err)
			}
			if err := ValidateNativeArguments([]string{tc.arg}, "windows", tc.encoding); err != nil {
				t.Fatal(err)
			}
		})
	}
	for _, tc := range []struct{ encoding, arg string }{
		{"", "中文"}, {"unknown-encoding", "ascii"}, {"Cp1252", "中文"}, {"MS936", "😀"}, {"MS932", "😀"},
	} {
		if data, err := EncodeRuntimeArgumentFile([]string{tc.arg}, "windows", tc.encoding); err == nil || data != nil {
			t.Fatal("lossy or unknown native encoding accepted")
		}
		if err := ValidateNativeArguments([]string{tc.arg}, "windows", tc.encoding); err == nil || strings.Contains(err.Error(), tc.arg) {
			t.Fatal("native argv accepted or leaked rejected argument")
		}
	}
	// Do not infer Unix encoding from a Windows JVM/launcher locale setting.
	data, err := EncodeRuntimeArgumentFile([]string{"中文😀"}, "linux", "Cp1252")
	if err != nil || string(data) != "\"中文😀\"\n" {
		t.Fatal("Unix UTF-8 transport changed")
	}
}

func TestArgumentFileBoundIsEncodedBytes(t *testing.T) {
	data, err := EncodeArgumentFile([]string{strings.Repeat("a", MaxArgumentFileSize-3)})
	if err != nil || len(data) != MaxArgumentFileSize {
		t.Fatalf("exact UTF-8 limit: %d %v", len(data), err)
	}
	// GBK's two-byte representation fits where UTF-8's three bytes do not.
	chinese := strings.Repeat("中", MaxArgumentFileSize/3)
	if _, err := EncodeRuntimeArgumentFile([]string{chinese}, "windows", "MS936"); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		nil, {strings.Repeat("a", MaxArgumentFileSize-2)},
		{strings.Repeat("\\", MaxArgumentFileSize/2)},
		{strings.Repeat("a", MaxArgumentFileSize/2), strings.Repeat("b", MaxArgumentFileSize/2)},
		{chinese}, {"invalid-" + string([]byte{0xff})}, {"secret\x00not-to-be-logged"},
	} {
		if data, err := EncodeArgumentFile(args); err == nil || data != nil {
			t.Fatal("invalid or oversized file accepted")
		}
	}
}

func TestProbePreservesNativeEncodingEvidence(t *testing.T) {
	base := "java.version = 17.0.16\nos.arch = amd64\n"
	for _, tc := range []struct{ properties, want string }{
		{"sun.jnu.encoding = MS936\nfile.encoding = UTF-8\n", "MS936"},
		{"native.encoding = Cp1252\nfile.encoding = UTF-8\n", "Cp1252"},
		{"sun.jnu.encoding = MS932\nnative.encoding = UTF-8\n", "MS932"},
		{"file.encoding = UTF-8\n", ""},
	} {
		runtime, err := ParseProperties(base + tc.properties)
		if err != nil || runtime.NativeEncoding != tc.want {
			t.Fatalf("encoding evidence %q, %v", runtime.NativeEncoding, err)
		}
	}
}
