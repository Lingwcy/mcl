package main

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
	"mcl/internal/app"
	launcherui "mcl/internal/ui"
)

func connectMemory(ctx context.Context, workers *sync.WaitGroup, window *mygo.Window, shell *launcherui.Shell, service *app.MemoryService) {
	shell.ConnectMemory(func(done func(app.MemorySnapshot, error)) func() {
		task, cancel := context.WithCancel(ctx)
		workers.Go(func() {
			defer cancel()
			ticker := time.NewTicker(3 * time.Second)
			defer ticker.Stop()
			var pending atomic.Bool
			for {
				// At most one immutable sample is queued even if a native dialog or a
				// temporarily stalled UI prevents Window.Update from running promptly.
				if pending.CompareAndSwap(false, true) {
					snapshot, err := service.Snapshot(task)
					if task.Err() != nil {
						return
					}
					window.Update(func() {
						pending.Store(false)
						if task.Err() == nil {
							done(snapshot, err)
						}
					})
				}
				select {
				case <-task.Done():
					return
				case <-ticker.C:
				}
			}
		})
		return cancel
	})
}
