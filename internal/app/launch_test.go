package app_test

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"mcl/internal/app"
	"mcl/internal/core/java"
	"mcl/internal/core/launch"
	"mcl/internal/core/meta"
	"mcl/internal/storage"
)

type fakeGame struct {
	done    chan struct{}
	stopped atomic.Int32
}

func (g *fakeGame) PID() int                      { return 123 }
func (g *fakeGame) Done() <-chan struct{}         { return g.done }
func (g *fakeGame) Wait() launch.Exit             { <-g.done; return launch.Exit{Code: 0} }
func (g *fakeGame) Stop(context.Context) error    { g.stopped.Add(1); return nil }
func (g *fakeGame) Detach()                       {}
func (g *fakeGame) Output() launch.OutputSnapshot { return launch.OutputSnapshot{} }
func (g *fakeGame) Changed() <-chan struct{}      { return nil }

func preparedFixture(t *testing.T, starter app.ProcessStarter) (installFixture, *app.LaunchService, app.Settings) {
	t.Helper()
	f := newInstallFixture(t)
	fallback := app.DefaultSettings()
	fallback.JavaPath = filepath.Join(f.root, "Java Home/bin/java")
	fallback.MaxMemoryMiB = 8192
	resolveJava := func(ctx context.Context, major int, target meta.Platform, preferred []string, _ func(java.Progress)) (java.Runtime, error) {
		path := preferred[1]
		if path != fallback.JavaPath {
			t.Error("wrong fallback Java")
		}
		return java.Runtime{Executable: path, Version: "17.0.1", Major: 17, Arch: meta.CurrentPlatform().Arch}, ctx.Err()
	}
	// Deliberately differ from both persisted heap values to exercise automatic
	// allocation rather than accidentally retaining the former 4096 MiB default.
	return f, app.NewLaunchService(f.service, resolveJava, func() (uint64, error) { return 12 << 30, nil }, starter, nil), fallback
}

func authenticatedFixture(t *testing.T) launch.Session {
	t.Helper()
	session, err := launch.NewSession("Player", strings.Repeat("a", 32), "synthetic-app-token-1234567890", "test-client", "12345", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	return session
}

func TestPreparedPreviewReservationAndCancel(t *testing.T) {
	f, service, settings := preparedFixture(t, nil)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	prepared, err := service.Prepare(ctx, f.value.ID, settings, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Close()
	preview, err := prepared.Preview()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(preview.Arguments, "<登录后提供>") || !slices.Contains(preview.Arguments, "-Xmx3072m") {
		t.Fatal("preview lost automatic memory allocation or invented authentication")
	}
	if _, err := f.service.Run(t.Context(), f.value.ID, nil); err == nil {
		t.Fatal("repair entered a reserved instance")
	}
	cancel()
	prepared.Close()
	if _, err := prepared.Start(authenticatedFixture(t)); err == nil {
		t.Fatal("canceled preparation started a process")
	}
	if _, err := f.service.Run(t.Context(), f.value.ID, nil); err != nil {
		t.Fatalf("preparation did not release reservation: %v", err)
	}
}

func TestLiveGameOwnsReservationAfterPreparationCancel(t *testing.T) {
	game := &fakeGame{done: make(chan struct{})}
	var alive *atomic.Bool
	f, service, settings := preparedFixture(t, func(plan launch.Plan, register func(launch.ProcessIdentity) error) (launch.Game, error) {
		alive.Store(true)
		if err := register(launch.ProcessIdentity{PID: 123, OS: runtime.GOOS, Stamp: "synthetic:1"}); err != nil {
			return nil, err
		}
		return game, nil
	})
	alive = f.running
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	prepared, err := service.Prepare(ctx, f.value.ID, settings, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Close()
	live, err := prepared.Start(authenticatedFixture(t))
	if err != nil || live != game {
		t.Fatalf("start %v", err)
	}
	cancel()
	prepared.Close()
	if game.stopped.Load() != 0 {
		t.Fatal("preparation canceled an already running game")
	}
	registry := storage.NewSessionStore(f.root, func(launch.ProcessIdentity) (bool, error) { return alive.Load(), nil })
	records, err := registry.Active(t.Context())
	if err != nil || len(records) != 1 || records[0].VersionSHA1 != f.value.Version.SHA1 || records[0].InstanceID != f.value.ID {
		t.Fatalf("persisted launch binding: %+v %v", records, err)
	}
	if _, err := f.service.Run(t.Context(), f.value.ID, nil); err == nil {
		t.Fatal("repair replaced live native files")
	}
	if _, err := prepared.Start(authenticatedFixture(t)); err == nil {
		t.Fatal("prepared instance started twice")
	}
	alive.Store(false)
	close(game.done)
	deadline := time.Now().Add(time.Second)
	for {
		if _, err := f.service.Run(t.Context(), f.value.ID, nil); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("exited game did not release the reservation")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestFailedStartRedactsAndReleases(t *testing.T) {
	token := "synthetic-app-token-1234567890"
	f, service, settings := preparedFixture(t, func(plan launch.Plan, _ func(launch.ProcessIdentity) error) (launch.Game, error) {
		return nil, fmt.Errorf("failed with %s", token)
	})
	prepared, err := service.Prepare(t.Context(), f.value.ID, settings, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Close()
	if _, err := prepared.Start(authenticatedFixture(t)); err == nil || strings.Contains(err.Error(), token) || !strings.Contains(err.Error(), "[REDACTED]") {
		t.Fatalf("unsafe start error: %v", err)
	}
	if _, err := f.service.Run(t.Context(), f.value.ID, nil); err != nil {
		t.Fatal("failed start retained the reservation")
	}
}

func TestJavaFailurePrecedesDownloads(t *testing.T) {
	f := newInstallFixture(t)
	service := app.NewLaunchService(f.service, func(context.Context, int, meta.Platform, []string, func(java.Progress)) (java.Runtime, error) {
		return java.Runtime{}, errors.New("bad Java")
	}, nil, nil, nil)
	settings := app.DefaultSettings()
	settings.JavaPath = filepath.Join(f.root, "Java/bin/java")
	if _, err := service.Prepare(t.Context(), f.value.ID, settings, nil); err == nil {
		t.Fatal("bad Java accepted")
	}
	if f.requests.Load() != 0 {
		t.Fatal("download began before Java validation")
	}
	if _, err := f.service.Run(t.Context(), f.value.ID, nil); err != nil {
		t.Fatal("failed Java check retained the reservation")
	}
}
