package launch

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"mcl/internal/core/meta"
)

func TestBuildFixesNativeEncoding(t *testing.T) {
	o := options(t, "1.20.4")
	o.Java.NativeEncoding = "MS932"
	p, err := Build(o, session(t))
	if err != nil || p.nativeEncoding != "MS932" {
		t.Fatal("native encoding lost between probe and launch", err)
	}
	o.Java.NativeEncoding = "UTF-8"
	if p.nativeEncoding != "MS932" {
		t.Fatal("plan encoding changed after build")
	}
}

func TestWindowsPlanNativeTransport(t *testing.T) {
	for _, major := range []int{8, 17, 21} {
		t.Run(fmt.Sprint(major), func(t *testing.T) {
			p := Plan{executable: "C:/Java/bin/java.exe", javaMajor: major, nativeEncoding: "MS936", target: meta.Platform{OS: "windows", Arch: "amd64"}, arguments: []string{"-cp", "C:/中文 空格/library.jar", "Main", "--accessToken", "synthetic-private-token"}, mainIndex: 2, secrets: []string{"synthetic-private-token"}}
			extension, data, err := p.SupportFile()
			if err != nil || bytes.Contains(data, []byte(p.secrets[0])) {
				t.Fatal("support file privacy", err)
			}
			if major != 8 && !bytes.Contains(data, []byte{0xd6, 0xd0, 0xce, 0xc4}) {
				t.Fatal("plan did not preserve JVM native encoding")
			}
			if major == 8 && len(manifestClasspath(t, data)) != 1 {
				t.Fatal("Java8 manifest lost classpath")
			}
			args, err := p.ProcessCommand("C:/中文 空格/runtime/support" + extension)
			if err != nil || args[len(args)-1] != p.secrets[0] {
				t.Fatal("game arguments lost or leaked", err)
			}
			args[len(args)-1] = "mutated"
			again, err := p.ProcessCommand("C:/中文 空格/runtime/support" + extension)
			if err != nil || again[len(again)-1] != p.secrets[0] {
				t.Fatal("exec arguments alias plan")
			}
			if _, err := p.ProcessCommand("C:/😀/support" + extension); err == nil {
				t.Fatal("unrepresentable support path accepted")
			}
			p.nativeEncoding = "Cp1252"
			if _, err := p.ProcessCommand("C:/中文/support" + extension); err == nil {
				t.Fatal("UTF-16 CreateProcess accepted but native Java path loses characters")
			}
			if major != 8 {
				if _, _, err := p.SupportFile(); err == nil {
					t.Fatal("JVM data silently encoded using UTF8 instead of selected native encoding")
				}
			}
			p.nativeEncoding = "UTF-8"
			if _, _, err := p.SupportFile(); err != nil {
				t.Fatal(err)
			}
			p.arguments = append(p.arguments, "player-😀")
			if _, err := p.ProcessCommand("C:/😀/support" + extension); err != nil {
				t.Fatal(err)
			}
			p.nativeEncoding = "MS936"
			if _, err := p.ProcessCommand("C:/ascii/support" + extension); err == nil || strings.Contains(err.Error(), "player-") {
				t.Fatal("lossy game args accepted or reported verbatim")
			}
		})
	}
}

func TestNativeEncodingPropertiesCannotOverrideProbe(t *testing.T) {
	for _, key := range []string{"sun.jnu.encoding", "native.encoding"} {
		if err := ValidateJVMArguments(GCNone, []string{"-D" + key + "=UTF-8"}); err == nil {
			t.Fatal("custom text overrides actual native encoding")
		}
		o := options(t, "1.20.4")
		o.Version.Arguments.JVM = append(o.Version.Arguments.JVM, meta.Argument{Values: []string{"-D" + key + "=UTF-8"}})
		if _, err := Build(o, session(t)); err == nil {
			t.Fatal("version JVM args override native encoding evidence")
		}
	}
	if err := ValidateJVMArguments(GCNone, []string{"-Dfile.encoding=UTF-8"}); err != nil {
		t.Fatal("application default charset is not launcher-native encoding", err)
	}
}
