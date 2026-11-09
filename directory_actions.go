package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/egoist/mygo"
	"mcl/internal/app"
	"mcl/internal/core/instance"
	"mcl/internal/core/transfer"
	launcherui "mcl/internal/ui"
)

func connectDirectories(ctx context.Context, workers *sync.WaitGroup, window *mygo.Window, shell *launcherui.Shell, service *app.DirectoryService) {
	confirm := func(task context.Context, title, message, detail, button string, destructive bool) (bool, error) {
		options := mygo.MessageOptions{Parent: window, Type: mygo.MessageWarning, Title: title, Message: message, Detail: detail, Buttons: []string{"取消", button}, DefaultButton: 0, CancelButton: 0}
		if destructive {
			options.DestructiveButtons = []int{1}
		}
		result, err := mygo.Dialog.Message(options)
		if err != nil {
			return false, err
		}
		return result.Button == 1 && task.Err() == nil, task.Err()
	}
	shell.ConnectDirectories(launcherui.DirectoryActions{
		Open: func(operation app.DirectoryOperation, done func(error)) func() {
			return marketWork(ctx, workers, window, func(task context.Context) (struct{}, error) {
				name, err := service.BackupDirectory(task, operation.InstanceID, operation.Token)
				if err != nil {
					return struct{}{}, err
				}
				if err := task.Err(); err != nil {
					return struct{}{}, err
				}
				return struct{}{}, mygo.Shell.OpenPath(name)
			}, func(_ struct{}, err error) { done(err) })
		},
		List: func(done func([]app.DirectoryOperation, error)) func() {
			return marketWork(ctx, workers, window, service.List, done)
		},
		InspectImport: func(request app.DirectoryImportRequest, progress func(transfer.Progress), done func(app.DirectoryPlan, error)) func() {
			return directoryWork(ctx, workers, window, func(task context.Context, report func(transfer.Progress)) (app.DirectoryPlan, error) {
				return service.InspectImport(task, request, report)
			}, progress, done)
		},
		InspectMove: func(id, to string, progress func(transfer.Progress), done func(app.DirectoryPlan, error)) func() {
			return directoryWork(ctx, workers, window, func(task context.Context, report func(transfer.Progress)) (app.DirectoryPlan, error) {
				return service.InspectMove(task, id, to, report)
			}, progress, done)
		},
		Confirm: func(plan app.DirectoryPlan, done func(bool, error)) func() {
			return marketWork(ctx, workers, window, func(task context.Context) (bool, error) {
				source, target, err := service.Paths(task, plan)
				if err != nil {
					return false, err
				}
				detail := fmt.Sprintf("Minecraft %s\n%d 个文件，%.2f GiB；逐文件 SHA-256 校验，不覆盖目标。", plan.After.Version.ID, len(plan.Tree.Files), float64(plan.Tree.Bytes)/(1<<30))
				detail += "\n\n源资料：" + source + "\n目标游戏目录：" + target
				if required, ok := plan.After.RequiredLoader(); ok {
					detail += "\n" + required.Type + " " + required.Version
				}
				message := "复制资料到「" + plan.After.Name + "」？"
				button := "复制为受管实例"
				if plan.Kind == "import" {
					detail += "\n\n原目录保持不变。仅复制明确勾选的游戏资料；不继承其他启动器账户、JVM 参数、脚本、客户端补丁或运行库。模组配置可能含私人信息，请按需取消该分类。\n\n文件复制完成不代表游戏可运行；之后使用 mcl 的官方安装/修复链补齐运行环境。"
				} else {
					message = "迁移「" + plan.After.Name + "」的游戏载荷？"
					button = "开始迁移"
					detail += "\n\n复制和核验完成后才提交新位置；原游戏、存档及专属备份作为可单独清理的旧副本保留。中央实例记录、账户、共享 Java/运行库与其他实例不移动。\n\n需要能同时容纳源数据与新副本的可用空间。磁盘离线、目标冲突或中断时保留恢复记录。"
				}
				return confirm(task, "确认目录操作", message, detail, button, false)
			}, done)
		},
		Run: func(plan app.DirectoryPlan, progress func(transfer.Progress), done func(instance.Instance, error)) func() {
			return directoryWork(ctx, workers, window, func(task context.Context, report func(transfer.Progress)) (instance.Instance, error) {
				return service.Run(task, plan, report)
			}, progress, done)
		},
		Resume: func(operation app.DirectoryOperation, progress func(transfer.Progress), done func(instance.Instance, error)) func() {
			return directoryWork(ctx, workers, window, func(task context.Context, report func(transfer.Progress)) (instance.Instance, error) {
				return service.Resume(task, operation.InstanceID, operation.Token, report)
			}, progress, done)
		},
		Abort: func(operation app.DirectoryOperation, done func(error)) func() {
			return marketWork(ctx, workers, window, func(task context.Context) (struct{}, error) {
				detail := "原资料保留。仅将本次操作新建的目标及暂存数据移入系统废纸篓；不会永久删除。若新位置已提交，不能撤销，必须先完成恢复后再另起回迁。"
				okay, err := confirm(task, "撤销目录操作", "撤销「"+operation.Name+"」的目录操作？", detail, "撤销并回收新副本", true)
				if err != nil {
					return struct{}{}, err
				}
				if !okay {
					return struct{}{}, context.Canceled
				}
				return struct{}{}, service.Abort(task, operation.InstanceID, operation.Token)
			}, func(_ struct{}, err error) { done(err) })
		},
		Clean: func(operation app.DirectoryOperation, done func(error)) func() {
			return marketWork(ctx, workers, window, func(task context.Context) (struct{}, error) {
				backupPath, err := service.BackupDirectory(task, operation.InstanceID, operation.Token)
				if err != nil && !errors.Is(err, os.ErrNotExist) {
					return struct{}{}, err
				}
				detail := "只清理此次迁移保留的旧位置副本，先复核所属标记及文件清单，再移入系统废纸篓。不删除当前游戏、共享缓存或其他实例。核验不符会保留备份。"
				detail += "\n\n" + backupPath
				if operation.Changed {
					detail = "注意：旧副本包含复制提交后的变化，未承诺这些变化已进入新位置；请先自行核对。\n\n" + detail
				}
				okay, err := confirm(task, "清理迁移旧副本", "清理「"+operation.Name+"」的旧副本？", detail, "移入废纸篓", true)
				if err != nil {
					return struct{}{}, err
				}
				if !okay {
					return struct{}{}, context.Canceled
				}
				return struct{}{}, service.CleanBackup(task, operation.InstanceID, operation.Token)
			}, func(_ struct{}, err error) { done(err) })
		},
	})
}

// At most one queued immutable progress snapshot per job. Terminal delivery is
// separate, never throttled or discarded; no timer or UI Context is captured.
func directoryWork[T any](parent context.Context, workers *sync.WaitGroup, window *mygo.Window, work func(context.Context, func(transfer.Progress)) (T, error), progress func(transfer.Progress), done func(T, error)) func() {
	ctx, cancel := context.WithCancel(parent)
	workers.Go(func() {
		defer cancel()
		var mu sync.Mutex
		var latest transfer.Progress
		pending := false
		value, err := work(ctx, func(p transfer.Progress) {
			if parent.Err() != nil {
				return
			}
			mu.Lock()
			latest = p
			if pending {
				mu.Unlock()
				return
			}
			pending = true
			mu.Unlock()
			window.Update(func() {
				mu.Lock()
				snapshot := latest
				pending = false
				mu.Unlock()
				if parent.Err() == nil {
					progress(snapshot)
				}
			})
		})
		if parent.Err() == nil {
			window.Update(func() {
				if parent.Err() == nil {
					done(value, err)
				}
			})
		}
	})
	return cancel
}
