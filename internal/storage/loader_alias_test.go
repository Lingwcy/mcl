package storage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mcl/internal/core/loader"
)

func aliasStoreRequest() loader.Request {
	return loader.Request{Type: "forge", GameVersion: "1.7.10", Version: "10.13.4.1614", BaseSHA1: strings.Repeat("a", 40), TargetOS: "osx", TargetArch: "amd64"}
}
func TestLoaderAliasNeedsCompletedCanonicalProfile(t *testing.T) {
	root := t.TempDir()
	store := NewLoaderStore(root)
	requested := aliasStoreRequest()
	if got, err := store.LoadRequestAlias(t.Context(), requested); err != nil || got != requested {
		t.Fatal(got, err)
	}
	canonical := requested
	canonical.Version += "-1.7.10"
	if err := store.SaveRequestAlias(t.Context(), requested, canonical); err == nil {
		t.Fatal("alias published without a validated runtime profile")
	}
	if _, err := os.Stat(filepath.Join(root, "cache", "loader-aliases", requested.Key()+".json")); !os.IsNotExist(err) {
		t.Fatal("incomplete alias created", err)
	}
	canonical.Version = "10.13.4.9999-1.7.10"
	if err := store.SaveRequestAlias(t.Context(), requested, canonical); err == nil {
		t.Fatal("another build published as pack alias")
	}
}
func TestLoaderAliasBindsOriginalRequestAndRejectsMalformedCache(t *testing.T) {
	root := t.TempDir()
	store := NewLoaderStore(root)
	requested := aliasStoreRequest()
	canonical := requested
	canonical.Version += "-1.7.10"
	parent := filepath.Join(root, "cache", "loader-aliases")
	if err := os.MkdirAll(parent, 0700); err != nil {
		t.Fatal(err)
	}
	name := filepath.Join(parent, requested.Key()+".json")
	body, _ := json.Marshal(loaderAliasDocument{Schema: 1, Requested: requested, Canonical: canonical})
	if err := os.WriteFile(name, body, 0600); err != nil {
		t.Fatal(err)
	}
	if got, err := store.LoadRequestAlias(t.Context(), requested); err != nil || got != canonical {
		t.Fatal(got, err)
	}
	canonical.TargetArch = "arm64"
	bad, _ := json.Marshal(loaderAliasDocument{Schema: 1, Requested: requested, Canonical: canonical})
	if err := os.WriteFile(name, bad, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.LoadRequestAlias(t.Context(), requested); err == nil {
		t.Fatal("target mismatch read as valid alias")
	}
	if after, _ := os.ReadFile(name); string(after) != string(bad) {
		t.Fatal("bad mapping was silently overwritten")
	}
	if err := os.WriteFile(name, []byte("broken-json"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.LoadRequestAlias(t.Context(), requested); err == nil {
		t.Fatal("corrupt mapping was ignored")
	}
}
