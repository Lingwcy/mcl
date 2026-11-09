package meta

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// All historical documents here are synthetic, derived from the existing
// modern fixture schema. Their hashes are NOT real historical release hashes.
func historicalFixture(t *testing.T, id string) map[string]any {
	t.Helper()
	var value map[string]any
	if err := json.Unmarshal(fixture(t, "1.20.4.json"), &value); err != nil {
		t.Fatal(err)
	}
	value["id"] = id
	delete(value, "arguments")
	delete(value, "javaVersion")
	value["minecraftArguments"] = "--username ${auth_player_name} --version ${version_name} --gameDir ${game_directory} --assetsDir ${assets_root} --assetIndex ${assets_index_name} --uuid ${auth_uuid} --accessToken ${auth_access_token} --userProperties ${user_properties}"
	return value
}

type noMetadataNetwork struct{}

func (noMetadataNetwork) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("unexpected network in offline fixture")
}

func fixtureVersionClient(t *testing.T, documents ...map[string]any) (*Client, map[string]ManifestEntry, map[string][]byte) {
	t.Helper()
	client := NewClient(t.TempDir())
	client.HTTP = &http.Client{Transport: noMetadataNetwork{}}
	entries := make(map[string]ManifestEntry)
	bodies := make(map[string][]byte)
	manifest := Manifest{}
	for _, document := range documents {
		body, err := json.Marshal(document)
		if err != nil {
			t.Fatal(err)
		}
		id := document["id"].(string)
		sum := fmt.Sprintf("%x", sha1.Sum(body))
		kind, _ := document["type"].(string)
		if kind == "" {
			kind = "release"
		}
		entry := ManifestEntry{ID: id, Type: kind, SHA1: sum, URL: "https://piston-meta.mojang.com/v1/packages/" + sum + "/" + id + ".json"}
		entries[id], bodies[entry.URL] = entry, body
		manifest.Versions = append(manifest.Versions, entry)
	}
	client.VerifiedFetch = func(ctx context.Context, address, digest string, limit int64) ([]byte, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		body, ok := bodies[address]
		if !ok {
			return nil, errors.New("unknown fixture document")
		}
		return slices.Clone(body), nil
	}
	encoded, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(client.CacheDir, "manifest.json"), encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	return client, entries, bodies
}

func TestVerifiedLegacyNormalizationPreservesRawCache(t *testing.T) {
	raw := historicalFixture(t, "1.12.2")
	client, entries, bodies := fixtureVersionClient(t, raw)
	entry := entries["1.12.2"]
	version, err := client.Version(t.Context(), entry)
	if err != nil {
		t.Fatal(err)
	}
	if version.Java.Major != 8 || version.Arguments == nil || version.LegacyArguments != "" || len(version.Sources) != 1 {
		t.Fatal("legacy document was not normalized")
	}
	if err := version.ValidateVanilla(); err != nil {
		t.Fatal(err)
	}
	for _, target := range []struct {
		os  string
		mac bool
	}{{"osx", true}, {"windows", false}, {"linux", false}} {
		args, err := SelectArguments(version.Arguments.JVM, Platform{OS: target.os, Arch: "amd64"})
		if err != nil || slices.Contains(args, "-XstartOnFirstThread") != target.mac || !slices.Contains(args, "-cp") {
			t.Fatalf("legacy JVM rules: %v %v", args, err)
		}
	}
	cached, err := os.ReadFile(filepath.Join(client.CacheDir, "versions", "mojang-v1", entry.SHA1+".json"))
	if err != nil || !bytes.Equal(cached, bodies[entry.URL]) {
		t.Fatal("transformed metadata was cached under raw digest")
	}
	client.VerifiedFetch = func(context.Context, string, string, int64) ([]byte, error) { return nil, errors.New("offline") }
	if _, err := client.Version(t.Context(), entry); err != nil {
		t.Fatal("verified immutable cache failed", err)
	}
}

func TestOfficialInheritanceComposesIdentityAndArguments(t *testing.T) {
	parent := historicalFixture(t, "1.12.2")
	child := map[string]any{"id": "child", "inheritsFrom": "1.12.2", "mainClass": "example.Child", "arguments": map[string]any{"game": []string{"--child", "child-value"}, "jvm": []string{"-Dchild=true"}}}
	client, entries, _ := fixtureVersionClient(t, parent, child)
	version, err := client.Version(t.Context(), entries["child"])
	if err != nil {
		t.Fatal(err)
	}
	if version.ID != "child" || version.MainClass != "example.Child" || version.ClientVersion != "1.12.2" || version.InheritsFrom != "" || version.Java.Major != 8 || len(version.Sources) != 2 {
		t.Fatal("inherited identities were lost")
	}
	args, _ := SelectArguments(version.Arguments.Game, Platform{OS: "linux", Arch: "amd64"})
	if !slices.Contains(args, "--username") || !slices.Contains(args, "--child") {
		t.Fatal("parent/child arguments were not composed")
	}
	// A declared client replaces the parent's client; no arbitrary jar lookup.
	child["downloads"] = map[string]any{"client": parent["downloads"].(map[string]any)["client"]}
	client, entries, _ = fixtureVersionClient(t, parent, child)
	version, err = client.Version(t.Context(), entries["child"])
	if err != nil || version.ClientVersion != "child" {
		t.Fatalf("client override: %+v %v", version, err)
	}
}

