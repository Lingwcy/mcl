package ui

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/egoist/mygo/ui"

	"mcl/internal/app"
	"mcl/internal/core/download"
	"mcl/internal/core/market"
)

type marketInstallState struct {
	archiveEdited                         bool
	archiveImported, archiveRuntimeReady  bool
	archiveStage                          string
	archiveDescription, archiveIconStyle  string
	archiveIcon                           []byte
	archiveBitmap                         *ui.Bitmap
	archiveIconOpen, archiveIconBusy      bool
	archiveIconCancel                     func()
	archiveIconGeneration                 uint64
	archiveIconChosen                     bool
	archiveIconURL                        string
	archiveDirectoryID                    *string
	project                               market.Project
	version                               market.Version
	request                               market.InstallRequest
	plan                                  *market.InstallPlan
	busy, installing, canceling, complete bool
	err                                   string
	worlds                                []string
	worldGeneration                       uint64
	progress                              download.Progress
	cancel                                func()
	archiveName                           string
	archivePlan                           *app.ArchiveImportPlan
	optional                              map[string]bool
	archiveList                           ui.ListState
	supplying                             bool
	archiveMissing                        int
	supplyNote                            string
	localArchive                          *market.File
}

func (i *marketInstallState) hasTask() bool {
	return i != nil && (i.busy || i.installing || i.complete || i.err != "" || i.supplyNote != "")
}

func (s *Shell) selectMarketInstall(project market.Project, version market.Version) {
	m := &s.market
	if m.install != nil && m.install.busy {
		m.message = "已有资产安装任务"
		return
	}
	if m.install != nil {
		s.cancelArchiveIcon(m.install)
	}
	if m.detail != nil && m.detail.Project.ID == project.ID && m.detail.Project.Source == project.Source && project.IconURL == "" {
		project.IconURL = m.detail.Project.IconURL
	}
	s.closeMarketVersion()
	id := m.targetID
	if id == "" {
		id = s.library.selectedID
	}
	loader := m.singleLoader()
	for _, value := range s.library.snapshot.Instances {
		if value.ID == id && project.Kind == market.Mod {
			if required, ok := value.RequiredLoader(); ok {
				loader = required.Type
			}
		}
	}
	if loader == "" && project.Kind == market.Mod && len(version.Loaders) == 1 {
		loader = version.Loaders[0]
	}
	m.install = &marketInstallState{project: project, version: version, request: market.InstallRequest{Source: project.Source, Kind: project.Kind, ProjectID: project.ID, VersionID: version.ID, InstanceID: id, Loader: loader}}
	m.showInstall, m.showKey, m.message = true, false, ""
	m.install.archiveName = project.Title
	if project.Kind == market.Modpack && m.importDirectoryID != nil {
		id := *m.importDirectoryID
		m.install.archiveDirectoryID = &id
	}
	if project.Kind == market.World {
		m.install.request.World = project.Slug
		if m.install.request.World == "" {
			m.install.request.World = "World-" + project.ID
		}
	}
	if project.Kind == market.DataPack {
		s.loadInstallWorlds()
	}
	if project.Kind == market.Modpack {
		m.install.archiveName = archiveInitialName(project.Title)
		directory := s.settings.GameDirectoryID
		if m.install.archiveDirectoryID != nil {
			directory = *m.install.archiveDirectoryID
		}
		m.install.archiveDirectoryID = &directory
		s.loadArchiveProjectIcon(m.install)
		s.inspectMarketArchive()
	}
}
func (s *Shell) loadInstallWorlds() {
	i := s.market.install
	if i == nil || s.market.actions.Worlds == nil || i.request.InstanceID == "" {
		return
	}
	i.worldGeneration++
	generation := i.worldGeneration
	i.worlds = nil
	i.request.World = ""
	s.market.actions.Worlds(i.request.InstanceID, func(worlds []string, err error) {
		if i != s.market.install || generation != i.worldGeneration {
			return
		}
		if err != nil {
			i.err = err.Error()
			return
		}
		i.worlds = worlds
		if len(worlds) > 0 {
			i.request.World = worlds[0]
		}
	})
}
func (s *Shell) planMarketInstall() {
	i := s.market.install
	if i == nil || i.busy || s.market.actions.Plan == nil {
		return
	}
	i.busy, i.canceling, i.installing, i.err = true, false, false, ""
	i.supplyNote = ""
	i.cancel = s.market.actions.Plan(i.request, func(plan market.InstallPlan, err error) {
		if s.market.install != i {
			return
		}
		i.busy, i.cancel = false, nil
		i.canceling = false
		if err != nil {
			i.err = err.Error()
			return
		}
		i.plan = &plan
	})
}

