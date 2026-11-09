package meta

import (
	"encoding/json"
	"testing"
)

// Reproduce the historical LWJGL OS-version predicate, not a full official
// client fixture. Missing host version must still fail, never skip this rule.
func TestLegacyLWJGLOSVersionRuleUsesHostSnapshot(t *testing.T) {
	var rules []Rule
	if err := json.Unmarshal([]byte(`[{"action":"allow"},{"action":"disallow","os":{"version":"^10\\.5\\.\\d$"}}]`), &rules); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		os, version string
		allowed     bool
	}{
		{"windows", "10.0", true}, {"linux", "6.8.0-52-generic", true},
		{"osx", "15.2.1", true}, {"osx", "10.5.8", false},
	} {
		got, err := Allowed(rules, Platform{OS: test.os, Arch: "amd64", Version: test.version})
		if err != nil || got != test.allowed {
			t.Fatalf("%s/%s: %v %v", test.os, test.version, got, err)
		}
	}
	if _, err := Allowed(rules, Platform{OS: "windows", Arch: "amd64"}); err == nil {
		t.Fatal("missing OS version silently bypassed rule")
	}
	var otherOS []Rule
	if err := json.Unmarshal([]byte(`[{"action":"allow"},{"action":"disallow","os":{"name":"osx","version":"^10\\.5\\.\\d$"}}]`), &otherOS); err != nil {
		t.Fatal(err)
	}
	if got, err := Allowed(otherOS, Platform{OS: "windows", Arch: "amd64"}); err != nil || !got {
		t.Fatal("unrelated OS rule unnecessarily required an OS version", err)
	}
}
