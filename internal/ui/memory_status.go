package ui

import (
	"context"
	"errors"
	"fmt"

	"github.com/egoist/mygo/ui"
	"mcl/internal/app"
	"mcl/internal/core/java"
	"mcl/internal/core/launch"
)

type WatchMemory func(func(app.MemorySnapshot, error)) func()
type memoryState struct {
	watch                  WatchMemory
	cancel                 func()
	active, blurred, valid bool
	generation             uint64
	snapshot               app.MemorySnapshot
	reportedError          string
}

func (s *Shell) ConnectMemory(watch WatchMemory) {
	s.memory.watch = watch
	if s.window != nil {
		s.window.Invalidate()
	}
}

// A guarded page/focus lifecycle, not a render-time probe. The injected watcher
// owns sampling and schedules immutable snapshots through Window.Update.
func (s *Shell) syncMemoryObservation() {
	m := &s.memory
	visible := s.page == gameSettingsPage && !m.blurred && m.watch != nil
	if !visible {
		if m.active {
			m.generation++
			m.active = false
			if m.cancel != nil {
				m.cancel()
			}
			m.cancel = nil
		}
		return
	}
	if m.active {
		return
	}
	m.active = true
	m.valid = false
	m.generation++
	generation := m.generation
	m.cancel = m.watch(func(snapshot app.MemorySnapshot, err error) {
		if !m.active || generation != m.generation {
			return
		}
		if err != nil {
			m.valid = false
			if !errors.Is(err, context.Canceled) && m.reportedError != err.Error() {
				s.Notify(ToastWarning, "无法读取内存状态："+err.Error())
				m.reportedError = err.Error()
			}
			return
		}
		m.snapshot, m.valid, m.reportedError = snapshot, true, ""
	})
}
func (s *Shell) memorySuggestionMiB() int {
	snapshot := s.memory.snapshot
	// A verified explicit 32-bit preference uses the same 1 GiB ceiling
	// as launch. Both estimates use this sample's available-memory budget.
	for _, environment := range s.java.inventory.Environments {
		if s.settings.JavaPath != "" && environment.Runtime.Executable == s.settings.JavaPath {
			return launch.AutomaticMemoryMiB(java.Runtime{Arch: environment.Runtime.Arch}, snapshot.AvailableBytes)
		}
	}
	return snapshot.SuggestedMiB
}
func memoryBarFractions(snapshot app.MemorySnapshot, suggestedMiB int) (used, planned float32) {
	if snapshot.Validate() != nil {
		return 0, 0
	}
	usedRatio := float64(snapshot.UsedBytes()) / float64(snapshot.TotalBytes)
	plannedRatio := float64(max(0, suggestedMiB)) * float64(1<<20) / float64(snapshot.TotalBytes)
	return float32(usedRatio), float32(min(plannedRatio, 1-usedRatio))
}
func memoryGiB(bytes uint64) string {
	return fmt.Sprintf("%.2f GiB", float64(bytes)/float64(uint64(1)<<30))
}

func (s *Shell) memoryStatusView(c *ui.Context) {
	m, t := &s.memory, s.theme
	palette := colors(t)
	snapshot := m.snapshot
	suggested := s.memorySuggestionMiB()
	ui.Column(c.Key("system-memory-status")).FillWidth().Shrink(0).Padding(10, 14, 12).Gap(9).Children(func() {
		ui.Row(c).FillWidth().Gap(6).Children(func() {
			ui.Text(c, "内存状态").FontSize(12).Grow(1)
			label := "正在读取…"
			if m.valid {
				label = fmt.Sprintf("%.2f / %s 已使用", float64(snapshot.UsedBytes())/float64(uint64(1)<<30), memoryGiB(snapshot.TotalBytes))
			} else if m.reportedError != "" {
				label = "暂不可用"
			}
			ui.Text(c, label).FontSize(11).TextColor(palette.Metadata)
		})
		used, planned := float32(0), float32(0)
		description := "内存状态尚不可用"
		if m.valid {
			used, planned = memoryBarFractions(snapshot, suggested)
			description = fmt.Sprintf("系统已用 %s，总内存 %s。Minecraft 自动上限预计 %s，仅为上限预览，不代表实际占用或预留。", memoryGiB(snapshot.UsedBytes()), memoryGiB(snapshot.TotalBytes), memoryGiB(uint64(max(0, suggested))<<20))
		}
		ui.Box(c.Key("memory-status-bar")).FillWidth().Height(10).Radius(3).Clip().Background(palette.MemoryTrack).Label(description).Draw(func(p *ui.Painter, r ui.Rect) {
			if used > 0 {
				p.Fill(ui.Rect{X: r.X, Y: r.Y, W: r.W * used, H: r.H}, palette.MemoryUsed, 0)
			}
			if planned > 0 {
				p.Fill(ui.Rect{X: r.X + r.W*used, Y: r.Y, W: r.W * planned, H: r.H}, palette.MemoryPlanned, 0)
			}
		})
		ui.Row(c).FillWidth().Wrap().Gap(10).Children(func() {
			memoryLegend(c, palette.MemoryUsed, "系统已用")
			label := "Minecraft 自动上限（预计）"
			if m.valid {
				label = "Minecraft 预计上限 " + memoryGiB(uint64(max(0, suggested))<<20)
			}
			memoryLegend(c, palette.MemoryPlanned, label)
		})
	})
}
func memoryLegend(c *ui.Context, color ui.Color, label string) {
	ui.Row(c).Gap(5).Children(func() {
		ui.Box(c).Size(7, 7).Shrink(0).Radius(4).Background(color)
		ui.Text(c, label).FontSize(10).TextColor(colors(c.Theme()).Metadata)
	})
}
