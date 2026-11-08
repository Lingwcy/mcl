package launch

import "mcl/internal/core/java"

// AutomaticMemoryMiB uses the current available RAM, not installed RAM:
// reserve 512 MiB, use 80% of the next 8 GiB and 20% of the remainder,
// capped at 16 GiB. A 32-bit Java retains its conservative 1 GiB ceiling.
// Zero is a real exhausted-memory result, never an unknown-probe fallback.
// The caller must distinguish a failed probe and reject an unusable heap.
func AutomaticMemoryMiB(runtime java.Runtime, availableBytes uint64) int {
	const reserve = uint64(512) << 20
	const threshold = uint64(8) << 30
	const maximum = uint64(16) << 30
	available := availableBytes - min(availableBytes, reserve)
	first := min(available, threshold)
	suggested := min(first*4/5+(available-first)/5, maximum) >> 20
	if runtime.Arch == "386" {
		suggested = min(suggested, 1024)
	}
	return int(suggested)
}
