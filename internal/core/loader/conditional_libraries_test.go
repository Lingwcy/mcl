package loader

import (
	"strings"
	"testing"

	"mcl/internal/core/meta"
)

func TestLoaderMergeKeepsPinnedConditionalVariants(t *testing.T) {
	_, base, _, _ := legacyFixture(t)
	request := Request{Type: "fabric", Version: "0.16.0", GameVersion: base.ID, BaseSHA1: strings.Repeat("a", 40)}
	core := meta.Library{Name: "net.fabricmc:fabric-loader:0.16.0"}
	coordinate, _ := meta.ParseCoordinate(core.Name)
	core.Downloads.Artifact = &meta.Download{Path: coordinate.Path(), URL: "https://maven.fabricmc.net/" + coordinate.Path(), SHA1: strings.Repeat("b", 40), Size: 100}
	conditional := meta.Library{Name: "example:conditional:1"}
	coordinate, _ = meta.ParseCoordinate(conditional.Name)
	conditional.Downloads.Artifact = &meta.Download{Path: coordinate.Path(), URL: "https://maven.fabricmc.net/" + coordinate.Path(), SHA1: strings.Repeat("c", 40), Size: 100}
	conditional.Rules = []meta.Rule{{Action: "allow"}}
	other := conditional
	other.Rules = []meta.Rule{{Action: "disallow"}}
	profile := Profile{Request: request, ID: "fabric-fixture", MainClass: "net.fabricmc.loader.impl.launch.knot.KnotClient", JavaMajor: 8, Libraries: []meta.Library{core, conditional, other}}
	effective, err := profile.Merge(base)
	if err != nil {
		t.Fatal(err)
	}
	if len(effective.Libraries) != 3 || len(effective.Libraries[2].Rules) != 1 || effective.Libraries[2].Rules[0].Action != "disallow" {
		t.Fatal("conditional variant dropped while overriding a module")
	}
	other.Downloads.Artifact = &meta.Download{Path: coordinate.Path(), URL: "https://maven.fabricmc.net/" + coordinate.Path(), SHA1: strings.Repeat("d", 40), Size: 100}
	profile.Libraries[2] = other
	if profile.Validate() == nil {
		t.Fatal("same conditional path accepted conflicting digest")
	}
}
