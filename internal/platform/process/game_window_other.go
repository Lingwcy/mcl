//go:build !darwin

package process

import "mcl/internal/core/launch"

func GameWindowProbe() func(launch.ProcessIdentity) (bool, error) { return nil }