func (s *Shell) supplyMarketFiles() {
	i := s.market.install
	if i == nil || i.busy || i.plan == nil || s.market.actions.SupplyFiles == nil {
		return
	}
	i.busy, i.supplying, i.canceling, i.err = true, true, false, ""
	i.supplyNote = ""
	i.cancel = s.market.actions.SupplyFiles(*i.plan, func(result app.LocalFileSupply, err error) {
		if s.market.install != i {
			return
		}
		i.busy, i.supplying, i.canceling, i.cancel = false, false, false, nil
		if len(result.Matched) > 0 {
			plan := *i.plan
			plan.ReadyLocal = slices.Concat(plan.ReadyLocal, result.Matched)
			slices.Sort(plan.ReadyLocal)
			plan.ReadyLocal = slices.Compact(plan.ReadyLocal)
			i.plan = &plan
		}
		if len(result.Matched) > 0 || result.Skipped > 0 {
			i.supplyNote = fmt.Sprintf("匹配 %d 项", len(result.Matched))
			if result.Skipped > 0 {
				i.supplyNote += fmt.Sprintf(" · %d 个文件未匹配", result.Skipped)
			}
		}
		if errors.Is(err, context.Canceled) {
			i.err = "已取消"
		} else if err != nil {
			i.err = err.Error()
		}
	})
}

func (s *Shell) runMarketInstall() {
	i := s.market.install
	if i == nil || i.busy || i.plan == nil || i.plan.MissingLocal() > 0 || s.market.actions.Install == nil {
		return
	}
	i.busy, i.installing, i.canceling, i.err = true, true, false, ""
	i.progress = download.Progress{}
	i.cancel = s.market.actions.Install(*i.plan, func(progress download.Progress) {
		if s.market.install == i {
			i.progress = progress
		}
	}, func(err error) {
		if s.market.install != i {
			return
		}
		i.busy, i.installing, i.canceling, i.cancel = false, false, false, nil
		if err != nil {
			var missing *app.MissingMarketFilesError
			if errors.As(err, &missing) {
				plan := *i.plan
				plan.ReadyLocal = missing.Ready
				i.plan = &plan
			}
			if errors.Is(err, context.Canceled) {
				i.err = "已取消"
			} else {
				i.err = err.Error()
			}
			return
		}
		i.complete = true
		if i.request.InstanceID == s.library.selectedID {
			s.refreshAssets()
		}
	})
}
func (s *Shell) marketInstallView(c *ui.Context) {
	i, t := s.market.install, s.theme
	if i.project.Kind == market.Modpack || i.project.Kind == market.World {
		s.archiveImportView(c)
		return
	}
	ui.Row(c).FillWidth().Height(28).Shrink(0).Gap(10).Children(func() {
		if !i.busy {
			iconButton(c, "install-back", "返回版本", iconBack, func() { s.market.install = nil })
		}
		ui.Text(c, i.project.Title).FontSize(16).FontWeight(500).Grow(1).MinWidth(0).MaxLines(1).Ellipsis("…")
	})
	ui.Text(c, i.version.Name).FontSize(12).TextColor(t.TextMuted).MaxLines(2)
	if i.complete {
		marketStatus(c, "文件已安装", iconCheck)
		ui.Button(c.Key("install-open-instance"), "查看实例").Height(28).OnClick(func() { id := i.request.InstanceID; s.market.install = nil; s.selectInstance(id) })
		return
	}
	if i.busy {
		label := "正在解析依赖…"
		switch {
		case i.supplying:
			label = "正在核验本地文件…"
		case i.installing && i.progress.TotalFiles > i.progress.Files:
			label = fmt.Sprintf("%d / %d · %s", i.progress.Files, i.progress.TotalFiles, formatBytes(i.progress.Bytes))
		case i.installing:
			label = "正在检查并写入实例…"
		}
		s.workStatus(c, s.marketTaskPaused(i), label)
		if i.installing && i.progress.TotalFiles > 0 {
			smoothProgress(c, float64(i.progress.Files)/float64(i.progress.TotalFiles)).FillWidth().Height(4)
		}
		ui.Button(c.Key("cancel-market-install"), "取消").Height(26).Disabled(i.canceling).OnClick(func() {
			i.canceling = true
			if i.cancel != nil {
				i.cancel()
			}
		})
		return
	}
	if i.err != "" {
		ui.Text(c, i.err).FontSize(12).TextColor(t.Danger).MaxLines(4)
	}
	if i.plan == nil {
		ui.Text(c, "目标实例").FontSize(11).TextColor(t.TextMuted)
		name := "选择实例"
		for _, v := range s.library.snapshot.Instances {
			if v.ID == i.request.InstanceID {
				name = v.Name + " · " + v.Version.ID
			}
		}
		ui.Button(c.Key("install-target"), name).Height(28).FillWidth().Menu(func(m *ui.Menu) {
			for _, v := range s.library.snapshot.Instances {
				if m.Item(v.Name + " · " + v.Version.ID).Chosen() {
					i.request.InstanceID = v.ID
					if required, ok := v.RequiredLoader(); ok && i.project.Kind == market.Mod {
						i.request.Loader = required.Type
					}
					i.err = ""
					if i.project.Kind == market.DataPack {
						s.loadInstallWorlds()
					}
				}
			}
		})
		if i.project.Kind == market.Mod {
			loader := i.request.Loader
			if loader == "" {
				loader = "选择加载器"
			}
			ui.Button(c.Key("install-loader"), loader).Height(28).Width(180).Menu(func(m *ui.Menu) {
				for _, loader := range i.version.Loaders {
					if m.Item(loader).Checked(i.request.Loader == loader).Chosen() {
						i.request.Loader = loader
					}
				}
			})
			for _, value := range s.library.snapshot.Instances {
				if value.ID != i.request.InstanceID {
					continue
				}
				if required, ok := value.RequiredLoader(); ok {
					ui.Text(c, required.Type+" "+required.Version).FontSize(11).TextColor(t.TextMuted)
				} else {
					ui.Text(c, "需要模组加载器").FontSize(11).TextColor(t.Warning)
				}
			}
		}
		if i.project.Kind == market.DataPack {
			world := i.request.World
			if world == "" {
				world = "选择存档"
			}
			ui.Button(c.Key("install-world"), world).Height(28).FillWidth().Menu(func(m *ui.Menu) {
				for _, world := range i.worlds {
					if m.Item(world).Chosen() {
						i.request.World = world
					}
				}
			})
		}
		ui.Spacer(c)
		ui.PrimaryButton(c.Key("prepare-market-install"), "继续").Height(30).Disabled(i.request.InstanceID == "").OnClick(s.planMarketInstall)
		return
	}
	for _, warning := range i.plan.Warnings {
		ui.Text(c, warning).FontSize(11).TextColor(t.Warning).MaxLines(2)
	}
	missing := i.plan.MissingLocal()
	hasManual := slices.ContainsFunc(i.plan.Files, func(file market.InstallFile) bool { return file.File.URL == "" })
	if hasManual {
		ui.Row(c).FillWidth().Height(28).Gap(8).Children(func() {
			if missing > 0 {
				ui.Textf(c, "%d 项待补齐", missing).FontSize(11).TextColor(t.Warning)
			}
			if i.supplyNote != "" {
				ui.Text(c, i.supplyNote).FontSize(11).TextColor(t.TextMuted)
			}
			ui.Spacer(c)
			if s.market.actions.SupplyFiles != nil {
				ui.Button(c.Key("market-supply"), "从本地补齐").Height(25).OnClick(s.supplyMarketFiles)
			}
		})
	}
	services := c.Services()
	ui.Scroll(c.Key("market-install-files")).Grow(1).MinHeight(0).FillWidth().Gap(8).Children(func() {
		for _, f := range i.plan.Files {
			ui.Row(c.Key(f.Path)).FillWidth().Gap(10).Children(func() {
				ui.Icon(c, iconFile).Size(15, 15).TextColor(colors(t).Icon)
				ui.Text(c, f.File.Filename).FontSize(12).Grow(1).MinWidth(0).MaxLines(1).Ellipsis("…")
				ui.Text(c, formatBytes(f.File.Size)).FontSize(10).TextColor(colors(t).Metadata)
				if f.File.URL == "" {
					if slices.Contains(i.plan.ReadyLocal, f.Path) {
						ui.Text(c, "已核验").FontSize(10).TextColor(colors(t).Metadata)
					} else if page := i.plan.LocalPages[f.Path]; market.SafeURL(page) {
						ui.Button(c.Key("market-supply-project"), "项目页").Height(22).OnClick(func() { services.OpenURL(page) })
					} else {
						ui.Text(c, "待补齐").FontSize(10).TextColor(t.Warning)
					}
				}
			})
		}
	})
	ui.PrimaryButton(c.Key("confirm-market-install"), fmt.Sprintf("安装 %d 个文件", len(i.plan.Files))).Height(30).
		Disabled(missing > 0 || s.market.actions.Install == nil).OnClick(s.runMarketInstall)
}

