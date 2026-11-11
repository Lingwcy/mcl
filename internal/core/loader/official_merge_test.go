package loader

import (
	"context"
	"crypto/sha1"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"mcl/internal/core/install"
	"mcl/internal/core/instance"
	"mcl/internal/core/java"
	"mcl/internal/core/launch"
	"mcl/internal/core/meta"
)

func officialMergeBase(t *testing.T, game string) (meta.Version, string) {
	t.Helper()
	digests := map[string]string{"1.7.10": "ed5d8789ed29872ea2ef1c348302b0c55e3f3468", "1.12.2": "832d95b9f40699d4961394dcf6cf549e65f15dc5", "1.20.4": "3753ef41f9540d84b13d8c203a48cc1afecc90d5"}
	raw, err := os.ReadFile("../meta/testdata/" + game + ".json")
	if err != nil {
		t.Fatal(err)
	}
	digest := digests[game]
	if fmt.Sprintf("%x", sha1.Sum(raw)) != digest {
		t.Fatal("official base bytes changed")
	}
	entry := meta.ManifestEntry{ID: game, Type: "release", SHA1: digest, URL: "https://piston-meta.mojang.com/v1/packages/" + digest + "/" + game + ".json"}
	client := meta.NewClient(t.TempDir())
	client.VerifiedFetch = func(ctx context.Context, url, sha string, limit int64) ([]byte, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if url != entry.URL || sha != digest || int64(len(raw)) > limit {
			return nil, errors.New("unexpected metadata request in offline fixture")
		}
		return slices.Clone(raw), nil
	}
	base, err := client.Version(t.Context(), entry)
	if err != nil {
		t.Fatal(err)
	}
	return base, digest
}

// These are declaration/plan tests, not artifact checks or installation tests.
// 1.7.10 omitted identities receive explicitly SYNTHETIC digest/size metadata
// so Merge can be exercised; no synthetic bytes are accepted as official files.
// Modern generated-file size is likewise synthetic, as in runtime_libraries_test.
func officialMergeProfile(t *testing.T, sample officialSample, base meta.Version, digest string) Profile {
	t.Helper()
	request := Request{Type: "forge", Version: sample.Build, GameVersion: sample.Game, BaseSHA1: digest}
	request, err := request.WithBase(base)
	if err != nil {
		t.Fatal(err)
	}
	if sample.Game == "1.20.4" {
		p, _ := bootstrapRuntimeFixture(t)
		p.Request = request
		return p
	}
	// Raw official legacy key differs from the internal normalized field name.
	var version struct {
		ID              string
		MainClass       string
		LegacyArguments string `json:"minecraftArguments"`
		Libraries       []meta.Library
	}
	if err := json.Unmarshal(officialProjection(t, sample.Version, sample.VersionProjectionSHA256), &version); err != nil {
		t.Fatal(err)
	}
	args, err := legacyForgeArguments(request, version.LegacyArguments)
	if err != nil {
		t.Fatal(err)
	}
	for i := range version.Libraries {
		lib := &version.Libraries[i]
		coordinate, err := meta.ParseCoordinate(lib.Name)
		if err != nil {
			t.Fatal(err)
		}
		if lib.Downloads.Artifact == nil {
			lib.Downloads.Artifact = &meta.Download{Path: coordinate.Path(), SHA1: strings.Repeat("a", 40), Size: 1}
		}
		lib.Downloads.Artifact.URL = ""
	}
	return Profile{Request: request, ID: version.ID, MainClass: version.MainClass, JavaMajor: 8, InstallerFormat: sample.Format, Generation: "ready-" + strings.Repeat("A", 26), InstallerSHA1: sample.SHA1, Legacy: sample.Format == ForgeFormatLegacy, LegacyGame: sample.Format == ForgeFormatEmbedded, Arguments: args, Libraries: version.Libraries}
}

func fixedMergeExpectations(t *testing.T, sample officialSample) []string {
	t.Helper()
	data, err := os.ReadFile("testdata/forge-official/merge-library-evidence.json")
	if err != nil {
		t.Fatal(err)
	}
	var ledger struct {
		Cases []struct {
			Game, Build string
			Projection  string   `json:"profile_projection_sha256"`
			Expected    []string `json:"expected_relevant_classpath"`
		}
	}
	if err := json.Unmarshal(data, &ledger); err != nil {
		t.Fatal(err)
	}
	for _, entry := range ledger.Cases {
		if entry.Game == sample.Game && entry.Build == sample.Build {
			if entry.Projection != sample.VersionProjectionSHA256 {
				t.Fatal("fixed merge evidence refers to another profile")
			}
			return entry.Expected
		}
	}
	t.Fatal("missing fixed expected module selection")
	return nil
}

