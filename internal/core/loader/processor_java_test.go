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
