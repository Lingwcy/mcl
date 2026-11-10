package java

import (
	"strings"
	"testing"

	"mcl/internal/core/meta"
)

func TestJava8AliasesNormalizeToSameUpdateAndBuild(t *testing.T) {
	var baseline Version
	for i, text := range []string{"1.8.0_442-b06", "8u442-b06", "8.0.442+6"} {
		got, err := ParseVersion(text)
		if err != nil {
			t.Fatal(text, err)
		}
		if got.Major() != 8 || got.Numbers[2] != 442 || !got.BuildKnown || got.Build != 6 || !got.Stable() {
			t.Fatal(got)
		}
		if i == 0 {
			baseline = got
		} else if got.Compare(baseline) != 0 {
			t.Fatal("equivalent updates compare differently", text, got)
		}
	}
	newer, _ := ParseVersion("1.8.0_482-b01")
	if newer.Compare(baseline) <= 0 {
		t.Fatal("update priority lost to old build")
	}
}
func TestModernVersionIgnoresVendorMetadataNumbers(t *testing.T) {
	a, err := ParseVersion("17.0.12+7-vendor.999")
	if err != nil {
		t.Fatal(err)
	}
	b, err := ParseVersion("17.0.12+7-vendor.1")
	if err != nil {
		t.Fatal(err)
	}
	if a.Compare(b) != 0 {
		t.Fatal("vendor suffix changes Java release ordering")
	}
	patch, _ := ParseVersion("17.0.12.1+1")
	if patch.Compare(a) <= 0 {
		t.Fatal("security patch ignored")
	}
	ea, err := ParseVersion("27-internal+12")
	if err != nil || ea.Stable() {
		t.Fatal("internal build treated as GA", ea, err)
	}
	for _, text := range []string{"", "unknown", "17..1", "17+", "17-ea-", "17.0.1+bad", "17.0.1/secret", strings.Repeat("1", 129)} {
		if _, err := ParseVersion(text); err == nil {
			t.Fatal("invalid version accepted", text)
		}
	}
}
func TestRuntimeChecksItsVersionEvidenceEvenWhenMajorFieldClaimsCompatibility(t *testing.T) {
	target := meta.Platform{OS: "linux", Arch: "amd64"}
	for _, r := range []Runtime{
		{Version: "17.0.12", Major: 21, Arch: "amd64"},
		{Version: "17.0.12", RuntimeVersion: "17.0.11+9", Major: 17, Arch: "amd64"},
		{Version: "17.0.12+7", RuntimeVersion: "17.0.12+8", Major: 17, Arch: "amd64"},
		{Version: "17-internal+1", Major: 17, Arch: "amd64"},
	} {
		if err := r.Check(17, target); err == nil {
			t.Fatal("inconsistent/EA runtime accepted", r)
		}
	}
	r, err := ParseProperties("java.version = 1.8.0_442\njava.runtime.version = 1.8.0_442-b06\nos.arch = amd64\njava.vm.name = OpenJDK 64-Bit Server VM")
	if err != nil {
		t.Fatal(err)
	}
	version, err := r.NormalizedVersion()
	if err != nil || version.Build != 6 || !version.BuildKnown {
		t.Fatal(version, err)
	}
}
func TestFixedPackageBuildIsRecheckedAfterProbe(t *testing.T) {
	p := Package{Provider: "temurin", Version: "8.0.442+6.0.LTS", Major: 8}
	r := Runtime{Version: "1.8.0_442", RuntimeVersion: "1.8.0_442-b06", Major: 8, Arch: "amd64"}
	if err := p.CheckRuntime(r); err != nil {
		t.Fatal(err)
	}
	r.RuntimeVersion = "1.8.0_442-b07"
	if err := p.CheckRuntime(r); err == nil {
		t.Fatal("different fixed build accepted")
	}
	r.RuntimeVersion = ""
	if err := p.CheckRuntime(r); err == nil {
		t.Fatal("missing fixed build evidence accepted")
	}
	zulu := Package{Provider: "zulu", Version: "8.0.442", Major: 8}
	if err := zulu.CheckRuntime(r); err != nil {
		t.Fatal("catalog without build invented a build requirement", err)
	}
	r.Version = "1.8.0_432"
	if err := zulu.CheckRuntime(r); err == nil {
		t.Fatal("wrong update accepted")
	}
}
