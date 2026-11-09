package main

import (
	"context"
	"github.com/egoist/mygo"
	"mcl/internal/app"
	"mcl/internal/core/download"
	"mcl/internal/storage"
	launcherui "mcl/internal/ui"
	"sync"
)

func connectDownloads(ctx context.Context, workers *sync.WaitGroup, window *mygo.Window, shell *launcherui.Shell, service *app.DownloadService, cache *storage.DownloadCache) {
	shell.ConnectDownloads(launcherui.DownloadActions{
		Pick: func(done func(string, error)) func() {
			return marketWork(ctx, workers, window, func(task context.Context) (string, error) {
				paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: window, Title: "选择下载副本缓存位置", ButtonLabel: "选择", Directory: true, CreateDirectories: true})
				if err != nil {
					return "", err
				}
				if len(paths) == 0 || task.Err() != nil {
					return "", context.Canceled
				}
				return paths[0], nil
			}, done)
		},
		Inspect: func(done func(download.CacheInfo, error)) func() {
			return marketWork(ctx, workers, window, service.CacheInfo, done)
		},
		Clear: func(info download.CacheInfo, done func(download.CacheInfo, error)) func() {
			return marketWork(ctx, workers, window, func(task context.Context) (download.CacheInfo, error) {
				result, err := mygo.Dialog.Message(mygo.MessageOptions{Parent: window, Type: mygo.MessageWarning, Title: "清理下载副本", Message: "清理这个目录中的下载副本？", Detail: info.Directory + "\n不会删除实例、运行库、Java 环境、本地补齐文件或事务备份。正在使用缓存的下载会阻止清理。", Buttons: []string{"取消", "清理"}, DefaultButton: 0, CancelButton: 0, DestructiveButtons: []int{1}})
				if err != nil {
					return download.CacheInfo{}, err
				}
				if task.Err() != nil || result.Button != 1 {
					return download.CacheInfo{}, context.Canceled
				}
				return service.ClearCache(task, info.Directory)
			}, done)
		},
		Check: func(source download.SourceMode, done func([]download.ConnectivityResult, error)) func() {
			return marketWork(ctx, workers, window, func(task context.Context) ([]download.ConnectivityResult, error) { return service.Check(task, source) }, done)
		},
		Open: func(done func(error)) func() {
			return marketWork(ctx, workers, window, func(task context.Context) (struct{}, error) {
				directory, err := cache.Location(task)
				if err != nil {
					return struct{}{}, err
				}
				if task.Err() != nil {
					return struct{}{}, task.Err()
				}
				return struct{}{}, mygo.Shell.OpenPath(directory)
			}, func(_ struct{}, err error) { done(err) })
		},
	})
}
