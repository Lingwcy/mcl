package main

import (
	"context"
	"sync"

	"github.com/egoist/mygo"
	"mcl/internal/app"
	launcherui "mcl/internal/ui"
)

func connectWallpapers(ctx context.Context, workers *sync.WaitGroup, window *mygo.Window, shell *launcherui.Shell, service *app.WallpaperService) {
	if shell == nil || service == nil {
		return
	}
	shell.ConnectWallpapers(launcherui.WallpaperActions{
		Load: func(done func(app.WallpaperSnapshot, error)) func() {
			return marketWork(ctx, workers, window, func(taskCtx context.Context) (app.WallpaperSnapshot, error) {
				return service.Load(taskCtx)
			}, done)
		},
		Add: func(done func(app.WallpaperSnapshot, error)) func() {
			return marketWork(ctx, workers, window, func(taskCtx context.Context) (app.WallpaperSnapshot, error) {
				paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: window, Title: "选择墙纸图片", ButtonLabel: "添加墙纸", Multiple: true})
				if err != nil {
					return app.WallpaperSnapshot{}, err
				}
				if len(paths) == 0 {
					return app.WallpaperSnapshot{}, context.Canceled
				}
				return service.Add(taskCtx, paths)
			}, done)
		},
		Select: func(id string, done func(app.WallpaperSnapshot, error)) func() {
			return marketWork(ctx, workers, window, func(taskCtx context.Context) (app.WallpaperSnapshot, error) {
				return service.Select(taskCtx, id)
			}, done)
		},
		Shuffle: func(done func(app.WallpaperSnapshot, error)) func() {
			return marketWork(ctx, workers, window, func(taskCtx context.Context) (app.WallpaperSnapshot, error) {
				return service.Shuffle(taskCtx)
			}, done)
		},
		SaveOptions: func(options app.WallpaperOptions, done func(app.WallpaperSnapshot, error)) func() {
			return marketWork(ctx, workers, window, func(taskCtx context.Context) (app.WallpaperSnapshot, error) {
				return service.SaveOptions(taskCtx, options)
			}, done)
		},
		Remove: func(id string, done func(app.WallpaperSnapshot, error)) func() {
			return marketWork(ctx, workers, window, func(taskCtx context.Context) (app.WallpaperSnapshot, error) {
				return service.Remove(taskCtx, id)
			}, done)
		},
	})
}
