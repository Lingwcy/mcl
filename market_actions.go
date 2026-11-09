package main

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"sync"

	"mcl/internal/app"
	"mcl/internal/core/download"
	"mcl/internal/core/install"
	"mcl/internal/core/market"
	"mcl/internal/platform"
	launcherui "mcl/internal/ui"

	"github.com/egoist/mygo"
)

func connectMarket(ctx context.Context, workers *sync.WaitGroup, window *mygo.Window, shell *launcherui.Shell, service *app.MarketService, installer *app.MarketInstaller, archiveImporter *app.ArchiveImporter, packImports *app.PackImportService) {
	icons := make(chan struct{}, 4)
	shell.ConnectMarket(launcherui.MarketActions{
		CurseForgeAvailable: service.Client().CurseForgeAvailable(),
		Search: func(query market.Query, done func(market.Results, error)) func() {
			return marketWork(ctx, workers, window, func(ctx context.Context) (market.Results, error) { return service.Search(ctx, query) }, done)
		},
		Categories: func(source market.Source, kind market.Kind, done func([]market.Category, error)) func() {
			return marketWork(ctx, workers, window, func(ctx context.Context) ([]market.Category, error) { return service.Categories(ctx, source, kind) }, done)
		},
		LoaderIcons: func(done func([]market.LoaderIcon, error)) func() {
			return marketWork(ctx, workers, window, func(ctx context.Context) ([]market.LoaderIcon, error) {
				return service.Client().LoaderIcons(ctx)
			}, done)
		},
		Detail: func(p market.Project, game, loader string, done func(app.MarketDetail, error)) func() {
			return marketWork(ctx, workers, window, func(ctx context.Context) (app.MarketDetail, error) { return service.Detail(ctx, p, game, loader) }, done)
		},
		VersionDetail: func(source market.Source, project, version string, done func(market.Version, error)) func() {
			return marketWork(ctx, workers, window, func(ctx context.Context) (market.Version, error) {
				return service.VersionDetail(ctx, source, project, version)
			}, done)
		},
		Icon: func(address string, done func(image.Image, error)) func() {
			return marketWork(ctx, workers, window, func(ctx context.Context) (image.Image, error) {
				select {
				case icons <- struct{}{}:
					defer func() { <-icons }()
				case <-ctx.Done():
					return nil, ctx.Err()
				}
				return platform.MarketImage(ctx, address)
			}, done)
		},
		InstanceIcon: func(address string, done func([]byte, error)) func() {
			return marketWork(ctx, workers, window, func(task context.Context) ([]byte, error) {
				select {
				case icons <- struct{}{}:
					defer func() { <-icons }()
				case <-task.Done():
					return nil, task.Err()
				}
				img, err := platform.MarketImage(task, address)
				if err != nil {
					return nil, err
				}
				var output bytes.Buffer
				if err := png.Encode(&output, img); err != nil {
					return nil, err
				}
				return output.Bytes(), task.Err()
			}, done)
		},
		SetKey: service.SetCurseForgeKey,
		InspectArchive: func(request app.ArchiveImportRequest, settings app.Settings, progress func(download.Progress), done func(app.ArchiveImportPlan, error)) func() {
			return runTask(ctx, workers, window, func(task context.Context, report func(install.Progress)) (app.ArchiveImportPlan, error) {
				return archiveImporter.Inspect(task, request, settings, func(p download.Progress) { report(install.Progress{Stage: "modpack", Progress: p}) })
			}, func(p install.Progress) { progress(p.Progress) }, done)
		},
		SupplyArchive: func(plan app.ArchiveImportPlan, done func(app.LocalFileSupply, error)) func() {
			return marketWork(ctx, workers, window, func(ctx context.Context) (app.LocalFileSupply, error) {
				paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: window, Title: "选择本地压缩包或整合包文件", Multiple: !plan.NeedsArchive, ButtonLabel: "核验文件"})
				if err != nil {
					return app.LocalFileSupply{}, err
				}
				return archiveImporter.Supply(ctx, plan, paths)
			}, done)
		},
		ImportArchive: func(plan app.ArchiveImportPlan, progress func(install.Progress), done func(app.PackImportResult, error)) func() {
			return runTask(ctx, workers, window, func(task context.Context, report func(install.Progress)) (app.PackImportResult, error) {
				return packImports.Run(task, plan, report)
			}, progress, done)
		},
		ResumePackImport: func(id string, progress func(install.Progress), done func(app.PackImportResult, error)) func() {
			return runTask(ctx, workers, window, func(task context.Context, report func(install.Progress)) (app.PackImportResult, error) {
				return packImports.Resume(task, id, report)
			}, progress, done)
		},
		InspectPackRepair: func(id string, done func(app.PackRepairPlan, error)) func() {
			return marketWork(ctx, workers, window, func(ctx context.Context) (app.PackRepairPlan, error) {
				return archiveImporter.InspectRepair(ctx, id)
			}, done)
		},
		SupplyPackRepair: func(plan app.PackRepairPlan, done func(app.LocalFileSupply, error)) func() {
			return marketWork(ctx, workers, window, func(ctx context.Context) (app.LocalFileSupply, error) {
				paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: window, Title: "选择整合包文件", Multiple: true, ButtonLabel: "核验文件"})
				if err != nil {
					return app.LocalFileSupply{}, err
				}
				return archiveImporter.SupplyRepair(ctx, plan, paths)
			}, done)
		},
		RestorePack: func(plan app.PackRepairPlan, paths []string, progress func(download.Progress), done func(int, error)) func() {
			return runTask(ctx, workers, window, func(ctx context.Context, report func(install.Progress)) (int, error) {
				return archiveImporter.RestorePack(ctx, plan, paths, func(p download.Progress) { report(install.Progress{Stage: "pack-repair", Progress: p}) })
			}, func(p install.Progress) { progress(p.Progress) }, done)
		},
		Plan: func(request market.InstallRequest, done func(market.InstallPlan, error)) func() {
			return marketWork(ctx, workers, window, func(ctx context.Context) (market.InstallPlan, error) { return installer.Plan(ctx, request) }, done)
		},
		SupplyFiles: func(plan market.InstallPlan, done func(app.LocalFileSupply, error)) func() {
			return marketWork(ctx, workers, window, func(ctx context.Context) (app.LocalFileSupply, error) {
				paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: window, Title: "选择市场资产文件", Multiple: true, ButtonLabel: "核验文件"})
				if err != nil {
					return app.LocalFileSupply{}, err
				}
				return installer.Supply(ctx, plan, paths)
			}, done)
		},
		Worlds: func(id string, done func([]string, error)) func() {
			return marketWork(ctx, workers, window, func(ctx context.Context) ([]string, error) { return installer.Worlds(ctx, id) }, done)
		},
		Install: func(plan market.InstallPlan, progress func(download.Progress), done func(error)) func() {
			return runTask(ctx, workers, window, func(ctx context.Context, report func(install.Progress)) (struct{}, error) {
				err := installer.Install(ctx, plan, func(p download.Progress) { report(install.Progress{Stage: "market", Progress: p}) })
				return struct{}{}, err
			}, func(p install.Progress) { progress(p.Progress) }, func(_ struct{}, err error) { done(err) })
		},
	})
}

// Cancellation still delivers a terminal result while the application is alive.
// Generation checks in the controller discard results from superseded work.
func marketWork[T any](parent context.Context, workers *sync.WaitGroup, window *mygo.Window, work func(context.Context) (T, error), done func(T, error)) func() {
	ctx, cancel := context.WithCancel(parent)
	workers.Go(func() {
		defer cancel()
		value, err := work(ctx)
		if parent.Err() != nil {
			return
		}
		window.Update(func() {
			if parent.Err() == nil {
				done(value, err)
			}
		})
	})
	return cancel
}
