package meta

import (
	"encoding/json"
	"slices"
	"testing"
)

func TestLegacyMacTemplateDoesNotForceGLFWFirstThread(t *testing.T) {
	for _, id := range []string{"1.6.1", "1.7.10", "1.12.2"} {
		client, entries, _ := fixtureVersionClient(t, historicalFixture(t, id))
		// Read twice so immutable cached raw metadata is normalized with the same
		// corrected defaults; no cache deletion or source fingerprint change needed.
		for range 2 {
			v, err := client.Version(t.Context(), entries[id])
			if err != nil {
				t.Fatal(err)
			}
			for _, os := range []string{"osx", "windows", "linux"} {
				args, err := SelectArguments(v.Arguments.JVM, Platform{OS: os, Arch: "amd64"})
				if err != nil || slices.Contains(args, "-XstartOnFirstThread") || !slices.Contains(args, "-cp") || !slices.Contains(args, "${classpath}") || !slices.Contains(args, "-Djava.library.path=${natives_directory}") {
					t.Fatal("legacy template inherited GLFW threading or lost launch paths", id, os, args, err)
				}
			}
		}
	}
}
func TestModernExplicitMacFirstThreadRuleIsPreserved(t *testing.T) {
	var raw map[string]any
	if err := json.Unmarshal(fixture(t, "1.20.4.json"), &raw); err != nil {
		t.Fatal(err)
	}
	client, entries, _ := fixtureVersionClient(t, raw)
	v, err := client.Version(t.Context(), entries["1.20.4"])
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range []struct {
		os    string
		first bool
	}{{"osx", true}, {"windows", false}, {"linux", false}} {
		args, err := SelectArguments(v.Arguments.JVM, Platform{OS: target.os, Arch: "amd64"})
		if err != nil || slices.Contains(args, "-XstartOnFirstThread") != target.first {
			t.Fatal("modern native metadata rule changed", target.os, args, err)
		}
	}
}
