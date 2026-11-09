//go:build darwin || linux

package process

// The bounded anonymous gate handoff remains the Unix transport limit. Kernel
// argument/environment limits are reported by exec without a shell fallback.
func validateCommandSize(string, []string) error { return nil }
