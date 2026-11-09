package loader

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"mcl/internal/core/download"
	"mcl/internal/core/instance"
	"mcl/internal/core/java"
	"mcl/internal/core/launch"
	"mcl/internal/core/meta"
)

// This is a synthetic schema fixture, not an official 10.13.4.1614 installer.
func legacy17Fixture(t *testing.T, wrapperVersion string, omitParent bool) (Request, meta.Version, map[string]any, map[string][]byte) {
	t.Helper()
	request, base, document, files := legacyFixture(t)
	request.GameVersion, request.Version = "1.7.10", "10.13.4.1614-1.7.10"
	base.ID = "1.7.10"
	id := "1.7.10-Forge" + request.Version
	core := "net.minecraftforge:forge:1.7.10-" + request.Version
	install := document["install"].(map[string]any)
	install["minecraft"], install["version"], install["path"] = base.ID, id, core
	version := document["versionInfo"].(map[string]any)
	version["id"], version["jar"], version["inheritsFrom"] = id, base.ID, base.ID
	if omitParent {
		delete(version, "inheritsFrom")
	}
	version["minecraftArguments"] = strings.ReplaceAll(version["minecraftArguments"].(string), legacyForgeTweaker, legacyForge17Tweaker) + " --userProperties $" + "{user_properties}"
	libraries := version["libraries"].([]any)
	libraries[0] = map[string]any{"name": core, "url": "http://files.minecraftforge.net/maven/"}
	wrapperName := "net.minecraft:launchwrapper:" + wrapperVersion
	coordinate, _ := meta.ParseCoordinate(wrapperName)
	wrapperURL := "https://libraries.minecraft.net/" + coordinate.Path()
	wrapper := legacyTestZip(t, map[string][]byte{"net/minecraft/launchwrapper/Launch.class": []byte("synthetic-not-executable")})
	files[wrapperURL] = wrapper
	libraries[1] = map[string]any{"name": wrapperName, "downloads": map[string]any{"artifact": map[string]any{"path": coordinate.Path(), "url": wrapperURL, "sha1": fmt.Sprintf("%x", sha1.Sum(wrapper)), "size": len(wrapper)}}}
	version["libraries"] = libraries
	files["forge-universal.jar"] = legacyTestZip(t, map[string][]byte{"cpw/mods/fml/common/launcher/FMLTweaker.class": []byte("synthetic-not-executable")})
	return request, base, document, files
}

func prepareLegacy17Fixture(t *testing.T, request Request, base meta.Version, document map[string]any, files map[string][]byte) (Profile, string, error) {
	t.Helper()
	raw, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	bundle := legacyTestZip(t, map[string][]byte{"install_profile.json": raw, "forge-universal.jar": files["forge-universal.jar"]})
	coordinate, origin, _, err := forgeInstaller(request)
	if err != nil {
		t.Fatal(err)
	}
	files[origin+"/"+coordinate.Path()] = bundle
	catalog := NewClient()
	catalog.HTTP = &http.Client{Transport: legacyTestTransport{files}}
	root := t.TempDir()
	client := download.NewClient(root)
	client.HTTP = &http.Client{Transport: legacyTestTransport{files}}
	installer := NewForgeInstallerWithJava(catalog, client, func(context.Context, string, string, []string) error {
		t.Fatal("old installer ran Java")
		return errors.New("unexpected tool")
	}, func(context.Context, int, meta.Platform, []string, func(java.Progress)) (java.Runtime, error) {
		t.Fatal("old installer selected Java")
		return java.Runtime{}, errors.New("unexpected resolver")
	})
	profile, err := installer.Prepare(t.Context(), request, base, "deliberately-no-java", meta.CurrentPlatform(), nil)
	return profile, root, err
}

func TestLegacy17UsesOwnEntryAndNoProcessorJava(t *testing.T) {
	for _, wrapper := range []string{"1.11", "1.12"} {
		for _, omitParent := range []bool{false, true} {
			request, base, document, files := legacy17Fixture(t, wrapper, omitParent)
			profile, directory, err := prepareLegacy17Fixture(t, request, base, document, files)
			if err != nil {
				t.Fatal(err)
			}
			if !profile.Legacy || profile.LegacyGame || profile.JavaMajor != 8 {
				t.Fatal("old generation acquired modern processor semantics")
			}
			encoded, _ := json.Marshal(profile)
			var restored Profile
			if err := json.Unmarshal(encoded, &restored); err != nil || restored.Validate() != nil || restored.Fingerprint() != profile.Fingerprint() {
				t.Fatal("old generation did not round trip")
			}
			root, err := os.OpenRoot(directory)
			if err != nil {
				t.Fatal(err)
			}
			if err := restored.VerifyRuntime(t.Context(), root); err != nil {
				t.Fatal(err)
			}
			root.Close()
			effective, err := profile.Merge(base)
			if err != nil {
				t.Fatal(err)
			}
			options := launch.Options{Root: directory, InstanceID: instance.NewID(), Version: effective, Platform: meta.CurrentPlatform(), Java: java.Runtime{Executable: filepath.Join(directory, "java"), Major: 8, Version: "1.8.0_442", Arch: meta.CurrentPlatform().Arch}, MaxMemoryMiB: 1024}
			options.NativeDirectory = "instances/" + options.InstanceID + "/natives/ready-" + strings.Repeat("A", 26)
			session, err := launch.NewSession("Player", strings.Repeat("b", 32), "synthetic-forge17-secret", "test-client", "1234", time.Now().Add(time.Hour))
			if err != nil {
				t.Fatal(err)
			}
			plan, err := launch.Build(options, session)
			if err != nil {
				t.Fatal(err)
			}
			args := plan.ProcessArguments(filepath.Join(directory, "support.jar"))
			if !slices.Contains(args, legacyForge17Tweaker) || slices.Contains(args, legacyForgeTweaker) || slices.Contains(args, "--demo") {
				t.Fatal("old entry or complete game template was lost")
			}
			ext, support, err := plan.SupportFile()
			if err != nil || ext != ".jar" || bytes.Contains(support, []byte("synthetic-forge17-secret")) {
				t.Fatal("old game did not use private credential-free Java8 support")
			}
		}
	}
}

