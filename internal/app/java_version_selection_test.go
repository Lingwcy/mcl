package app

import (
	"mcl/internal/core/java"
	"mcl/internal/core/meta"
	"testing"
)

func TestGameAndProcessorShareNormalizedJava8UpdateOrdering(t *testing.T) {
	env := []JavaEnvironment{
		{Runtime: java.Runtime{Executable: "old-alias", Version: "8u402", Major: 8, Arch: "amd64"}},
		{Runtime: java.Runtime{Executable: "new-legacy", Version: "1.8.0_442", Major: 8, Arch: "amd64"}},
	}
	target := meta.Platform{OS: "linux", Arch: "amd64"}
	game, ok := matchingJava(env, 8, target, nil)
	if !ok || game.Executable != "new-legacy" {
		t.Fatal(game, ok)
	}
	tool, ok := matchingProcessorJava(env, 8, target, nil)
	if !ok || tool.Executable != "new-legacy" {
		t.Fatal(tool, ok)
	}
	game, ok = matchingJava(env, 8, target, []string{"old-alias"})
	if !ok || game.Executable != "old-alias" {
		t.Fatal("compatible explicit preference ignored", game)
	}
}
func TestVendorBuildMetadataDoesNotWinAutomaticJavaSelection(t *testing.T) {
	env := []JavaEnvironment{
		{Runtime: java.Runtime{Executable: "zulu", Version: "17.0.12+7-vendor.999", Major: 17, Arch: "amd64"}},
		{Runtime: java.Runtime{Executable: "adoptium", Version: "17.0.12+7-vendor.1", Major: 17, Arch: "amd64"}},
		{Runtime: java.Runtime{Executable: "invalid", Version: "21.0.12", Major: 17, Arch: "amd64"}},
	}
	target := meta.Platform{OS: "linux", Arch: "amd64"}
	game, ok := matchingJava(env, 17, target, nil)
	if !ok || game.Executable != "adoptium" {
		t.Fatal(game, ok)
	}
	tool, ok := matchingProcessorJava(env, 17, target, nil)
	if !ok || tool.Executable != "adoptium" {
		t.Fatal(tool, ok)
	}
}
