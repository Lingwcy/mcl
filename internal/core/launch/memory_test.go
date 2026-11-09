package launch

import (
	"mcl/internal/core/java"
	"testing"
)

func TestAutomaticMemoryAllocation(t *testing.T) {
	for _, test := range []struct {
		name, arch string
		available  uint64
		want       int
	}{
		{"zero availability", "arm64", 0, 0},
		{"reserve consumes budget", "amd64", 512 << 20, 0},
		{"below reserve", "amd64", 256 << 20, 0},
		{"one GiB available", "amd64", 1 << 30, 409},
		{"four GiB available", "arm64", 4 << 30, 2867},
		{"eight GiB after reserve", "amd64", (8 << 30) + (512 << 20), 6553},
		{"twelve GiB available", "arm64", 12 << 30, 7270},
		{"thirty-two GiB available", "amd64", 32 << 30, 11366},
		{"maximum", "amd64", 64 << 30, 16384},
		{"32-bit ceiling", "386", 32 << 30, 1024},
		{"32-bit low availability", "386", 1 << 30, 409},
		{"minimum viable heap", "arm64", 1152 << 20, 512},
		{"just below viable heap", "arm64", (1152 << 20) - 1, 511},
		{"large counter cannot overflow", "amd64", ^uint64(0), 16384},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := AutomaticMemoryMiB(java.Runtime{Arch: test.arch}, test.available); got != test.want {
				t.Fatalf("allocated %d MiB, want %d", got, test.want)
			}
		})
	}
}

func TestAutomaticMemoryBudgetNeverExceedsAvailableAfterReserve(t *testing.T) {
	for _, available := range []uint64{0, 1, 512 << 20, 1 << 30, 4 << 30, 9 << 30, 32 << 30, 128 << 30} {
		got := uint64(AutomaticMemoryMiB(java.Runtime{Arch: "amd64"}, available)) << 20
		budget := available - min(available, 512<<20)
		if got > budget || got > 16<<30 {
			t.Fatalf("heap %d exceeds budget %d", got, budget)
		}
	}
}
