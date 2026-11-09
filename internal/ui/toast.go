package ui

import (
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

type ToastKind string

const (
	ToastInfo    ToastKind = "info"
	ToastSuccess ToastKind = "success"
	ToastWarning ToastKind = "warning"
	ToastError   ToastKind = "error"
)

type toastState struct {
	pending []ui.Toast
	active  []string
	sources map[string]string
}

// Notify is UI-thread only. Workers must publish through Window.Update first.
// No Context, Element, timers or goroutines are retained by the component.
func (s *Shell) Notify(kind ToastKind, message string) {
	message = strings.TrimSpace(message)
	if message == "" {
		return
	}
	toast := ui.Toast{ID: string(kind) + ":" + message, Title: message, Type: string(kind), Timeout: 5 * time.Second}
	if kind == ToastError || kind == ToastWarning {
		toast.Timeout = 8 * time.Second
	}
	for i, queued := range s.toasts.pending {
		if queued.ID == toast.ID {
			s.toasts.pending[i] = toast
			return
		}
	}
	if len(s.toasts.pending) == 3 {
		s.toasts.pending = s.toasts.pending[1:]
	}
	s.toasts.pending = append(s.toasts.pending, toast)
}
func (s *Shell) deliverToast(c *ui.Context, toast ui.Toast) {
	state := &s.toasts
	if !slices.Contains(state.active, toast.ID) {
		if len(state.active) == 3 {
			c.CloseToast(state.active[0])
		}
		state.active = append(state.active, toast.ID)
	}
	id := toast.ID
	toast.OnClose = func() { state.active = slices.DeleteFunc(state.active, func(value string) bool { return value == id }) }
	if toast.Action == "" && len([]rune(toast.Title)) > 80 {
		message := toast.Title
		services := c.Services()
		toast.Action = "复制详情"
		toast.OnAction = func() { services.WriteClipboard(message) }
	}
	c.AddToast(toast)
}

// Bridge existing operation states once per change, never once per build pass.
// Retaining these states keeps retry, conflict and persistence logic intact.
func (s *Shell) feedbackToast(c *ui.Context, key, message string, kind ToastKind, action string, run func()) {
	state := &s.toasts
	if state.sources == nil {
		state.sources = make(map[string]string)
	}
	if state.sources[key] == message {
		return
	}
	state.sources[key] = message
	id := "feedback:" + key
	if message == "" {
		c.CloseToast(id)
		return
	}
	timeout := 5 * time.Second
	if kind == ToastError || kind == ToastWarning {
		timeout = 8 * time.Second
	}
	s.deliverToast(c, ui.Toast{ID: id, Title: message, Type: string(kind), Timeout: timeout, Action: action, OnAction: run})
}
func (s *Shell) feedbackToasts(c *ui.Context) {
	message := ""
	if s.noticeFail {
		message = s.notice
	}
	s.feedbackToast(c, "settings", message, ToastError, "", nil)
	message = ""
	if s.java.failed {
		message = s.java.message
	}
	s.feedbackToast(c, "java", message, ToastError, "", nil)
	s.feedbackToast(c, "accounts", s.accounts.message, ToastError, "", nil)
	s.feedbackToast(c, "assets", s.assets.message, func() ToastKind {
		if s.assets.failed {
			return ToastError
		}
		return ToastWarning
	}(), "", nil)
	s.feedbackToast(c, "market-search", s.market.error, ToastError, "", nil)
	s.feedbackToast(c, "market-key", s.market.keyError, ToastError, "", nil)
	s.feedbackToast(c, "market-action", s.market.message, ToastInfo, "", nil)
	s.feedbackToast(c, "creation", s.creation.err, ToastError, "", nil)
	message = ""
	if s.installation.failed {
		message = s.installation.message
	}
	s.feedbackToast(c, "installation", message, ToastError, "", nil)
	message = ""
	if s.java.job.failed {
		message = s.java.job.message
	}
	s.feedbackToast(c, "java-install", message, ToastError, "", nil)

	message = ""
	if s.downloadTools.failed {
		message = s.downloadTools.message
	}
	s.feedbackToast(c, "downloads", message, ToastError, "", nil)
	message = ""
	if s.library.failed {
		message = s.library.message
	}
	s.feedbackToast(c, "library", message, ToastError, "", nil)
	message = ""
	if s.loadErr != nil {
		message = "无法读取设置，原文件已保留。" + s.loadErr.Error()
	}
	s.feedbackToast(c, "settings-load", message, ToastError, "", nil)
	message, action := "", ""
	var retry func()
	if state := s.instanceEditor; state != nil {
		message = state.validation
		if message == "" {
			message = state.saveError
		}
		if state.saveError != "" && !state.conflict {
			action = "重试"
			retry = func() {
				if s.instanceEditor == state {
					s.saveInstancePreferences(state)
				}
			}
		}
	}
	s.feedbackToast(c, "instance-settings", message, ToastError, action, retry)
	pending := s.toasts.pending
	s.toasts.pending = nil
	for _, toast := range pending {
		s.deliverToast(c, toast)
	}
	toastViewport(c)
}
func toastViewport(c *ui.Context) {
	t, p := c.Theme(), colors(c.Theme())
	ui.ToastViewportBase(c, func(viewport ui.Element, toasts []ui.Toast) {
		viewport.Top(titlebarHeight+10).Padding(0, 12, 12).Justify(ui.Start).AlignItems(ui.End).Gap(8)
		for _, toast := range toasts {
			parts := ui.ToastBase(c, toast)
			icon, color := iconInfo, p.Icon
			switch ToastKind(toast.Type) {
			case ToastError:
				icon, color = iconError, t.Danger
			case ToastWarning:
				icon, color = iconWarning, p.Warning
			case ToastSuccess:
				icon, color = iconCheck, p.Success
			}
			parts.Root.Row().AlignItems(ui.Start).Width(320).MaxWidth(320).Padding(10, 12).Gap(8).Radius(10).Background(p.Raised).Border(0.5, p.Border).Shadow(0, 4, 16, 0, p.Shadow).Transition(motionToast).Children(func() {
				ui.Icon(c, icon).Size(14, 14).Shrink(0).Margin(2, 0, 0).TextColor(color)
				ui.Column(c).Grow(1).MinWidth(0).Gap(8).Children(func() {
					ui.Text(c, toast.Title).FontSize(12).TextColor(t.Text).MaxLines(4)
					if toast.Action != "" {
						parts.ActionButton().Label(toast.Action).Height(24).Padding(0, 8).Radius(5).Background(p.Control).Children(func() { ui.Text(c, toast.Action).FontSize(11) })
					}
				})
				parts.CloseButton().Label("关闭通知").Tooltip("关闭通知").Size(18, 18).Shrink(0).Radius(4).Center().Children(func() { ui.Icon(c, iconClose).Size(12, 12).TextColor(p.Metadata) })
			})
		}
	})
}
