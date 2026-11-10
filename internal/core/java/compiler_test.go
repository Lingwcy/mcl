package java

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCompilerEvidenceStaysWithSelectedJavaHome(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, "bin"), 0700); err != nil {
		t.Fatal(err)
	}
	selected := filepath.Join(home, "bin", "java")
	if err := os.WriteFile(selected, []byte("fixture"), 0700); err != nil {
		t.Fatal(err)
	}
	r := Runtime{Executable: selected, HomeDirectory: home, Major: 17}
	if got := inspectCompiler(r); got != "absent" {
		t.Fatal(got)
	}
	if err := os.WriteFile(filepath.Join(home, "bin", compilerName()), []byte("fixture"), 0700); err != nil {
		t.Fatal(err)
	}
	if got := inspectCompiler(r); got != "present" {
		t.Fatal(got)
	}
	r.HomeDirectory = t.TempDir()
	if got := inspectCompiler(r); got != "unknown" {
		t.Fatal("foreign reported home supplied compiler evidence", got)
	}
	r.HomeDirectory = home
	nested := filepath.Join(home, "jre", "bin", "java")
	if err := os.MkdirAll(filepath.Dir(nested), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(nested, []byte("fixture"), 0700); err != nil {
		t.Fatal(err)
	}
	r.Executable, r.HomeDirectory, r.Major = nested, filepath.Dir(filepath.Dir(nested)), 8
	if got := inspectCompiler(r); got != "present" {
		t.Fatal("Java8 nested JRE lost same-JDK compiler evidence", got)
	}
}
