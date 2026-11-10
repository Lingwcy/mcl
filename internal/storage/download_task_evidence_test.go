package storage

import (
	"testing"

	"mcl/internal/core/diagnostics"
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
	group.Schema = 3
	diagnostics.SetFacts(ctx, diagnostics.Facts{HostArch: "arm64", LauncherArch: "amd64", HostArchSource: "sysctl-hardware", Desktop: "quartz-unchecked", WindowObserver: "quartz-onscreen"})
	diagnostics.ReportArtifact(ctx, diagnostics.Artifact{Path: "cache/native.jar", Member: "lib.dylib", Role: "native-binary", Format: "macho", Architectures: "amd64", State: "header"})
	if err := store.Save(t.Context(), group); err != nil {
		t.Fatal(err)
	}
	got, err = store.Get(t.Context(), group.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Schema != 3 || got.Evidence.Facts.HostArch != "arm64" || got.Evidence.Facts.LauncherArch != "amd64" || len(got.Evidence.Artifacts) != 1 {
		t.Fatal(got)
	}
	group.Schema = 4
	diagnostics.SetFacts(ctx, diagnostics.Facts{Compiler: "absent", ToolCompiler: "present"})
	if err := store.Save(t.Context(), group); err != nil {
		t.Fatal(err)
	}
	got, err = store.Get(t.Context(), group.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Schema != 4 || got.Evidence.Facts.Compiler != "absent" || got.Evidence.Facts.ToolCompiler != "present" {
		t.Fatal(got)
	}
	group.Schema = 5
	group.Evidence.Finish("metadata-budget")
	if err := store.Save(t.Context(), group); err != nil {
		t.Fatal(err)
	}
	got, err = store.Get(t.Context(), group.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Schema != 5 || got.Evidence.Code != "metadata-budget" {
		t.Fatal(got)
	}
	group.Schema = 6
	diagnostics.ReportArtifact(ctx, diagnostics.Artifact{Path: "cache/installer.bin", Role: "loader-installer", Coordinate: "net.minecraftforge:forge:1.12.2-14.23.5.2860:installer", Repository: "forge", State: "planned"})
	if err := store.Save(t.Context(), group); err != nil {
		t.Fatal(err)
	}
	got, err = store.Get(t.Context(), group.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Schema != 6 || len(got.Evidence.Artifacts) != 2 || got.Evidence.Artifacts[1].Repository != "forge" {
		t.Fatal(got)
	}
}
