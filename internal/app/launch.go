package app

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"mcl/internal/core/download"
	"mcl/internal/core/install"
	"mcl/internal/core/instance"
	"mcl/internal/core/java"
	"mcl/internal/core/launch"
)

type RuntimeProbe func(context.Context, string) (java.Runtime, error)

// ProcessStarter must register identity while the target is still gated or
// suspended, and abort the target if registration fails. The production adapter
// is platform/process.StartTracked; executing before registration is forbidden.
type ProcessStarter func(launch.Plan, func(launch.ProcessIdentity) error) (launch.Game, error)

type GameDirectoryResolver func(context.Context, string) (string, error)

type LaunchService struct {
	Tasks           *TaskService
	gameDirectory   GameDirectoryResolver
	installations   *InstallationService
	resolveJava     JavaResolver
	memoryProbe     MemoryProbe
	start           ProcessStarter
	windowVisible   func(launch.ProcessIdentity) (bool, error)
	windowWatchDone <-chan struct{}
	stopWindowWatch func()
	gamesMu         sync.Mutex
	games           map[string]*managedGame
	history         []string
	changed         chan struct{}
}

type PreparationReport struct {
	InstanceID, VersionID string
	Java                  java.Runtime
	MemoryMiB             int
	Preview               launch.Preview
	Receipt               install.Receipt
}

// Check prepares and releases an instance. A launch prepares again under its
// own reservation before using the selected identity.
func (s *LaunchService) Check(ctx context.Context, id string, settings Settings, report func(install.Progress)) (PreparationReport, error) {
	if s.Tasks != nil && taskScope(ctx) == nil {
		intent, err := taskGameIntent(ctx, s.installations.instances, id, "preparation")
		if err != nil {
			return PreparationReport{}, err
		}
		return performTask(ctx, s.Tasks, intent, func(task context.Context) (PreparationReport, error) {
			if err := checkTaskTarget(task, s.installations.instances, intent); err != nil {
				return PreparationReport{}, err
			}
			return s.Check(task, id, settings, report)
		})
	}
	p, err := s.Prepare(ctx, id, settings, report)
	if err != nil {
		return PreparationReport{}, err
	}
	defer p.Close()
	preview, err := p.Preview()
	if err != nil {
		return PreparationReport{}, err
	}
	return PreparationReport{InstanceID: id, VersionID: p.options.Version.ID, Java: p.options.Java, MemoryMiB: p.options.MaxMemoryMiB, Preview: preview, Receipt: p.receipt}, nil
}

func NewLaunchService(installations *InstallationService, resolveJava JavaResolver, memoryProbe MemoryProbe, start ProcessStarter, windowVisible func(launch.ProcessIdentity) (bool, error), directories ...GameDirectoryResolver) *LaunchService {
	windowWatchDone := make(chan struct{})
	var directory GameDirectoryResolver
	if len(directories) > 0 {
		directory = directories[0]
	}
	return &LaunchService{gameDirectory: directory,
		installations: installations, resolveJava: resolveJava, memoryProbe: memoryProbe, start: start,
		windowVisible: windowVisible, windowWatchDone: windowWatchDone,
		stopWindowWatch: sync.OnceFunc(func() { close(windowWatchDone) }),
		games:           make(map[string]*managedGame), changed: make(chan struct{}, 1),
	}
}

type LaunchResult struct {
	Warnings []string
	Receipt  install.Receipt
	GameID   string
}

func (s *LaunchService) Run(ctx context.Context, id string, settings Settings, resolve func(context.Context) (launch.Session, error), report func(install.Progress)) (LaunchResult, error) {
	if s.Tasks != nil && taskScope(ctx) == nil {
		intent, err := taskGameIntent(ctx, s.installations.instances, id, "preparation")
		if err != nil {
			return LaunchResult{}, err
		}
		return performTask(ctx, s.Tasks, intent, func(task context.Context) (LaunchResult, error) {
			if err := checkTaskTarget(task, s.installations.instances, intent); err != nil {
				return LaunchResult{}, err
			}
			return s.Run(task, id, settings, resolve, report)
		})
	}
	p, err := s.Prepare(ctx, id, settings, report)
	if err != nil {
		return LaunchResult{}, err
	}
	defer p.Close()
	if report != nil {
		report(install.Progress{Stage: "account"})
	}
	session, err := resolve(ctx)
	if err != nil {
		return LaunchResult{}, err
	}
	if report != nil {
		report(install.Progress{Stage: "launch"})
	}
	game, err := p.Start(session)
	if err != nil {
		return LaunchResult{}, err
	}
	return LaunchResult{Receipt: p.receipt, GameID: p.gameID, Warnings: launch.StartupWarnings(game)}, nil
}

// Prepared owns the instance reservation. Call Close when abandoning it, or
// Start to transfer that reservation until the game session finishes. Canceling
// the preparation context closes an unstarted preparation, never a live game.
type Prepared struct {
	memoryProbe MemoryProbe
	mu          sync.Mutex
	ctx         context.Context
	options     launch.Options
	start       ProcessStarter
	release     func()
	lease       SessionLease
	versionSHA1 string
	receipt     install.Receipt
	gameID      string
	settled     chan struct{}
	state       int // 0 ready, 1 started, 2 closed
	started     func(launch.Record, launch.Game, func(), func(string) string)
}

