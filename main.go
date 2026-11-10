package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"image"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"mcl/internal/app"
	"mcl/internal/branding"
	"mcl/internal/core/auth"
	"mcl/internal/core/download"
	"mcl/internal/core/install"
	"mcl/internal/core/instance"
	"mcl/internal/core/java"
	"mcl/internal/core/launch"
	"mcl/internal/core/loader"
	"mcl/internal/core/market"
	"mcl/internal/core/meta"
	"mcl/internal/platform"
	"mcl/internal/platform/process"
	"mcl/internal/storage"
	launcherui "mcl/internal/ui"
)

func main() {
	if handled, code := process.GateMain(os.Args[1:]); handled {
		os.Exit(code)
	}
	if err := run(); err != nil {
		slog.Error("mcl stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	dataDir := flag.String("data-dir", "", "Override the application data directory")
	flag.Parse()
	paths, err := platform.DataPaths(*dataDir)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	var workers sync.WaitGroup
	var detachGames func()
	defer func() {
		cancel()
		workers.Wait()
		if detachGames != nil {
			detachGames()
		}
	}()
	repository := storage.NewSettingsStore(paths.ConfigFile)
	settings, loadErr := repository.Load(ctx)
	if loadErr != nil {
		slog.Warn("settings could not be loaded; preserving the existing file", "error", loadErr)
		settings = app.DefaultSettings()
	}
	service := app.NewSettingsService(repository)
	catalog := meta.NewClient(paths.Metadata)
	instances := storage.NewInstanceStore(paths.Instances)
	loaderCatalog := loader.NewClient()
	host, hostErr := platform.HostPlatform()
	if hostErr != nil {
		// Modern versions without an OS-version predicate can still be managed.
		// Rules requiring the unavailable version continue to fail closed.
		slog.Warn("OS version could not be read; version-sensitive rules will fail", "error", hostErr)
	}
	library := app.NewLibraryServiceWithLoaders(catalog, instances, host, loaderCatalog)
	sessions := storage.NewSessionStore(paths.Root, process.Running)
	assetStore := storage.NewAssetStore(paths.Instances)
	assets := app.NewAssetsService(instances, assetStore, sessions)
	transfers, err := download.NewTransferControl(settings.Download)
	if err != nil {
		return err
	}
	taskService := app.NewTaskService(ctx, storage.NewDownloadTaskStore(paths.Root))
	sharedDownloads := download.NewClient(paths.Root)
	sharedDownloads.Track = taskService.TrackDownloads
	sharedDownloads.Coordination = storage.NewDownloadCoordination(paths.Root)
	sharedDownloads.Transfers = transfers
	downloadCache := storage.NewDownloadCache(paths.Root, settings.Download.CacheDirectory)
	sharedDownloads.Cache = downloadCache
	network, err := platform.NewDownloadNetwork(settings.Download.Proxy)
	if err != nil {
		return err
	}
	defer network.CloseIdleConnections()
	downloadService := app.NewDownloadService(downloadCache, network.Snapshot)
	sharedDownloads.HTTP.Transport = network
	catalog.HTTP.Transport = network
	catalog.VerifiedFetch = sharedDownloads.FetchMetadata
	loaderCatalog.HTTP.Transport = network
	key := os.Getenv("MCL_CURSEFORGE_API_KEY")
	if key == "" {
		key = os.Getenv("GRUN_CURSEFORGE_API_KEY")
	}
	marketClient := market.NewClient(key)
	marketClient.HTTP.Transport = network
	marketService := app.NewMarketService(marketClient)
	assetIdentity := app.NewAssetIdentityService(instances, sessions, assetStore, marketService)
	assetUpdates := app.NewAssetUpdateService(instances, sessions, storage.NewAssetUpdateStore(paths.Root), marketService, sharedDownloads)
	assetDiagnostics := app.NewAssetDiagnosticService(instances, sessions, assetStore, java.Probe)
	marketInstaller := app.NewMarketInstaller(marketService, instances, sessions, storage.NewMarketStore(paths.Root), sharedDownloads)
	archiveImporter := app.NewArchiveImporter(marketService, catalog, instances, sessions, storage.NewArchiveStore(paths.Root), sharedDownloads)
	javaCatalog := java.NewCatalogClient()
	javaCatalog.HTTP.Transport = network
	javaService := app.NewJavaService(storage.NewJavaStore(paths.Root), platform.JavaCandidates,
		&java.Installer{Downloads: sharedDownloads, Directory: filepath.Join(paths.Root, "java"), Transfers: transfers, Transport: network, Cache: downloadCache}, javaCatalog)
	forgeInstaller := loader.NewForgeInstallerWithJava(loaderCatalog, sharedDownloads, process.RunLoaderProcessor, javaService.EnsureProcessor)
	loaders := app.NewLoaderService(loaderCatalog, storage.NewLoaderStore(paths.Root), forgeInstaller)
	installations := app.NewInstallationServiceWithGameDirectory(catalog, instances, storage.NewInstallationStore(paths.Root), sharedDownloads, host, sessions, loaders, assetStore.Directory, assetStore.OpenGame)
	installations.CheckExecutionPlatform = platform.CheckExecutionPlatform
	installations.RuntimeEvidence = platform.RuntimeEvidence()
	instanceSettings := app.NewInstanceSettingsService(instances, sessions)
	instanceDeletion := app.NewInstanceDeletionService(storage.NewInstanceDeletionStore(paths.Root), sessions, mygo.Shell.TrashItem)
	directoryService := app.NewDirectoryService(storage.NewDirectoryTransferStore(paths.Root, platform.ReadSwiftProfiles), sessions, catalog, loaderCatalog, mygo.Shell.TrashItem)
	memoryProbe := platform.NewMemoryProbe()
	memoryService := app.NewMemoryService(memoryProbe, host.Arch)
	launcher := app.NewLaunchService(installations, javaService.Ensure, memoryProbe, func(plan launch.Plan, register func(launch.ProcessIdentity) error) (launch.Game, error) {
		game, err := assetStore.OpenGame(ctx, plan.InstanceID())
		if err != nil {
			return nil, err
		}
		defer game.Close()
		if filepath.Clean(game.Name()) != filepath.Clean(plan.Directory()) {
			return nil, errors.New("实例游戏目录与启动计划不一致")
		}
		return process.StartTracked(plan, register, game)
	}, process.GameWindowProbe(), assetStore.Directory)
	archiveImporter.Tasks = taskService
	installations.Tasks = taskService
	javaService.Tasks = taskService
	marketInstaller.Tasks = taskService
	assetUpdates.Tasks = taskService
	launcher.Tasks = taskService
	packImports := app.NewPackImportService(archiveImporter, installations, javaService.Ensure)
	packImports.Tasks = taskService
	taskReplayer := &app.TaskReplayer{Installations: installations, Java: javaService, Market: marketInstaller, Packs: packImports, Updates: assetUpdates, Downloads: sharedDownloads, Instances: instances, Settings: repository.Load}
	taskService.SetReplay(taskReplayer.Replay)
	detachGames = launcher.DetachAll
	clientID, configErr := platform.MicrosoftClientID(ctx, paths.Root)
	if configErr != nil {
		slog.Warn("Microsoft application configuration could not be loaded", "error", configErr)
	}
	microsoft, microsoftErr := auth.NewMicrosoftClient(clientID)
	accounts := app.NewAccountService(storage.NewAccountStore(paths.Root), platform.NewAccountCredentials(paths.Root), microsoft)

	mygo.App.SetName(branding.Name)
	var portable *portableUpdates
	if usesUpdatePlugin() {
		mygo.Use(updatePlugin)
	} else if portable, err = newPortableUpdates(ctx, paths.Root); err != nil {
		return fmt.Errorf("prepare self-updates: %w", err)
	}
	mygo.App.OnWillQuit(func(*mygo.QuitEvent) {
		if portable != nil {
			portable.finish()
		}
		cancel()
	})
	bindImportIntents := importFileIntents()
	mygo.App.WhenReady(func() {
		var window *mygo.Window
		save := func(snapshot app.Settings, done func(error)) {
			workers.Go(func() {
				var err error
				if downloadCache.DirectoryFor(snapshot.Download.CacheDirectory) != downloadCache.Directory() {
					err = downloadCache.Prepare(ctx, snapshot.Download.CacheDirectory)
				}
				if err == nil {
					err = service.Save(ctx, snapshot)
				}
				if err == nil {
					err = network.SetProxy(snapshot.Download.Proxy)
				}
				if err == nil {
					err = transfers.SetPreferences(snapshot.Download)
					if err == nil {
						downloadCache.SetDirectory(snapshot.Download.CacheDirectory)
					}
				}
				window.Update(func() {
					if ctx.Err() == nil {
						done(err)
					}
				})
			})
		}
		shell := launcherui.NewShell(settings, paths.Root, loadErr, save)
		shell.BindAppearance(mygo.Theme.IsDark(), func(dark bool) {
			source := mygo.ThemeLight
			if dark {
				source = mygo.ThemeDark
			}
			mygo.Theme.SetSource(source)
		})
		window = mygo.NewWindow(mygo.WindowOptions{
			Title: branding.Name + " — " + branding.FullName, Width: launcherui.DefaultWidth, Height: launcherui.DefaultHeight,
			MinWidth: launcherui.MinWidth, MinHeight: launcherui.MinHeight,
			TitleBarStyle: mygo.TitleBarHidden, TitleBarHeight: 44, HideWindowButtons: true,
			StateKey: "main-workspace-v2", Content: ui.View(shell.View),
		})
		shell.BindWindow(window)
		connectUpdates(window, shell, portable)
		connectDownloadTasks(ctx, &workers, window, shell, taskService, sharedDownloads)
		connectJava(ctx, &workers, window, shell, javaService)
		connectDownloads(ctx, &workers, window, shell, downloadService, downloadCache)
		connectGameDirectories(ctx, &workers, window, shell, app.NewGameDirectoryService(storage.NewGameDirectoryStore(paths.Root)), assetStore)
		connectExistingGames(ctx, &workers, window, shell, app.NewExistingGamesService(storage.NewExistingGamesStore(paths.Root, platform.ReadSwiftProfiles)))
		connectDirectories(ctx, &workers, window, shell, directoryService)
		connectMemory(ctx, &workers, window, shell, memoryService)
		connectAccounts(ctx, &workers, window, shell, accounts, microsoftErr == nil)
		// This subscription has the application's lifetime, like its single window.
		mygo.Theme.OnUpdated(func() {
			window.Update(func() {
				if ctx.Err() == nil {
					shell.SetAppearance(mygo.Theme.IsDark())
				}
			})
		})
		connectMarket(ctx, &workers, window, shell, marketService, marketInstaller, archiveImporter, packImports)
		shell.ConnectAssets(launcherui.AssetActions{
			Icon: func(id string, file app.AssetFile, done func(image.Image, error)) func() {
				return marketWork(ctx, &workers, window, func(ctx context.Context) (image.Image, error) {
					if file.Origin != nil && file.Origin.Metadata != nil && file.Origin.Metadata.IconURL != "" {
						if img, err := platform.MarketImage(ctx, file.Origin.Metadata.IconURL); err == nil {
							return img, nil
						}
						if ctx.Err() != nil {
							return nil, ctx.Err()
						}
					}
					return assetStore.Icon(ctx, id, file)
				}, done)
			},
			Associate: func(request app.AssetAssociation, done func(market.InstallFile, error)) func() {
				return marketWork(ctx, &workers, window, func(ctx context.Context) (market.InstallFile, error) {
					return assetIdentity.Associate(ctx, request)
				}, done)
			},
			Diagnose: func(id string, progress func(download.Progress), done func(app.AssetDiagnosticResult, error)) func() {
				return runTask(ctx, &workers, window, func(ctx context.Context, report func(install.Progress)) (app.AssetDiagnosticResult, error) {
					return assetDiagnostics.Diagnose(ctx, id, func(p download.Progress) { report(install.Progress{Stage: "diagnose-assets", Progress: p}) })
				}, func(p install.Progress) { progress(p.Progress) }, done)
			},
			UpdateVersions: func(id string, file app.AssetFile, source market.Source, done func([]market.Version, error)) func() {
				return runTask(ctx, &workers, window, func(ctx context.Context, _ func(install.Progress)) ([]market.Version, error) {
					return assetUpdates.Versions(ctx, id, file, source)
				}, nil, done)
			},
			PlanUpdate: func(request market.UpdateRequest, done func(market.UpdatePlan, error)) func() {
				return runTask(ctx, &workers, window, func(ctx context.Context, _ func(install.Progress)) (market.UpdatePlan, error) {
					return assetUpdates.Plan(ctx, request)
				}, nil, done)
			},
			SupplyUpdate: func(plan market.UpdatePlan, done func(app.LocalFileSupply, error)) func() {
				return marketWork(ctx, &workers, window, func(ctx context.Context) (app.LocalFileSupply, error) {
					paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: window, Title: "选择更新文件", Multiple: true, ButtonLabel: "核验文件"})
					if err != nil {
						return app.LocalFileSupply{}, err
					}
					return assetUpdates.Supply(ctx, plan, paths)
				}, done)
			},
			ApplyUpdate: func(plan market.UpdatePlan, progress func(app.AssetUpdateProgress), done func(app.AssetUpdateResult, error)) func() {
				return runTask(ctx, &workers, window, func(ctx context.Context, report func(install.Progress)) (app.AssetUpdateResult, error) {
					return assetUpdates.Apply(ctx, plan, func(p app.AssetUpdateProgress) { report(install.Progress{Stage: p.Stage, Progress: p.Progress}) })
				}, func(p install.Progress) { progress(app.AssetUpdateProgress{Stage: p.Stage, Progress: p.Progress}) }, done)
			},
			RecoverUpdate: func(id, transaction string, done func(app.AssetUpdateResult, error)) func() {
				return runTask(ctx, &workers, window, func(ctx context.Context, _ func(install.Progress)) (app.AssetUpdateResult, error) {
					return assetUpdates.Recover(ctx, id, transaction)
				}, nil, done)
			},
			Identify: func(id string, source market.Source, progress func(app.AssetIdentityProgress), done func(app.AssetIdentityResult, error)) func() {
				return runTask(ctx, &workers, window, func(ctx context.Context, report func(install.Progress)) (app.AssetIdentityResult, error) {
					return assetIdentity.Identify(ctx, id, source, func(p app.AssetIdentityProgress) { report(install.Progress{Stage: p.Stage, Progress: p.Progress}) })
				}, func(p install.Progress) { progress(app.AssetIdentityProgress{Stage: p.Stage, Progress: p.Progress}) }, done)
			},
			Change: func(request app.AssetMutation, done func(app.AssetChange, error)) func() {
				return runTask(ctx, &workers, window, func(ctx context.Context, _ func(install.Progress)) (app.AssetChange, error) {
					return assets.Change(ctx, request)
				}, nil, done)
			},
			Restore: func(id, backup string, done func(error)) func() {
				return runTask(ctx, &workers, window, func(ctx context.Context, _ func(install.Progress)) (struct{}, error) {
					return struct{}{}, assets.Restore(ctx, id, backup)
				}, nil, func(_ struct{}, err error) { done(err) })
			},
			List: func(id string, done func(app.AssetSnapshot, error)) func() {
				task, stop := context.WithCancel(ctx)
				background(task, &workers, window, func(ctx context.Context) (app.AssetSnapshot, error) {
					return assets.List(ctx, id)
				}, func(snapshot app.AssetSnapshot, err error) { stop(); done(snapshot, err) })
				return stop
			},
			OpenDirectory: func(id string, done func(error)) {
				background(ctx, &workers, window, func(ctx context.Context) (string, error) {
					return assets.Directory(ctx, id)
				}, func(directory string, err error) {
					if err == nil {
						err = mygo.Shell.OpenPath(directory)
					}
					done(err)
				})
			},
		})
		shell.ConnectLibrary(launcherui.LibraryActions{
			DeleteInstance: func(id string, done func(error)) func() {
				return marketWork(ctx, &workers, window, func(task context.Context) (struct{}, error) {
					plan, err := instanceDeletion.Inspect(task, id)
					if err != nil {
						return struct{}{}, err
					}
					detail := "将删除该实例下的全部数据，包括存档、模组、资源包、配置、日志、安装记录及实例专属备份，并移入系统废纸篓。不是仅移除列表记录。\n\n" + plan.MetadataPath
					if plan.ExternalPayload != "" {
						detail += "\n" + plan.ExternalPayload
					}
					for _, backup := range plan.MigrationBackups {
						detail += "\n迁移旧副本：" + backup.Path
					}
					detail += "\n\n共享 Java、公共下载/运行缓存和其他实例的数据不会删除。无法移入废纸篓时会报错，不会自动永久删除。"
					if plan.Pending {
						detail = "上次删除未完成，将继续处理剩余数据。\n\n" + detail
					}
					result, err := mygo.Dialog.Message(mygo.MessageOptions{Parent: window, Type: mygo.MessageWarning, Title: "删除实例", Message: "删除「" + plan.Name + "」及其全部数据？", Detail: detail, Buttons: []string{"取消", "删除全部数据"}, DefaultButton: 0, CancelButton: 0, DestructiveButtons: []int{1}})
					if err != nil {
						return struct{}{}, err
					}
					if task.Err() != nil || result.Button != 1 {
						return struct{}{}, context.Canceled
					}
					return struct{}{}, instanceDeletion.Delete(task, plan)
				}, func(_ struct{}, err error) { done(err) })
			},
			LoaderReleases: func(kind, game string, done func([]loader.Release, error)) func() {
				return marketWork(ctx, &workers, window, func(ctx context.Context) ([]loader.Release, error) { return loaderCatalog.Releases(ctx, kind, game) }, done)
			},
			PickIcon: func(done func([]byte, error)) func() {
				return marketWork(ctx, &workers, window, func(ctx context.Context) ([]byte, error) {
					paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: window, Title: "选择实例图标", ButtonLabel: "使用图片"})
					if err != nil || len(paths) == 0 {
						return nil, err
					}
					return platform.ReadInstanceIcon(ctx, paths[0])
				}, done)
			},
			InspectArchiveFile: func(name string, settings app.Settings, done func(app.ArchiveImportPlan, error)) func() {
				return marketWork(ctx, &workers, window, func(task context.Context) (app.ArchiveImportPlan, error) {
					return archiveImporter.InspectLocal(task, name, settings)
				}, done)
			},
			PickArchive: func(settings app.Settings, done func(app.ArchiveImportPlan, error)) func() {
				return marketWork(ctx, &workers, window, func(ctx context.Context) (app.ArchiveImportPlan, error) {
					paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: window, Title: "导入整合包", ButtonLabel: "读取清单"})
					if err != nil || len(paths) == 0 {
						return app.ArchiveImportPlan{}, err
					}
					return archiveImporter.InspectLocal(ctx, paths[0], settings)
				}, done)
			},
			Load: func(refresh bool, done func(app.LibrarySnapshot, error)) {
				background(ctx, &workers, window, func(ctx context.Context) (app.LibrarySnapshot, error) {
					return library.Load(ctx, refresh)
				}, done)
			},
			CreateTask: func(request app.CreateInstance, done func(instance.Instance, error)) func() {
				return marketWork(ctx, &workers, window, func(ctx context.Context) (instance.Instance, error) {
					return library.Create(ctx, request)
				}, done)
			},
			LoadInstance: func(id string, done func(instance.Instance, error)) {
				background(ctx, &workers, window, func(ctx context.Context) (instance.Instance, error) {
					return instanceSettings.Load(ctx, id)
				}, done)
			},
			SavePreferences: func(id string, expected, desired app.InstancePreferences, done func(instance.Instance, error)) {
				background(ctx, &workers, window, func(ctx context.Context) (instance.Instance, error) {
					return instanceSettings.Save(ctx, id, expected, desired)
				}, done)
			},
		})
		bindImportIntents(ctx, &workers, window, shell)
		shell.ConnectInstallations(launcherui.InstallationActions{
			Load: func(done func(map[string]install.Receipt, error)) {
				background(ctx, &workers, window, installations.Load, done)
			},
			Start: func(id string, progress func(install.Progress), done func(install.Receipt, error)) func() {
				return runTask(ctx, &workers, window, func(task context.Context, report func(install.Progress)) (install.Receipt, error) {
					current, _ := repository.Load(task)
					return installations.RunWithJava(task, id, current.JavaPath, report)
				}, progress, done)
			},
			Check: func(id string, settings app.Settings, progress func(install.Progress), done func(app.PreparationReport, error)) func() {
				return runTask(ctx, &workers, window, func(task context.Context, report func(install.Progress)) (app.PreparationReport, error) {
					return launcher.Check(task, id, settings, report)
				}, progress, done)
			},
			Launch: func(id, accountID string, settings app.Settings, progress func(install.Progress), done func(app.LaunchResult, error)) func() {
				return runTask(ctx, &workers, window, func(task context.Context, report func(install.Progress)) (app.LaunchResult, error) {
					return launcher.Run(task, id, settings, func(ctx context.Context) (launch.Session, error) {
						return accounts.Session(ctx, accountID)
					}, report)
				}, progress, done)
			},
			Sessions: func(done func([]launch.Record, error)) { background(ctx, &workers, window, sessions.Active, done) },
		})
		shell.ConnectGames(launcherui.GameActions{Stop: func(id string, done func(error)) {
			background(ctx, &workers, window, func(ctx context.Context) (struct{}, error) {
				stop, finish := context.WithTimeout(ctx, 5*time.Second)
				defer finish()
				return struct{}{}, launcher.Stop(stop, id)
			}, func(_ struct{}, err error) { done(err) })
		}})
		workers.Go(func() {
			var pendingMu sync.Mutex
			var latest []app.GameSnapshot
			scheduled := false
			publish := func() {
				rows := launcher.Sessions()
				pendingMu.Lock()
				latest = rows
				if scheduled {
					pendingMu.Unlock()
					return
				}
				scheduled = true
				pendingMu.Unlock()
				window.Update(func() {
					pendingMu.Lock()
					rows := latest
					latest = nil
					scheduled = false
					pendingMu.Unlock()
					if ctx.Err() == nil {
						shell.SetGames(rows)
					}
				})
			}
			publish()
			for {
				select {
				case <-ctx.Done():
					return
				case <-launcher.Changed():
				}
				// Coalesce log bursts. Changes carry no unbounded event queue;
				// publish always reads the latest process state, including exit.
				timer := time.NewTimer(100 * time.Millisecond)
				select {
				case <-ctx.Done():
					timer.Stop()
					return
				case <-timer.C:
				}
				publish()
			}
		})
	})
	if err := mygo.App.Run(); err != nil {
		return fmt.Errorf("run native application: %w", err)
	}
	return nil
}

func runTask[T any](ctx context.Context, workers *sync.WaitGroup, window *mygo.Window, work func(context.Context, func(install.Progress)) (T, error), progress func(install.Progress), done func(T, error)) func() {
	task, stop := context.WithCancel(ctx)
	workers.Go(func() {
		defer stop()
		var last time.Time
		var stage string
		value, err := work(task, func(p install.Progress) {
			if ctx.Err() != nil {
				return
			}
			if p.Stage == stage && time.Since(last) < 100*time.Millisecond && p.Files != p.TotalFiles {
				return
			}
			last, stage = time.Now(), p.Stage
			window.Update(func() {
				if ctx.Err() == nil {
					progress(p)
				}
			})
		})
		if ctx.Err() != nil {
			return
		}
		window.Update(func() {
			if ctx.Err() == nil {
				done(value, err)
			}
		})
	})
	return stop
}

func background[T any](ctx context.Context, workers *sync.WaitGroup, window *mygo.Window, work func(context.Context) (T, error), done func(T, error)) {
	workers.Go(func() {
		value, err := work(ctx)
		if ctx.Err() != nil {
			return
		}
		window.Update(func() {
			if ctx.Err() == nil {
				done(value, err)
			}
		})
	})
}
