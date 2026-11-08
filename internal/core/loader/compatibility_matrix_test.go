package loader

import (
	"slices"
	"strings"
	"testing"

	"mcl/internal/core/meta"
)

func TestHistoricalForgeGenerationMatrix(t *testing.T) {
	for _, game := range []string{"1.6.1", "1.6.2", "1.6.4", "1.7.2", "1.7.10", "1.8", "1.8.9", "1.9.4", "1.10.2", "1.11.2", "1.12", "1.12.1", "1.12.2"} {
		request, base, document, files := legacyFixture(t)
		request.GameVersion, request.Version, base.ID = game, "1.2.3-fixture", game
		tweaker, supported := legacyForgeTweakerFor(request)
		if !supported {
			t.Fatal("missing declared generation", game)
		}
		id := game + "-forge" + game + "-" + request.Version
		if tweaker == legacyForge17Tweaker {
			id = game + "-Forge" + request.Version
		}
		core := "net.minecraftforge:forge:" + game + "-" + request.Version
		install := document["install"].(map[string]any)
		install["minecraft"], install["path"], install["version"] = game, core, id
		version := document["versionInfo"].(map[string]any)
		version["id"], version["jar"] = id, game
		delete(version, "inheritsFrom") // Fixed installer + checked core bind this omission.
		version["minecraftArguments"] = strings.ReplaceAll(version["minecraftArguments"].(string), legacyForgeTweaker, tweaker)
		libraries := version["libraries"].([]any)
		libraries[0] = map[string]any{"name": core}
		version["libraries"] = libraries
		files["forge-universal.jar"] = legacyTestZip(t, map[string][]byte{strings.ReplaceAll(tweaker, ".", "/") + ".class": []byte("synthetic-only")})
		if strings.HasPrefix(game, "1.6.") {
			version["minecraftArguments"] = "--username $" + "{auth_player_name} --version $" + "{version_name} --gameDir $" + "{game_directory} --assetsDir $" + "{game_assets} --session $" + "{auth_session} --tweakClass " + tweaker
		}
		document["spec"] = 0
		profile, _, err := prepareLegacy17Fixture(t, request, base, document, files)
		if err != nil {
			t.Fatalf("generation %s: %v", game, err)
		}
		effective, err := profile.Merge(base)
		if err != nil {
			t.Fatal(err)
		}
		gameArgs, _ := meta.SelectArguments(effective.Arguments.Game, meta.CurrentPlatform())
		if !slices.Contains(gameArgs, tweaker) || effective.Java.Major != 8 {
			t.Fatal("generation or game Java was lost")
		}
		if strings.HasPrefix(game, "1.6.") && !slices.Contains(gameArgs, "$"+"{auth_session}") {
			t.Fatal("old session template was lost")
		}
	}
}

func TestLegacyMissingParentRequiresInstallerIdentity(t *testing.T) {
	request, base, document, files := legacyFixture(t)
	version := document["versionInfo"].(map[string]any)
	delete(version, "inheritsFrom")
	delete(version, "jar")
	if _, _, err := readLegacyForgeSpec(t.Context(), legacyArchive(t, document, files), request, base); err != nil {
		t.Fatal("fixed installer identity should bind an omitted parent", err)
	}
	delete(document["install"].(map[string]any), "minecraft")
	if _, _, err := readLegacyForgeSpec(t.Context(), legacyArchive(t, document, files), request, base); err == nil {
		t.Fatal("unbound omission accepted")
	}
}
