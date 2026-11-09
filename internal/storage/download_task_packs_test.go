package storage

import (
	"os"
	"path/filepath"
	"testing"

	"mcl/internal/app"
	"mcl/internal/core/instance"
	"mcl/internal/core/market"
)

func TestPackPublicationCannotAdoptOtherArchiveOrEditedPayload(t *testing.T) {
	plan := app.ArchiveImportPlan{Instance: directoryValue(), Archive: market.File{SHA1: "1111111111111111111111111111111111111111", Size: 12}}
	prepared, published := t.TempDir(), t.TempDir()
	directoryWrite(t, prepared, "game/mods/a.jar", "fixed")
	directoryWrite(t, published, "game/mods/a.jar", "fixed")
	a, err := os.OpenRoot(prepared)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	b, err := os.OpenRoot(published)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	if err := writeDirectoryJSON(t.Context(), b, packPublicationOwner, packPublicationFor(plan)); err != nil {
		t.Fatal(err)
	}
	if err := readPackPublication(t.Context(), b, plan); err != nil {
		t.Fatal(err)
	}
	if err := samePackPayload(t.Context(), a, b); err != nil {
		t.Fatal(err)
	}
	other := plan
	other.Archive.SHA1 = "2222222222222222222222222222222222222222"
	if err := readPackPublication(t.Context(), b, other); err == nil {
		t.Fatal("foreign archive adopted")
	}
	other = plan
	other.Instance.ID = instance.NewID()
	if err := readPackPublication(t.Context(), b, other); err == nil {
		t.Fatal("foreign instance adopted")
	}
	if err := os.WriteFile(filepath.Join(published, "game/mods/a.jar"), []byte("edited"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := samePackPayload(t.Context(), a, b); err == nil {
		t.Fatal("edited orphan payload silently reused")
	}
	data, _ := os.ReadFile(filepath.Join(published, "game/mods/a.jar"))
	if string(data) != "edited" {
		t.Fatal("conflicting payload was overwritten")
	}
}
