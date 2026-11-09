package storage

import (
	"mcl/internal/core/diagnostics"
	"testing"
)

func TestTaskStoreReadsLegacyAndRoundTripsBoundedEvidence(t *testing.T) {
	store := NewDownloadTaskStore(t.TempDir())
	group := storedTask()
	if err := store.Create(t.Context(), group); err != nil {
		t.Fatal(err)
	}
	old, err := store.Get(t.Context(), group.ID)
	if err != nil || old.Schema != 1 {
		t.Fatal(old, err)
	}
	group.Schema = 2
	ctx := diagnostics.WithObserver(t.Context(), group.Evidence.Apply)
	_, span := diagnostics.Start(ctx, "network", "libraries/a.jar")
	span.Network("forge", 403, 1)
	span.End()
	if err := store.Save(t.Context(), group); err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(t.Context(), group.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Schema != 2 || len(got.Evidence.Records) != 1 || got.Evidence.Records[0].Code != "forbidden" {
		t.Fatal(got)
	}
}
