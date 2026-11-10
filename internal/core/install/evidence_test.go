package install

import (
	"encoding/binary"
	"os"
	"testing"

	"mcl/internal/core/diagnostics"
)

func TestNativeEvidenceUsesHeaderNotFilenameAndTracksPublication(t *testing.T) {
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	header := make([]byte, 32)
	binary.LittleEndian.PutUint32(header, 0xfeedfacf)
	binary.LittleEndian.PutUint32(header[4:], 0x1000007)
	archive := nativeFixture(t, root, map[string]string{"arm64/lib.dylib": string(header), "skip/other.dylib": string(header)}, false)
	var evidence diagnostics.Snapshot
	ctx := diagnostics.WithObserver(t.Context(), evidence.Apply)
	directory, err := PrepareNatives(ctx, root, "natives", []NativeArchive{archive})
	if err != nil {
		t.Fatal(err)
	}
	if len(evidence.Artifacts) != 1 {
		t.Fatal(evidence.Artifacts)
	}
	a := evidence.Artifacts[0]
	if a.Architectures != "amd64" || a.Format != "macho" || a.SHA1 != archive.File.SHA1 || !a.Published || a.Destination != directory+"/arm64/lib.dylib" {
		t.Fatal(a)
	}
	if err := evidence.Validate(); err != nil {
		t.Fatal(err)
	}
}
func TestNativeHeaderObservationRejectsArchiveDigestDrift(t *testing.T) {
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	archive := nativeFixture(t, root, map[string]string{"lib.so": "unknown header"}, false)
	archive.File.SHA1 = "0000000000000000000000000000000000000000"
	var evidence diagnostics.Snapshot
	ctx := withNativeEvidenceBudget(diagnostics.WithObserver(t.Context(), evidence.Apply))
	observeNativeArchive(ctx, root, archive.File, nil, "")
	if len(evidence.Artifacts) != 1 || evidence.Artifacts[0].State != "unavailable" || evidence.Artifacts[0].Format != "" {
		t.Fatal(evidence.Artifacts)
	}
}
func TestNativeEvidenceBudgetDoesNotCreateInstallerFailure(t *testing.T) {
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	archive := nativeFixture(t, root, map[string]string{"lib.so": "unknown"}, false)
	var evidence diagnostics.Snapshot
	ctx := withNativeEvidenceBudget(diagnostics.WithObserver(t.Context(), evidence.Apply))
	ctx.Value(nativeEvidenceKey{}).(*nativeEvidenceBudget).hashBytes = 256 << 20
	observeNativeArchive(ctx, root, archive.File, nil, "")
	if len(evidence.Artifacts) != 1 || evidence.Artifacts[0].State != "limit" {
		t.Fatal(evidence.Artifacts)
	}
}
