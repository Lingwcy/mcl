package install

import (
	"context"
	"crypto/sha1"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"strings"
	"testing"

	"mcl/internal/core/download"
	"mcl/internal/core/instance"
)

func legacyAssetFixture(t *testing.T, virtual, resources bool) (*os.Root, *os.Root, string, AssetPlan) {
	t.Helper()
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { root.Close() })
	game, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { game.Close() })
	index := AssetIndex{Objects: map[string]AssetObject{}, Virtual: virtual, MapToResources: resources}
	for name, body := range map[string]string{"sound/a.ogg": "sound-one", "sound/b.ogg": "sound-two"} {
		hash := fmt.Sprintf("%x", sha1.Sum([]byte(body)))
		index.Objects[name] = AssetObject{Hash: hash, Size: int64(len(body))}
		file := "cache/assets/objects/" + hash[:2] + "/" + hash
		if err := root.MkdirAll(path.Dir(file), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := root.WriteFile(file, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	data, _ := json.Marshal(index)
	plan, err := ParseAssetPlan(data)
	if err != nil {
		t.Fatal(err)
	}
	return root, game, instance.NewID(), plan
}

func TestLegacyVirtualAndResourcesAreIndependentVerifiedCopies(t *testing.T) {
	root, game, id, plan := legacyAssetFixture(t, true, true)
	var progress []int
	directory, err := PrepareLegacyAssets(t.Context(), root, game, id, strings.Repeat("a", 40), plan, false, func(p download.Progress) {
		if p.TotalFiles != 4 {
			t.Errorf("wrong combined mapping total %d", p.TotalFiles)
		}
		progress = append(progress, p.Files)
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(progress) != 4 || progress[0] != 1 || progress[3] != 4 {
		t.Fatalf("non-monotonic progress %v", progress)
	}
	for _, mapping := range plan.Mappings {
		original, err := root.Stat(mapping.File.Path)
		if err != nil {
			t.Fatal(err)
		}
		virtual := path.Join(directory, "files", mapping.Name)
		info, err := root.Stat(virtual)
		if err != nil {
			t.Fatal(err)
		}
		if os.SameFile(original, info) {
			t.Fatal("virtual asset hardlinks shared cache")
		}
		if err := verifyAsset(t.Context(), root, virtual, mapping.File.Size, mapping.File.SHA1); err != nil {
			t.Fatal(err)
		}
		if err := verifyAsset(t.Context(), game, "resources/"+mapping.Name, mapping.File.Size, mapping.File.SHA1); err != nil {
			t.Fatal(err)
		}
	}
	// Editing a game copy must not mutate the shared object or virtual generation.
	if err := game.WriteFile("resources/sound/a.ogg", []byte("user edit"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := PrepareLegacyAssets(t.Context(), root, game, id, strings.Repeat("a", 40), plan, false, nil); err == nil {
		t.Fatal("modified resources overwritten")
	}
	body, _ := game.ReadFile("resources/sound/a.ogg")
	if string(body) != "user edit" {
		t.Fatal("user resource changed")
	}
	for _, mapping := range plan.Mappings {
		if err := verifyAsset(t.Context(), root, mapping.File.Path, mapping.File.Size, mapping.File.SHA1); err != nil {
			t.Fatal(err)
		}
	}
}

func TestResourceCancellationCanResumeWithoutRemovingPublishedFiles(t *testing.T) {
	root, game, id, plan := legacyAssetFixture(t, false, true)
	ctx, cancel := context.WithCancel(t.Context())
	_, err := PrepareLegacyAssets(ctx, root, game, id, strings.Repeat("a", 40), plan, false, func(download.Progress) { cancel() })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel: %v", err)
	}
	if _, err := game.Stat("resources/sound/a.ogg"); err != nil {
		t.Fatal("verified published subset was removed")
	}
	if _, err := PrepareLegacyAssets(t.Context(), root, game, id, strings.Repeat("a", 40), plan, false, nil); err != nil {
		t.Fatal("retry failed", err)
	}
	for _, mapping := range plan.Mappings {
		if err := verifyAsset(t.Context(), game, "resources/"+mapping.Name, mapping.File.Size, mapping.File.SHA1); err != nil {
			t.Fatal(err)
		}
	}
}

func TestVirtualCancellationAndCorruptSourcesNeverPublish(t *testing.T) {
	root, game, id, plan := legacyAssetFixture(t, true, false)
	ctx, cancel := context.WithCancel(t.Context())
	if _, err := PrepareLegacyAssets(ctx, root, game, id, strings.Repeat("a", 40), plan, false, func(download.Progress) { cancel() }); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel: %v", err)
	}
	parent := "instances/" + id + "/legacy-assets"
	dir, err := root.Open(parent)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := dir.ReadDir(-1)
	dir.Close()
	if err != nil || len(entries) != 0 {
		t.Fatal("canceled virtual stage retained")
	}
	if err := root.WriteFile(plan.Mappings[0].File.Path, []byte("corrupted"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := PrepareLegacyAssets(t.Context(), root, game, id, strings.Repeat("a", 40), plan, false, nil); err == nil {
		t.Fatal("corrupted source copied")
	}
}

func TestLegacyRecoveryCleansOnlyMarkedStagesAndKeepsReceiptGeneration(t *testing.T) {
	root, game, id, plan := legacyAssetFixture(t, true, false)
	keep, err := PrepareLegacyAssets(t.Context(), root, game, id, strings.Repeat("a", 40), plan, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	abandoned, err := PrepareLegacyAssets(t.Context(), root, game, id, strings.Repeat("a", 40), plan, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	parent := path.Dir(keep)
	unknown := path.Join(parent, "ready-"+strings.Repeat("B", 26))
	if err := root.Mkdir(unknown, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := CleanupLegacyAssets(t.Context(), root, id, keep); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{keep, unknown} {
		if _, err := root.Stat(name); err != nil {
			t.Fatal("retained/unknown generation removed")
		}
	}
	if _, err := root.Stat(abandoned); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("abandoned marked generation retained")
	}
	stageName := ".mcl-resource-stage-" + strings.Repeat("C", 26)
	if err := game.Mkdir(stageName, 0o700); err != nil {
		t.Fatal(err)
	}
	stage, err := game.OpenRoot(stageName)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeAssetOwner(t.Context(), stage, assetOwner{1, "resources-stage", id, strings.Repeat("C", 26), strings.Repeat("a", 40)}); err != nil {
		t.Fatal(err)
	}
	stage.Close()
	plan.Virtual = false
	plan.MapToResources = true
	if _, err := PrepareLegacyAssets(t.Context(), root, game, id, strings.Repeat("a", 40), plan, false, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := game.Stat(stageName); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("owned resources stage not recovered")
	}
}

func TestResourceLinksAndMissingGameHandleAreRejected(t *testing.T) {
	root, game, id, plan := legacyAssetFixture(t, false, true)
	if _, err := PrepareLegacyAssets(t.Context(), root, nil, id, strings.Repeat("a", 40), plan, false, nil); err == nil {
		t.Fatal("missing validated game root fell back")
	}
	if err := game.Mkdir("foreign", 0o700); err != nil {
		t.Fatal(err)
	}
	if err := game.Symlink("foreign", "resources"); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := PrepareLegacyAssets(t.Context(), root, game, id, strings.Repeat("a", 40), plan, false, nil); err == nil {
		t.Fatal("in-root link redirected resource writes")
	}
}

func TestExplicitGameAssetsRequestAndReceiptPathBounds(t *testing.T) {
	root, game, id, plan := legacyAssetFixture(t, false, false)
	directory, err := PrepareLegacyAssets(t.Context(), root, game, id, strings.Repeat("a", 40), plan, true, nil)
	if err != nil || directory == "" {
		t.Fatal("explicit game_assets did not prepare named files", err)
	}
	parent := "instances/" + id + "/legacy-assets"
	if !ValidLegacyAssetsDirectory(parent, directory) || ValidLegacyAssetsDirectory(parent, "instances/other/legacy-assets/ready-"+strings.Repeat("A", 26)) {
		t.Fatal("legacy receipt paths not confined")
	}
	if err := RemoveLegacyAssets(t.Context(), root, id, directory); err != nil {
		t.Fatal(err)
	}
}
