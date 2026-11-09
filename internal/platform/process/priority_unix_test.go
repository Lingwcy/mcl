//go:build darwin || linux

package process

import (
	"mcl/internal/core/launch"
	"testing"
)

func TestUnixPriorityMappingHasNoRealtimeOrOutOfRangeValue(t *testing.T) {
	for _, tc := range []struct {
		priority launch.ProcessPriority
		nice     int
	}{{launch.PriorityLow, 5}, {launch.PriorityBelowNormal, 1}, {launch.PriorityNormal, 0}, {launch.PriorityAboveNormal, -1}, {launch.PriorityHigh, -5}} {
		got, err := priorityNice(tc.priority)
		if err != nil || got != tc.nice {
			t.Fatalf("%s: %d %v", tc.priority, got, err)
		}
	}
	if _, err := priorityNice(launch.ProcessPriority("realtime")); err == nil {
		t.Fatal("accepted an unsupported priority")
	}
}
