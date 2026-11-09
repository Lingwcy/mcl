package main

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/egoist/mygo"

	"mcl/internal/app"
	"mcl/internal/core/download"
	"mcl/internal/core/tasks"
	launcherui "mcl/internal/ui"
)

func connectDownloadTasks(ctx context.Context, workers *sync.WaitGroup, window *mygo.Window, shell *launcherui.Shell, service *app.TaskService, client *download.Client) {
	shell.ConnectDownloadTasks(launcherui.DownloadTaskActions{
		Refresh: func(done func(error)) {
			background(ctx, workers, window, func(task context.Context) ([]tasks.Group, error) { return service.List(task) }, func(groups []tasks.Group, err error) {
				if err == nil {
					shell.SetDownloadTasks(groups)
				}
				done(err)
			})
		},
		Control: func(id, command string, done func(error)) {
			background(ctx, workers, window, func(task context.Context) (struct{}, error) { return struct{}{}, service.Control(task, id, command) }, func(_ struct{}, err error) { done(err) })
		},
		Resume: func(id string, done func(error)) {
			workers.Go(func() {
				started := false
				err := service.ResumeWithStart(ctx, id, func(err error) {
					started = err == nil
					window.Update(func() {
						if ctx.Err() == nil {
							done(err)
						}
					})
				})
				if started && err != nil && ctx.Err() == nil && !errors.Is(err, context.Canceled) && !errors.Is(err, tasks.ErrCanceled) {
					window.Update(func() {
						if ctx.Err() == nil {
							shell.Notify(launcherui.ToastError, "任务未完成，请在任务列表核对状态或返回对应实例处理")
						}
					})
				}
			})
		},
		Remove: func(id string, done func(error)) {
			background(ctx, workers, window, func(task context.Context) (struct{}, error) { return struct{}{}, service.Remove(task, id) }, func(_ struct{}, err error) { done(err) })
		},
		Details: func(id string, done func(tasks.Group, error)) func() {
			return marketWork(ctx, workers, window, func(task context.Context) (tasks.Group, error) { return service.Details(task, id) }, done)
		},
		Clear: func(parts bool, done func(error)) {
			background(ctx, workers, window, func(task context.Context) (struct{}, error) {
				message, detail, button := "清理已结束的任务记录？", "只移除任务历史，已发布的文件与实例不会删除。未完成的任务保留。", "清理记录"
				if parts {
					message = "清理未完成的下载分片？"
					detail = "只清理应用拥有的临时下载分片，不删除已校验的资源、Java、实例或独立下载缓存。已暂停或中断的任务将重新下载尚未完成的文件。有下载正在写入时会拒绝清理。"
					button = "清理分片"
				}
				result, err := mygo.Dialog.Message(mygo.MessageOptions{Parent: window, Type: mygo.MessageWarning, Title: "下载任务", Message: message, Detail: detail, Buttons: []string{"取消", button}, DefaultButton: 0, CancelButton: 0})
				if err != nil {
					return struct{}{}, err
				}
				if result.Button != 1 || task.Err() != nil {
					return struct{}{}, context.Canceled
				}
				if parts {
					_, err = client.ClearParts(task)
				} else {
					err = service.ClearHistory(task)
				}
				return struct{}{}, err
			}, func(_ struct{}, err error) { done(err) })
		},
	})
	// One pending immutable UI delivery; event-driven snapshots do not reread
	// every task document on each progress update. Cross-process refresh is explicit.
	workers.Go(func() {
		var mu sync.Mutex
		var latest []tasks.Group
		pending := false
		publish := func(groups []tasks.Group) {
			mu.Lock()
			latest = groups
			if pending {
				mu.Unlock()
				return
			}
			pending = true
			mu.Unlock()
			window.Update(func() {
				mu.Lock()
				snapshot := latest
				latest = nil
				pending = false
				mu.Unlock()
				if ctx.Err() == nil {
					shell.SetDownloadTasks(snapshot)
				}
			})
		}
		groups, err := service.List(ctx)
		if err != nil {
			window.Update(func() {
				if ctx.Err() == nil {
					shell.Notify(launcherui.ToastError, "下载任务记录无法读取，原文件已保留")
				}
			})
		} else {
			publish(groups)
		}
		for {
			select {
			case <-ctx.Done():
				return
			case <-service.Changed():
			}
			timer := time.NewTimer(150 * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
			publish(service.Snapshot())
		}
	})
}
