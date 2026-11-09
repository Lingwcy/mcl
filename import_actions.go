package main

import (
	"context"
	"sync"

	"github.com/egoist/mygo"
	"mcl/internal/app"
	launcherui "mcl/internal/ui"
)

type importIntent struct {
	path    string
	confirm bool
	problem string
}

// Register before App.Run; macOS may deliver launch intents before WhenReady.
// Keep only one pending request, not an unbounded queue of paths from IPC.
func importFileIntents() func(context.Context, *sync.WaitGroup, *mygo.Window, *launcherui.Shell) {
	var pending *importIntent
	var receive func(importIntent)
	dispatch := func(intent importIntent) {
		if receive != nil {
			receive(intent)
		} else {
			copy := intent
			pending = &copy
		}
	}
	mygo.App.OnOpenFile(func(path string) { dispatch(importIntent{path: path}) })
	mygo.App.OnOpenURL(func(address string) {
		path, err := app.ImportLinkPath(address)
		if err != nil {
			dispatch(importIntent{problem: err.Error()})
			return
		}
		dispatch(importIntent{path: path, confirm: true})
	})
	return func(ctx context.Context, workers *sync.WaitGroup, window *mygo.Window, shell *launcherui.Shell) {
		confirming := false
		receive = func(intent importIntent) {
			if ctx.Err() != nil {
				return
			}
			if intent.problem != "" {
				shell.Notify(launcherui.ToastError, intent.problem)
				return
			}
			if !intent.confirm {
				shell.ImportArchiveFile(intent.path)
				return
			}
			if confirming {
				shell.Notify(launcherui.ToastWarning, "请先处理当前导入请求")
				return
			}
			confirming = true
			marketWork(ctx, workers, window, func(task context.Context) (bool, error) {
				result, err := mygo.Dialog.Message(mygo.MessageOptions{Parent: window, Type: mygo.MessageQuestion, Title: "导入整合包", Message: "允许读取这个本地整合包吗？", Detail: intent.path + "\n只读取并显示导入确认，不会自动安装、启动或执行包内命令。", Buttons: []string{"取消", "读取"}, DefaultButton: 0, CancelButton: 0})
				return err == nil && result.Button == 1 && task.Err() == nil, err
			}, func(allowed bool, err error) {
				confirming = false
				if err != nil {
					shell.Notify(launcherui.ToastError, "无法确认整合包导入")
					return
				}
				if allowed {
					shell.ImportArchiveFile(intent.path)
				}
			})
		}
		window.OnFileDrop(func(event *mygo.FileDropEvent) {
			if len(event.Paths) != 1 {
				shell.Notify(launcherui.ToastWarning, "请一次拖入一个整合包")
				return
			}
			shell.ImportArchiveFile(event.Paths[0])
		})
		if pending != nil {
			intent := *pending
			pending = nil
			receive(intent)
		}
	}
}
