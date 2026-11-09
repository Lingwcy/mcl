package ui

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"

	"mcl/internal/app"
	"mcl/internal/core/java"
)

type JavaActions struct {
	Inventory func(string, func(app.JavaInventory, error)) func()
	Pick      func(bool, func(app.JavaEnvironment, error)) func()
	Remove    func(string, func(error))
	Catalog   func(int, func(java.Catalog, error)) func()
	Install   func(java.Package, func(java.Progress), func(app.JavaEnvironment, error)) func()
	Open      func(string, func(error))
}

type javaState struct {
	actions                               JavaActions
	inventory                             app.JavaInventory
	catalog                               java.Catalog
	major                                 int
	scanning, editing, loading, downloads bool
	scanGeneration, catalogGeneration     uint64
	cancelScan, cancelCatalog, cancelPick func()
	message                               string
	failed                                bool
	job                                   javaJob
}

type javaJob struct {
	packageID, title, message string
	active, canceling, failed bool
	progress                  java.Progress
	cancel                    func()
}

func (s *Shell) ConnectJava(actions JavaActions) {
	s.java.actions, s.java.major = actions, 21
	s.refreshJava()
}

func (s *Shell) refreshJava() {
	j := &s.java
	if j.actions.Inventory == nil {
		return
	}
	if j.cancelScan != nil {
		j.cancelScan()
	}
	j.scanGeneration++
	generation := j.scanGeneration
	j.scanning, j.message, j.failed = true, "", false
	j.cancelScan = j.actions.Inventory(s.settings.JavaPath, func(value app.JavaInventory, err error) {
		if generation != j.scanGeneration {
			return
		}
		j.scanning, j.cancelScan = false, nil
		if err != nil {
			if !errors.Is(err, context.Canceled) {
				j.message, j.failed = err.Error(), true
			}
			return
		}
		j.inventory = value
	})
}

func (s *Shell) pickJava(home bool) {
	j := &s.java
	if j.editing || j.actions.Pick == nil {
		return
	}
	j.editing, j.message = true, ""
	j.cancelPick = j.actions.Pick(home, func(_ app.JavaEnvironment, err error) {
		j.editing, j.cancelPick = false, nil
		if err != nil {
			if !errors.Is(err, context.Canceled) {
				j.message, j.failed = err.Error(), true
			}
			return
		}
		s.refreshJava()
	})
}

func (s *Shell) removeJava(value app.JavaEnvironment) {
	j := &s.java
	if j.editing || j.actions.Remove == nil {
		return
	}
	j.editing, j.message = true, ""
	j.actions.Remove(value.Runtime.Executable, func(err error) {
		j.editing = false
		if err != nil {
			j.message, j.failed = err.Error(), true
			return
		}
		s.refreshJava()
	})
}

func (s *Shell) defaultJava(value app.JavaEnvironment) {
	if s.saving || s.loadErr != nil || s.save == nil {
		return
	}
	snapshot := s.settings
	snapshot.JavaPath = value.Runtime.Executable
	s.saving, s.java.message = true, ""
	s.save(snapshot, func(err error) {
		s.saving = false
		if err != nil {
			s.java.message, s.java.failed = err.Error(), true
		} else {
			s.settings.JavaPath = snapshot.JavaPath
		}
		if s.savePending {
			s.saveSettings()
		}
	})
}

func (s *Shell) openJavaDownloads() {
	s.page, s.java.downloads = settingsPage, true
	if s.java.catalog.Packages == nil {
		s.loadJavaCatalog()
	}
}

func (s *Shell) loadJavaCatalog() {
	j := &s.java
	if j.actions.Catalog == nil {
		return
	}
	if j.cancelCatalog != nil {
		j.cancelCatalog()
	}
	j.catalogGeneration++
	generation := j.catalogGeneration
	j.loading, j.message, j.failed = true, "", false
	j.catalog.Packages = nil
	j.cancelCatalog = j.actions.Catalog(j.major, func(value java.Catalog, err error) {
		if generation != j.catalogGeneration {
			return
		}
		j.loading, j.cancelCatalog = false, nil
		j.catalog = value
		if err != nil && !errors.Is(err, context.Canceled) {
			j.message, j.failed = err.Error(), true
		}
	})
}

