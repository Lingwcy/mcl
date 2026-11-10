package loader

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"mcl/internal/core/meta"
)

type officialSample struct {
	Game, Build, Coordinate, URL, SHA1, SHA256, Format, Profile, Version string
	Size                                                                 int64
	ProfileProjectionSHA256                                              string `json:"profile_projection_sha256"`
	VersionProjectionSHA256                                              string `json:"version_projection_sha256"`
	Sidecar                                                              struct {
		Status  int
		Matches bool `json:"matches_installer"`
		SHA1    string
	}
}

func officialMetadataSamples(t *testing.T) []officialSample {
	t.Helper()
	data, err := os.ReadFile("testdata/forge-official/samples.json")
	if err != nil {
		t.Fatal(err)
	}
	var samples []officialSample
	if err := json.Unmarshal(data, &samples); err != nil {
		t.Fatal(err)
	}
	return samples
}
func officialProjection(t *testing.T, name, digest string) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/forge-official/" + name)
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprintf("%x", sha256.Sum256(data)) != digest {
		t.Fatal("projection changed without updating provenance ledger", name)
	}
	return data
}
func TestOfficialInstallerMetadataShapes(t *testing.T) {
	for _, sample := range officialMetadataSamples(t) {
		t.Run(sample.Game+"-"+sample.Build, func(t *testing.T) {
			if sample.Sidecar.Status != 200 || !sample.Sidecar.Matches || sample.Sidecar.SHA1 != sample.SHA1 || sample.Size <= 0 {
				t.Fatal("missing independent checksum observation", sample)
			}
			data := officialProjection(t, sample.Profile, sample.ProfileProjectionSHA256)
			version := officialProjection(t, sample.Version, sample.VersionProjectionSHA256)
			format, err := installerFormat(data)
			if err != nil || format != sample.Format {
				t.Fatal(format, err)
			}
			if format == ForgeFormatLegacy {
				return
			}
			request := Request{Type: "forge", GameVersion: sample.Game, Version: sample.Build, BaseSHA1: strings.Repeat("a", 40)}
			var raw forgeSpec
			if err := json.Unmarshal(data, &raw); err != nil {
				t.Fatal(err)
			}
			files := map[string][]byte{"install_profile.json": data, strings.TrimPrefix(raw.JSON, "/"): version}
			if format == ForgeFormatEmbedded {
				c, err := meta.ParseCoordinate(raw.Path)
				if err != nil {
					t.Fatal(err)
				}
				// Structural metadata-only recognition, NOT an official core payload.
				files["maven/"+c.Path()] = []byte("metadata-only placeholder, not executable or hash-valid")
			}
			z := officialMetadataArchive(t, files)
			coordinate, _, _, err := forgeInstaller(request)
			if err != nil || coordinate.String() != sample.Coordinate {
				t.Fatal(coordinate, err)
			}
			spec, parsed, err := readForgeSpec(t.Context(), z, request)
			if err != nil {
				t.Fatal("fixed official metadata rejected", err)
			}
			if format == ForgeFormatEmbedded && (spec.Spec != 0 || !spec.LegacyGame || len(spec.Processors) != 0 || parsed.Java.Major != 8) {
				t.Fatal(spec, parsed.Java)
			}
			if format == ForgeFormatProcessors && (spec.Spec != 1 || spec.LegacyGame || len(spec.Processors) != 5 || parsed.ID != "1.20.4-forge49.0.3") {
				t.Fatal(spec.Spec, spec.LegacyGame, len(spec.Processors), parsed.ID)
			}
		})
	}
}
func TestForgeAdapterPolicySeparatesCacheAndAliasIdentity(t *testing.T) {
	old := shortForgeRequest()
	current, err := old.WithPlatform(meta.Platform{OS: "osx", Arch: "amd64"})
	if err != nil {
		t.Fatal(err)
	}
	if current.AdapterPolicy != ForgeAdapterPolicy || old.Key() == current.Key() {
		t.Fatal("new format policy reused old cache identity")
	}
	canonical := current
	canonical.Version += "-1.7.10"
	if err := ValidateForgeAlias(current, canonical); err != nil {
		t.Fatal(err)
	}
	if err := ValidateForgeAlias(old, canonical); err == nil {
		t.Fatal("alias crossed adapter policy")
	}
}