func (s *Shell) marketTaskCard(c *ui.Context) {
	i := s.market.install
	if !i.hasTask() {
		return
	}
	paused := s.marketTaskPaused(i)
	section(c, i.project.Title, func() {
		ui.Row(c).FillWidth().Gap(8).Children(func() {
			status := "资产安装"
			switch {
			case i.supplying:
				status = "正在核验本地文件"
			case i.complete:
				status = "文件已安装"
			case !i.busy && i.err != "":
				status = i.err
			case !i.busy && i.supplyNote != "":
				status = i.supplyNote
			}
			if paused {
				status = "已暂停"
			} else if i.busy {
				busyIndicator(c, 13)
			}
			ui.Text(c.Key("market-task-status"), status).FontSize(12).Grow(1).MaxLines(2).ContentTransition(ui.ContentTransition{Animation: &motionFade})
			ui.Button(c.Key("show-market-task"), "查看").Height(26).OnClick(func() { s.page = marketPage; s.market.showInstall = true; s.market.showKey = false })
			if i.busy {
				ui.Button(c.Key("cancel-market-task"), "取消").Height(26).Disabled(i.canceling).OnClick(func() {
					i.canceling = true
					if i.cancel != nil {
						i.cancel()
					}
				})
			}
		})
		if i.busy && i.progress.TotalFiles > 0 {
			smoothProgress(c, float64(i.progress.Files)/float64(i.progress.TotalFiles)).FillWidth().Height(4)
		}
	})
}
