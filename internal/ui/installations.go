package ui

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"mcl/internal/app"
	"mcl/internal/core/install"
	"mcl/internal/core/instance"
	"mcl/internal/core/launch"
	"mcl/internal/core/loader"
	"mcl/internal/core/meta"

	"github.com/egoist/mygo/ui"
)

// InstallationActions delivers callbacks on the UI thread. Start returns a
// nonblocking cancellation request; done runs only after all workers stop.
type InstallationActions struct {
	Load     func(func(map[string]install.Receipt, error))
	Start    func(string, func(install.Progress), func(install.Receipt, error)) func()
	Check    func(string, app.Settings, func(install.Progress), func(app.PreparationReport, error)) func()
	Launch   func(string, string, app.Settings, func(install.Progress), func(app.LaunchResult, error)) func()
	Sessions func(func([]launch.Record, error))
}

type installationState struct {
	actions           InstallationActions
	receipts          map[string]install.Receipt
	generation        uint64
	instanceID, name  string
	active, canceling bool
	cancel            func()
	progress          install.Progress
	message           string
	failed            bool
	checking          bool
	launching         bool
	checked           *app.PreparationReport
	sessions          []launch.Record
	sessionsBusy      bool
	sessionsError     string
	events            []launch.Line
	eventSequence     uint64
	lastStage         string
	launchGameID      string
	requestLaunchStop func()
}

func (s *Shell) taskEvent(text string, failed bool) {
	i := &s.installation
	i.eventSequence++
	stream := "stdout"
	if failed {
		stream = "stderr"
	}
	truncated := len(text) > 8192
	if truncated {
		text = strings.ToValidUTF8(text[:8192], "�")
	}
	i.events = append(i.events, launch.Line{Sequence: i.eventSequence, Time: time.Now().UTC(), Stream: stream, Text: text, Truncated: truncated})
	if len(i.events) > 200 {
		i.events = i.events[len(i.events)-200:]
	}
}

func (s *Shell) taskProgress(progress install.Progress) {
	i := &s.installation
	if progress.Stage != i.lastStage {
		s.taskEvent(installationStage(progress.Stage), false)
		i.lastStage = progress.Stage
	}
	i.progress = progress
}

func (s *Shell) ConnectInstallations(actions InstallationActions) {
	s.installation.actions = actions
	s.refreshSessions()
	if actions.Load != nil {
		generation := s.installation.generation
		actions.Load(func(receipts map[string]install.Receipt, err error) {
			if generation != s.installation.generation {
				return
			}
			s.installation.receipts = receipts
			if err != nil {
				s.installation.message, s.installation.failed = err.Error(), true
			}
		})
	}
}

func (s *Shell) refreshSessions() {
	i := &s.installation
	if i.sessionsBusy || i.actions.Sessions == nil {
		return
	}
	i.sessionsBusy = true
	i.actions.Sessions(func(records []launch.Record, err error) {
		i.sessionsBusy = false
		if err != nil {
			i.sessionsError = err.Error()
			return
		}
		i.sessions, i.sessionsError = records, ""
	})
}

func (s *Shell) startLaunchCheck() {
	i := &s.installation
	value, ok := s.selectedInstance()
	if !ok || i.active || i.actions.Check == nil {
		return
	}
	i.generation++
	generation := i.generation
	i.instanceID, i.name, i.active, i.checking = value.ID, value.Name, true, true
	i.launching = false
	i.canceling, i.failed, i.message, i.checked = false, false, "", nil
	i.events, i.lastStage = nil, ""
	s.taskEvent("开始检查启动条件", false)
	i.progress = install.Progress{Stage: "java"}
	s.page = activityPage
	i.cancel = i.actions.Check(value.ID, s.settings, func(p install.Progress) {
		if i.active && i.generation == generation {
			s.taskProgress(p)
		}
	}, func(report app.PreparationReport, err error) {
		if i.generation != generation {
			return
		}
		i.active, i.cancel, i.failed = false, nil, err != nil
		if err != nil {
			if i.canceling {
				i.message, i.failed = "检查已取消，已校验文件可在下次复用。", false
			} else {
				i.message = err.Error()
			}
			i.canceling = false
			s.taskEvent(i.message, i.failed)
			return
		}
		i.checked = &report
		if i.receipts == nil {
			i.receipts = make(map[string]install.Receipt)
		}
		i.receipts[report.InstanceID] = report.Receipt
		i.message = "游戏文件与 Java 已就绪。"
		s.taskEvent(i.message, false)
	})
}