func (s *LaunchService) Prepare(ctx context.Context, id string, fallback Settings, report func(install.Progress)) (*Prepared, error) {
	if err := fallback.Validate(); err != nil {
		return nil, err
	}
	if s.resolveJava == nil {
		return nil, errors.New("Java 自动管理服务未配置")
	}
	reserved, err := s.installations.reserve(ctx, id)
	if err != nil {
		return nil, err
	}
	transferred := false
	defer func() {
		if !transferred {
			reserved.Close()
		}
	}()
	value, err := s.installations.instances.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := checkTaskInstance(ctx, value); err != nil {
		return nil, err
	}

	gameDirectory := ""
	if s.gameDirectory != nil {
		directory, err := s.gameDirectory(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("读取实例游戏目录: %w", err)
		}
		gameDirectory = directory
	} else if value.GameDirectoryID != "" {
		return nil, errors.New("外部游戏目录解析未配置")
	}

	if report != nil {
		report(install.Progress{Stage: "metadata"})
	}
	base, err := s.installations.catalog.Version(ctx, value.Version)
	if err != nil {
		return nil, err
	}
	if _, err := install.Build(base, s.installations.platform); err != nil {
		return nil, fmt.Errorf("客户端平台兼容性: %w", err)
	}
	javaProgress := func(p java.Progress) {
		if report != nil {
			report(install.Progress{Stage: p.Stage, Progress: download.Progress{Bytes: p.Bytes, TotalBytes: p.Total}})
		}
	}
	if err := taskBaseIdentity(ctx, value, base); err != nil {
		return nil, err
	}
	preferred := []string{value.JavaPath, fallback.JavaPath}
	runtime, err := s.resolveJava(ctx, base.Java.Major, s.installations.platform, preferred, javaProgress)
	if err != nil {
		return nil, err
	}
	if err := runtime.Check(base.Java.Major, s.installations.platform); err != nil {
		return nil, err
	}
	// Pass the verified game executable only as a hint. The loader resolves its
	// independent tool Java and must never raise the game major to run a tool.
	loaderInstance := value
	loaderInstance.JavaPath = runtime.Executable
	resolved, err := s.installations.resolveLoader(ctx, loaderInstance, base, runtime.Executable, report)
	if err != nil {
		return nil, err
	}
	if resolved.version.Java.Major > runtime.Major {
		runtime, err = s.resolveJava(ctx, resolved.version.Java.Major, s.installations.platform, preferred, javaProgress)
		if err != nil {
			return nil, err
		}
	}
	if err := runtime.Check(resolved.version.Java.Major, s.installations.platform); err != nil {
		return nil, err
	}
	receipt, err := s.installations.execute(ctx, value, resolved, report)
	if err != nil {
		return nil, err
	}
	memoryMiB, err := launchMemoryMiB(ctx, s.memoryProbe, runtime)
	if err != nil {
		return nil, err
	}
	jvmArguments, err := launch.ParseJVMArguments(fallback.JVMArguments)
	if err != nil {
		return nil, err
	}
	options := launch.Options{
		Root: s.installations.client.Root, InstanceID: id, Version: resolved.version, GameDirectory: gameDirectory,
		Platform: s.installations.platform, Java: runtime, NativeDirectory: receipt.NativeDirectory, LegacyAssetsDirectory: receipt.LegacyAssetsDirectory,
		MaxMemoryMiB: memoryMiB,
		GCMode:       fallback.GCMode, JVMArguments: jvmArguments, Game: fallback.Game,
	}
	if _, err := launch.PreviewOptions(options); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	p := &Prepared{memoryProbe: s.memoryProbe, ctx: ctx, options: options, receipt: receipt, versionSHA1: value.Version.SHA1, start: s.start, release: reserved.Close, lease: reserved.lease, settled: make(chan struct{}), started: s.manage}
	transferred = true
	go func() {
		select {
		case <-ctx.Done():
			p.Close()
		case <-p.settled:
		}
	}()
	return p, nil
}

func (p *Prepared) Preview() (launch.Preview, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.state != 0 {
		return launch.Preview{}, errors.New("启动准备已结束")
	}
	if err := p.ctx.Err(); err != nil {
		return launch.Preview{}, err
	}
	return launch.PreviewOptions(p.options)
}

func (p *Prepared) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.state == 0 {
		p.state = 2
		p.release()
		close(p.settled)
	}
}

func (p *Prepared) Start(session launch.Session) (launch.Game, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.state != 0 {
		return nil, errors.New("启动准备已结束，不能重复启动")
	}
	if err := p.ctx.Err(); err != nil {
		p.state = 2
		p.release()
		close(p.settled)
		return nil, err
	}
	if p.start == nil {
		return nil, errors.New("游戏进程服务未配置")
	}
	// Authentication may outlive preparation. Re-read current availability
	// before creating the process rather than launch a stale memory budget.
	memoryMiB, err := launchMemoryMiB(p.ctx, p.memoryProbe, p.options.Java)
	if err != nil {
		return nil, err
	}
	p.options.MaxMemoryMiB = memoryMiB
	plan, err := launch.Build(p.options, session)
	if err != nil {
		return nil, err
	}
	record := launch.Record{ID: instance.NewID(), InstanceID: p.options.InstanceID, VersionID: p.options.Version.ID, VersionSHA1: p.versionSHA1, NativeDirectory: p.options.NativeDirectory}
	game, err := p.start(plan, func(identity launch.ProcessIdentity) error {
		record.Process, record.CreatedAt = identity, time.Now().UTC()
		return p.lease.Track(p.ctx, record)
	})
	if err != nil || game == nil {
		p.state = 2
		p.release()
		close(p.settled)
		if err == nil {
			err = errors.New("进程服务没有返回会话")
		}
		return nil, fmt.Errorf("启动游戏失败: %s", plan.Redact(err.Error()))
	}
	p.state = 1
	p.gameID = record.ID
	close(p.settled)
	p.started(record, game, p.release, plan.Redact)
	return game, nil
}
