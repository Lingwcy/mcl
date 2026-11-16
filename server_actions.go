package main

import (
	"context"
	"sync"

	"github.com/egoist/mygo"
	"mcl/internal/app"
	"mcl/internal/core/server"
	launcherui "mcl/internal/ui"
)

type addServerResult struct {
	added app.ServerRecord
	list  []app.ServerRecord
}

func connectServers(ctx context.Context, workers *sync.WaitGroup, window *mygo.Window, shell *launcherui.Shell, service *app.ServerService) {
	if shell == nil || service == nil {
		return
	}
	pings := make(chan struct{}, 6)
	shell.ConnectServers(launcherui.ServerActions{
		Sync: func(forceRescan bool, done func([]app.ServerRecord, error)) func() {
			return marketWork(ctx, workers, window, func(taskCtx context.Context) ([]app.ServerRecord, error) {
				return service.LoadAndSync(taskCtx, forceRescan)
			}, done)
		},
		Add: func(request app.AddServerRequest, done func(app.ServerRecord, []app.ServerRecord, error)) func() {
			return marketWork(ctx, workers, window, func(taskCtx context.Context) (addServerResult, error) {
				added, list, err := service.Add(taskCtx, request)
				return addServerResult{added: added, list: list}, err
			}, func(result addServerResult, err error) {
				if done != nil {
					done(result.added, result.list, err)
				}
			})
		},
		SetInstance: func(key, instanceID string, done func([]app.ServerRecord, error)) func() {
			return marketWork(ctx, workers, window, func(taskCtx context.Context) ([]app.ServerRecord, error) {
				return service.SetInstance(taskCtx, key, instanceID)
			}, done)
		},
		Remove: func(key string, done func([]app.ServerRecord, error)) func() {
			return marketWork(ctx, workers, window, func(taskCtx context.Context) ([]app.ServerRecord, error) {
				return service.Remove(taskCtx, key)
			}, done)
		},
		Ping: func(key, address string, done func(server.Status, error)) func() {
			return marketWork(ctx, workers, window, func(taskCtx context.Context) (server.Status, error) {
				select {
				case pings <- struct{}{}:
					defer func() { <-pings }()
				case <-taskCtx.Done():
					return server.Status{}, taskCtx.Err()
				}
				return service.Ping(taskCtx, key, address)
			}, done)
		},
	})
}
