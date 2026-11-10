package loader

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"slices"
	"testing"

	"mcl/internal/core/archive"
	"mcl/internal/core/download"
	"mcl/internal/core/java"
	"mcl/internal/core/meta"
)

func officialMetadataArchive(t *testing.T, files map[string][]byte) *archive.Zip {
	t.Helper()
	data := legacyTestZip(t, files)
	z, err := archive.Open(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	return z
}
func embeddedMetadata(t *testing.T) (officialSample, map[string]any, map[string]any) {
	t.Helper()
	for _, sample := range officialMetadataSamples(t) {
		if sample.Game != "1.12.2" || sample.Build != "14.23.5.2860" {
			continue
		}
		var p, v map[string]any
		if err := json.Unmarshal(officialProjection(t, sample.Profile, sample.ProfileProjectionSHA256), &p); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(officialProjection(t, sample.Version, sample.VersionProjectionSHA256), &v); err != nil {
			t.Fatal(err)
		}
		return sample, p, v
	}
	t.Fatal("missing official embedded metadata")
	return officialSample{}, nil, nil
}
func TestEmbeddedMetadataRejectsMixedIdentityAndExecution(t *testing.T) {
	cases := []struct {
		name        string
		mutate      func(map[string]any, map[string]any)
		missingCore bool
	}{
		{name: "spec-null", mutate: func(p, v map[string]any) { p["spec"] = nil }},
		{name: "mixed-install", mutate: func(p, v map[string]any) { p["install"] = nil }},
		{name: "json-null", mutate: func(p, v map[string]any) { p["json"] = nil }},
		{name: "json-empty", mutate: func(p, v map[string]any) { p["json"] = "" }},
		{name: "data-null", mutate: func(p, v map[string]any) { p["data"] = nil }},
		{name: "libraries-null", mutate: func(p, v map[string]any) { p["libraries"] = nil }},
		{name: "data-task", mutate: func(p, v map[string]any) {
			p["data"] = map[string]any{"PATCHED": map[string]any{"client": "[net.minecraftforge:forge:other]"}}
		}},
		{name: "server-processors-still-forbidden", mutate: func(p, v map[string]any) { p["processors"] = []any{map[string]any{"sides": []string{"server"}}} }},
		{name: "other-core", mutate: func(p, v map[string]any) { p["path"] = "net.minecraftforge:forge:1.12.2-14.23.5.2859" }},
		{name: "conflicting-core-checksum", mutate: func(p, v map[string]any) {
			core := p["libraries"].([]any)[1].(map[string]any)
			core["downloads"].(map[string]any)["artifact"].(map[string]any)["sha1"] = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
		}},
		{name: "parent-null", mutate: func(p, v map[string]any) { v["inheritsFrom"] = nil }},
		{name: "other-parent", mutate: func(p, v map[string]any) { v["inheritsFrom"] = "1.7.10" }},
		{name: "java-null", mutate: func(p, v map[string]any) { v["javaVersion"] = nil }},
		{name: "java-zero", mutate: func(p, v map[string]any) { v["javaVersion"] = map[string]any{"majorVersion": 0} }},
		{name: "missing-embedded-core", mutate: func(p, v map[string]any) {}, missingCore: true},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			sample, p, v := embeddedMetadata(t)
			core, _ := meta.ParseCoordinate(p["path"].(string))
			test.mutate(p, v)
			profile, _ := json.Marshal(p)
			version, _ := json.Marshal(v)
			files := map[string][]byte{"install_profile.json": profile, "version.json": version}
			if !test.missingCore {
				files["maven/"+core.Path()] = []byte("non-executable placeholder")
			}
			request := Request{Type: "forge", GameVersion: sample.Game, Version: sample.Build, BaseSHA1: sample.SHA1}
			spec, parsed, err := readForgeSpec(t.Context(), officialMetadataArchive(t, files), request)
			if err == nil {
				_, err = forgeLibraries(spec.Libraries, parsed.Libraries)
			}
			if err == nil {
				t.Fatal("accepted contradictory metadata")
			}
		})
	}
}

