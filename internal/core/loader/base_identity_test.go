package loader

import (
	"strings"
	"testing"

	"mcl/internal/core/meta"
)

func chainSource(id, digest string) meta.ManifestEntry {
	return meta.ManifestEntry{ID: id, Type: "release", SHA1: digest, URL: "https://piston-meta.mojang.com/v1/packages/" + digest + "/" + id + ".json"}
}
func TestLoaderCachePinsEntireOfficialBaseChain(t *testing.T) {
	request, base, _, _ := legacyFixture(t)
	single := base
	single.Sources = []meta.ManifestEntry{chainSource(base.ID, request.BaseSHA1)}
	bound, err := request.WithBase(single)
	if err != nil || bound != request || bound.Key() != request.Key() {
		t.Fatal("single-document cache compatibility lost", err)
	}
	base.Sources = []meta.ManifestEntry{chainSource("1.12.1", strings.Repeat("b", 40)), chainSource(base.ID, request.BaseSHA1)}
	base.ClientVersion = "1.12.1"
	first, err := request.WithBase(base)
	if err != nil || first.BaseChainSHA256 == "" {
		t.Fatal("inherited base did not bind", err)
	}
	windows, _ := first.WithPlatform(meta.Platform{OS: "windows", Arch: "amd64"})
	arm, _ := first.WithPlatform(meta.Platform{OS: "osx", Arch: "arm64"})
	if windows.Key() == arm.Key() {
		t.Fatal("different platforms shared processor cache identity")
	}
	if request.ValidateBase(base) == nil {
		t.Fatal("single-digest cache reused for inherited version")
	}
	changed := base
	changed.Sources = append([]meta.ManifestEntry(nil), base.Sources...)
	changed.Sources[0] = chainSource("1.12.1", strings.Repeat("c", 40))
	second, err := request.WithBase(changed)
	if err != nil || second.Key() == first.Key() {
		t.Fatal("changed parent reused a processor generation")
	}
	if first.ValidateBase(changed) == nil {
		t.Fatal("stale parent identity accepted")
	}
	base.Sources[1].SHA1 = strings.Repeat("d", 40)
	if _, err := request.WithBase(base); err == nil {
		t.Fatal("instance leaf digest was not checked")
	}
	changed.Sources[0].URL = "https://untrusted.invalid/parent.json"
	if _, err := request.WithBase(changed); err == nil {
		t.Fatal("third-party parent evidence accepted")
	}
}
