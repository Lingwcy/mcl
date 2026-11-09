package platform

import (
	"errors"
	"testing"

	"mcl/internal/core/meta"
)

func TestHostPlatformPreservesNativeVersionAndIdentity(t *testing.T) {
	for _, test := range []struct{ os, arch, version string }{
		{"osx", "arm64", "15.2.1"}, {"windows", "amd64", "10.0"}, {"linux", "amd64", "6.8.0-52-generic"},
	} {
		calls := 0
		got, err := hostPlatform(meta.Platform{OS: test.os, Arch: test.arch}, func() (string, error) { calls++; return test.version, nil })
		if err != nil || got.OS != test.os || got.Arch != test.arch || got.Version != test.version || calls != 1 {
			t.Fatalf("wrong snapshot: %+v %v", got, err)
		}
	}
}

func TestHostPlatformNeverInventsVersionOnFailure(t *testing.T) {
	sentinel := errors.New("synthetic native version failure")
	got, err := hostPlatform(meta.Platform{OS: "windows", Arch: "amd64"}, func() (string, error) { return "", sentinel })
	if !errors.Is(err, sentinel) || got.Version != "" || got.OS != "windows" || got.Arch != "amd64" {
		t.Fatal("OS probe failure was hidden", got, err)
	}
	for _, value := range []string{"", "   ", string([]byte{49, 48, 10, 48})} {
		got, err := hostPlatform(meta.Platform{OS: "osx", Arch: "amd64"}, func() (string, error) { return value, nil })
		if err == nil || got.Version != "" {
			t.Fatal("missing/invalid version replaced with a guessed value")
		}
	}
}
