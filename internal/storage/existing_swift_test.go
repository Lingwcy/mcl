package storage

import (
	"context"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"mcl/internal/app"
	"mcl/internal/core/discovery"
)

func swiftFixture(t *testing.T) (string, string, discovery.SwiftProfile) {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	name := "Fabulously Optimized-26.1-20260611-223603"
	profile := filepath.Join(root, "profiles", name)
	for _, name := range []string{profile, filepath.Join(root, "meta"), filepath.Join(root, "data")} {
		if err := os.MkdirAll(name, 0700); err != nil {
			t.Fatal(err)
		}
	}
	image := make([]byte, 512)
	copy(image, "SQLite format 3\x00")
	binary.BigEndian.PutUint16(image[16:18], 512)
	image[18], image[19] = 2, 2
	binary.BigEndian.PutUint32(image[56:60], 1)
	if err := os.WriteFile(filepath.Join(root, "data/data.db"), image, 0600); err != nil {
		t.Fatal(err)
	}
	return root, profile, discovery.SwiftProfile{ID: "E82FC5E4-4AC8-4124-8C76-ADE81B114D37", WorkingPath: root, Name: name, GameVersion: "26.1", Loader: "fabric", LoaderVersion: "0.18.0", MainClass: "net.fabricmc.loader.impl.launch.knot.KnotClient"}
}
func TestSwiftDiscoveryAcceptsLauncherProfilesAndSpecificProfile(t *testing.T) {
	for _, choice := range []string{"launcher", "profiles", "profile"} {
		t.Run(choice, func(t *testing.T) {
			root, profile, row := swiftFixture(t)
			selected := root
			if choice == "profiles" {
				selected = filepath.Join(root, "profiles")
			}
			if choice == "profile" {
				selected = profile
			}
			reader := func(ctx context.Context, image []byte, name string) ([]discovery.SwiftProfile, error) {
				if image[18] != 1 || image[19] != 1 {
					t.Fatal("foreign WAL mode passed into native reader")
				}
				if choice == "profile" && name != row.Name {
					t.Fatal("explicit profile scope widened")
				}
				return []discovery.SwiftProfile{row}, nil
			}
			data := t.TempDir()
			store := NewExistingGamesStore(data, reader)
			source, err := store.Add(t.Context(), selected)
			if err != nil {
				t.Fatal(err)
			}
			if source.Path != selected {
				t.Fatal("registration widened or redirected selected source")
			}
			report, err := store.Scan(t.Context(), source.ID)
			if err != nil || len(report.Candidates) != 1 {
				t.Fatal(report, err)
			}
			candidate := report.Candidates[0]
			if candidate.GameVersion != "26.1" || candidate.Format != "swift" || len(candidate.GamePaths) != 1 || candidate.GamePaths[0] != profile || candidate.UnknownComponents || candidate.Loaders[0].Version != "0.18.0" {
				t.Fatal(candidate)
			}
			copies := NewDirectoryTransferStore(data, reader)
			_, _, tree, err := copies.InspectSource(t.Context(), app.DirectoryImportRequest{SourceID: source.ID, CandidateID: candidate.ID, DataPath: profile, Groups: []string{"saves"}}, nil)
			if err != nil || len(tree.Files) != 0 {
				t.Fatal("D2 source recheck lost Swift format", err)
			}
			for _, name := range []string{"existing-sources.json", "owner.json", "data.db-shm", "data.db-journal"} {
				if _, err := os.Stat(filepath.Join(root, name)); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("foreign source received launcher state", name)
				}
			}
		})
	}
}
func TestSwiftDiscoveryRejectsChangedIndexAndDoesNotReadDebugConfig(t *testing.T) {
	root, profile, row := swiftFixture(t)
	// This is Minecraft's unrelated debug-HUD configuration, not a version
	// manifest. It must never be used as a game-version guess.
	if err := os.WriteFile(filepath.Join(profile, "debug-profile.json"), []byte("not-version-metadata"), 0600); err != nil {
		t.Fatal(err)
	}
	reader := func(ctx context.Context, image []byte, name string) ([]discovery.SwiftProfile, error) {
		if err := os.WriteFile(filepath.Join(root, "data/data.db-wal"), []byte("appeared during query"), 0600); err != nil {
			t.Fatal(err)
		}
		return []discovery.SwiftProfile{row}, nil
	}
	store := NewExistingGamesStore(t.TempDir(), reader)
	source, err := store.Add(t.Context(), profile)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Scan(t.Context(), source.ID); err == nil {
		t.Fatal("mixed database/WAL generations accepted")
	}
}
func TestSwiftDiscoveryCannotRedirectOutsideSelectedProfile(t *testing.T) {
	root, profile, row := swiftFixture(t)
	outside := row
	outside.ID = "F82FC5E4-4AC8-4124-8C76-ADE81B114D37"
	outside.Name = "../outside"
	other := row
	other.ID = "A82FC5E4-4AC8-4124-8C76-ADE81B114D37"
	other.Name = "other-profile"
	reader := func(context.Context, []byte, string) ([]discovery.SwiftProfile, error) {
		return []discovery.SwiftProfile{row, outside, other}, nil
	}
	store := NewExistingGamesStore(t.TempDir(), reader)
	source, err := store.Add(t.Context(), profile)
	if err != nil {
		t.Fatal(err)
	}
	report, err := store.Scan(t.Context(), source.ID)
	if err != nil || len(report.Candidates) != 1 || report.Candidates[0].Path != profile {
		t.Fatal("profile scope escaped", report, err)
	}
	// An app data root under related Swift metadata is protected even when the
	// chosen game directory is only one sibling profile.
	nested := NewExistingGamesStore(filepath.Join(root, "data", "mcl"), reader)
	if _, err := nested.Add(t.Context(), profile); err == nil {
		t.Fatal("related foreign metadata allowed as mcl write root")
	}
}
