package launch

import (
	"context"
	"time"
)

type Exit struct {
	Code                                int
	Error                               string
	StartedAt, FinishedAt               time.Time
	Stopped, Detached, OutputIncomplete bool
}

// Game separates the live process from cancelable installation/preparation.
// A platform adapter owns reaping and draining; the application owns whether
// to stop the process explicitly or detach its observation during quit.
type Game interface {
	PID() int
	Done() <-chan struct{}
	Wait() Exit
	Stop(context.Context) error
	Detach()
	Output() OutputSnapshot
	Changed() <-chan struct{}
}