func (s *Shell) startGame() {
	i := &s.installation
	value, ok := s.selectedInstance()
	if !ok || i.active || i.actions.Launch == nil {
		return
	}
	profile, ok := s.accounts.data.Active()
	if !ok {
		s.openAccounts()
		return
	}
	i.generation++
	generation := i.generation
	i.instanceID, i.name, i.active = value.ID, value.Name, true
	i.checking, i.launching, i.checked = false, true, nil
	i.canceling, i.failed, i.message = false, false, ""
	i.events, i.lastStage = nil, ""
	i.progress = install.Progress{Stage: "metadata"}
	i.launchGameID = ""
	stopAfterStart := false
	i.requestLaunchStop = func() {
		// If the process already passed the handoff, cancellation cannot
		// take it back. Stop the exact session when its result arrives.
		stopAfterStart = true
		s.cancelInstallation()
	}
	s.taskEvent("准备启动游戏 · "+profile.Name, false)
	s.page = launchPage
	i.cancel = i.actions.Launch(value.ID, profile.ID(), s.settings, func(p install.Progress) {
		if i.active && i.generation == generation {
			s.taskProgress(p)
		}
	}, func(result app.LaunchResult, err error) {
		if stopAfterStart && result.GameID != "" {
			s.stopGame(result.GameID)
		}
		if i.generation != generation {
			return
		}
		i.active, i.cancel, i.canceling = false, nil, false
		i.requestLaunchStop = nil
		switch {
		case errors.Is(err, context.Canceled):
			i.message = "已取消启动准备。"
		case err != nil:
			i.message, i.failed = err.Error(), true
		default:
			if i.receipts == nil {
				i.receipts = make(map[string]install.Receipt)
			}
			i.receipts[value.ID] = result.Receipt
			i.launchGameID = result.GameID
			i.message = "游戏进程已启动。"
			s.loadAccounts()
		}
		s.taskEvent(i.message, i.failed)
		if err == nil {
			s.refreshJava()
			s.refreshSessions()
		}
	})
}

func (s *Shell) startInstallation() {
	i := &s.installation
	if i.active || i.actions.Start == nil {
		return
	}
	value, ok := s.selectedInstance()
	if !ok {
		return
	}
	i.generation++
	i.checking, i.launching, i.checked = false, false, nil
	generation := i.generation
	i.instanceID, i.name, i.active, i.canceling, i.failed = value.ID, value.Name, true, false, false
	i.message, i.progress = "", install.Progress{Stage: "queued"}
	i.events, i.lastStage = nil, ""
	s.taskEvent("开始安装与校验", false)
	s.page = activityPage
	i.cancel = i.actions.Start(value.ID, func(progress install.Progress) {
		if i.generation == generation && i.active {
			s.taskProgress(progress)
		}
	}, func(receipt install.Receipt, err error) {
		if i.generation != generation {
			return
		}
		i.active, i.cancel, i.failed = false, nil, err != nil
		if err != nil {
			if i.canceling {
				i.message, i.failed = "任务已取消，已校验的文件将在重试时复用。", false
			} else {
				i.message = err.Error()
			}
			i.canceling = false
			s.taskEvent(i.message, i.failed)
			return
		}
		if i.receipts == nil {
			i.receipts = make(map[string]install.Receipt)
		}
		i.receipts[receipt.InstanceID] = receipt
		i.message = "安装与校验完成。游戏启动和微软登录将在后续阶段接入。"
		s.taskEvent(i.message, false)
	})
}

func (s *Shell) cancelInstallation() {
	i := &s.installation
	if i.active && !i.canceling && i.cancel != nil {
		i.canceling = true
		i.cancel()
	}
}

func installationStage(stage string) string {
	switch stage {
	case "queued":
		return "等待安装"
	case "account":
		return "准备玩家账户"
	case "launch":
		return "启动游戏进程"
	case "java":
		return "检查 Java 版本与架构"
	case "java-discovery":
		return "寻找合适的 Java"
	case "java-catalog":
		return "获取 Java 运行环境"
	case "java-download":
		return "自动下载 Java"
	case "java-extract":
		return "安装并检查 Java"
	case "metadata":
		return "读取版本信息"
	case "loader":
		return "配置模组加载器"
	case "loader-download":
		return "下载加载器与处理器依赖"
	case "loader-processors":
		return "生成加载器客户端文件"
	case "files":
		return "下载与校验游戏文件"
	case "assets":
		return "下载与校验资产对象"
	case "natives":
		return "准备本地库"
	default:
		return "准备任务"
	}
}

