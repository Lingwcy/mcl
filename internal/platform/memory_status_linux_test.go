package platform

import "testing"

func TestLinuxMemoryUsesAvailableRatherThanOnlyFreePages(t *testing.T) {
	text := "MemTotal: 16777216 kB\nMemFree: 1024 kB\nMemAvailable: 4194304 kB\nCached: 4000000 kB\n"
	got, err := parseLinuxMemoryStatus(text)
	if err != nil || got.TotalBytes != 16<<30 || got.AvailableBytes != 4<<30 {
		t.Fatalf("bad available-memory calculation: %+v %v", got, err)
	}
	for _, text := range []string{"MemTotal: 100 kB\n", "MemTotal: 100 kB\nMemAvailable: 101 kB\n", "MemTotal: -1 kB\nMemAvailable: 0 kB\n", "MemTotal: 100 MB\nMemAvailable: 0 kB\n", "MemTotal: 100 kB\nMemTotal: 100 kB\nMemAvailable: 0 kB\n", "MemTotal: 18446744073709551615 kB\nMemAvailable: 0 kB\n"} {
		if _, err := parseLinuxMemoryStatus(text); err == nil {
			t.Fatalf("accepted invalid statistics %q", text)
		}
	}
}
