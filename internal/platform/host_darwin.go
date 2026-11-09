package platform

import "golang.org/x/sys/unix"

func operatingSystemVersion() (string, error) {
	// Mojang's historical ^10\.5\.\d$ rule means the macOS product version,
	// not Darwin's kernel release. Do not use kern.osrelease as a fallback.
	return unix.Sysctl("kern.osproductversion")
}
