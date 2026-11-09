package ui

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/egoist/mygo/ui"

	"mcl/internal/app"
	"mcl/internal/core/download"
	"mcl/internal/core/install"
	"mcl/internal/core/instance"
	"mcl/internal/core/market"
)

func (s *Shell) inspectMarketArchive() {
	i := s.market.install
	if i == nil || i.busy || s.market.actions.InspectArchive == nil {
		return
	}
	i.busy, i.canceling, i.err = true, false, ""
	i.installing = false
	i.progress = download.Progress{}
	request := app.ArchiveImportRequest{Source: i.project.Source, Kind: i.project.Kind, ProjectID: i.project.ID, VersionID: i.version.ID, InstanceID: i.request.InstanceID, Name: strings.TrimSpace(i.archiveName), World: strings.TrimSpace(i.request.World)}
	request.LocalArchive = i.localArchive
	if i.archiveEdited {
		description := i.archiveDescription
		request.Description = &description
		request.Icon = slices.Clone(i.archiveIcon)
		request.IconStyle = i.archiveIconStyle
	}
	request.GameDirectoryID = i.archiveDirectoryID
	if i.archivePlan != nil && i.archivePlan.Request.GameDirectoryID != nil {
		id := *i.archivePlan.Request.GameDirectoryID
		request.GameDirectoryID = &id
	}
	i.cancel = s.market.actions.InspectArchive(request, s.settings, func(progress download.Progress) {
		if s.market.install == i {
			i.progress = progress
		}
	}, func(plan app.ArchiveImportPlan, err error) {
		if s.market.install != i {
			return
		}
		i.busy, i.cancel, i.canceling = false, nil, false
		if err != nil {
			i.err = err.Error()
			return
		}
		s.initializeArchiveEditor(i, plan)
		i.archivePlan = &plan
		i.archiveMissing = plan.MissingRequired()
		i.supplyNote = ""
		oldOptional := i.optional
		i.optional = make(map[string]bool)
		for _, file := range plan.Files {
			if file.Optional && oldOptional[file.Path] {
				i.optional[file.Path] = true
			}
		}
		i.archiveList = ui.ListState{}
	})
}

