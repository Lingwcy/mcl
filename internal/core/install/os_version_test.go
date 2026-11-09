package install

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"mcl/internal/core/meta"
)

func TestLegacyClientPlanReceivesOSVersion(t *testing.T) {
	// Structural reproduction using the existing modern fixture; no claim that
	// this is the complete official 1.6.1 client or a runnable native combination.
	version := fixture(t, "1.20.4")
	version.ID, version.Java.Major = "1.6.1", 8
	var library meta.Library
	if err := json.Unmarshal([]byte(`{"name":"org.lwjgl.lwjgl:lwjgl:2.9.0","rules":[{"action":"allow"},{"action":"disallow","os":{"version":"^10\\.5\\.\\d$"}}]}`), &library); err != nil {
		t.Fatal(err)
	}
	coordinate, _ := meta.ParseCoordinate(library.Name)
	library.Downloads.Artifact = &meta.Download{Path: coordinate.Path(), URL: "https://libraries.minecraft.net/" + coordinate.Path(), SHA1: strings.Repeat("a", 40), Size: 100}
	version.Libraries = []meta.Library{library}
	host := meta.Platform{OS: "windows", Arch: "amd64", Version: "10.0"}
	plan, err := Build(version, host)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Platform.Version != host.Version || !slices.Contains(plan.Classpath, "cache/libraries/"+coordinate.Path()) {
		t.Fatal("platform snapshot lost before library selection")
	}
	host.Version = ""
	if _, err := Build(version, host); err == nil {
		t.Fatal("unknown OS version was treated as compatible")
	}
}