func (s *Shell) closeJavaDownloads() {
	j := &s.java
	j.downloads = false
	if j.cancelCatalog != nil {
		j.cancelCatalog()
		j.cancelCatalog = nil
		j.catalogGeneration++
		j.loading = false
	}
	j.message = ""
}

func (s *Shell) installJava(p java.Package) {
	j := &s.java
	if j.job.active || j.actions.Install == nil {
		return
	}
	j.job = javaJob{packageID: p.ID(), title: p.Name + " " + p.Version + " " + strings.ToUpper(p.Kind), active: true}
	j.job.cancel = j.actions.Install(p, func(progress java.Progress) {
		if j.job.active {
			j.job.progress = progress
		}
	}, func(_ app.JavaEnvironment, err error) {
		j.job.active, j.job.cancel, j.job.canceling = false, nil, false
		switch {
		case errors.Is(err, context.Canceled):
			j.job.message = "已取消"
		case err != nil:
			j.job.message, j.job.failed = err.Error(), true
		default:
			j.job.message = "已安装"
			j.downloads = false
			s.refreshJava()
		}
	})
}

func (s *Shell) cancelJavaInstall() {
	job := &s.java.job
	if job.active && !job.canceling && job.cancel != nil {
		job.canceling = true
		job.cancel()
	}
}

func (s *Shell) javaView(c *ui.Context) {
	j := &s.java
	ui.Column(c.Key("java-workspace")).Grow(1).MinHeight(0).FillWidth().Padding(18, 24, 20).Gap(8).Children(func() {
		ui.Row(c.Key("java-toolbar")).FillWidth().Height(28).Shrink(0).Gap(8).Children(func() {
			if j.downloads {
				iconButton(c, "back-java", "返回 Java 管理", iconBack, s.closeJavaDownloads)
				settingsGroupLabel(c, "下载 Java").Grow(1).MinWidth(0)
				s.javaVersionMenu(c)
				iconButton(c, "refresh-java-catalog", "刷新下载目录", iconRefresh, s.loadJavaCatalog).Disabled(j.loading)
			} else {
				ui.Spacer(c)
				ui.PrimaryButton(c.Key("download-java"), "下载").Height(26).OnClick(s.openJavaDownloads)
				ui.Button(c.Key("add-java"), "添加").Label("添加本机 Java").Height(26).Disabled(j.editing).Menu(func(menu *ui.Menu) {
					if menu.Item("选择 Java 目录").Chosen() {
						s.pickJava(true)
					}
					if menu.Item("选择可执行文件").Chosen() {
						s.pickJava(false)
					}
				})
				iconButton(c, "refresh-java", "重新发现本机 Java", iconRefresh, s.refreshJava).Disabled(j.editing)
			}
		})
		s.javaTaskCard(c)
		if j.downloads {
			s.javaDownloadList(c)
		} else {
			s.javaInventoryList(c)
		}
	})
}

func (s *Shell) javaInventoryList(c *ui.Context) {
	j, t := &s.java, s.theme
	if j.scanning || j.editing {
		ui.Row(c.Key("java-discovery-state")).FillWidth().Height(18).Gap(7).Children(func() {
			busyIndicator(c, 12)
			label := "正在发现 Java…"
			if j.editing {
				label = "正在检查 Java…"
			}
			ui.Text(c, label).FontSize(11).TextColor(colors(t).Metadata)
		})
	}
	if len(j.inventory.Warnings) > 0 {
		ui.Textf(c, "%d 个路径无法读取或运行", len(j.inventory.Warnings)).FontSize(11).TextColor(t.Warning).MaxLines(1)
	}
	ui.Scroll(c.Key("java-inventory-scroll")).Grow(1).MinHeight(0).FillWidth().Children(func() {
		if len(j.inventory.Environments) == 0 && !j.scanning {
			ui.Column(c).FillWidth().Padding(32, 8).Center().Gap(8).Children(func() {
				ui.Icon(c, iconSettingsRuntime).Size(24, 24).TextColor(colors(t).IconMuted)
				ui.Text(c, "尚未发现 Java").FontSize(12).TextColor(t.TextMuted)
			})
			return
		}
		if len(j.inventory.Environments) == 0 {
			return
		}
		ui.Column(c.Key("java-environments")).FillWidth().Radius(10).Border(0.5, colors(t).ListDivider).Clip().Children(func() {
			for index, environment := range j.inventory.Environments {
				s.javaEnvironmentRow(c, environment)
				if index < len(j.inventory.Environments)-1 {
					ui.Box(c).FillWidth().Height(0.5).Margin(0, 12).Background(colors(t).ListDivider)
				}
			}
		})
	})
}

