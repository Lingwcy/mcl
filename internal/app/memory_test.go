package app

import (
	"context"
	"errors"
	"mcl/internal/core/java"
	"testing"
)

func TestMemorySnapshotUsesTheLaunchAutomaticPolicy(t *testing.T) {
	const gib = uint64(1) << 30
	for _, tc := range []struct {
		total, available uint64
		arch             string
		want             int
	}{{16 * gib, 4 * gib, "arm64", 2867}, {64 * gib, 32 * gib, "amd64", 11366}, {4 * gib, gib, "amd64", 409}, {16 * gib, 4 * gib, "386", 1024}} {
		service := NewMemoryService(func(context.Context) (MemoryStatus, error) {
			return MemoryStatus{TotalBytes: tc.total, AvailableBytes: tc.available}, nil
		}, tc.arch)
		got, err := service.Snapshot(t.Context())
		if err != nil || got.SuggestedMiB != tc.want || got.UsedBytes() != tc.total-tc.available || got.CapturedAt.IsZero() {
			t.Fatalf("bad memory snapshot: %+v %v", got, err)
		}
	}
}
func TestMemorySnapshotRejectsInvalidOrCanceledRead(t *testing.T) {
	for _, status := range []MemoryStatus{{}, {TotalBytes: 1, AvailableBytes: 2}, {TotalBytes: 1 << 61}} {
		service := NewMemoryService(func(context.Context) (MemoryStatus, error) { return status, nil }, "amd64")
		if _, err := service.Snapshot(t.Context()); err == nil {
			t.Fatal("invalid system statistics accepted")
		}
	}
	called := false
	service := NewMemoryService(func(context.Context) (MemoryStatus, error) { called = true; return MemoryStatus{}, nil }, "amd64")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := service.Snapshot(ctx); !errors.Is(err, context.Canceled) || called {
		t.Fatal("canceled memory probe still ran")
	}
}

func TestLaunchMemoryRejectsExhaustionAndProbeFailure(t *testing.T) {
	runtime := java.Runtime{Arch: "arm64"}
	exhausted := func(context.Context) (MemoryStatus, error) {
		return MemoryStatus{TotalBytes: 16 << 30, AvailableBytes: 1 << 30}, nil
	}
	if _, err := launchMemoryMiB(t.Context(), exhausted, runtime); err == nil {
		t.Fatal("exhausted memory silently forced the former 2 GiB minimum")
	}
	cause := errors.New("memory unavailable")
	broken := func(context.Context) (MemoryStatus, error) { return MemoryStatus{}, cause }
	if _, err := launchMemoryMiB(t.Context(), broken, runtime); !errors.Is(err, cause) {
		t.Fatal("probe failure became an invented available-memory budget")
	}
	if _, err := launchMemoryMiB(t.Context(), nil, runtime); err == nil {
		t.Fatal("missing probe became a successful allocation")
	}
	enough := func(context.Context) (MemoryStatus, error) {
		return MemoryStatus{TotalBytes: 16 << 30, AvailableBytes: 1152 << 20}, nil
	}
	if got, err := launchMemoryMiB(t.Context(), enough, runtime); err != nil || got != 512 {
		t.Fatalf("minimum viable heap: %d %v", got, err)
	}
}