func TestLegacy17RejectsCrossGenerationAndUnboundBase(t *testing.T) {
	for _, change := range []func(map[string]any){
		func(document map[string]any) {
			document["versionInfo"].(map[string]any)["minecraftArguments"] = strings.ReplaceAll(document["versionInfo"].(map[string]any)["minecraftArguments"].(string), legacyForge17Tweaker, legacyForgeTweaker)
		},
		func(document map[string]any) { document["versionInfo"].(map[string]any)["jar"] = "1.12.2" },
		func(document map[string]any) { document["versionInfo"].(map[string]any)["jar"] = nil },
		func(document map[string]any) { delete(document["install"].(map[string]any), "minecraft") },
		func(document map[string]any) { document["versionInfo"].(map[string]any)["inheritsFrom"] = nil },
		func(document map[string]any) { document["versionInfo"].(map[string]any)["inheritsFrom"] = "" },
		func(document map[string]any) { document["versionInfo"].(map[string]any)["inheritsFrom"] = "1.6.4" },
		func(document map[string]any) {
			document["versionInfo"].(map[string]any)["libraries"].([]any)[0].(map[string]any)["clientreq"] = false
		},
		func(document map[string]any) { document["processors"] = []any{} },
	} {
		request, base, document, files := legacy17Fixture(t, "1.12", true)
		change(document)
		if _, recognized, err := readLegacyForgeSpec(t.Context(), legacyArchive(t, document, files), request, base); !recognized || err == nil {
			t.Fatal("unbound/mixed old installer accepted")
		}
	}
	request, base, document, files := legacy17Fixture(t, "1.10", false)
	if _, _, err := readLegacyForgeSpec(t.Context(), legacyArchive(t, document, files), request, base); err == nil {
		t.Fatal("unknown LaunchWrapper generation accepted")
	}
}

func TestLegacy17EntrypointAndConflictingRuntimeAreProtected(t *testing.T) {
	request, base, document, files := legacy17Fixture(t, "1.12", false)
	files["forge-universal.jar"] = legacyTestZip(t, map[string][]byte{"net/minecraftforge/fml/common/launcher/FMLTweaker.class": []byte("wrong-generation")})
	if _, _, err := prepareLegacy17Fixture(t, request, base, document, files); err == nil {
		t.Fatal("modern FML entry substituted for cpw entry")
	}
	for _, library := range []meta.Library{
		{Name: "net.minecraft:launchwrapper:1.10"},
		{Name: "net.minecraftforge:forge:1.7.10-10.13.4.1614"}, // Cannot drop the branch suffix.
		{Name: "net.minecraftforge:forge:1.12.2-unrelated"},
		{Name: "net.minecraft:launchwrapper:1.12", Rules: []meta.Rule{{Action: "allow"}}},
	} {
		if err := validateLegacyRuntimeLibrary(request, library, false); err == nil {
			t.Fatal("conflicting or conditional core/wrapper accepted")
		}
	}
	if normalizeProcessedLegacyVersion(request, &meta.Version{ID: document["versionInfo"].(map[string]any)["id"].(string), MainClass: legacyForgeMain}) == nil {
		t.Fatal("empty processed legacy game template accepted")
	}
	if !processedLegacyCore(request, "net.minecraftforge:forge:1.7.10-"+request.Version) {
		t.Fatal("explicit old-generation core identity was lost")
	}
	request.GameVersion = "1.5.2"
	if _, ok := legacyForgeTweakerFor(request); ok {
		t.Fatal("unimplemented intermediate generation inferred by range")
	}
}

func TestLegacyPublicRepositoryAliasesRemainExact(t *testing.T) {
	for input, want := range map[string]string{
		"http://libraries.minecraft.net/":       "https://libraries.minecraft.net",
		"http://repo1.maven.org/maven2/":        "https://repo.maven.apache.org/maven2",
		"https://repo.maven.apache.org/maven2/": "https://repo.maven.apache.org/maven2",
	} {
		if got, err := legacyForgeLibraryRoot(input); err != nil || got != want {
			t.Fatalf("public root %q: %q %v", input, got, err)
		}
	}
	for _, bad := range []string{"http://repo1.maven.org.evil.invalid/maven2", "http://user:secret@repo1.maven.org/maven2", "https://repo.maven.apache.org/maven2/extra", "http://libraries.minecraft.net?proxy=1"} {
		if _, err := legacyForgeLibraryRoot(bad); err == nil {
			t.Fatal("arbitrary repository rewrite accepted")
		}
	}
}