// Full publication boundary with explicitly synthetic payloads and game base.
// Original official version IDs, passive format, libraries and argument layout
// remain intact; every payload and payload checksum below is a test replacement.
// No HTTP leaves this transport; no Java, processor or real game is executed.
func TestSyntheticEmbeddedPublicationNeedsNoToolsOrUnusedMCP(t *testing.T) {
	sample, p, v := embeddedMetadata(t)
	request, base, _, _ := legacyFixture(t)
	request.Version = sample.Build
	request, err := request.WithPlatform(meta.CurrentPlatform())
	if err != nil {
		t.Fatal(err)
	}
	files := make(map[string][]byte)
	bundleFiles := make(map[string][]byte)
	client := []byte("synthetic official game payload")
	base.Downloads["client"] = meta.Download{URL: "https://piston-data.mojang.com/synthetic/client.jar", SHA1: fmt.Sprintf("%x", sha1.Sum(client)), Size: int64(len(client))}
	files[base.Downloads["client"].URL] = client
	for _, item := range v["libraries"].([]any) {
		lib := item.(map[string]any)
		name := lib["name"].(string)
		coordinate, _ := meta.ParseCoordinate(name)
		payloadEntries := map[string][]byte{"fixture.txt": []byte("synthetic:" + name)}
		if legacyForgeCore(request, name) {
			payloadEntries["net/minecraftforge/fml/common/launcher/FMLTweaker.class"] = []byte("not executable bytecode")
		}
		if legacyForgeWrapper(request, name) {
			payloadEntries["net/minecraft/launchwrapper/Launch.class"] = []byte("not executable bytecode")
		}
		payload := legacyTestZip(t, payloadEntries)
		artifact := lib["downloads"].(map[string]any)["artifact"].(map[string]any)
		artifact["sha1"], artifact["size"] = fmt.Sprintf("%x", sha1.Sum(payload)), len(payload)
		if artifact["url"] == "" {
			bundleFiles["maven/"+coordinate.Path()] = payload
			for _, entry := range p["libraries"].([]any) {
				other := entry.(map[string]any)
				if other["name"] == name {
					other["downloads"] = lib["downloads"]
				}
			}
		} else {
			files[artifact["url"].(string)] = payload
		}
	}
	profileJSON, _ := json.Marshal(p)
	versionJSON, _ := json.Marshal(v)
	bundleFiles["install_profile.json"], bundleFiles["version.json"] = profileJSON, versionJSON
	coordinate, origin, _, err := forgeInstaller(request)
	if err != nil {
		t.Fatal(err)
	}
	files[origin+"/"+coordinate.Path()] = legacyTestZip(t, bundleFiles)
	transport := legacyTestTransport{files}
	catalog := NewClient()
	catalog.HTTP = &http.Client{Transport: transport}
	rootPath := t.TempDir()
	clientDownload := download.NewClient(rootPath)
	clientDownload.HTTP = &http.Client{Transport: transport}
	runner := func(context.Context, string, string, []string) error {
		t.Fatal("passive spec0 executed processor")
		return nil
	}
	resolver := func(context.Context, int, meta.Platform, []string, func(java.Progress)) (java.Runtime, error) {
		t.Fatal("passive spec0 selected tool Java")
		return java.Runtime{}, nil
	}
	installer := NewForgeInstallerWithJava(catalog, clientDownload, runner, resolver)
	profile, err := installer.Prepare(t.Context(), request, base, "must-not-probe-java", meta.CurrentPlatform(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if profile.InstallerFormat != ForgeFormatEmbedded || profile.Legacy || !profile.LegacyGame || profile.JavaMajor != 8 {
		t.Fatal(profile.InstallerFormat, profile.Legacy, profile.LegacyGame, profile.JavaMajor)
	}
	for _, output := range profile.Outputs {
		if output.Path == "de/oceanlabs/mcp/mcp_config/1.12.2-20200226.224830/mcp_config-1.12.2-20200226.224830.zip" {
			t.Fatal("unused installer metadata became runtime input")
		}
	}
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err := profile.VerifyRuntime(t.Context(), root); err != nil {
		t.Fatal(err)
	}
	effective, err := profile.Merge(base)
	if err != nil {
		t.Fatal(err)
	}
	arguments, err := meta.SelectArguments(effective.Arguments.Game, meta.CurrentPlatform())
	if err != nil || !slices.Contains(arguments, "Forge") {
		t.Fatal(arguments, err)
	}
}