func TestLegacyChildReplacesGameTemplateAndDoesNotAliasParent(t *testing.T) {
	parentDocument := historicalFixture(t, "1.12.2")
	parentBytes, _ := json.Marshal(parentDocument)
	parent, err := ParseVersion(parentBytes)
	if err != nil {
		t.Fatal(err)
	}
	parent, err = normalizeOfficial(parent)
	if err != nil {
		t.Fatal(err)
	}
	childBytes, _ := json.Marshal(map[string]any{"id": "child", "type": "release", "inheritsFrom": "1.12.2", "minecraftArguments": "--username ${auth_player_name} --custom 'two words'"})
	child, err := ParseVersion(childBytes)
	if err != nil {
		t.Fatal(err)
	}
	before := cloneVersion(parent)
	merged, err := mergeOfficial(parent, child)
	if err != nil {
		t.Fatal(err)
	}
	values, _ := SelectArguments(merged.Arguments.Game, Platform{})
	if !reflect.DeepEqual(values, []string{"--username", "${auth_player_name}", "--custom", "two words"}) {
		t.Fatalf("legacy replacement %v", values)
	}
	merged.Arguments.JVM[0].Values[0] = "changed"
	merged.Libraries[0].Downloads.Artifact.SHA1 = strings.Repeat("b", 40)
	if !reflect.DeepEqual(parent, before) {
		t.Fatal("composed version aliases parent data")
	}
}

func TestOfficialInheritanceFailuresAreClosed(t *testing.T) {
	cases := []struct {
		name      string
		documents []map[string]any
		selectID  string
	}{
		{"cycle", []map[string]any{{"id": "a", "inheritsFrom": "b"}, {"id": "b", "inheritsFrom": "a"}}, "a"},
		{"missing-parent", []map[string]any{{"id": "a", "inheritsFrom": "missing"}}, "a"},
		{"unknown-java", []map[string]any{historicalFixture(t, "unreviewed-release")}, "unreviewed-release"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			client, entries, _ := fixtureVersionClient(t, test.documents...)
			if _, err := client.Version(t.Context(), entries[test.selectID]); err == nil {
				t.Fatal("invalid historical metadata accepted")
			}
		})
	}
	parent := historicalFixture(t, "1.12.2")
	for _, change := range []map[string]any{
		{"javaVersion": nil}, {"arguments": nil}, {"mainClass": ""}, {"minecraftArguments": ""}, {"jar": "unrelated"}, {"patches": []any{map[string]any{"id": "custom"}}},
	} {
		child := map[string]any{"id": "child", "inheritsFrom": "1.12.2"}
		for key, value := range change {
			child[key] = value
		}
		client, entries, _ := fixtureVersionClient(t, parent, child)
		if _, err := client.Version(t.Context(), entries["child"]); err == nil {
			t.Fatalf("invalid override accepted: %v", change)
		}
	}
	var chain []map[string]any
	for i := range maxInheritanceDepth + 1 {
		chain = append(chain, map[string]any{"id": fmt.Sprintf("node%d", i), "inheritsFrom": fmt.Sprintf("node%d", i+1)})
	}
	client, entries, _ := fixtureVersionClient(t, chain...)
	if _, err := client.Version(t.Context(), entries["node0"]); err == nil || !strings.Contains(err.Error(), "8层") {
		t.Fatalf("depth limit: %v", err)
	}
}

func TestOfficialProvenanceAndDigestCannotBeInferredFromName(t *testing.T) {
	document := historicalFixture(t, "1.12.2")
	client, entries, bodies := fixtureVersionClient(t, document)
	entry := entries["1.12.2"]
	for _, url := range []string{"https://example.invalid/1.12.2.json", strings.ReplaceAll(entry.URL, "piston-meta.mojang.com", "piston-meta.mojang.com.evil.invalid"), entry.URL + "?query=1"} {
		changed := entry
		changed.URL = url
		bodies[url] = bodies[entry.URL]
		if _, err := client.Version(t.Context(), changed); err == nil {
			t.Fatal("release ID alone authorized legacy normalization")
		}
	}
	// A wrong digest cannot use the verified cache and cannot be normalized.
	entry.SHA1 = strings.Repeat("a", 40)
	entry.URL = "https://piston-meta.mojang.com/v1/packages/" + entry.SHA1 + "/1.12.2.json"
	for _, body := range bodies {
		bodies[entry.URL] = body
		break
	}
	if _, err := client.Version(t.Context(), entry); err == nil {
		t.Fatal("wrong digest accepted")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := client.Version(ctx, entries["1.12.2"]); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel became cache success: %v", err)
	}
}

func TestGeneralCacheCannotAcquireOfficialProvenance(t *testing.T) {
	client, entries, bodies := fixtureVersionClient(t, historicalFixture(t, "1.12.2"))
	official := entries["1.12.2"]
	general := official
	general.URL = "https://example.invalid/synthetic.json"
	bodies[general.URL] = bodies[official.URL]
	if _, err := client.Version(t.Context(), general); err == nil {
		t.Fatal("general legacy endpoint accepted")
	}
	client.VerifiedFetch = func(context.Context, string, string, int64) ([]byte, error) {
		return nil, errors.New("official origin unavailable")
	}
	if _, err := client.Version(t.Context(), official); err == nil {
		t.Fatal("same-hash general cache was promoted to official provenance")
	}
}