func (s *Shell) supplyMarketArchive() {
	i := s.market.install
	if i == nil || i.busy || i.archivePlan == nil || s.market.actions.SupplyArchive == nil {
		return
	}
	i.busy, i.supplying, i.installing, i.canceling, i.err = true, true, false, false, ""
	plan := *i.archivePlan
	i.cancel = s.market.actions.SupplyArchive(plan, func(result app.LocalFileSupply, err error) {
		if s.market.install != i {
			return
		}
		i.busy, i.supplying, i.canceling, i.cancel = false, false, false, nil
		if plan.NeedsArchive && err == nil && len(result.Matched) > 0 {
			s.inspectMarketArchive()
			return
		}
		if len(result.Matched) > 0 {
			plan.Supplied = append(slices.Clone(plan.Supplied), result.Matched...)
			slices.Sort(plan.Supplied)
			plan.Supplied = slices.Compact(plan.Supplied)
			i.archivePlan, i.archiveMissing = &plan, plan.MissingRequired()
		}
		if len(result.Matched) > 0 || result.Skipped > 0 {
			i.supplyNote = ""
			if len(plan.Supplied) > 0 {
				i.supplyNote = fmt.Sprintf("已匹配 %d 项", len(plan.Supplied))
			}
			if result.Skipped > 0 {
				if i.supplyNote != "" {
					i.supplyNote += " · "
				}
				i.supplyNote += fmt.Sprintf("本次跳过 %d 个文件", result.Skipped)
			}
		}
		if err != nil && !errors.Is(err, context.Canceled) {
			i.err = err.Error()
		}
	})
}
func (s *Shell) importMarketArchive() {
	i := s.market.install
	if i == nil || i.busy || i.archivePlan == nil || i.archivePlan.NeedsArchive || s.market.actions.ImportArchive == nil {
		return
	}
	plan := *i.archivePlan
	if i.archiveIconBusy {
		return
	}
	if plan.Request.Kind == market.Modpack {
		plan.Request.Name = strings.TrimSpace(i.archiveName)
		description := i.archiveDescription
		plan.Request.Description = &description
		plan.Request.Icon = slices.Clone(i.archiveIcon)
		plan.Request.IconStyle = i.archiveIconStyle
		plan.Instance.Name = plan.Request.Name
		plan.Instance.Description = strings.TrimSpace(description)
	}
	plan.SelectedOptional = nil
	for name, enabled := range i.optional {
		if enabled {
			plan.SelectedOptional = append(plan.SelectedOptional, name)
		}
	}
	sort.Strings(plan.SelectedOptional)
	i.busy, i.installing, i.canceling, i.err = true, true, false, ""
	i.cancel = s.market.actions.ImportArchive(plan, func(p install.Progress) {
		if s.market.install == i {
			i.progress = p.Progress
			i.archiveStage = installationStage(p.Stage)
		}
	}, func(result app.PackImportResult, err error) { s.finishPackImport(i, result, err) })
}
func (s *Shell) finishPackImport(i *marketInstallState, result app.PackImportResult, err error) {
	if s.market.install != i {
		return
	}
	i.busy, i.installing, i.cancel, i.canceling = false, false, nil, false
	value := result.Instance
	if result.Imported && value.ID != "" {
		i.archiveImported, i.archiveRuntimeReady = true, result.RuntimeReady
		i.request.InstanceID = value.ID
		if i.archivePlan != nil {
			i.archivePlan.Instance = value
		}
		if i.project.Kind == market.Modpack {
			replaced := false
			for index, row := range s.library.snapshot.Instances {
				if row.ID == value.ID {
					s.library.snapshot.Instances[index] = value
					replaced = true
					break
				}
			}
			if !replaced {
				s.library.snapshot.Instances = append([]instance.Instance{value}, s.library.snapshot.Instances...)
			}
			s.cacheInstanceIcons([]instance.Instance{value})
			s.refreshGameDirectories()
			if result.RuntimeReady {
				if s.installation.receipts == nil {
					s.installation.receipts = make(map[string]install.Receipt)
				}
				s.installation.receipts[value.ID] = result.Receipt
			}
		} else if value.ID == s.library.selectedID {
			s.refreshAssets()
		}
	}
	if err != nil {
		if errors.Is(err, context.Canceled) {
			if result.Imported {
				i.err = "实例已导入，运行环境安装已取消"
			} else {
				i.err = "已取消"
			}
		} else {
			i.err = err.Error()
		}
		if !errors.Is(err, context.Canceled) {
			s.Notify(ToastError, i.err)
		}
		return
	}
	i.complete = result.Imported && (i.project.Kind != market.Modpack || result.RuntimeReady)
	if i.project.Kind == market.Modpack {
		s.refreshJava()
	}
}
func (s *Shell) resumePackImport() {
	i := s.market.install
	if i == nil || i.busy || !i.archiveImported || i.request.InstanceID == "" || s.market.actions.ResumePackImport == nil {
		return
	}
	i.busy, i.installing, i.canceling, i.err = true, true, false, ""
	i.cancel = s.market.actions.ResumePackImport(i.request.InstanceID, func(p install.Progress) {
		if s.market.install == i {
			i.progress = p.Progress
			i.archiveStage = installationStage(p.Stage)
		}
	}, func(result app.PackImportResult, err error) { s.finishPackImport(i, result, err) })
}

