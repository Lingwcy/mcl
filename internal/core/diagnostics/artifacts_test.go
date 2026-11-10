package diagnostics

import (
	"fmt"
	"strings"
	"testing"
)

func TestObservedNativeEvidenceSurvivesLargeClasspathAndIsCloned(t *testing.T) {
	var snapshot Snapshot
	ctx := WithObserver(t.Context(), snapshot.Apply)
	for i := range MaxArtifacts + 16 {
		ReportArtifact(ctx, Artifact{Path: fmt.Sprintf("cache/libraries/%d.jar", i), Role: "classpath", State: "planned"})
	}
	a := Artifact{Path: "cache/native.jar", Member: "lib.dylib", Destination: "instances/a/natives/ready-b/lib.dylib", SHA1: strings.Repeat("a", 40), Role: "native-binary", State: "header", Format: "macho", Architectures: "amd64.arm64"}
	ReportArtifact(ctx, a)
	if len(snapshot.Artifacts) != MaxArtifacts || snapshot.ArtifactsDropped != 17 {
		t.Fatal(len(snapshot.Artifacts), snapshot.ArtifactsDropped)
	}
	PublishedNatives(ctx, "instances/a/natives/ready-other")
	for _, got := range snapshot.Artifacts {
		if got.Role == "native-binary" && got.Published {
			t.Fatal("unrelated directory published this native")
		}
	}
	PublishedNatives(ctx, "instances/a/natives/ready-b")
	if !strings.Contains(snapshot.Text(), "published=true") || !strings.Contains(snapshot.Text(), "macho-amd64.arm64") {
		t.Fatal(snapshot.Text())
	}
	clone := snapshot.Clone()
	clone.Artifacts[0].Path = "changed"
	if snapshot.Artifacts[0].Path == "changed" {
		t.Fatal("clone aliases artifacts")
	}
	if err := snapshot.Validate(); err != nil {
		t.Fatal(err)
	}
}
func TestArtifactBoundaryRejectsRawPathsAndInventedHeaderEvidence(t *testing.T) {
	for _, a := range []Artifact{
		{Path: "/Users/private/lib.so", Role: "native-binary", State: "unknown"},
		{Path: "cache/a.jar", Member: "https://secret/?token=x", Role: "native-binary", State: "unknown"},
		{Path: "cache/a.jar", Role: "native-binary", State: "header"},
		{Path: "cache/a.jar", Role: "native-binary", State: "unknown", Format: "elf", Architectures: "amd64"},
	} {
		if err := a.Validate(); err == nil {
			t.Fatal("unsafe/invented evidence accepted", a)
		}
	}
}