func (s *Shell) javaEnvironmentRow(c *ui.Context, value app.JavaEnvironment) {
	r, t := value.Runtime, s.theme
	active := r.Executable == s.settings.JavaPath
	services := c.Services()
	ui.Row(c.Key(r.Executable)).FillWidth().Height(42).Shrink(0).Padding(0, 12).Gap(10).Children(func() {
		color := colors(t).Metadata
		if active {
			color = colors(t).SettingsNavAccent
		}
		ui.Icon(c, iconSettingsRuntime).Size(16, 16).Shrink(0).TextColor(color)
		ui.Text(c, "Java "+r.Version).FontSize(12).MaxLines(1).Ellipsis("…").Shrink(0)
		marketTag(c, value.Kind, false)
		marketTag(c, javaArchLabel(r.Arch), false)
		if active {
			ui.Icon(c, iconCheck).Label("全局默认 Java").Size(13, 13).Shrink(0).TextColor(colors(t).AccentInk)
		}
		ui.Text(c, compactJavaPath(r.Home())).FontSize(11).TextColor(colors(t).Metadata).Grow(1).MinWidth(0).MaxLines(1).Ellipsis("…").TextAlign(ui.End)
		ui.ButtonBase(c.Key("java-options")).Label("Java "+r.Version+" 的操作").Tooltip("更多操作").Size(26, 26).Shrink(0).Radius(5).Center().Disabled(s.java.editing).Menu(func(menu *ui.Menu) {
			if menu.Item("设为全局默认").Disabled(active || s.saving || s.loadErr != nil).Chosen() {
				s.defaultJava(value)
			}
			if menu.Item("打开所在文件夹").Chosen() && s.java.actions.Open != nil {
				s.java.actions.Open(r.Home(), func(err error) {
					if err != nil {
						s.java.message, s.java.failed = err.Error(), true
					}
				})
			}
			if menu.Item("复制 Java 路径").Chosen() {
				services.WriteClipboard(r.Executable)
			}
			menu.Separator()
			if menu.Item("从列表移除").Chosen() {
				s.removeJava(value)
			}
		}).Children(func() { ui.Text(c, "···").FontSize(15).FontWeight(600) })
	})
}

func (s *Shell) javaVersionMenu(c *ui.Context) {
	j := &s.java
	ui.Button(c.Key("java-download-major"), fmt.Sprintf("Java %d", j.major)).Label("Java 主版本").Width(110).Height(26).Menu(func(menu *ui.Menu) {
		majors := j.catalog.Majors
		if len(majors) == 0 {
			majors = []int{25, 21, 17, 11, 8}
		}
		for _, major := range majors {
			if menu.Item(fmt.Sprintf("Java %d", major)).Checked(j.major == major).Chosen() && j.major != major {
				j.major = major
				s.loadJavaCatalog()
			}
		}
	})
}

