package process

import (
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	"mcl/internal/core/launch"
)

func TestPriorityIsAppliedAfterIdentityRegistration(t *testing.T) {
	identity := launch.ProcessIdentity{PID: 1234, OS: "linux", Stamp: "fixture-boot:456"}
	var events []string
	notice, err := prepareChildPriority(identity.PID, launch.PriorityHigh,
		func(actual launch.ProcessIdentity) error {
			events = append(events, "register")
			if actual != identity {
				t.Fatal("registered another process")
			}
			return nil
		},
		func(pid int) (launch.ProcessIdentity, error) {
			events = append(events, "inspect")
			if pid != identity.PID {
				t.Fatal("inspected another PID")
			}
			return identity, nil
		},
		func(actual launch.ProcessIdentity, priority launch.ProcessPriority) error {
			events = append(events, "priority")
			if actual != identity || priority != launch.PriorityHigh {
				t.Fatal("changed identity or priority")
			}
			return nil
		})
	if err != nil || notice != "" || !reflect.DeepEqual(events, []string{"inspect", "register", "priority"}) {
		t.Fatalf("handoff order: %v, %q, %v", events, notice, err)
	}
}

func TestPriorityRefusalWarnsButIdentityFailureAborts(t *testing.T) {
	identity := launch.ProcessIdentity{PID: 1234, OS: "linux", Stamp: "fixture:456"}
	inspect := func(int) (launch.ProcessIdentity, error) { return identity, nil }
	registrationFailure := errors.New("registration failed")
	applied := false
	_, err := prepareChildPriority(identity.PID, launch.PriorityHigh, func(launch.ProcessIdentity) error { return registrationFailure }, inspect,
		func(launch.ProcessIdentity, launch.ProcessPriority) error { applied = true; return nil })
	if !errors.Is(err, registrationFailure) || applied {
		t.Fatal("priority applied before a failed registration")
	}
	for _, tc := range []struct {
		cause error
		want  string
	}{{os.ErrPermission, "系统权限不足"}, {errors.New("native internal detail with secret"), "系统未接受设置"}} {
		notice, err := prepareChildPriority(identity.PID, launch.PriorityHigh, func(launch.ProcessIdentity) error { return nil }, inspect,
			func(launch.ProcessIdentity, launch.ProcessPriority) error { return tc.cause })
		if err != nil || !strings.Contains(notice, tc.want) || strings.Contains(notice, "secret") {
			t.Fatalf("priority refusal not safely reported: %q %v", notice, err)
		}
	}
	_, err = prepareChildPriority(identity.PID, launch.PriorityHigh, func(launch.ProcessIdentity) error { return nil }, inspect,
		func(launch.ProcessIdentity, launch.ProcessPriority) error { return errPriorityIdentityChanged })
	if !errors.Is(err, errPriorityIdentityChanged) {
		t.Fatal("identity mismatch was downgraded to a warning")
	}
}

func TestPriorityCannotTargetAnUntrackedOrDifferentProcess(t *testing.T) {
	inspected, applied, registered := false, false, false
	inspect := func(pid int) (launch.ProcessIdentity, error) {
		inspected = true
		return launch.ProcessIdentity{PID: pid + 1, OS: "linux", Stamp: "boot:1"}, nil
	}
	apply := func(launch.ProcessIdentity, launch.ProcessPriority) error { applied = true; return nil }
	register := func(launch.ProcessIdentity) error { registered = true; return nil }
	if _, err := prepareChildPriority(1234, launch.PriorityHigh, nil, inspect, apply); err == nil || inspected || applied {
		t.Fatal("untracked priority change was allowed")
	}
	if _, err := prepareChildPriority(1234, launch.PriorityHigh, register, inspect, apply); !errors.Is(err, errPriorityIdentityChanged) || applied || registered {
		t.Fatal("PID mismatch reached registration or priority")
	}
	inspected = false
	if _, err := prepareChildPriority(1234, launch.ProcessPriority("realtime"), register, inspect, apply); err == nil || inspected {
		t.Fatal("unknown/real-time priority reached the process adapter")
	}
}
