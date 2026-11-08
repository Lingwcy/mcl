package process

import (
	"golang.org/x/sys/windows"
	"mcl/internal/core/launch"
	"testing"
)

func TestWindowsPriorityMappingDoesNotExposeRealtime(t *testing.T) {
	for _, tc := range []struct {
		priority launch.ProcessPriority
		class    uint32
	}{{launch.PriorityLow, windows.IDLE_PRIORITY_CLASS}, {launch.PriorityBelowNormal, windows.BELOW_NORMAL_PRIORITY_CLASS}, {launch.PriorityNormal, windows.NORMAL_PRIORITY_CLASS}, {launch.PriorityAboveNormal, windows.ABOVE_NORMAL_PRIORITY_CLASS}, {launch.PriorityHigh, windows.HIGH_PRIORITY_CLASS}} {
		got, err := priorityClass(tc.priority)
		if err != nil || got != tc.class || got == windows.REALTIME_PRIORITY_CLASS {
			t.Fatalf("%s: %d %v", tc.priority, got, err)
		}
	}
	if _, err := priorityClass(launch.ProcessPriority("realtime")); err == nil {
		t.Fatal("accepted real-time scheduling")
	}
}