func (s *Shell) activityView(c *ui.Context) {
	i, t := &s.installation, s.theme
	ui.Scroll(c).Grow(1).FillWidth().Padding(18, 24, 24).Gap(8).Children(func() {
		ui.Row(c).FillWidth().Height(28).Shrink(0).Gap(8).Children(func() {
			settingsGroupLabel(c, "下载与任务").Grow(1)
			ui.Button(c.Key("refresh-sessions"), "刷新会话").Height(24).Disabled(i.sessionsBusy).OnClick(s.refreshSessions)
		})
		s.marketTaskCard(c)
		s.javaTaskCard(c)
		s.assetAssociationTask(c)
		s.assetIdentityTaskCard(c)
		s.assetUpdateTaskCard(c)
		s.assetDiagnosticTaskCard(c)
		s.packRepairTaskCard(c)
		s.assetChangeStatus(c)
		s.gameCards(c)
		if s.games.message != "" {
			ui.Text(c, s.games.message).FontSize(11).TextColor(t.TextMuted)
		}
		if i.sessionsError != "" {
			ui.Text(c, i.sessionsError).FontSize(11).TextColor(t.Danger)
		}
		for _, record := range i.sessions {
			if s.knownGame(record.ID) {
				continue
			}
			name := record.VersionID
			for _, value := range s.library.snapshot.Instances {
				if value.ID == record.InstanceID {
					name = value.Name
					break
				}
			}
			section(c, name, func() {
				ui.Textf(c, "游戏仍在运行 · Minecraft %s · PID %d", record.VersionID, record.Process.PID).FontSize(12)
				ui.Text(c, "请在游戏窗口中退出。上次启动器的日志无法在此恢复。").FontSize(11).TextColor(t.TextMuted)
			})
		}
		if i.instanceID == "" {
			marketTask := s.market.install.hasTask()
			repair := s.assets.repair
			packTask := repair != nil && (repair.job != "" || repair.message != "")
			association := s.market.association
			associationTask := association != nil && (association.active || association.message != "")
			if !marketTask && !packTask && !associationTask && s.java.job.packageID == "" && !s.assets.changing && s.assets.identity == nil && s.assets.update == nil && s.assets.diagnostic == nil && len(s.games.rows) == 0 && len(i.sessions) == 0 {
				ui.Text(c, "暂无任务").FontSize(13).TextColor(t.TextMuted)
			}
			if i.message != "" {
				ui.Text(c, i.message).FontSize(11).TextColor(t.Danger)
			}
			return
		}
		section(c, i.name, func() {
			ui.Button(c.Key("task-logs"), "任务日志").Width(90).Height(24).OnClick(func() { s.showLogs("") })
			if game, found := s.launchGameSnapshot(); s.waitingForGameWindow(game, found) {
				ui.Row(c).Gap(12).Children(func() {
					ui.Text(c, "等待游戏窗口…").Grow(1).FontSize(12).TextColor(t.TextMuted)
					ui.Button(c.Key("view-launch"), "查看启动").Height(26).OnClick(func() { s.page = launchPage })
				})
				return
			}
			if i.active {
				ui.Row(c).Gap(12).Children(func() {
					busyIndicator(c, 14)
					ui.Text(c.Key("installation-stage"), installationStage(i.progress.Stage)).Grow(1).ContentTransition(ui.ContentTransition{Animation: &motionFade})
					label := "取消任务"
					if i.canceling {
						label = "正在取消…"
					}
					ui.Button(c.Key("cancel-install"), label).Height(26).Disabled(i.canceling).OnClick(s.cancelInstallation)
				})
				p := i.progress.Progress
				if p.TotalFiles > 0 {
					ui.Text(c, fmt.Sprintf("%d / %d 个文件 · %.1f / %.1f MiB · 复用 %d 个", p.Files, p.TotalFiles, float64(p.Bytes)/(1<<20), float64(p.TotalBytes)/(1<<20), p.Reused)).FontSize(11).TextColor(colors(t).Metadata)
					smoothProgress(c, float64(p.Files)/float64(p.TotalFiles)).Label("安装进度").FillWidth().Height(4)
				}
			} else {
				color := t.TextMuted
				if i.failed {
					color = t.Danger
				}
				ui.Text(c, i.message).FontSize(12).TextColor(color)
				if report := i.checked; report != nil {
					ui.Textf(c, "Minecraft %s · Java %s · %s · %d MiB", report.VersionID, report.Java.Version, report.Java.Arch, report.MemoryMiB).FontSize(12).TextColor(t.TextMuted)
					ui.Text(c, report.Java.Executable).FontSize(11).TextColor(colors(t).Metadata).Selectable()
				}
				ui.Row(c).Gap(8).Children(func() {
					label := "再次校验 / 重试"
					if i.checking {
						label = "重新检查"
					} else if i.launching {
						label = "重新启动"
					}
					ui.Button(c.Key("retry-install"), label).Height(26).OnClick(func() {
						s.library.selectedID = i.instanceID
						if i.checking {
							s.startLaunchCheck()
						} else if i.launching {
							s.startGame()
						} else {
							s.startInstallation()
						}
					})
					ui.Button(c.Key("task-instance-settings"), "版本设置").Height(26).OnClick(func() { s.library.selectedID = i.instanceID; s.openInstanceSettings() })
				})
			}
		})
		ui.Text(c, "进度按完整校验的文件计数。取消后可重新安装，完整文件会直接复用。").FontSize(11).TextColor(t.TextMuted)
	})
}