func (s *Shell) javaDownloadList(c *ui.Context) {
	j, t := &s.java, s.theme
	if j.loading {
		loadingStatus(c, "读取 Java 官方目录")
		return
	}
	ui.Scroll(c.Key("java-download-scroll")).Grow(1).MinHeight(0).FillWidth().Gap(8).Children(func() {
		if len(j.catalog.Packages) == 0 {
			if j.message == "" {
				ui.Text(c, "此版本暂无适合本机的发行包").FontSize(12).TextColor(t.TextMuted).Margin(20, 0)
			}
			return
		}
		ui.Column(c).FillWidth().Radius(10).Border(0.5, colors(t).ListDivider).Clip().Children(func() {
			for index, p := range j.catalog.Packages {
				ui.Row(c.Key(p.ID())).FillWidth().Height(52).Shrink(0).Padding(0, 12).Gap(10).Children(func() {
					ui.Icon(c, iconSettingsRuntime).Size(16, 16).Shrink(0).TextColor(colors(t).Metadata)
					ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
						ui.Row(c).Gap(6).Children(func() {
							ui.Text(c, p.Name).FontSize(12).MaxLines(1).Ellipsis("…").MinWidth(0)
							marketTag(c, strings.ToUpper(p.Kind), false)
						})
						ui.Text(c, p.Version).FontSize(10).TextColor(colors(t).Metadata).MaxLines(1).Ellipsis("…")
					})
					ui.Text(c, formatBytes(p.Size)).FontSize(11).TextColor(colors(t).Metadata).Shrink(0)
					label := "安装"
					if s.javaPackageInstalled(p) {
						label = "已安装"
					}
					ui.Button(c.Key("install-java"), label).Height(26).Disabled(j.job.active || s.javaPackageInstalled(p)).OnClick(func() { s.installJava(p) })
				})
				if index < len(j.catalog.Packages)-1 {
					ui.Box(c).FillWidth().Height(0.5).Margin(0, 12).Background(colors(t).ListDivider)
				}
			}
		})
	})
}

func (s *Shell) javaPackageInstalled(p java.Package) bool {
	return slices.ContainsFunc(s.java.inventory.Environments, func(environment app.JavaEnvironment) bool {
		return environment.Managed && strings.Contains(environment.Runtime.Executable, p.ID())
	})
}

func (s *Shell) javaTaskCard(c *ui.Context) {
	job, t := &s.java.job, s.theme
	paused := s.javaDownloadPaused(job.packageID)
	if job.packageID == "" {
		return
	}
	ui.Column(c.Key("java-install-task")).FillWidth().Shrink(0).Padding(10, 12).Gap(7).Radius(8).Background(t.Surface).Children(func() {
		ui.Row(c).FillWidth().Gap(8).Children(func() {
			ui.Text(c, job.title).FontSize(12).Grow(1).MinWidth(0).MaxLines(1).Ellipsis("…")
			if job.active {
				if s.page == settingsPage {
					iconButton(c, "cancel-java-install", "取消安装 Java", iconClose, s.cancelJavaInstall).Disabled(job.canceling)
				} else {
					ui.Button(c.Key("view-java-install"), "查看").Height(24).OnClick(func() { s.page = settingsPage })
				}
			} else {
				iconButton(c, "dismiss-java-job", "关闭 Java 任务", iconClose, func() { s.java.job = javaJob{} })
			}
		})
		label := job.message
		if job.active {
			label = job.progress.Stage
			if paused {
				label = "已暂停"
			}
			if job.canceling {
				label = "正在取消…"
			}
			ui.Row(c).FillWidth().Gap(7).Children(func() {
				if !paused {
					busyIndicator(c, 11)
				}
				ui.Text(c, label).FontSize(11).TextColor(t.TextMuted)
				ui.Spacer(c)
				if job.progress.Total > 0 {
					ui.Text(c, formatBytes(job.progress.Bytes)+" / "+formatBytes(job.progress.Total)).FontSize(10).TextColor(colors(t).Metadata)
				}
			})
			if job.progress.Total > 0 && job.progress.Bytes < job.progress.Total {
				smoothProgress(c, float64(job.progress.Bytes)/float64(job.progress.Total))
			}
		} else {
			color := t.TextMuted
			if job.failed {
				color = t.Danger
			}
			ui.Text(c, label).FontSize(11).TextColor(color).MaxLines(3)
		}
	})
}

func javaArchLabel(arch string) string {
	switch arch {
	case "amd64":
		return "x64"
	case "arm64":
		return "ARM64"
	case "386":
		return "x86"
	default:
		return arch
	}
}

func compactJavaPath(path string) string {
	if len([]rune(path)) > 44 {
		return "…" + string([]rune(path)[len([]rune(path))-43:])
	}
	return path
}
