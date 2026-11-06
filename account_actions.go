package main

import (
	"context"
	"sync"

	"mcl/internal/app"
	"mcl/internal/core/auth"
	"mcl/internal/fileio"
	launcherui "mcl/internal/ui"

	"github.com/egoist/mygo"
)

func connectAccounts(ctx context.Context, workers *sync.WaitGroup, window *mygo.Window, shell *launcherui.Shell, service *app.AccountService, microsoftAvailable bool) {
	shell.ConnectAccounts(launcherui.AccountActions{
		MicrosoftAvailable: microsoftAvailable,
		Load:               func(done func(app.Accounts, error)) { background(ctx, workers, window, service.Load, done) },
		Microsoft: func(id string, done func(app.Accounts, error)) func() {
			return marketWork(ctx, workers, window, func(ctx context.Context) (app.Accounts, error) {
				return service.SignIn(ctx, id, mygo.Shell.OpenExternal)
			}, done)
		},
		Offline: func(name string, done func(app.Accounts, error)) {
			background(ctx, workers, window, func(ctx context.Context) (app.Accounts, error) { return service.AddOffline(ctx, name) }, done)
		},
		Select: func(id string, done func(app.Accounts, error)) {
			background(ctx, workers, window, func(ctx context.Context) (app.Accounts, error) { return service.Select(ctx, id) }, done)
		},
		Remove: func(id string, done func(app.Accounts, error)) {
			background(ctx, workers, window, func(ctx context.Context) (app.Accounts, error) { return service.Remove(ctx, id) }, done)
		},
		RefreshHead: func(id string, replace bool, done func([]byte, error)) {
			background(ctx, workers, window, func(ctx context.Context) ([]byte, error) {
				return service.RefreshOfflineHead(ctx, id, replace)
			}, done)
		},
		PickSkin: func(id string, done func(app.Accounts, error)) {
			background(ctx, workers, window, func(ctx context.Context) (app.Accounts, error) {
				files, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: window, Title: "选择 Minecraft PNG 皮肤", ButtonLabel: "使用头像"})
				if err != nil {
					return app.Accounts{}, err
				}
				if len(files) == 0 {
					return app.Accounts{}, context.Canceled
				}
				data, err := fileio.Read(ctx, files[0], auth.MaxSkinBytes)
				if err != nil {
					return app.Accounts{}, err
				}
				head, err := auth.SkinHead(data)
				if err != nil {
					return app.Accounts{}, err
				}
				return service.SetOfflineHead(ctx, id, head)
			}, done)
		},
	})
}
