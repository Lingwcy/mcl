package loader

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"mcl/internal/core/archive"
	"mcl/internal/core/download"
	"mcl/internal/core/instance"
	"mcl/internal/core/java"
	"mcl/internal/core/launch"
	"mcl/internal/core/meta"
)

func legacyTestZip(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var out bytes.Buffer
	z := zip.NewWriter(&out)
	for _, name := range slices.Sorted(func(yield func(string) bool) {
		for name := range files {
			if !yield(name) {
				return
			}
		}
	}) {
		entry, err := z.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write(files[name]); err != nil {
			t.Fatal(err)
		}
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}
func legacyFixture(t *testing.T) (Request, meta.Version, map[string]any, map[string][]byte) {
	t.Helper()
	raw, err := os.ReadFile("../meta/testdata/1.20.4.json")
	if err != nil {
		t.Fatal(err)
	}
	base, err := meta.ParseVersion(raw)
	if err != nil {
		t.Fatal(err)
	}
	// Deliberately synthetic format fixture, not real 1.12.2 bytes or classes.
	base.ID, base.Java.Major, base.Libraries = "1.12.2", 8, nil
	request := Request{Type: "forge", Version: "14.23.5.2860", GameVersion: base.ID, BaseSHA1: strings.Repeat("a", 40)}
	id := "1.12.2-forge1.12.2-14.23.5.2860"
	core := "net.minecraftforge:forge:1.12.2-14.23.5.2860"
	wrapper := legacyTestZip(t, map[string][]byte{"net/minecraft/launchwrapper/Launch.class": []byte("synthetic-not-bytecode")})
	wrapperPath := "net/minecraft/launchwrapper/1.12/launchwrapper-1.12.jar"
	wrapperURL := "https://libraries.minecraft.net/" + wrapperPath
	args := "--username $" + "{auth_player_name} --version $" + "{version_name} --gameDir $" + "{game_directory} --assetsDir $" + "{assets_root} --assetIndex $" + "{assets_index_name} --uuid $" + "{auth_uuid} --accessToken $" + "{auth_access_token} --userType $" + "{user_type} --versionType $" + "{version_type} --tweakClass " + legacyForgeTweaker
	profile := map[string]any{
		"install": map[string]any{"path": core, "version": id, "filePath": "forge-universal.jar", "minecraft": base.ID},
		"versionInfo": map[string]any{"id": id, "type": "release", "inheritsFrom": base.ID, "jar": base.ID, "assets": base.Assets, "mainClass": legacyForgeMain, "minecraftArguments": args,
			"libraries": []any{map[string]any{"name": core}, map[string]any{"name": "net.minecraft:launchwrapper:1.12", "downloads": map[string]any{"artifact": map[string]any{"path": wrapperPath, "url": wrapperURL, "sha1": fmt.Sprintf("%x", sha1.Sum(wrapper)), "size": len(wrapper)}}}}},
	}
	files := map[string][]byte{"forge-universal.jar": legacyTestZip(t, map[string][]byte{"net/minecraftforge/fml/common/launcher/FMLTweaker.class": []byte("synthetic-not-bytecode")}), wrapperURL: wrapper}
	return request, base, profile, files
}
func legacyArchive(t *testing.T, profile map[string]any, files map[string][]byte) *archive.Zip {
	t.Helper()
	body, err := json.Marshal(profile)
	if err != nil {
		t.Fatal(err)
	}
	payload := map[string][]byte{"install_profile.json": body}
	for name, body := range files {
		if archive.SafePath(name) {
			payload[name] = body
		}
	}
	encoded := legacyTestZip(t, payload)
	z, err := archive.Open(bytes.NewReader(encoded), int64(len(encoded)))
	if err != nil {
		t.Fatal(err)
	}
	return z
}

type legacyTestTransport struct{ files map[string][]byte }

func (t legacyTestTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	address := request.URL.String()
	isDigest := strings.HasSuffix(address, ".sha1")
	if isDigest {
		address = strings.TrimSuffix(address, ".sha1")
	}
	body, ok := t.files[address]
	if !ok {
		return nil, fmt.Errorf("unexpected fixture request %s", request.URL.Host)
	}
	if isDigest {
		body = []byte(fmt.Sprintf("%x", sha1.Sum(body)))
	}
	header := make(http.Header)
	header.Set("Content-Length", strconv.Itoa(len(body)))
	if request.Method == http.MethodHead {
		return &http.Response{StatusCode: 200, Header: header, Body: io.NopCloser(bytes.NewReader(nil)), ContentLength: int64(len(body)), Request: request}, nil
	}
	return &http.Response{StatusCode: 200, Header: header, Body: io.NopCloser(bytes.NewReader(body)), ContentLength: int64(len(body)), Request: request}, nil
}

func TestLegacyForgeInstallerUsesNoJavaAndLaunchesOnlyControlledPlan(t *testing.T) {
	request, base, document, files := legacyFixture(t)
	documentBytes, _ := json.Marshal(document)
	bundle := legacyTestZip(t, map[string][]byte{"install_profile.json": documentBytes, "forge-universal.jar": files["forge-universal.jar"]})
	coordinate, origin, _, err := forgeInstaller(request)
	if err != nil {
		t.Fatal(err)
	}
	address := origin + "/" + coordinate.Path()
	files[address] = bundle
	transport := legacyTestTransport{files}
	catalog := NewClient()
	catalog.HTTP = &http.Client{Transport: transport}
	directory := t.TempDir()
	client := download.NewClient(directory)
	client.HTTP = &http.Client{Transport: transport}
	installer := NewForgeInstaller(catalog, client, func(context.Context, string, string, []string) error {
		t.Fatal("legacy branch executed a processor")
		return errors.New("unexpected processor")
	})
	profile, err := installer.Prepare(t.Context(), request, base, "not-a-java-executable", meta.CurrentPlatform(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !profile.Legacy || profile.JavaMajor != 8 || profile.MainClass != legacyForgeMain || len(profile.Outputs) != 0 {
		t.Fatal("wrong legacy generation")
	}
	encoded, err := json.Marshal(profile)
	if err != nil {
		t.Fatal(err)
	}
	var restored Profile
	if err := json.Unmarshal(encoded, &restored); err != nil {
		t.Fatal(err)
	}
	if restored.Validate() != nil || !restored.Legacy || restored.Fingerprint() != profile.Fingerprint() {
		t.Fatal("legacy cache identity did not round trip")
	}
	root, err := os.OpenRoot(directory)
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
	options := launch.Options{Root: directory, InstanceID: instance.NewID(), Version: effective, Platform: meta.CurrentPlatform(), Java: java.Runtime{Executable: filepath.Join(directory, "java"), Major: 8, Version: "1.8.0_442", Arch: meta.CurrentPlatform().Arch}, MaxMemoryMiB: 1024}
	options.NativeDirectory = "instances/" + options.InstanceID + "/natives/ready-" + strings.Repeat("A", 26)
	login, err := launch.NewSession("Player", strings.Repeat("b", 32), "synthetic-legacy-forge-secret", "test-client", "1234", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	plan, err := launch.Build(options, login)
	if err != nil {
		t.Fatal(err)
	}
	args := plan.ProcessArguments(filepath.Join(directory, "support.jar"))
	if slices.Contains(args, "--demo") || !slices.Contains(args, legacyForgeTweaker) || !slices.Contains(args, legacyForgeMain) {
		t.Fatal("legacy complete template did not replace vanilla game flags")
	}
	count := 0
	for _, arg := range args {
		if arg == "--username" {
			count++
		}
	}
	if count != 1 {
		t.Fatal("vanilla and legacy identities were duplicated")
	}
	ext, support, err := plan.SupportFile()
	if err != nil || ext != ".jar" || bytes.Contains(support, []byte("synthetic-legacy-forge-secret")) {
		t.Fatal("legacy launch support leaked identity")
	}
	artifact := profile.Libraries[0].Downloads.Artifact
	if err := root.WriteFile(profile.LibraryDirectory()+"/"+artifact.Path, []byte("bad"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := profile.VerifyRuntime(t.Context(), root); err == nil {
		t.Fatal("corrupt legacy generation accepted")
	}
}

func TestLegacyForgeRejectsUnknownFormatsAndEntrypoints(t *testing.T) {
	for _, change := range []func(map[string]any){
		func(v map[string]any) { v["processors"] = []any{} },
		func(v map[string]any) {
			v["versionInfo"].(map[string]any)["libraries"].([]any)[0].(map[string]any)["checksums"] = []string{"invalid"}
		},
		func(v map[string]any) { v["versionInfo"].(map[string]any)["mainClass"] = "custom.Loader" },
		func(v map[string]any) { v["versionInfo"].(map[string]any)["inheritsFrom"] = "other" },
		func(v map[string]any) { v["install"].(map[string]any)["filePath"] = "../escape.jar" },
		func(v map[string]any) { v["install"].(map[string]any)["version"] = "unrelated" },
		func(v map[string]any) {
			v["versionInfo"].(map[string]any)["minecraftArguments"] = "--tweakClass custom.Tweaker"
		},
	} {
		request, base, document, files := legacyFixture(t)
		change(document)
		_, recognized, err := readLegacyForgeSpec(t.Context(), legacyArchive(t, document, files), request, base)
		if !recognized || err == nil {
			t.Fatal("unsafe legacy installer accepted")
		}
	}
	request, base, document, files := legacyFixture(t)
	request.GameVersion = "1.7.10"
	if _, _, err := readLegacyForgeSpec(t.Context(), legacyArchive(t, document, files), request, base); err == nil {
		t.Fatal("unimplemented earlier generation accepted")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, _, err := readLegacyForgeSpec(ctx, legacyArchive(t, document, files), request, base); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel: %v", err)
	}
}

func TestLegacyRepositoryAndArgumentSafety(t *testing.T) {
	if root, err := legacyForgeLibraryRoot("http://files.minecraftforge.net/maven/"); err != nil || root != "https://maven.minecraftforge.net" {
		t.Fatal("official historical alias not normalized")
	}
	for _, value := range []string{"http://example.invalid/", "https://maven.minecraftforge.net.evil.invalid/", "https://user:secret@maven.minecraftforge.net/", "https://libraries.minecraft.net/extra/"} {
		if _, err := legacyForgeLibraryRoot(value); err == nil {
			t.Fatal("unsafe legacy repository accepted")
		}
	}
	request, base, document, files := legacyFixture(t)
	spec, _, err := readLegacyForgeSpec(t.Context(), legacyArchive(t, document, files), request, base)
	if err != nil {
		t.Fatal(err)
	}
	values, _ := meta.SelectArguments(spec.version.Arguments.Game, meta.CurrentPlatform())
	values = append(values, "--tweakClass", legacyForgeTweaker)
	if err := validateLegacyForgeArguments(values); err == nil {
		t.Fatal("duplicate tweaker accepted")
	}
}

func TestModernForgeSpecDoesNotAcquireLegacySemantics(t *testing.T) {
	request, base, _, _ := legacyFixture(t)
	document := map[string]any{"spec": 1, "minecraft": base.ID, "version": "modern", "processors": []any{}, "json": "/version.json"}
	z := legacyArchive(t, document, nil)
	if _, legacy, err := readLegacyForgeSpec(t.Context(), z, request, base); err != nil || legacy {
		t.Fatalf("modern spec reinterpreted as legacy: %v", err)
	}
}

func TestLegacyProfileCannotOverrideRuntimeOrTweaker(t *testing.T) {
	request, base, document, files := legacyFixture(t)
	spec, _, err := readLegacyForgeSpec(t.Context(), legacyArchive(t, document, files), request, base)
	if err != nil {
		t.Fatal(err)
	}
	// No Java is executed: the helpers are pure cache/format validation.
	profile := Profile{Request: request, ID: spec.version.ID, Legacy: true, MainClass: legacyForgeMain, JavaMajor: 8, Arguments: *spec.version.Arguments}
	for _, lib := range spec.libraries {
		profile.Libraries = append(profile.Libraries, lib.Library)
	}
	if err := validateLegacyForgeProfile(profile); err != nil {
		t.Fatal(err)
	}
	profile.JavaMajor = 17
	if err := validateLegacyForgeProfile(profile); err == nil {
		t.Fatal("legacy config raised the game Java major")
	}
	profile.JavaMajor = 8
	profile.Request.Type = "neoforge"
	if err := validateLegacyForgeProfile(profile); err == nil {
		t.Fatal("legacy config impersonated NeoForge")
	}
}