func TestOfficialForgeMergeLibraryAndTemplateMatrix(t *testing.T) {
	for _, sample := range officialMetadataSamples(t) {
		t.Run(sample.Game+"/"+sample.Build, func(t *testing.T) {
			base, digest := officialMergeBase(t, sample.Game)
			p := officialMergeProfile(t, sample, base, digest)
			merged, err := p.Merge(base)
			if err != nil {
				t.Fatal(err)
			}
			target := meta.CurrentPlatform()
			target.Arch = "amd64"
			root := filepath.Join(t.TempDir(), "中文 空格")
			session, err := launch.NewOfflineSession("--tweakClass", strings.Repeat("a", 32))
			if err != nil {
				t.Fatal(err)
			}
			id := instance.NewID()
			command, err := launch.Build(launch.Options{Root: root, InstanceID: id, Version: merged, Platform: target, Java: java.Runtime{Executable: filepath.Join(root, "synthetic-java"), NativeEncoding: "UTF-8", Major: merged.Java.Major, Version: fmt.Sprint(merged.Java.Major), Arch: target.Arch}, NativeDirectory: "instances/" + id + "/natives/ready-" + strings.Repeat("B", 26), MaxMemoryMiB: 2048}, session)
			if err != nil {
				t.Fatal("official declaration composition rejected by final launch planner", err)
			}
			extension, _, err := command.SupportFile()
			if err != nil {
				t.Fatal(err)
			}
			if _, err := command.ProcessCommand(filepath.Join(root, "private-support"+extension)); err != nil {
				t.Fatal(err)
			}
			if merged.MainClass != p.MainClass || merged.LoaderProfile != p.Fingerprint() || merged.Downloads["client"] != base.Downloads["client"] || merged.AssetIndex != base.AssetIndex {
				t.Fatal("merged main/client/assets identity differs from fixed plan")
			}
			for _, target := range []string{"windows", "osx", "linux"} {
				platform := meta.Platform{OS: target, Arch: "amd64"}
				plan, err := install.Build(merged, platform)
				if err != nil {
					t.Fatalf("%s plan: %v", target, err)
				}
				if plan.Classpath[len(plan.Classpath)-1] != plan.ClientPath {
					t.Fatal("client must remain last")
				}
				var relevant []string
				for _, lib := range plan.Libraries {
					c, _ := meta.ParseCoordinate(lib.Coordinate)
					if c.Group == "org.ow2.asm" || c.Group == "org.scala-lang" || c.Group == "org.scala-lang.plugins" || c.Group == "com.google.guava" && c.Artifact == "guava" {
						relevant = append(relevant, lib.Coordinate)
					}
				}
				want := fixedMergeExpectations(t, sample)
				if !slices.Equal(relevant, want) {
					t.Fatalf("%s selected declarations %v, want %v", target, relevant, want)
				}
				if sample.Game == "1.7.10" && (!slices.Contains(relevant, "com.google.guava:guava:17.0") || slices.Contains(relevant, "com.google.guava:guava:15.0")) {
					t.Fatal("Forge Guava did not replace Mojang Guava")
				}
				if sample.Game == "1.12.2" && !slices.Contains(relevant, "com.google.guava:guava:21.0") {
					t.Fatal("Forge did not declare Guava; official Guava must survive")
				}
				game, err := meta.SelectArguments(merged.Arguments.Game, platform)
				if err != nil {
					t.Fatal(err)
				}
				if sample.Game != "1.20.4" {
					if len(game) == 0 {
						t.Fatal("missing legacy game template")
					}
					if len(game) < 2 || slices.Index(game, "--tweakClass") == -1 {
						t.Fatal("legacy tweaker lost")
					}
					for _, flag := range []string{"--username", "--accessToken", "--tweakClass"} {
						n := 0
						for _, v := range game {
							if v == flag {
								n++
							}
						}
						if n != 1 {
							t.Fatalf("legacy complete template duplicated %s", flag)
						}
					}
				} else if n := slices.Index(game, "--launchTarget"); n < 0 || n+1 >= len(game) || game[n+1] != "forge_client" {
					t.Fatal("modern client target lost")
				}
				jvm, err := meta.SelectArguments(merged.Arguments.JVM, platform)
				if err != nil {
					t.Fatal(err)
				}
				if sample.Game != "1.20.4" && slices.Contains(jvm, "-XstartOnFirstThread") {
					t.Fatal("modern first-thread flag synthesized into legacy template")
				}
			}
		})
	}
}
