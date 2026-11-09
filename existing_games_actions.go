package main

import (
	"context"
	"sync"

	"github.com/egoist/mygo"
	"mcl/internal/app"
	"mcl/internal/core/discovery"
	launcherui "mcl/internal/ui"
)

func connectExistingGames(ctx context.Context, workers *sync.WaitGroup, window *mygo.Window, shell *launcherui.Shell, service *app.ExistingGamesService) {
	shell.ConnectExistingGames(launcherui.ExistingGamesActions{
		List: func(done func([]app.ExistingSource, error)) func() {
			return marketWork(ctx, workers, window, service.List, done)
		},
		Add: func(done func(app.ExistingSource, error)) func() {
			return marketWork(ctx, workers, window, func(task context.Context) (app.ExistingSource, error) {
				paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: window, Title: "选择已有游戏目录", ButtonLabel: "选择", Directory: true})
				if err != nil {
					return app.ExistingSource{}, err
				}
				if len(paths) == 0 || task.Err() != nil {
					return app.ExistingSource{}, context.Canceled
				}
				return service.Add(task, paths[0])
			}, done)
		},
		Remove: func(id string, done func(error)) func() {
			return marketWork(ctx, workers, window, func(task context.Context) (struct{}, error) { return struct{}{}, service.Remove(task, id) }, func(_ struct{}, err error) { done(err) })
		},
		Scan: func(id string, done func(discovery.Report, error)) func() {
			return marketWork(ctx, workers, window, func(task context.Context) (discovery.Report, error) { return service.Scan(task, id) }, done)
		},
	})
}
