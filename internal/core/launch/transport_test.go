package launch

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"mcl/internal/core/meta"
)

func manifestClasspath(t *testing.T, data []byte) []string {
	t.Helper()
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil || len(archive.File) != 1 || archive.File[0].Name != "META-INF/MANIFEST.MF" {
		t.Fatalf("manifest-only JAR: %v", err)
	}
	file, err := archive.File[0].Open()
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	manifest, err := io.ReadAll(file)
	if err != nil {
		t.Fatal(err)
	}
	crlf := string([]byte{13, 10})
	for _, line := range strings.Split(string(manifest), crlf) {
		if len(line) > 70 {
			t.Fatal("manifest folding limit")
		}
	}
	unfolded := strings.ReplaceAll(string(manifest), crlf+" ", "")
	prefix := "Manifest-Version: 1.0" + crlf + "Class-Path: "
	if !strings.HasPrefix(unfolded, prefix) || !strings.HasSuffix(unfolded, crlf+crlf) {
		t.Fatal("invalid manifest")
	}
	return strings.Fields(strings.TrimSuffix(strings.TrimPrefix(unfolded, prefix), crlf+crlf))
}

func TestJava8AndModernTransportPrivacy(t *testing.T) {
	for _, major := range []int{8, 17, 21} {
		t.Run(fmt.Sprint(major), func(t *testing.T) {
			// Synthetic metadata: transport only, not real historical game compatibility.
			o := options(t, "1.20.4")
			o.Platform = meta.CurrentPlatform()
			o.Java.Arch = o.Platform.Arch
			o.Version.Java.Major = major
			o.Java.Major, o.Java.Version = major, fmt.Sprint(major)
			s := session(t)
			p, err := Build(o, s)
			if err != nil {
				t.Fatal(err)
			}
			extension, data, err := p.SupportFile()
			if err != nil || bytes.Contains(data, []byte(s.token)) {
				t.Fatalf("support privacy: %v", err)
			}
			file := filepath.Join(o.Root, "runtime", "支持 文件"+extension)
			args := p.ProcessArguments(file)
			if !slices.Contains(args, s.token) {
				t.Fatal("token must remain in exec arguments")
			}
			if major != 8 {
				original, _ := p.JVMArgumentFile()
				if extension != ".args" || !bytes.Equal(original, data) || args[0] != "@"+file || args[1] != o.Version.MainClass {
					t.Fatal("modern transport changed")
				}
				return
			}
			if extension != ".jar" || slices.ContainsFunc(args, func(arg string) bool { return strings.HasPrefix(arg, "@") }) {
				t.Fatal("Java 8 got argfile")
			}
			if _, err := p.JVMArgumentFile(); err == nil {
				t.Fatal("Java 8 exposed argfile")
			}
			index := slices.Index(args, "-cp") + 1
			if index == 0 || args[index] != file || args[p.mainIndex] != o.Version.MainClass {
				t.Fatal("Java 8 boundaries changed")
			}
			urls := manifestClasspath(t, data)
			separator := ":"
			if o.Platform.OS == "windows" {
				separator = ";"
			}
			original := strings.Split(p.arguments[index], separator)
			if len(urls) != len(original) {
				t.Fatal("classpath entries lost")
			}
			for i, encoded := range urls {
				u, err := url.Parse(encoded)
				want := original[i]
				if o.Platform.OS == "windows" {
					want = "/" + strings.ReplaceAll(want, string(rune(92)), "/")
				}
				if err != nil || u.Scheme != "file" || u.Host != "" || u.Path != want || u.RawQuery != "" || u.Fragment != "" {
					t.Fatalf("URL round trip %d: %v", i, err)
				}
			}
			args[index] = "mutated"
			if p.ProcessArguments(file)[index] != file {
				t.Fatal("arguments alias plan")
			}
		})
	}
}

func TestClasspathJARLongUnicodeAndEscaping(t *testing.T) {
	for _, target := range []string{"osx", "linux", "windows"} {
		t.Run(target, func(t *testing.T) {
			var paths []string
			for i := range 1000 {
				path := fmt.Sprintf("/中文 空格/库/#%%问号?/单引号'/长目录%s/%d.jar", strings.Repeat("目录", 8), i)
				if target == "windows" {
					path = strings.ReplaceAll("C:"+strings.ReplaceAll(path, "?", ""), "/", string(rune(92)))
				}
				paths = append(paths, path)
			}
			if len(strings.Join(paths, ";")) <= 32767 {
				t.Fatal("fixture is not a long classpath")
			}
			data, err := classpathJAR(paths, target)
			if err != nil {
				t.Fatal(err)
			}
			urls := manifestClasspath(t, data)
			if len(urls) != len(paths) {
				t.Fatal("long classpath lost entries")
			}
			for i, encoded := range urls {
				u, err := url.Parse(encoded)
				want := paths[i]
				if target == "windows" {
					want = "/" + strings.ReplaceAll(want, string(rune(92)), "/")
				}
				if err != nil || u.Path != want || u.Host != "" || u.RawQuery != "" || u.Fragment != "" || strings.ContainsAny(encoded, string([]byte{34, 32, 13, 10})) {
					t.Fatalf("escaped path %d failed: %v", i, err)
				}
			}
		})
	}
}

func TestJava8TransportRejectsUnsafeOrAmbiguousInputs(t *testing.T) {
	for _, tc := range []struct{ path, os string }{
		{"relative.jar", "linux"}, {"/a" + string(rune(10)) + ".jar", "linux"}, {"/a/*.jar", "osx"},
		{"//server/share/a.jar", "windows"}, {"//?/C:/a.jar", "windows"}, {"C:relative.jar", "windows"},
	} {
		if _, err := classpathJAR([]string{tc.path}, tc.os); err == nil {
			t.Fatalf("accepted unsafe path for %s", tc.os)
		}
	}
	if _, err := classpathJAR([]string{"/" + strings.Repeat("x", maxClasspathManifest)}, "linux"); err == nil {
		t.Fatal("unbounded manifest accepted")
	}
	p := Plan{javaMajor: 8, target: meta.Platform{OS: "linux"}, arguments: []string{"-cp", "/library.jar", "Main", "secret"}, mainIndex: 2, secrets: []string{"library"}}
	if _, _, err := p.SupportFile(); err == nil {
		t.Fatal("credential-bearing JVM data persisted")
	}
	p.arguments = []string{"-cp", "/library.jar", "-classpath", "/other.jar", "Main", "secret"}
	p.mainIndex = 4
	if _, _, err := p.SupportFile(); err == nil || p.ProcessArguments("/support.jar") != nil {
		t.Fatal("ambiguous classpath accepted")
	}
	o := options(t, "1.20.4")
	o.Version.Java.Major, o.Java.Major = 8, 17
	if _, err := Build(o, session(t)); err == nil {
		t.Fatal("newer JVM substituted for Java 8")
	}
}
