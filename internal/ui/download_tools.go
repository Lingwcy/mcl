package ui

import (
	"context"
	"errors"
	"fmt"

	"github.com/egoist/mygo/ui"

	"mcl/internal/core/download"
)

type DownloadActions struct {
	// Directory resolves a configured parent without touching the filesystem.
	Directory func(string) string
	Pick      func(func(string, error)) func()
	Inspect   func(func(download.CacheInfo, error)) func()
	Clear     func(download.CacheInfo, func(download.CacheInfo, error)) func()
	Check     func(download.SourceMode, func([]download.ConnectivityResult, error)) func()
	Open      func(func(error)) func()
}
type downloadToolsState struct {
	actions            DownloadActions
	operation, message string
	failed, closed     bool
	generation         uint64
	cancel             func()
	info               download.CacheInfo
	results            []download.ConnectivityResult
}

func (s *Shell) ConnectDownloads(actions DownloadActions) {
	s.downloadTools.actions = actions
	s.refreshDownloadCacheUsage()
}

// Called by navigation and committed preference changes, never by rendering.
func (s *Shell) refreshDownloadCacheUsage() {
	if s.page == downloadSettingsPage && !s.downloadTools.closed {
		s.inspectDownloadCache()
	}
}
func (s *Shell) cancelDownloadTools() {
	d := &s.downloadTools
	if d.cancel != nil {
		d.cancel()
		d.cancel = nil
	}
	d.generation++
	d.operation = ""
}
func (s *Shell) downloadPreferencesCommitted() {
	clearing := s.downloadTools.operation == "clear"
	s.cancelDownloadTools()
	d := &s.downloadTools
	d.info = download.CacheInfo{}
	d.results = nil
	d.message = ""
	if clearing {
		s.Notify(ToastWarning, "清理已中断，可能已处理部分缓存")
	}

	d.failed = false
	s.refreshDownloadCacheUsage()
}
func (s *Shell) beginDownloadTool(operation string) uint64 {
	s.cancelDownloadTools()
	d := &s.downloadTools
	d.operation = operation
	d.message = ""
	d.failed = false
	if operation == "check" {
		d.results = nil
	}
	return d.generation
}
func (s *Shell) finishDownloadTool(generation uint64, err error) bool {
	d := &s.downloadTools
	if d.closed || generation != d.generation {
		return false
	}
	d.operation = ""
	d.cancel = nil
	d.failed = err != nil && !errors.Is(err, context.Canceled)
	if d.failed {
		d.message = err.Error()
	}

	return err == nil
}
func (s *Shell) pickDownloadCache() {
	d := &s.downloadTools
	if d.operation != "" || d.actions.Pick == nil {
		return
	}
	gen := s.beginDownloadTool("pick")
	d.cancel = d.actions.Pick(func(directory string, err error) {
		if s.finishDownloadTool(gen, err) {
			s.downloadSettings.cacheDirectory = directory
			s.saveSettings()
		}
	})
}
func (s *Shell) inspectDownloadCache() {
	d := &s.downloadTools
	if d.closed || d.operation != "" || d.actions.Inspect == nil {
		return
	}
	gen := s.beginDownloadTool("inspect")
	d.cancel = d.actions.Inspect(func(info download.CacheInfo, err error) {
		if s.page != downloadSettingsPage {
			return
		}
		if s.finishDownloadTool(gen, err) {
			d.info = info
			d.message = fmt.Sprintf("%d 个下载副本 · %s", info.Files, formatBytes(info.Bytes))
			if info.TemporaryFiles > 0 {
				d.message += fmt.Sprintf(" · %d 个临时项（%s）", info.TemporaryFiles, formatBytes(info.TemporaryBytes))
			}
			if info.Skipped > 0 {
				s.Notify(ToastWarning, fmt.Sprintf("已保留 %d 个非缓存条目", info.Skipped))
			}
		}
	})
}
func (s *Shell) clearDownloadCache() {
	d := &s.downloadTools
	if d.operation != "" || d.actions.Clear == nil || d.actions.Inspect == nil {
		return
	}
	gen := s.beginDownloadTool("inspect-clear")
	// One explicit action: inspect the selected directory, then confirm the
	// same snapshot with the existing protected native clear action.
	d.cancel = d.actions.Inspect(func(info download.CacheInfo, err error) {
		if gen != d.generation {
			return
		}
		if err != nil {
			s.finishDownloadTool(gen, err)
			return
		}
		d.operation = "clear"
		d.info = download.CacheInfo{}
		d.cancel = d.actions.Clear(info, func(cleared download.CacheInfo, err error) {
			if gen != d.generation {
				return
			}
			if s.finishDownloadTool(gen, err) {
				d.info = download.CacheInfo{Directory: cleared.Directory, Skipped: cleared.Skipped}
				s.Notify(ToastSuccess, "已清理下载缓存")
				if cleared.Skipped > 0 {
					s.Notify(ToastWarning, fmt.Sprintf("已保留 %d 个非缓存条目", cleared.Skipped))
				}
			} else if err != nil && !errors.Is(err, context.Canceled) {
				d.message = "清理未完成，可能已处理部分缓存。" + d.message
				s.Notify(ToastError, d.message)
			}
			s.refreshDownloadCacheUsage()
		})
	})
}
func (s *Shell) checkDownloadConnectivity() {
	d := &s.downloadTools
	if d.operation != "" || d.actions.Check == nil {
		return
	}
	gen := s.beginDownloadTool("check")
	d.cancel = d.actions.Check(s.settings.Download.Source, func(results []download.ConnectivityResult, err error) {
		if s.finishDownloadTool(gen, err) {
			d.results = append([]download.ConnectivityResult(nil), results...)
			failed := 0
			for _, result := range results {
				if result.Failure != "" {
					failed++
				}
			}
			if failed > 0 {
				s.Notify(ToastWarning, fmt.Sprintf("%d 项服务未能连接", failed))
			}
		}
	})
}
func (s *Shell) openDownloadCache() {
	d := &s.downloadTools
	if d.operation != "" || d.actions.Open == nil {
		return
	}
	gen := s.beginDownloadTool("open")
	d.cancel = d.actions.Open(func(err error) { s.finishDownloadTool(gen, err) })
}
func (s *Shell) downloadToolsView(c *ui.Context, blocked bool) {
	t, d := s.theme, &s.downloadTools
	disabled := blocked || d.operation != "" || s.saving
	prefs, err := s.downloadSettings.preferences()
	unsaved := err != nil || prefs != s.settings.Download
	ui.Column(c.Key("download-utilities")).FillWidth().Radius(10).Border(0.5, colors(t).ListDivider).Clip().Children(func() {
		gameSettingRow(c, "download-cache", "下载缓存", func() {
			location := s.downloadSettings.cacheDirectory
			if d.actions.Directory != nil {
				location = d.actions.Directory(location)
			}
			if s.downloadSettings.cacheDirectory == s.settings.Download.CacheDirectory && d.info.Directory != "" {
				location = d.info.Directory
			}
			ui.Text(c, location).FontSize(12).TextColor(colors(t).Metadata).Grow(1).MinWidth(0).MaxLines(1).Ellipsis("…")
			summary := "—"
			if d.info.Directory != "" && s.downloadSettings.cacheDirectory == s.settings.Download.CacheDirectory {
				summary = formatBytes(d.info.Bytes + d.info.TemporaryBytes)
			}
			if d.operation == "inspect" || d.operation == "inspect-clear" {
				summary = "正在统计…"
			}
			ui.Text(c, summary).FontSize(11).TextColor(colors(t).Metadata).Shrink(0)
			if d.operation == "inspect" || d.operation == "inspect-clear" || d.operation == "clear" || d.operation == "open" || d.operation == "pick" {
				busyIndicator(c, 12)
				iconButton(c, "cancel-cache-operation", "取消缓存操作", iconClose, s.cancelDownloadToolAction)
			} else {
				iconButton(c, "open-download-cache", "打开缓存目录", iconFolder, s.openDownloadCache).Disabled(disabled || unsaved || d.actions.Open == nil)
				ui.ButtonBase(c.Key("cache-options")).Label("缓存操作").Tooltip("缓存操作").Size(24, 26).Shrink(0).Radius(5).Center().Disabled(disabled).Menu(func(menu *ui.Menu) {
					if menu.Item("更改位置…").Disabled(d.actions.Pick == nil).Chosen() {
						s.pickDownloadCache()
					}
					if menu.Item("恢复默认位置").Disabled(s.downloadSettings.cacheDirectory == "").Chosen() {
						s.downloadSettings.cacheDirectory = ""
						s.saveSettings()
					}
					menu.Separator()
					if menu.Item("清理缓存…").Disabled(unsaved || d.actions.Inspect == nil || d.actions.Clear == nil).Chosen() {
						s.clearDownloadCache()
					}
				}).Children(func() { ui.Icon(c, iconChevronDown).Size(12, 12).TextColor(colors(t).Icon) })
			}
		})
		gameSettingsDivider(c)
		gameSettingRow(c, "connectivity-check", "网络检查", func() {
			ui.Spacer(c)
			if d.operation == "check" {
				busyIndicator(c, 12)
				ui.Button(c.Key("cancel-connectivity"), "取消").Height(26).OnClick(s.cancelDownloadToolAction)
			} else {
				ui.Button(c.Key("connectivity-start"), "检查").Height(26).Disabled(disabled || unsaved || d.actions.Check == nil).OnClick(s.checkDownloadConnectivity)
			}
		})
		for _, result := range d.results {
			gameSettingsDivider(c)
			ui.Row(c.Key("connectivity-"+result.Name)).FillWidth().Height(36).Padding(0, 14).Gap(12).Children(func() {
				ui.Text(c, result.Name).FontSize(11).Grow(1).MinWidth(0).MaxLines(1).Ellipsis("…")
				label := fmt.Sprintf("HTTP %d · %d ms", result.Status, result.Elapsed.Milliseconds())
				color := colors(t).Metadata
				if result.Failure != "" {
					label = result.Failure
					color = t.Danger
				}
				ui.Text(c, label).FontSize(11).TextColor(color).Shrink(0)
			})
		}
	})
}
func (s *Shell) cancelDownloadToolAction() {
	clearing := s.downloadTools.operation == "clear"
	s.cancelDownloadTools()
	if clearing {
		s.Notify(ToastWarning, "清理已中断，可能已处理部分缓存")
	}
}