func (s *Shell) archiveImportView(c *ui.Context) {
	i, t := s.market.install, s.theme
	ui.Row(c).FillWidth().Height(28).Shrink(0).Gap(8).Children(func() {
		if !i.busy {
			iconButton(c, "archive-back", "返回", iconBack, func() {
				if i.archivePlan != nil && !i.complete && i.localArchive == nil && !i.archiveImported {
					i.archivePlan = nil
					i.err = ""
				} else {
					s.market.install = nil
				}
			})
		}
		ui.Text(c, i.project.Title).FontSize(16).FontWeight(500).Grow(1).MinWidth(0).MaxLines(1).Ellipsis("…")
	})
	if i.complete {
		title := "存档已安装"
		if i.project.Kind == market.Modpack {
			title = "整合包已导入"
		}
		marketStatus(c, title, iconCheck)
		if i.project.Kind == market.Modpack {
			ui.Text(c, "运行环境已就绪").FontSize(12).TextColor(t.TextMuted)
		}
		if !i.archiveRuntimeReady && i.project.Kind == market.Modpack && len(i.archivePlan.Instance.Pack.Loaders) > 0 {
			value := i.archivePlan.Instance
			ui.Text(c, s.instanceRuntimeStatus(value)).FontSize(11).TextColor(t.TextMuted)
			if supportedPackLoader(value) {
				ui.Button(c.Key("archive-install-runtime"), "安装运行环境").Height(28).OnClick(func() {
					id := i.request.InstanceID
					s.market.install = nil
					s.selectInstance(id)
					s.startInstallation()
				})
			}
		}
		ui.Button(c.Key("archive-open-instance"), "查看实例").Height(28).OnClick(func() { id := i.request.InstanceID; s.market.install = nil; s.selectInstance(id) })
		return
	}
	if i.busy {
		label := "正在读取压缩包…"
		if i.supplying {
			label = "正在匹配本地文件…"
		}
		if i.installing {
			label = i.archiveStage
			if label == "" {
				label = "正在导入…"
			}
			if i.progress.TotalFiles > 0 {
				label += fmt.Sprintf(" · %d / %d", i.progress.Files, i.progress.TotalFiles)
			}
		}
		if !i.installing && i.progress.TotalBytes > 0 {
			label += " · " + formatBytes(i.progress.Bytes) + " / " + formatBytes(i.progress.TotalBytes)
		}
		s.workStatus(c, s.marketTaskPaused(i), label)
		if i.installing && i.progress.TotalFiles > 0 {
			smoothProgress(c, float64(i.progress.Files)/float64(i.progress.TotalFiles)).FillWidth().Height(4)
		}
		ui.Button(c.Key("archive-cancel"), "取消").Height(26).Disabled(i.canceling).OnClick(func() {
			i.canceling = true
			if i.cancel != nil {
				i.cancel()
			}
		})
		return
	}
	if i.archiveImported && !i.archiveRuntimeReady && i.project.Kind == market.Modpack {
		marketStatus(c, "实例已导入，运行环境未完成", iconInfo)
		ui.Row(c).Gap(8).Children(func() {
			ui.PrimaryButton(c.Key("resume-pack-runtime"), "继续安装运行环境").Height(28).Disabled(s.market.actions.ResumePackImport == nil).OnClick(s.resumePackImport)
			ui.Button(c.Key("open-imported-instance"), "查看实例").Height(28).OnClick(func() { id := i.request.InstanceID; s.market.showInstall = false; s.selectInstance(id) })
		})
		return
	}

	if i.archivePlan == nil {
		if i.project.Kind == market.Modpack {
			ui.TextInput(c.Key("archive-instance-name"), &i.archiveName).Label("新实例名称").Placeholder("实例名称").Height(30).FillWidth()
			ui.Row(c.Key("archive-location-choice")).FillWidth().Height(40).Gap(8).Children(func() {
				ui.Text(c, "游戏目录").FontSize(12).Width(84)
				selected := s.settings.GameDirectoryID
				if i.archiveDirectoryID != nil {
					selected = *i.archiveDirectoryID
				}
				s.gameDirectoryMenu(c, "archive-game-directory", selected, false, func(id string) { i.archiveDirectoryID = &id })
			})
		} else {
			name := "选择实例"
			for _, value := range s.library.snapshot.Instances {
				if value.ID == i.request.InstanceID {
					name = value.Name
				}
			}
			ui.Button(c.Key("archive-target"), name).Label("地图目标实例").Height(28).FillWidth().Menu(func(menu *ui.Menu) {
				for _, value := range s.library.snapshot.Instances {
					if menu.Item(value.Name).Chosen() {
						i.request.InstanceID = value.ID
					}
				}
			})
			ui.TextInput(c.Key("archive-world-name"), &i.request.World).Label("新存档名称").Placeholder("存档名称").Height(30).FillWidth()
		}
		ui.Spacer(c)
		ui.PrimaryButton(c.Key("archive-inspect"), "继续").Height(30).OnClick(s.inspectMarketArchive)
		return
	}
	plan := i.archivePlan
	if plan.Request.Kind == market.Modpack && !plan.NeedsArchive {
		s.archivePreferencesView(c, i)
	}
	if plan.NeedsArchive {
		ui.Text(c, plan.Archive.Filename).FontSize(12).MaxLines(1).Ellipsis("…")
		if i.supplyNote != "" {
			ui.Text(c, i.supplyNote).FontSize(11).TextColor(t.Warning)
		}
		services := c.Services()
		page := plan.ArchivePage
		if page == "" {
			page = i.project.PageURL
		}
		ui.Row(c).FillWidth().Gap(8).Children(func() {
			if market.SafeURL(page) {
				ui.Button(c.Key("archive-local-page"), "项目页").Height(26).OnClick(func() { services.OpenURL(page) })
			}
			ui.PrimaryButton(c.Key("archive-local-select"), "选择本地压缩包").Height(28).
				Disabled(s.market.actions.SupplyArchive == nil).OnClick(s.supplyMarketArchive)
		})
		return
	}
	if plan.Request.Kind == market.World {
		ui.Textf(c, "%s · %d 个文件", plan.Request.World, len(plan.WorldFiles)).FontSize(12)
		ui.Spacer(c)
		ui.PrimaryButton(c.Key("archive-confirm"), "安装地图").Height(30).OnClick(s.importMarketArchive)
		return
	}
	for _, warning := range plan.Warnings {
		ui.Text(c, warning).FontSize(11).TextColor(t.Warning).MaxLines(2)
	}
	ui.Row(c).FillWidth().Gap(8).Children(func() {
		if len(plan.Files) > 0 && s.market.actions.SupplyArchive != nil {
			ui.Button(c.Key("archive-supply"), "从本地补齐").Height(25).OnClick(s.supplyMarketArchive)
		}
		if len(plan.Missing) > 0 {
			ui.Button(c.Key("archive-reread"), "重读文件信息").Height(25).OnClick(s.inspectMarketArchive)
		}
		if i.archiveMissing > 0 {
			ui.Textf(c, "%d 项待补齐", i.archiveMissing).FontSize(11).TextColor(t.Warning)
		}
		if i.supplyNote != "" {
			ui.Text(c, i.supplyNote).FontSize(11).TextColor(t.TextMuted).MaxLines(1).Ellipsis("…")
		}
	})
	supplied := make(map[string]bool, len(plan.Supplied))
	for _, name := range plan.Supplied {
		supplied[name] = true
	}
	services := c.Services()
	files := plan.Files
	i.archiveList.Key = func(index int) any {
		if index < len(files) {
			return files[index].Path
		}
		if index < len(files)+len(plan.Manifest.Overrides) {
			return "override:" + plan.Manifest.Overrides[index-len(files)].Target
		}
		return fmt.Sprintf("missing:%d", index)
	}
	ui.List(c.Key("archive-file-list"), &i.archiveList, len(files)+len(plan.Manifest.Overrides)+len(plan.Missing), func(index int) {
		ui.Row(c).FillWidth().Height(32).Gap(8).Children(func() {
			if index < len(files) {
				f := files[index]
				pending := f.Size > 0 && len(f.URLs) == 0 && !supplied[f.Path]
				if f.Optional {
					enabled := i.optional[f.Path]
					ui.Checkbox(c.Key(f.Path), &enabled, "").Label("安装可选文件 " + f.Path).Disabled(pending).OnClick(func() { i.optional[f.Path] = enabled })
				} else {
					ui.Icon(c, iconFile).Size(13, 13).TextColor(colors(t).Icon)
				}
				ui.Text(c, f.Path).FontSize(11).Grow(1).MinWidth(0).MaxLines(1).Ellipsis("…")
				if supplied[f.Path] {
					ui.Text(c, "已匹配").FontSize(10).TextColor(t.Success)
				} else if pending {
					ui.Text(c, "需本地文件").FontSize(10).TextColor(t.Warning)
					if market.SafeURL(f.PageURL) {
						ui.Button(c.Key("archive-file-page"), "项目页").Height(22).OnClick(func() { services.OpenURL(f.PageURL) })
					}
				}
				ui.Text(c, formatBytes(f.Size)).FontSize(10).TextColor(colors(t).Metadata)
			} else if index < len(files)+len(plan.Manifest.Overrides) {
				f := plan.Manifest.Overrides[index-len(files)]
				ui.Icon(c, iconFolder).Size(13, 13).TextColor(colors(t).Icon)
				ui.Text(c, f.Target).FontSize(11).Grow(1).MinWidth(0).MaxLines(1).Ellipsis("…")
			} else {
				missing := plan.Missing[index-len(files)-len(plan.Manifest.Overrides)]
				label := "缺少可核验信息"
				if missing.Optional {
					label = "可选 · " + label
				}
				ui.Text(c, missing.ProjectID+" / "+missing.VersionID+" · "+label).FontSize(11).TextColor(t.Warning)
			}
		})
	}).Grow(1).MinHeight(0).FillWidth()
	ui.PrimaryButton(c.Key("archive-confirm"), "导入并安装").Height(30).Disabled(i.archiveMissing > 0 || i.archiveIconBusy || instance.ValidateName(strings.TrimSpace(i.archiveName)) != nil || instance.ValidateDescription(i.archiveDescription) != nil || !s.gameDirectoryAvailable(plan.Instance.GameDirectoryID)).OnClick(s.importMarketArchive)
}
