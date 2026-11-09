package loader

import (
	"context"
	"crypto/sha1"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"mcl/internal/core/download"
	"mcl/internal/core/java"
	"mcl/internal/core/meta"
)

func syntheticClass(major, minor uint16) []byte {
	body := make([]byte, 8)
	binary.BigEndian.PutUint32(body[:4], 0xcafebabe)
	binary.BigEndian.PutUint16(body[4:6], minor)
	binary.BigEndian.PutUint16(body[6:8], major)
	return body // Headers only, deliberately NOT executable bytecode.
}
func stagedProcessorJar(t *testing.T, entries map[string][]byte) (*os.Root, meta.Download) {
	t.Helper()
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { root.Close() })
	if _, exists := entries["META-INF/MANIFEST.MF"]; !exists {
		entries["META-INF/MANIFEST.MF"] = []byte("Manifest-Version: 1.0" + string([]byte{13, 10}) + "Main-Class: test.Main" + string([]byte{13, 10, 13, 10}))
	}
	data := legacyTestZip(t, entries)
	if err := root.WriteFile("tool.jar", data, 0o600); err != nil {
		t.Fatal(err)
	}
	return root, meta.Download{Path: "tool.jar", SHA1: fmt.Sprintf("%x", sha1.Sum(data)), Size: int64(len(data))}
}

func TestProcessorClassRequirementIsIndependentAndMultiReleaseAware(t *testing.T) {
	root, fixed := stagedProcessorJar(t, map[string][]byte{
		"test/Main.class":                        syntheticClass(52, 0),
		"test/Helper.class":                      syntheticClass(61, 0),
		"META-INF/versions/21/test/Helper.class": syntheticClass(65, 0),
		"META-INF/versions/26/test/Helper.class": syntheticClass(70, 0),
	})
	count := 0
	minimum, err := processorJarMinimum(t.Context(), root, "tool.jar", fixed, "test.Main", &count)
	if err != nil || minimum != 17 || count != 3 {
		t.Fatalf("tool minimum %d count %d: %v", minimum, count, err)
	}
}

func TestProcessorClassScanRejectsUnsafeOrUnsupportedData(t *testing.T) {
	for _, entries := range []map[string][]byte{
		{"test/Main.class": syntheticClass(61, 65535)},
		{"test/Main.class": syntheticClass(70, 0)},
		{"test/Main.class": []byte("bad")},
		{"test/Other.class": syntheticClass(61, 0)},
		{"test/Main.class": syntheticClass(52, 0), "META-INF/versions/9/test/Main.class": syntheticClass(65, 0)},
		{"../test/Main.class": syntheticClass(61, 0)},
		{"test/Main.class": syntheticClass(61, 0), "META-INF/MANIFEST.MF": []byte("Main-Class: test.Main" + string([]byte{10}) + "Class-Path: https://unfixed.invalid/code.jar" + string([]byte{10, 10}))},
		{"test/Main.class": syntheticClass(61, 0), "test/Other.class": syntheticClass(61, 0), "META-INF/MANIFEST.MF": []byte("Main-Class: test.Other" + string([]byte{10, 10}))},
	} {
		root, fixed := stagedProcessorJar(t, entries)
		count := 0
		if _, err := processorJarMinimum(t.Context(), root, "tool.jar", fixed, "test.Main", &count); err == nil {
			t.Fatal("unsupported tool JAR accepted")
		}
	}
	root, fixed := stagedProcessorJar(t, map[string][]byte{"test/Main.class": syntheticClass(61, 0)})
	count := maxProcessorClasses
	if _, err := processorJarMinimum(t.Context(), root, "tool.jar", fixed, "test.Main", &count); err == nil {
		t.Fatal("class scan budget ignored")
	}
	fixed.SHA1 = strings.Repeat("a", 40)
	count = 0
	if _, err := processorJarMinimum(t.Context(), root, "tool.jar", fixed, "test.Main", &count); err == nil {
		t.Fatal("unverified tool bytes scanned")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := processorJarMinimum(ctx, root, "tool.jar", fixed, "test.Main", &count); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel: %v", err)
	}
}

func TestProcessorResolverCannotSupplyEAOrWrongArchitecture(t *testing.T) {
	for _, runtime := range []java.Runtime{
		{Major: 8, Arch: "amd64"}, {Major: 17, Arch: "arm64"}, {Major: 17, Arch: "amd64", EarlyAccess: true}, {Major: 26, Arch: "amd64"},
	} {
		installer := NewForgeInstallerWithJava(nil, nil, nil, func(context.Context, int, meta.Platform, []string, func(java.Progress)) (java.Runtime, error) {
			return runtime, nil
		})
		if _, err := installer.processorRuntime(t.Context(), 17, meta.Platform{OS: "windows", Arch: "amd64"}, nil, nil); err == nil {
			t.Fatal("incompatible tool runtime accepted")
		}
	}
}

// These fixtures use class headers and an injected runner only. They exercise
// the complete selection handoff without spawning Java or using the network.
func TestProcessorsCombineGameBaselineAndBytecodeFloor(t *testing.T) {
	for _, tc := range []struct {
		name              string
		game, class, want int
	}{
		{"forge47-java8-bytecode-on-arm64", 17, 8, 17},
		{"modern-game21-with-old-tool", 21, 8, 21},
		{"tool21-on-game17", 17, 21, 21},
		{"historical-game8-tool8", 8, 8, 8},
		{"historical-game8-new-tool", 8, 17, 17},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, fixed := stagedProcessorJar(t, map[string][]byte{"test/Main.class": syntheticClass(uint16(tc.class+44), 0)})
			stagePath := root.Name()
			gamePath := stagePath + "/game-java"
			toolPath := stagePath + "/tool-java"
			target := meta.Platform{OS: "osx", Arch: "arm64"}
			resolved, runs := 0, 0
			resolver := func(ctx context.Context, minimum int, platform meta.Platform, preferred []string, report func(java.Progress)) (java.Runtime, error) {
				resolved++
				if minimum != tc.want || platform.OS != target.OS || platform.Arch != target.Arch || len(preferred) != 1 || preferred[0] != gamePath {
					t.Fatalf("wrong selection request: %d %+v %v", minimum, platform, preferred)
				}
				return java.Runtime{Executable: toolPath, Major: tc.want, Version: fmt.Sprintf("%d.0.1", tc.want), Arch: platform.Arch}, nil
			}
			runner := func(ctx context.Context, executable, work string, args []string) error {
				runs++
				if executable != toolPath {
					t.Fatal("wrong tool executable", executable)
				}
				return ctx.Err()
			}
			installer := NewForgeInstallerWithJava(nil, nil, runner, resolver)
			base := meta.Version{}
			base.Java.Major = tc.game
			processor := preparedProcessor{jar: "tool.jar", classpath: []string{"tool.jar"}, inputs: []string{"tool.jar"}}
			err := installer.runProcessors(t.Context(), root, root, stagePath, gamePath, target, base, []preparedProcessor{processor}, map[string]meta.Download{"tool.jar": fixed}, func(download.Progress) {}, nil)
			if err != nil || resolved != 1 || runs != 1 {
				t.Fatal(err, resolved, runs)
			}
			if base.Java.Major != tc.game {
				t.Fatal("processor selection changed game Java")
			}
		})
	}
}