func (s *Shell) launchButton() (label, subtitle string, action func(), disabled bool) {
	value, selected := s.selectedInstance()
	if !selected {
		return "没有可启动的版本", "先创建一个实例", s.openNewInstance, false
	}
	i := &s.installation
	if i.instanceID == value.ID {
		if game, found := s.launchGameSnapshot(); s.waitingForGameWindow(game, found) {
			return "等待游戏窗口", value.Name, func() { s.page = launchPage }, false
		}
	}
	for _, game := range s.games.rows {
		if game.InstanceID == value.ID && game.Running {
			return "游戏正在运行", value.Name, func() { s.showLogs(game.ID) }, false
		}
	}
	for _, record := range i.sessions {
		if s.knownGame(record.ID) {
			continue
		}
		if record.InstanceID == value.ID {
			return "游戏仍在运行", value.Name, func() { s.page = activityPage; s.refreshSessions() }, false
		}
	}
	if i.active {
		if i.launching {
			return "查看启动任务", i.name + " · " + installationStage(i.progress.Stage), func() { s.page = launchPage }, false
		}
		return "查看安装任务", i.name + " · " + installationStage(i.progress.Stage), func() { s.page = activityPage }, false
	}
	if s.identifyingAssets(value.ID) {
		return "识别资产中", value.Name, func() { s.page = activityPage }, false
	}
	if s.associatingAsset(value.ID) {
		return "核对资产来源中", value.Name, func() { s.showAssetAssociation(s.market.association) }, false
	}
	if s.diagnosingAssets(value.ID) {
		return "正在检查资产", value.Name, func() { s.viewAssetDiagnostic(s.assets.diagnostic) }, false
	}
	if s.assets.changeID == value.ID {
		return "修改资产中", value.Name, func() { s.page = activityPage }, false
	}
	if s.repairingPack(value.ID) {
		return "正在恢复整合包文件", value.Name, func() { s.viewPackRepair(s.assets.repair) }, false
	}
	if s.assetUpdateBlocked(value.ID) {
		return "查看资产更新", "请先完成或恢复资产更新", func() {
			if state := s.assets.update; state != nil && state.instanceID == value.ID && state.busy {
				s.viewAssetUpdate(state)
			} else {
				s.assets.showUpdate, s.page = false, libraryPage
			}
		}, false
	}
	if required, ok := value.RequiredLoader(); ok && !supportedPackLoader(value) {
		return "暂不支持此加载器", required.Type + " " + required.Version, s.openInstanceSettings, false
	}
	if i.actions.Launch != nil {
		if profile, ok := s.accounts.data.Active(); ok {
			return "启动游戏", value.Name + " · " + profile.Name, s.startGame, false
		}
		return "选择账户", "添加微软账户或离线 ID", s.openAccounts, false
	}
	if s.instanceInstalled(value) {
		return "检查启动条件", value.Name + " · Java " + strconv.Itoa(value.RequiredJava) + "+", s.startLaunchCheck, i.actions.Check == nil
	}
	return "安装游戏文件", value.Name + " · " + value.Version.ID, s.startInstallation, i.actions.Start == nil
}

func (s *Shell) instanceInstalled(value instance.Instance) bool {
	receipt, ok := s.installation.receipts[value.ID]
	return ok && receipt.Matches(value, meta.CurrentPlatform())
}

func supportedPackLoader(value instance.Instance) bool {
	required, ok := value.RequiredLoader()
	return !ok || loader.Supported(required.Type)
}

func (s *Shell) instanceRuntimeStatus(value instance.Instance) string {
	required, ok := value.RequiredLoader()
	if !ok {
		return ""
	}
	status := "运行环境未安装"
	if !supportedPackLoader(value) {
		status = "安装处理器尚未接入"
	} else if s.instanceInstalled(value) {
		status = "已安装"
	}
	return required.Type + " " + required.Version + " · " + status
}
