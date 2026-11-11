package process

import (
	"errors"
	"testing"

	"mcl/internal/core/launch"
)

func TestWindowObservationRequiresSameProcessBeforeAndAfter(t *testing.T) {
	expected := launch.ProcessIdentity{PID: 123, OS: "windows", Stamp: "1234"}
	for _, tc := range []struct {
		name                  string
		before, after, window bool
		want                  bool
	}{
		{"visible", true, true, true, true}, {"not-visible", true, true, false, false}, {"already-exited", false, true, true, false}, {"pid-reused", true, false, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			checks, queries := 0, 0
			alive := func(got launch.ProcessIdentity) (bool, error) {
				if got != expected {
					t.Fatal("different identity queried")
				}
				checks++
				if checks == 1 {
					return tc.before, nil
				}
				return tc.after, nil
			}
			query := func(pid int) (bool, error) {
				queries++
				if pid != expected.PID {
					t.Fatal("foreign PID queried")
				}
				return tc.window, nil
			}
			visible, err := observeProcessWindow(expected, alive, query)
			if err != nil || visible != tc.want {
				t.Fatal(visible, err)
			}
			if !tc.before && queries != 0 {
				t.Fatal("desktop queried for an expired process")
			}
			if tc.before && tc.window && checks != 2 {
				t.Fatal("visible observation not followed by identity check")
			}
		})
	}
	sentinel := errors.New("desktop unavailable")
	if visible, err := observeProcessWindow(expected, func(launch.ProcessIdentity) (bool, error) { return true, nil }, func(int) (bool, error) { return false, sentinel }); visible || !errors.Is(err, sentinel) {
		t.Fatal("probe failure became absence/success", visible, err)
	}
}

func TestVisibleWindowFiltersMinimizedCloakedAndForeignWindows(t *testing.T) {
	for _, tc := range []struct {
		owner                       int
		visible, minimized, cloaked bool
		width, height               int64
		want                        bool
	}{
		{123, true, false, false, 854, 480, true}, {456, true, false, false, 854, 480, false},
		{123, false, false, false, 854, 480, false}, {123, true, true, false, 854, 480, false},
		{123, true, false, true, 854, 480, false}, {123, true, false, false, 1, 1, false}, {123, true, false, false, -1, 480, false},
	} {
		if got := visibleClientWindow(123, tc.owner, tc.visible, tc.minimized, tc.cloaked, tc.width, tc.height); got != tc.want {
			t.Fatal("window evidence was overclaimed")
		}
	}
}
