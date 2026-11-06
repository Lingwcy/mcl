package launch

import (
	"testing"

	"mcl/internal/core/java"
)

func TestAutomaticMemoryAllocation(t *testing.T) {
	for _, test := range []struct {
		name string
		arch string
		ram  uint64
		want int
	}{
		{"32-bit JVM", "386", 32 << 30, 1024},
		{"minimum", "amd64", 4 << 30, 2048},
		{"quarter", "arm64", 12 << 30, 3072},
		{"maximum", "amd64", 64 << 30, 6144},
		{"unknown physical memory", "arm64", 0, 2048},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := AutomaticMemoryMiB(java.Runtime{Arch: test.arch}, test.ram); got != test.want {
				t.Fatalf("allocated %d MiB, want %d", got, test.want)
			}
		})
	}
}
