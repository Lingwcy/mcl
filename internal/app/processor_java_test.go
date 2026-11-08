package app

import (
	"context"
	"errors"
	"testing"

	"mcl/internal/core/java"
	"mcl/internal/core/meta"
)

func TestProcessorJavaPolicyDoesNotReplaceExactGameJava(t *testing.T) {
	environment := func(path, version, arch string, major int, ea bool) JavaEnvironment {
		return JavaEnvironment{Runtime: java.Runtime{Executable: path, Version: version, Arch: arch, Major: major, EarlyAccess: ea}}
	}
	values := []JavaEnvironment{
		environment("game8", "1.8.0_442", "amd64", 8, false),
		environment("tool17-old", "17.0.10", "amd64", 17, false),
		environment("tool17-new", "17.0.16", "amd64", 17, false),
		environment("tool21", "21.0.8", "amd64", 21, false),
		environment("ea", "25-ea", "amd64", 25, true),
		environment("foreign", "21.0.8", "arm64", 21, false),
	}
	target := meta.Platform{OS: "windows", Arch: "amd64"}
	legacy, found := matchingProcessorJava(values, 8, target, []string{"tool21"})
	if !found || legacy.Executable != "game8" {
		t.Fatal("old tool was silently moved to a modern JVM")
	}
	tool, ok := matchingProcessorJava(values, 17, target, []string{"game8", "ea", "foreign"})
	if !ok || tool.Executable != "tool17-new" {
		t.Fatalf("wrong tool choice %+v", tool)
	}
	tool, ok = matchingProcessorJava(values, 17, target, []string{"tool21"})
	if !ok || tool.Executable != "tool21" {
		t.Fatal("compatible explicit tool hint ignored")
	}
	game, ok := matchingJava(values, 8, target, []string{"tool21"})
	if !ok || game.Executable != "game8" {
		t.Fatal("tool selection changed exact game-major semantics")
	}
	if _, ok := matchingProcessorJava(values, 22, target, nil); ok {
		t.Fatal("EA or foreign tool environment accepted")
	}
	for minimum, want := range map[int]int{8: 8, 9: 17, 17: 17, 18: 21, 21: 21, 22: 25, 25: 25} {
		got, err := processorDownloadMajor(minimum)
		if err != nil || got != want {
			t.Fatalf("download policy %d: %d %v", minimum, got, err)
		}
	}
	for _, bad := range []int{7, 26, 100} {
		if _, err := processorDownloadMajor(bad); err == nil {
			t.Fatal("unknown tool requirement accepted")
		}
	}
}

func TestEnsureProcessorUsesExistingInventoryFailureAndCancellation(t *testing.T) {
	sentinel := errors.New("synthetic inventory failure")
	service := NewJavaService(selectionFailureStore{sentinel}, nil, nil, nil)
	_, err := service.EnsureProcessor(t.Context(), 17, meta.Platform{OS: "linux", Arch: "amd64"}, nil, nil)
	if !errors.Is(err, sentinel) {
		t.Fatalf("tool discovery bypassed service inventory: %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := service.EnsureProcessor(ctx, 17, meta.Platform{OS: "linux", Arch: "amd64"}, nil, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel: %v", err)
	}
}
