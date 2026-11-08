package app

import (
	"context"
	"errors"
	"testing"

	"mcl/internal/core/java"
	"mcl/internal/core/meta"
)

func TestMatchingJava8RequiresStableExactMajorArchitecture(t *testing.T) {
	environment := func(path, version, arch string, major int, ea bool) JavaEnvironment {
		return JavaEnvironment{Runtime: java.Runtime{Executable: path, Version: version, Major: major, Arch: arch, EarlyAccess: ea}}
	}
	environments := []JavaEnvironment{
		environment("modern", "17.0.12", "amd64", 17, false),
		environment("ea", "1.8.0_999-ea", "amd64", 8, true),
		environment("foreign", "1.8.0_999", "arm64", 8, false),
		environment("older", "1.8.0_402", "amd64", 8, false),
		environment("latest", "1.8.0_442", "amd64", 8, false),
	}
	target := meta.Platform{OS: "windows", Arch: "amd64"}
	got, ok := matchingJava(environments, 8, target, []string{"modern", "ea", "foreign"})
	if !ok || got.Executable != "latest" {
		t.Fatalf("wrong Java 8 choice: %+v", got)
	}
	got, ok = matchingJava(environments, 8, target, []string{"older"})
	if !ok || got.Executable != "older" {
		t.Fatal("compatible explicit preference ignored")
	}
	if _, ok := matchingJava(environments[:3], 8, target, nil); ok {
		t.Fatal("incompatible fallback accepted")
	}
}

type selectionFailureStore struct{ err error }

func (s selectionFailureStore) Load(context.Context) (JavaRegistration, error) {
	return JavaRegistration{}, s.err
}
func (selectionFailureStore) Register(context.Context, string) error { panic("unexpected register") }
func (selectionFailureStore) Hide(context.Context, string) error     { panic("unexpected hide") }

func TestEnsureJava8EntersExistingInventoryPipeline(t *testing.T) {
	sentinel := errors.New("synthetic inventory failure")
	service := NewJavaService(selectionFailureStore{sentinel}, nil, nil, nil)
	_, err := service.Ensure(t.Context(), 8, meta.Platform{OS: "linux", Arch: "amd64"}, nil, nil)
	if !errors.Is(err, sentinel) {
		t.Fatalf("Java 8 rejected before inventory: %v", err)
	}
}
