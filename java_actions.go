package main

import (
	"context"
	"sync"

	"mcl/internal/app"
	"mcl/internal/core/java"
	"mcl/internal/core/meta"
	"mcl/internal/platform"
	launcherui "mcl/internal/ui"

	"github.com/egoist/mygo"
)

func connectJava(ctx context.Context, workers *sync.WaitGroup, window *mygo.Window, shell *launcherui.Shell, service *app.JavaService) {
	shell.ConnectJava(launcherui.JavaActions{
		Inventory: func(selected string, done func(app.JavaInventory, error)) func() {
			return marketWork(ctx, workers, window, func(ctx context.Context) (app.JavaInventory, error) {
				return service.Inventory(ctx, selected)
			}, done)
		},
		Pick: func(home bool, done func(app.JavaEnvironment, error)) func() {
			return marketWork(ctx, workers, window, func(ctx context.Context) (app.JavaEnvironment, error) {
				title := "选择 Java 可执行文件"
				if home {
					title = "选择 Java Home 或 JDK 目录"
				}
				files, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
					Parent: window, Title: title, ButtonLabel: "添加", Directory: home, TreatPackagesAsDirectories: true,
				})
				if err != nil {
					return app.JavaEnvironment{}, err
				}
				if len(files) == 0 {
					return app.JavaEnvironment{}, context.Canceled
				}
				path := files[0]
				if home {
					path = platform.JavaExecutable(path)
				}
				return service.Add(ctx, path)
			}, done)
		},
		Remove: func(path string, done func(error)) {
			background(ctx, workers, window, func(ctx context.Context) (struct{}, error) {
				return struct{}{}, service.Remove(ctx, path)
			}, func(_ struct{}, err error) { done(err) })
		},
		Catalog: func(major int, done func(java.Catalog, error)) func() {
			return marketWork(ctx, workers, window, func(ctx context.Context) (java.Catalog, error) {
				return service.Catalog(ctx, major, meta.CurrentPlatform())
			}, done)
		},
		Install: func(p java.Package, progress func(java.Progress), done func(app.JavaEnvironment, error)) func() {
			task, cancel := context.WithCancel(ctx)
			workers.Go(func() {
				defer cancel()
				value, err := service.Install(task, p, func(p java.Progress) {
					window.Update(func() {
						if ctx.Err() == nil {
							progress(p)
						}
					})
				})
				window.Update(func() {
					if ctx.Err() == nil {
						done(value, err)
					}
				})
			})
			return cancel
		},
		Open: func(path string, done func(error)) {
			background(ctx, workers, window, func(context.Context) (struct{}, error) {
				return struct{}{}, mygo.Shell.OpenPath(path)
			}, func(_ struct{}, err error) { done(err) })
		},
	})
}
