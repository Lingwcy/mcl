package meta

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestLegacyArgumentLexerDoesNotSplitSubstitutionsOrRunShell(t *testing.T) {
	quote, slash := string(rune(34)), string(rune(92))
	text := "--path " + quote + "C:" + slash + "中文 空格" + slash + "mods" + quote + " --name 'two words' --empty " + quote + quote + " --literal " + quote + "a" + slash + quote + "b" + quote + " --template ${game_directory} --shell $(not-a-command);$HOME"
	args, err := SplitLegacyArguments(text)
	want := []string{"--path", "C:" + slash + "中文 空格" + slash + "mods", "--name", "two words", "--empty", "", "--literal", "a" + quote + "b", "--template", "${game_directory}", "--shell", "$(not-a-command);$HOME"}
	if err != nil || !reflect.DeepEqual(args, want) {
		t.Fatalf("lexed %v, %v", args, err)
	}
	for _, bad := range []string{"", "--name 'unclosed", "x" + string(rune(10)) + "y", string([]byte{255}), strings.Repeat("a", maxLegacyArgumentBytes+1), strings.Repeat("x ", 257), quote + "x" + string(rune(9)) + "y" + quote} {
		if _, err := SplitLegacyArguments(bad); err == nil {
			t.Fatal("invalid legacy template accepted")
		}
	}
}

func TestInheritedLibraryGroupsRetainConditionalRules(t *testing.T) {
	parent := historicalFixture(t, "1.12.2")
	parent["libraries"] = []any{
		map[string]any{"name": "example:lib:1", "rules": []any{map[string]any{"action": "allow", "os": map[string]any{"name": "windows"}}}, "downloads": map[string]any{"artifact": map[string]any{"url": "https://example.invalid/one.jar", "sha1": strings.Repeat("a", 40), "size": 1}}},
		map[string]any{"name": "example:lib:2", "rules": []any{map[string]any{"action": "allow", "os": map[string]any{"name": "linux"}}}, "downloads": map[string]any{"artifact": map[string]any{"url": "https://example.invalid/two.jar", "sha1": strings.Repeat("b", 40), "size": 1}}},
		map[string]any{"name": "example:other:1", "downloads": map[string]any{"artifact": map[string]any{"url": "https://example.invalid/other.jar", "sha1": strings.Repeat("c", 40), "size": 1}}},
	}
	// Parent and child variants have the same module but distinct OS rules.
	// All selected child variants survive, not just the first coordinate.
	children := parent["libraries"].([]any)[:2]
	child := map[string]any{"id": "child", "inheritsFrom": "1.12.2", "libraries": children}
	client, entries, _ := fixtureVersionClient(t, parent, child)
	version, err := client.Version(t.Context(), entries["child"])
	if err != nil {
		t.Fatal(err)
	}
	if len(version.Libraries) != 3 {
		t.Fatal("library group merge lost or duplicated a variant")
	}
	for _, target := range []struct{ os, name string }{{"windows", "example:lib:1"}, {"linux", "example:lib:2"}} {
		var selected []string
		for _, library := range version.Libraries {
			allowed, err := Allowed(library.Rules, Platform{OS: target.os, Arch: "amd64"})
			if err != nil {
				t.Fatal(err)
			}
			if allowed {
				selected = append(selected, library.Name)
			}
		}
		if len(selected) != 2 || !slices.Contains(selected, target.name) || !slices.Contains(selected, "example:other:1") {
			t.Fatalf("conditional variants: %v", selected)
		}
	}
}

func TestHistoricalFallbackAndComposedLimits(t *testing.T) {
	for _, id := range []string{"1.5.2", "1.12.2-forge", "1.17", "unknown"} {
		client, entries, _ := fixtureVersionClient(t, historicalFixture(t, id))
		if _, err := client.Version(t.Context(), entries[id]); err == nil {
			t.Fatal("unreviewed Java fallback accepted")
		}
	}
	raw := historicalFixture(t, "1.12.2")
	raw["type"] = "snapshot"
	client, entries, _ := fixtureVersionClient(t, raw)
	if _, err := client.Version(t.Context(), entries["1.12.2"]); err == nil {
		t.Fatal("snapshot received release-only fallback")
	}
	raw = historicalFixture(t, "1.12.2")
	data, _ := json.Marshal(raw)
	parent, err := ParseVersion(data)
	if err != nil {
		t.Fatal(err)
	}
	parent, err = normalizeOfficial(parent)
	if err != nil {
		t.Fatal(err)
	}
	parent.Arguments.Game = []Argument{{Values: make([]string, maxComposedArguments+1)}}
	if err := compositionBounds(parent); err == nil {
		t.Fatal("multi-value argument bypassed count bound")
	}
	parent.Arguments.Game = []Argument{{Values: []string{strings.Repeat("a", (256<<10)+1)}}}
	if err := compositionBounds(parent); err == nil {
		t.Fatal("argument byte limit bypassed")
	}
}

func TestRawPartialOrPatchMetadataCannotBypassNormalization(t *testing.T) {
	raw := historicalFixture(t, "1.12.2")
	for _, patch := range []map[string]any{
		{"patches": []any{map[string]any{"id": "external"}}},
		{"jar": "other"},
		{"inheritsFrom": "parent"},
	} {
		document := historicalFixture(t, "1.12.2")
		for key, value := range patch {
			document[key] = value
		}
		data, _ := json.Marshal(document)
		version, err := ParseVersion(data)
		if err != nil {
			t.Fatal(err)
		}
		if err := version.ValidateVanilla(); err == nil {
			t.Fatal("raw patch entered installation pipeline")
		}
	}
	raw["javaVersion"] = map[string]any{"majorVersion": 0}
	client, entries, _ := fixtureVersionClient(t, raw)
	if _, err := client.Version(t.Context(), entries["1.12.2"]); err == nil {
		t.Fatal("explicit invalid Java requirement received fallback")
	}
}
