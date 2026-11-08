package ui

import (
	"mcl/internal/app"
	"testing"
)

func TestMemoryPreviewSegmentsNeverOverflowOrClaimAReservation(t *testing.T) {
	const gib = uint64(1) << 30
	for _, tc := range []struct {
		total, available uint64
		miB              int
		used, planned    float32
	}{{16 * gib, 4 * gib, 4096, .75, .25}, {16 * gib, gib, 4096, .9375, .0625}, {16 * gib, 16 * gib, 4096, 0, .25}, {16 * gib, 0, 4096, 1, 0}, {0, 0, 4096, 0, 0}} {
		snapshot := app.MemorySnapshot{MemoryStatus: app.MemoryStatus{TotalBytes: tc.total, AvailableBytes: tc.available}}
		used, planned := memoryBarFractions(snapshot, tc.miB)
		if used != tc.used || planned != tc.planned || used+planned > 1 {
			t.Fatalf("memory bar: %v %v", used, planned)
		}
	}
}
func TestMemoryObservationIsScopedAndRejectsLateCallbacks(t *testing.T) {
	shell := &Shell{page: gameSettingsPage}
	var callbacks []func(app.MemorySnapshot, error)
	starts, stops := 0, 0
	shell.ConnectMemory(func(done func(app.MemorySnapshot, error)) func() {
		starts++
		callbacks = append(callbacks, done)
		return func() { stops++ }
	})
	shell.syncMemoryObservation()
	shell.syncMemoryObservation()
	if starts != 1 {
		t.Fatal("rebuild started another sampler")
	}
	sample := app.MemorySnapshot{MemoryStatus: app.MemoryStatus{TotalBytes: 16 << 30, AvailableBytes: 8 << 30}, SuggestedMiB: 4096}
	callbacks[0](sample, nil)
	if !shell.memory.valid {
		t.Fatal("live snapshot was ignored")
	}
	shell.page = libraryPage
	shell.syncMemoryObservation()
	if stops != 1 || shell.memory.active {
		t.Fatal("navigation did not stop sampling")
	}
	shell.page = gameSettingsPage
	shell.syncMemoryObservation()
	if starts != 2 || shell.memory.valid {
		t.Fatal("new observation reused an old result as fresh")
	}
	callbacks[0](sample, nil)
	if shell.memory.valid {
		t.Fatal("late canceled callback replaced the current observation")
	}
	callbacks[1](sample, nil)
	if !shell.memory.valid {
		t.Fatal("current sample was dropped")
	}
	shell.memory.blurred = true
	shell.syncMemoryObservation()
	if stops != 2 {
		t.Fatal("background window kept polling")
	}
}
