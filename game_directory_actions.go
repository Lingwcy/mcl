package main

import (
	"context"
	"errors"
	"sync"

	"github.com/egoist/mygo"
	"mcl/internal/app"
	"mcl/internal/storage"
	launcherui "mcl/internal/ui"
)

func connectGameDirectories(ctx context.Context, workers *sync.WaitGroup, window *mygo.Window, shell *launcherui.Shell, service *app.GameDirectoryService, assets *storage.AssetStore) {
	shell.ConnectGameDirectories(launcherui.GameDirectoryActions{
		List: func(done func([]app.GameDirectory, error)) func() {
			return marketWork(ctx, workers, window, service.List, done)
		},
		Add: func(done func(app.GameDirectory, error)) func() {
			return marketWork(ctx, workers, window, func(task context.Context) (app.GameDirectory, error) {
				paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: window, Title: "选择游戏安装位置", ButtonLabel: "选择", Directory: true, CreateDirectories: true})
				if err != nil {
					return app.GameDirectory{}, err
				}
				if len(paths) == 0 || task.Err() != nil {
					return app.GameDirectory{}, context.Canceled
				}
				result, err := mygo.Dialog.Message(mygo.MessageOptions{Parent: window, Type: mygo.MessageInfo, Title: "添加游戏目录", Message: "在这个位置保存新实例？", Detail: paths[0] + "\n新实例将保存在其中的 mcl-games-v1 子目录，每个实例独立。不会移动或接管已有游戏，应用数据及共享运行文件仍保留原位置。", Buttons: []string{"取消", "添加"}, DefaultButton: 1, CancelButton: 0})
				if err != nil {
					return app.GameDirectory{}, err
				}
				if result.Button != 1 || task.Err() != nil {
					return app.GameDirectory{}, context.Canceled
				}
				return service.Add(task, paths[0])
			}, done)
		},
		Rename: func(id, name string, done func(error)) func() {
			return marketWork(ctx, workers, window, func(task context.Context) (struct{}, error) { return struct{}{}, service.Rename(task, id, name) }, func(_ struct{}, err error) { done(err) })
		},
		Remove: func(id string, done func(error)) func() {
			return marketWork(ctx, workers, window, func(task context.Context) (struct{}, error) {
				result, err := mygo.Dialog.Message(mygo.MessageOptions{Parent: window, Type: mygo.MessageWarning, Title: "移除游戏目录", Message: "仅移除这个位置的登记？", Detail: "不会删除磁盘上的任何文件。仍有关联实例或作为默认位置的目录不能移除。", Buttons: []string{"取消", "移除登记"}, DefaultButton: 0, CancelButton: 0})
				if err != nil {
					return struct{}{}, err
				}
				if result.Button != 1 || task.Err() != nil {
					return struct{}{}, context.Canceled
				}
				return struct{}{}, service.Remove(task, id)
			}, func(_ struct{}, err error) { done(err) })
		},
		Open: func(id string, done func(error)) func() {
			return marketWork(ctx, workers, window, func(task context.Context) (struct{}, error) {
				directory, err := service.Directory(task, id)
				if err != nil {
					return struct{}{}, err
				}
				if err := task.Err(); err != nil {
					return struct{}{}, err
				}
				return struct{}{}, mygo.Shell.OpenPath(directory)
			}, func(_ struct{}, err error) { done(err) })
		},
		OpenInstance: func(id string, done func(error)) func() {
			return marketWork(ctx, workers, window, func(task context.Context) (struct{}, error) {
				directory, err := assets.Directory(task, id)
				if err != nil {
					return struct{}{}, err
				}
				if err := task.Err(); err != nil {
					return struct{}{}, err
				}
				if directory == "" {
					return struct{}{}, errors.New("游戏目录不可用")
				}
				return struct{}{}, mygo.Shell.OpenPath(directory)
			}, func(_ struct{}, err error) { done(err) })
		},
	})
}
