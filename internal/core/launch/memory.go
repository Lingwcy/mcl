package launch

import "mcl/internal/core/java"

// AutomaticMemoryMiB mirrors the reference launcher's default: one quarter of
// physical RAM, bounded to 2–6 GiB on 64-bit JVMs and 1 GiB on 32-bit JVMs.
// A failed system probe (zero) selects the conservative 2 GiB floor.
func AutomaticMemoryMiB(runtime java.Runtime, physicalBytes uint64) int {
	if runtime.Arch == "386" {
		return 1024
	}
	return int(max(2048, min((physicalBytes>>20)/4, 6144)))
}
