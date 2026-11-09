package install

import (
	"context"
	"crypto/sha1"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"testing"

	"mcl/internal/core/download"
	"mcl/internal/core/meta"
)

type noAssetNetwork struct{}

func (noAssetNetwork) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("unexpected network: fixture should reuse verified cache")
}

func TestExecutePreparesPinnedHistoricalLayoutsAndReturnsGeneration(t *testing.T) {
	root, game, id, assets := legacyAssetFixture(t, true, true)
	index := AssetIndex{Objects: map[string]AssetObject{}, Virtual: true, MapToResources: true}
	for _, mapping := range assets.Mappings {
		index.Objects[mapping.Name] = AssetObject{Hash: mapping.File.SHA1, Size: mapping.File.Size}
	}
	data, _ := json.Marshal(index)
	hash := fmt.Sprintf("%x", sha1.Sum(data))
	name := "cache/assets/indexes/legacy-test.json"
	if err := root.MkdirAll(path.Dir(name), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := root.WriteFile(name, data, 0o600); err != nil {
		t.Fatal(err)
	}
	file := download.File{Path: name, Download: meta.Download{URL: "https://example.invalid/index.json", Size: int64(len(data)), SHA1: hash}}
	plan := Plan{VersionID: "synthetic", AssetIndexPath: name, Files: []download.File{file}, NeedsGameAssets: true}
	client := download.NewClient(root.Name())
	client.HTTP = &http.Client{Transport: noAssetNetwork{}}
	result, err := Execute(t.Context(), client, plan, "instances/"+id+"/natives", nil, game)
	if err != nil {
		t.Fatal(err)
	}
	if result.LegacyAssetsDirectory == "" || result.NativeDirectory == "" {
		t.Fatal("missing runtime generation")
	}
	for _, mapping := range assets.Mappings {
		if err := verifyAsset(t.Context(), root, path.Join(result.LegacyAssetsDirectory, "files", mapping.Name), mapping.File.Size, mapping.File.SHA1); err != nil {
			t.Fatal(err)
		}
	}
	// A replaced index after the file-download stage must not control mapping.
	_, err = Execute(t.Context(), client, plan, "instances/"+id+"/natives", func(p Progress) {
		if p.Stage == "files" && p.Files == p.TotalFiles {
			root.WriteFile(name, []byte(strings.Repeat("x", len(data))), 0o600)
		}
	}, game)
	if err == nil {
		t.Fatal("replaced verified index accepted")
	}
}

func TestMappedAssetDescriptorDetectsReplacement(t *testing.T) {
	root, _, _, plan := legacyAssetFixture(t, true, false)
	mapping := plan.Mappings[0]
	f, before, err := openAssetFile(root, mapping.File.Path, mapping.File.Size)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := root.Rename(mapping.File.Path, mapping.File.Path+".old"); err != nil {
		t.Fatal(err)
	}
	if err := root.WriteFile(mapping.File.Path, []byte(strings.Repeat("z", int(mapping.File.Size))), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := sameAssetFile(root, mapping.File.Path, f, before); err == nil {
		t.Fatal("pathname replacement went unnoticed")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := streamAsset(ctx, f, io.Discard, mapping.File.Size, mapping.File.SHA1); !errors.Is(err, context.Canceled) {
		t.Fatal("canceled stream continued")
	}
}
