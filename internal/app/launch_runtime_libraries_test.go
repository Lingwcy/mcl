package app

import (
	"context"
	"crypto/sha1"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mcl/internal/core/instance"
	"mcl/internal/core/java"
	"mcl/internal/core/launch"
	"mcl/internal/core/meta"
)

func TestPreparedStartRejectsLookupChangedAfterPreparation(t *testing.T) {
	raw, err := os.ReadFile("../core/meta/testdata/1.20.4.json")
	if err != nil {
		t.Fatal(err)
	}
	version, err := meta.ParseVersion(raw)
	if err != nil {
		t.Fatal(err)
	}
	version.LibraryDirectory = "cache/loader-runtimes/" + strings.Repeat("a", 64) + "/ready-" + strings.Repeat("A", 26) + "/libraries"
	coordinate, _ := meta.ParseCoordinate("net.minecraftforge:forge:1.20.4-49.0.3:client")
	data := []byte("synthetic generated client, never executed")
	version.RuntimeLookups = []meta.Download{{Path: coordinate.Path(), Size: int64(len(data)), SHA1: fmt.Sprintf("%x", sha1.Sum(data))}}
	root := t.TempDir()
	name := filepath.Join(root, filepath.FromSlash(version.LibraryDirectory), filepath.FromSlash(coordinate.Path()))
	if err := os.MkdirAll(filepath.Dir(name), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, data, 0600); err != nil {
		t.Fatal(err)
	}
	id := instance.NewID()
	target := meta.CurrentPlatform()
	options := launch.Options{Root: root, InstanceID: id, Version: version, Platform: target, Java: java.Runtime{Executable: filepath.Join(root, "java"), Major: 17, Version: "17.0.16", Arch: target.Arch}, NativeDirectory: "instances/" + id + "/natives/ready-" + strings.Repeat("B", 26), MaxMemoryMiB: 2048}
	if err := verifyLaunchLibraries(t.Context(), options); err != nil {
		t.Fatal(err)
	}
	// Simulate mutation while an interactive authentication flow is pending.
	wrong := append([]byte(nil), data...)
	wrong[0] = 'S'
	if err := os.WriteFile(name, wrong, 0600); err != nil {
		t.Fatal(err)
	}
	started := false
	released := false
	p := &Prepared{ctx: t.Context(), options: options, settled: make(chan struct{}), release: func() { released = true }, memoryProbe: func(context.Context) (MemoryStatus, error) { return MemoryStatus{AvailableBytes: 8 << 30}, nil }, start: func(launch.Plan, func(launch.ProcessIdentity) error) (launch.Game, error) {
		started = true
		return nil, nil
	}}
	session, err := launch.NewOfflineSession("Player", strings.Repeat("a", 32))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Start(session); err == nil || started {
		t.Fatal("changed output reached process handoff", err)
	}
	p.Close()
	if !released {
		t.Fatal("reservation was not released on close")
	}
}
