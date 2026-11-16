package ui

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"mcl/internal/app"
	"mcl/internal/core/instance"
	"mcl/internal/core/server"
)

// ServerActions schedules server scanning, global persistence, and SLP probes
// outside the UI thread, delivering callbacks on the UI thread.
type ServerActions struct {
	Sync        func(forceRescan bool, done func([]app.ServerRecord, error)) func()
	Add         func(app.AddServerRequest, func(app.ServerRecord, []app.ServerRecord, error)) func()
	SetInstance func(key, instanceID string, done func([]app.ServerRecord, error)) func()
	Remove      func(key string, done func([]app.ServerRecord, error)) func()
	Ping        func(key, address string, done func(server.Status, error)) func()
}

type serverPingState struct {
	busy       bool
	hasResult  bool
	status     server.Status
	err        string
	generation uint64
	cancel     func()
}

type serverState struct {
	actions            ServerActions
	records            []app.ServerRecord
	icons              map[string]*ui.Bitmap
	pings              map[string]*serverPingState
	loaded             bool
	syncing            bool
	adding             bool
	closed             bool
	syncGeneration     uint64
	mutationGeneration uint64
	cancelSync         func()
	cancelMutation     func()
	error              string
	filter             string
	instanceFilter     string
	search             string
	showAdd            bool
	addName            string
	addAddress         string
	addInstanceID      string
	addError           string
}

var defaultServerBitmap = pixelPreset([16]string{
	"................",
	"..SSSSSSSSSSSS..",
	".SssssssssssssS.",
	".SsGGssssttsssS.",
	".SssssssssssssS.",
	".SffffffffffffS.",
	"..SSSSSSSSSSSS..",
	"................",
	"..SSSSSSSSSSSS..",
	".SssssssssssssS.",
	".SsGGssssttsssS.",
	".SssssssssssssS.",
	".SffffffffffffS.",
	"..SSSSSSSSSSSS..",
	"................",
	"................",
})

func (s *Shell) ConnectServers(actions ServerActions) {
	s.servers.actions = actions
}

func (s *Shell) cancelServers() {
	sv := &s.servers
	sv.syncGeneration++
	sv.mutationGeneration++
	if sv.cancelSync != nil {
		sv.cancelSync()
		sv.cancelSync = nil
	}
	if sv.cancelMutation != nil {
		sv.cancelMutation()
		sv.cancelMutation = nil
	}
	sv.syncing = false
	sv.adding = false
	for _, ping := range sv.pings {
		ping.generation++
		if ping.cancel != nil {
			ping.cancel()
			ping.cancel = nil
		}
		ping.busy = false
	}
}

func (s *Shell) openServers() {
	s.cancelInstanceSettingsIcon()
	s.page, s.sidebarCollapsed = serversPage, false
	s.servers.error = ""
	if !s.servers.syncing {
		s.syncServers(false)
	}
}

func (s *Shell) rescanServers() {
	s.syncServers(true)
}

func (s *Shell) syncServers(forceRescan bool) {
	sv := &s.servers
	if sv.closed || sv.actions.Sync == nil {
		return
	}
	if sv.cancelSync != nil {
		sv.cancelSync()
		sv.cancelSync = nil
	}
	sv.syncGeneration++
	generation := sv.syncGeneration
	sv.syncing, sv.error = true, ""
	sv.cancelSync = sv.actions.Sync(forceRescan, func(records []app.ServerRecord, err error) {
		if sv.closed || generation != sv.syncGeneration {
			return
		}
		sv.syncing, sv.cancelSync = false, nil
		if err != nil {
			if !errors.Is(err, context.Canceled) {
				sv.error = err.Error()
			}
			return
		}
		sv.loaded = true
		s.applyServerRecords(records, forceRescan)
	})
}

func (s *Shell) applyServerRecords(records []app.ServerRecord, pingExisting bool) {
	sv := &s.servers
	sv.records = records
	if sv.icons == nil {
		sv.icons = make(map[string]*ui.Bitmap)
	}
	if sv.pings == nil {
		sv.pings = make(map[string]*serverPingState)
	}
	present := make(map[string]bool, len(records))
	for _, rec := range records {
		present[rec.Key] = true
		if len(rec.Icon) > 0 {
			if _, ok := sv.icons[rec.Key]; !ok {
				if bitmap, err := ui.DecodeBitmap(rec.Icon); err == nil {
					sv.icons[rec.Key] = bitmap
				}
			}
		}
	}
	for key, ping := range sv.pings {
		if !present[key] {
			ping.generation++
			if ping.cancel != nil {
				ping.cancel()
				ping.cancel = nil
			}
			delete(sv.pings, key)
		}
	}
	for key := range sv.icons {
		if !present[key] {
			delete(sv.icons, key)
		}
	}
	for _, rec := range records {
		ping := sv.pings[rec.Key]
		if ping == nil || (!ping.busy && !ping.hasResult) || pingExisting {
			s.pingServer(rec.Key, rec.Address)
		}
	}
}

func (s *Shell) refreshServerStatuses() {
	sv := &s.servers
	sv.error = ""
	if !sv.loaded && !sv.syncing {
		s.syncServers(false)
		return
	}
	for _, rec := range sv.records {
		s.pingServer(rec.Key, rec.Address)
	}
}

func (s *Shell) pingServer(key, address string) {
	sv := &s.servers
	if sv.closed || sv.actions.Ping == nil || key == "" || address == "" {
		return
	}
	if sv.pings == nil {
		sv.pings = make(map[string]*serverPingState)
	}
	state := sv.pings[key]
	if state == nil {
		state = &serverPingState{}
		sv.pings[key] = state
	}
	if state.cancel != nil {
		state.cancel()
		state.cancel = nil
	}
	state.generation++
	generation := state.generation
	state.busy, state.err = true, ""
	state.cancel = sv.actions.Ping(key, address, func(status server.Status, err error) {
		if sv.closed || state.generation != generation {
			return
		}
		state.busy, state.cancel = false, nil
		state.hasResult = true
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			state.status = server.Status{Online: false}
			state.err = formatServerPingError(err)
			return
		}
		state.status = status
		state.err = ""
		if len(status.Favicon) > 0 {
			if bitmap, decodeErr := ui.DecodeBitmap(status.Favicon); decodeErr == nil {
				if sv.icons == nil {
					sv.icons = make(map[string]*ui.Bitmap)
				}
				sv.icons[key] = bitmap
			}
		}
		for idx := range sv.records {
			if sv.records[idx].Key == key {
				if len(status.MOTD) > 0 {
					sv.records[idx].MOTD = status.MOTD
					sv.records[idx].PlainMOTD = status.PlainMOTD
				}
				if status.Version != "" {
					sv.records[idx].Version = status.Version
				}
				if len(status.Favicon) > 0 {
					sv.records[idx].Icon = status.Favicon
				}
				break
			}
		}
	})
}

func (s *Shell) openAddServer() {
	sv := &s.servers
	sv.showAdd = true
	sv.addError = ""
	if sv.addInstanceID == "" {
		if sv.instanceFilter != "" {
			sv.addInstanceID = sv.instanceFilter
		} else if inst, ok := s.selectedInstance(); ok {
			sv.addInstanceID = inst.ID
		}
	}
}

func (s *Shell) submitAddServer() {
	sv := &s.servers
	if sv.closed || sv.adding || sv.actions.Add == nil {
		return
	}
	if strings.TrimSpace(sv.addAddress) == "" {
		sv.addError = "请输入服务器地址"
		return
	}
	sv.adding, sv.addError, sv.error = true, "", ""
	if sv.cancelMutation != nil {
		sv.cancelMutation()
		sv.cancelMutation = nil
	}
	sv.mutationGeneration++
	generation := sv.mutationGeneration
	req := app.AddServerRequest{
		Name:       sv.addName,
		Address:    sv.addAddress,
		InstanceID: sv.addInstanceID,
	}
	sv.cancelMutation = sv.actions.Add(req, func(added app.ServerRecord, records []app.ServerRecord, err error) {
		if sv.closed || generation != sv.mutationGeneration {
			return
		}
		sv.adding, sv.cancelMutation = false, nil
		if err != nil {
			if !errors.Is(err, context.Canceled) {
				sv.addError = err.Error()
			}
			return
		}
		sv.loaded = true
		sv.showAdd = false
		sv.addName, sv.addAddress, sv.addError = "", "", ""
		s.applyServerRecords(records, false)
		s.pingServer(added.Key, added.Address)
		s.Notify(ToastSuccess, "已添加到全局服务器列表")
	})
}

func (s *Shell) setServerInstance(key, instanceID string) {
	sv := &s.servers
	if sv.closed || sv.actions.SetInstance == nil {
		return
	}
	for i := range sv.records {
		if sv.records[i].Key == key {
			sv.records[i].InstanceID = instanceID
			break
		}
	}
	sv.mutationGeneration++
	generation := sv.mutationGeneration
	if sv.cancelMutation != nil {
		sv.cancelMutation()
		sv.cancelMutation = nil
	}
	sv.cancelMutation = sv.actions.SetInstance(key, instanceID, func(records []app.ServerRecord, err error) {
		if sv.closed || generation != sv.mutationGeneration {
			return
		}
		sv.cancelMutation = nil
		if err != nil {
			if !errors.Is(err, context.Canceled) {
				sv.error = err.Error()
			}
			return
		}
		s.applyServerRecords(records, false)
	})
}

func (s *Shell) removeServer(key string) {
	sv := &s.servers
	if sv.closed || sv.actions.Remove == nil {
		return
	}
	sv.error = ""
	sv.mutationGeneration++
	generation := sv.mutationGeneration
	if sv.cancelMutation != nil {
		sv.cancelMutation()
		sv.cancelMutation = nil
	}
	sv.cancelMutation = sv.actions.Remove(key, func(records []app.ServerRecord, err error) {
		if sv.closed || generation != sv.mutationGeneration {
			return
		}
		sv.cancelMutation = nil
		if err != nil {
			if !errors.Is(err, context.Canceled) {
				sv.error = err.Error()
			}
			return
		}
		s.applyServerRecords(records, false)
	})
}

func (s *Shell) serversSidebar(c *ui.Context) {
	const entryHeight = 28
	sv, t := &s.servers, s.theme
	ui.Row(c).FillWidth().Height(44).Shrink(0).Padding(0, 12, 0, 16).Gap(6).Children(func() {
		ui.Text(c, "服务器").FontSize(12).FontWeight(600).Grow(1)
		iconButton(c, "rescan-instance-servers", "重新收录实例服务器", iconScan, s.rescanServers).Disabled(sv.syncing || sv.actions.Sync == nil)
		iconButton(c, "sidebar-add-server", "添加服务器", iconPlus, s.openAddServer)
	})
	ui.Scroll(c.Key("servers-sidebar-scroll")).Grow(1).MinHeight(0).FillWidth().Padding(4, 0, 10).Gap(1).Children(func() {
		s.secondaryEntry(c, "servers-all", "全部服务器", iconServer, entryHeight, sv.filter == "" && sv.instanceFilter == "", func() {
			sv.filter, sv.instanceFilter = "", ""
		})
		s.secondaryEntry(c, "servers-online", "在线服务器", iconCheck, entryHeight, sv.filter == "online" && sv.instanceFilter == "", func() {
			sv.filter, sv.instanceFilter = "online", ""
		})
		s.secondaryEntry(c, "servers-manual", "全局添加", iconPlus, entryHeight, sv.filter == "manual" && sv.instanceFilter == "", func() {
			sv.filter, sv.instanceFilter = "manual", ""
		})
		if len(s.library.snapshot.Instances) > 0 {
			ui.Divider(c).Margin(6, 14)
			ui.Text(c, "按实例筛选").FontSize(10).FontWeight(600).TextColor(colors(t).Metadata).Padding(4, 16, 4, 16)
			for _, inst := range s.library.snapshot.Instances {
				id := inst.ID
				selected := sv.instanceFilter == id
				fill := ui.Transparent
				textColor := t.Text
				countColor := colors(t).Metadata
				if selected {
					fill = colors(t).Selected
					if t.Dark {
						textColor, countColor = colors(t).AccentInk, colors(t).AccentInk
					}
				}
				button := ui.ButtonBase(c.Key("server-filter-"+id)).Label(inst.Name).Width(secondarySidebarWidth(c)-16).Height(34).Margin(0, 8, 2).Padding(0, 9).Gap(8).Radius(8).Justify(ui.Start)
				count := s.instanceServerCount(id)
				rowMotion(c, button, fill).OnClick(func() {
					sv.filter = ""
					sv.instanceFilter = id
				}).Children(func() {
					s.instanceAvatar(c, inst, 20)
					ui.Text(c, inst.Name).FontSize(12).TextColor(textColor).Grow(1).MinWidth(0).MaxLines(1).Ellipsis("…")
					if count > 0 {
						ui.Text(c, strconv.Itoa(count)).FontSize(10).TextColor(countColor).Shrink(0)
					}
				})
			}
		}
	})
}

func (s *Shell) serversView(c *ui.Context) {
	sv, t := &s.servers, s.theme
	rows := s.filteredServers()
	pingingCount := s.pingingServerCount()

	workspace := ui.Column(c.Key("servers-workspace")).Grow(1).MinHeight(0).FillWidth().Padding(16, 24, 20).Gap(12)
	dialogProgress := animateDialog(workspace, "add-server-dialog", sv.showAdd)
	workspace.Children(func() {
		ui.Row(c.Key("servers-toolbar")).FillWidth().Height(28).Shrink(0).Gap(8).Children(func() {
			ui.Text(c, s.serversHeading()).FontSize(15).FontWeight(600).MinWidth(0).SingleLine()
			if len(rows) > 0 {
				ui.Textf(c, "%d", len(rows)).FontSize(11).TextColor(colors(t).Metadata).Shrink(0)
			}
			ui.Spacer(c)
			if sv.syncing || pingingCount > 0 {
				busyIndicator(c, 12)
			}
			ui.Row(c).Width(172).MinWidth(84).Shrink(1).Height(26).Padding(0, 8).Gap(6).Radius(6).Background(t.Surface).Children(func() {
				ui.Icon(c, iconSearch).Size(13, 13).TextColor(colors(t).Icon)
				ui.TextInputBase(c.Key("server-search"), &sv.search).Label("搜索服务器").Placeholder("搜索名称或地址").Grow(1).MinWidth(0).FillHeight().FontSize(11)
			})
			sizedIconButton(c, "refresh-servers", "刷新服务器状态", iconRefresh, 26, 15, 6, s.refreshServerStatuses).Disabled(sv.syncing)
			ui.PrimaryButton(c.Key("add-server-open"), "添加服务器").Height(26).FontSize(12).OnClick(s.openAddServer)
		})

		if sv.syncing && len(sv.records) == 0 {
			loadingList(c, false)
			return
		}
		if len(rows) == 0 {
			s.serversEmptyState(c)
			return
		}
		ui.Scroll(c.Key("servers-scroll")).Grow(1).MinHeight(0).FillWidth().Gap(7).Children(func() {
			for _, rec := range rows {
				s.serverBannerCard(c, rec)
			}
		})
	})
	s.addServerDialog(c, dialogProgress)
}

func (s *Shell) addServerDialog(c *ui.Context, progress float32) {
	sv, t := &s.servers, s.theme
	if !sv.showAdd && progress <= 0.005 {
		return
	}
	p := colors(t)
	open := true
	dlg := ui.DialogBase(c.Key("add-server-dialog"), &open, func(backdrop, panel ui.Element) {
		panel.Label("添加服务器").Width(340).Padding(14, 16).Radius(12).
			Background(p.Raised).Border(1, p.Border)
		styleDialogMotion(backdrop, panel, p, progress, sv.showAdd)
		ui.Column(c).FillWidth().Gap(10).Children(func() {
			ui.Row(c).FillWidth().Height(20).Children(func() {
				ui.Text(c, "添加服务器").FontSize(13).FontWeight(600)
				ui.Spacer(c)
				sizedIconButton(c, "close-add-server", "关闭", iconClose, 20, 13, 5, func() {
					sv.showAdd = false
					sv.addError = ""
				})
			})
			input := ui.TextInput(c.Key("add-server-address"), &sv.addAddress).Label("服务器地址").
				Placeholder("输入服务器地址，如 mc.example.com").FillWidth().Height(30).FontSize(12).
				Disabled(sv.adding).
				OnChange(func() { sv.addError = "" }).OnSubmit(s.submitAddServer)
			if sv.showAdd {
				input.AutoFocus()
			}
			ui.Row(c).FillWidth().Height(26).Gap(6).AlignItems(ui.Center).Children(func() {
				targetInst, hasInst := s.instanceByID(sv.addInstanceID)
				targetLabel := "自动选择"
				if hasInst {
					targetLabel = targetInst.Name
				}
				instBtn := ui.ButtonBase(c.Key("add-server-instance")).Label("默认启动实例").Tooltip("启动实例："+targetLabel).
					Height(26).MaxWidth(150).Padding(0, 8).Gap(5).Radius(6).Disabled(sv.adding)
				rowMotion(c, instBtn, p.Control).Menu(func(menu *ui.Menu) {
					if menu.Item("自动选择").Checked(sv.addInstanceID == "").Chosen() {
						sv.addInstanceID = ""
					}
					for _, inst := range s.library.snapshot.Instances {
						id := inst.ID
						if menu.Item(inst.Name + " · " + inst.Version.ID).Checked(sv.addInstanceID == id).Chosen() {
							sv.addInstanceID = id
						}
					}
				}).Children(func() {
					if hasInst {
						s.instanceAvatar(c, targetInst, 13)
					}
					ui.Text(c, targetLabel).FontSize(11).MinWidth(0).SingleLine()
					ui.Icon(c, iconChevronDown).Size(10, 10).Shrink(0).TextColor(p.Metadata)
				})
				ui.Spacer(c)
				ui.Button(c.Key("add-server-cancel"), "取消").Height(26).FontSize(12).Disabled(sv.adding).OnClick(func() {
					sv.showAdd = false
					sv.addError = ""
				})
				ui.PrimaryButton(c.Key("add-server-submit"), "添加").Height(26).FontSize(12).
					Disabled(sv.adding || strings.TrimSpace(sv.addAddress) == "").OnClick(s.submitAddServer)
			})
			if sv.addError != "" {
				ui.Text(c, sv.addError).FontSize(11).TextColor(t.Danger).MaxLines(2)
			}
		})
	})
	if (!open || dlg.Dismissed()) && sv.showAdd {
		sv.showAdd = false
		sv.addError = ""
	}
}

func (s *Shell) serversEmptyState(c *ui.Context) {
	sv, t := &s.servers, s.theme
	ui.Column(c.Key("servers-empty")).Grow(1).MinHeight(0).FillWidth().Padding(24).Center().Gap(10).Children(func() {
		ui.Icon(c, iconServer).Size(28, 28).TextColor(colors(t).IconMuted)
		message := "暂无已收录的服务器"
		detail := "在任意实例游戏内添加服务器，或点击右上角添加全局服务器。"
		if strings.TrimSpace(sv.search) != "" || sv.filter != "" || sv.instanceFilter != "" {
			message = "没有匹配的服务器"
			detail = "可以调整筛选条件或清除搜索关键词。"
		}
		ui.Text(c, message).FontSize(13).FontWeight(500)
		ui.Text(c, detail).FontSize(11).TextColor(t.TextMuted).TextAlign(ui.Center)
		if len(sv.records) == 0 {
			ui.Row(c).Gap(8).Margin(4, 0, 0, 0).Children(func() {
				ui.PrimaryButton(c.Key("empty-add-server"), "添加服务器").Height(28).OnClick(s.openAddServer)
				ui.Button(c.Key("empty-rescan-servers"), "收录实例服务器").Height(28).Disabled(sv.syncing || sv.actions.Sync == nil).OnClick(s.rescanServers)
			})
		}
	})
}

func (s *Shell) serverBannerCard(c *ui.Context, rec app.ServerRecord) {
	sv, t := &s.servers, s.theme
	p := colors(t)
	ping := sv.pings[rec.Key]
	targetInst, hasInst := s.resolveServerInstance(rec)
	services := c.Services()

	online := ping != nil && ping.hasResult && ping.status.Online
	probing := ping != nil && ping.busy
	failed := ping != nil && ping.hasResult && !ping.status.Online

	versionText := rec.Version
	if online && ping.status.Version != "" {
		versionText = ping.status.Version
	}

	motd := rec.MOTD
	if online && len(ping.status.MOTD) > 0 {
		motd = ping.status.MOTD
	}

	key := rec.Key
	address := rec.Address

	ui.Row(c.Key("server-"+rec.Key)).FillWidth().Shrink(0).Radius(8).Border(0.5, p.ListDivider).Background(t.Surface).Padding(7, 12).Gap(10).AlignItems(ui.Center).Children(func() {
		// Server 64x64 artwork
		ui.Box(c.Key("icon")).Size(42, 42).Shrink(0).Radius(8).Clip().Background(p.Tag).Center().Children(func() {
			if bmp := sv.icons[rec.Key]; bmp != nil {
				ui.Image(c.Key("favicon"), bmp).Size(42, 42).Fit(ui.Cover).Transition(motionArtwork)
			} else {
				ui.Image(c.Key("fallback"), defaultServerBitmap).Size(30, 30).Fit(ui.Contain)
			}
		})

		// Address + MOTD Banner + Status & Actions
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			// Top row: Address, source badge, spacer, online status, more menu
			ui.Row(c).FillWidth().Height(18).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, rec.Address).FontSize(13).FontWeight(600).MinWidth(0).Shrink(1).SingleLine().ContextMenu(func(menu *ui.Menu) {
					if menu.Item("复制服务器地址").Chosen() {
						services.WriteClipboard(address)
					}
				})
				if rec.Source == "manual" {
					ui.Text(c, "全局").FontSize(10).TextColor(p.AccentInk).Padding(1, 5).Radius(4).Background(p.Selected).Shrink(0).SingleLine()
				}
				ui.Spacer(c)

				statusRow := ui.Row(c).Shrink(0).Gap(6).AlignItems(ui.Center)
				if versionText != "" {
					statusRow.Tooltip("服务端：" + versionText)
				}
				statusRow.Children(func() {
					dotColor := p.IconMuted
					switch {
					case probing:
						dotColor = p.Warning
					case online:
						dotColor = t.Success
					case failed:
						dotColor = t.Danger
					}
					ui.Box(c).Size(6, 6).Radius(3).Background(dotColor)
					switch {
					case probing:
						busyIndicator(c, 11)
						ui.Text(c, "探测中").FontSize(11).TextColor(p.Metadata).Shrink(0)
					case online:
						ui.Textf(c, "%d / %d", ping.status.OnlinePlayers, ping.status.MaxPlayers).FontSize(11).FontWeight(500).TextColor(t.TextMuted).Shrink(0)
						latencyColor := t.Success
						if ping.status.LatencyMS > 180 {
							latencyColor = t.Danger
						} else if ping.status.LatencyMS > 80 {
							latencyColor = p.Warning
						}
						ui.Textf(c, "%d ms", ping.status.LatencyMS).FontSize(10).FontWeight(500).TextColor(latencyColor).Padding(1, 5).Radius(4).Background(p.Tag).Shrink(0)
					case failed:
						ui.Text(c, "离线").FontSize(11).TextColor(t.Danger).Shrink(0)
					default:
						ui.Text(c, "未探测").FontSize(11).TextColor(p.Metadata).Shrink(0)
					}
				})

				moreBtn := ui.ButtonBase(c.Key("server-more")).Label(address+" 的更多操作").Tooltip("更多操作").Size(20, 20).Shrink(0).Radius(5).Center()
				rowMotion(c, moreBtn, ui.Transparent).Menu(func(menu *ui.Menu) {
					if menu.Item("刷新状态").Chosen() {
						s.pingServer(key, address)
					}
					if menu.Item("复制服务器地址").Chosen() {
						services.WriteClipboard(address)
					}
					menu.Separator()
					if menu.Item("从启动器移除").Chosen() {
						s.removeServer(key)
					}
				}).Children(func() { ui.Text(c, "···").FontSize(14).FontWeight(600) })
			})

			// Bottom row: Compact MOTD banner + instance selector + join button
			ui.Row(c).FillWidth().Gap(12).AlignItems(ui.Center).Children(func() {
				ui.Column(c.Key("motd-banner")).Grow(1).MinWidth(0).Gap(1).Children(func() {
					switch {
					case len(motd) > 0:
						shown := 0
						for lineIdx, line := range motd {
							if shown >= 2 {
								break
							}
							clean := trimmedMOTDSpans(line.Spans)
							if len(clean) == 0 {
								continue
							}
							spans := make([]ui.Span, 0, len(clean))
							for _, sp := range clean {
								span := ui.Span{Text: sp.Text}
								if sp.Bold {
									span.Weight = 600
								}
								if col, ok := adaptMOTDColor(sp.Color, t); ok {
									span.Color = col
								}
								spans = append(spans, span)
							}
							ui.RichText(c.Key(lineIdx), spans...).FillWidth().MinWidth(0).FontSize(11).TextColor(t.Text).SingleLine()
							shown++
						}
						if shown == 0 {
							ui.Text(c, rec.Address).FontSize(11).TextColor(p.Metadata).SingleLine()
						}
					case probing:
						ui.Text(c, "正在探测服务器信息与 MOTD…").FontSize(11).TextColor(p.Metadata).SingleLine()
					case failed && ping.err != "":
						ui.Text(c, ping.err).FontSize(11).TextColor(t.TextMuted).MaxLines(2).Ellipsis("…")
					default:
						ui.Text(c, "点击右上角刷新可探测服务器 MOTD 与在线状态").FontSize(11).TextColor(p.Metadata).SingleLine()
					}
				})

				ui.Row(c).Shrink(0).Gap(6).AlignItems(ui.Center).Children(func() {
					instLabel := "无可用实例"
					if hasInst {
						instLabel = targetInst.Name
					}
					instBtn := ui.ButtonBase(c.Key("server-instance")).Label("选择启动实例").Tooltip("启动实例："+instLabel).Height(24).MaxWidth(116).Padding(0, 7).Gap(5).Radius(5).Disabled(len(s.library.snapshot.Instances) == 0)
					rowMotion(c, instBtn, p.Tag).Menu(func(menu *ui.Menu) {
						for _, inst := range s.library.snapshot.Instances {
							instID := inst.ID
							checked := hasInst && targetInst.ID == instID
							if menu.Item(inst.Name + " · " + inst.Version.ID).Checked(checked).Chosen() {
								s.setServerInstance(key, instID)
							}
						}
					}).Children(func() {
						if hasInst {
							s.instanceAvatar(c, targetInst, 13)
						}
						ui.Text(c, instLabel).FontSize(11).MinWidth(0).SingleLine()
						ui.Icon(c, iconChevronDown).Size(10, 10).Shrink(0).TextColor(p.Metadata)
					})

					joinDisabled := !hasInst || s.installation.active
					joinFill, joinInk := p.Control, p.Metadata
					if !joinDisabled {
						joinFill, joinInk = p.Accent, p.OnAccent
					}
					joinBtn := ui.ButtonBase(c.Key("join-server")).Label("启动并加入 "+address).Tooltip("使用「"+instLabel+"」启动并加入服务器").Height(24).Padding(0, 9).Gap(5).Radius(5).Disabled(joinDisabled)
					rowMotion(c, joinBtn, joinFill).OnClick(func() { s.launchServer(rec) }).Children(func() {
						ui.Icon(c, iconPlay).Size(10, 10).TextColor(joinInk)
						ui.Text(c, "加入").FontSize(11).FontWeight(500).TextColor(joinInk)
					})
				})
			})
		})
	})
}

func (s *Shell) launchServer(rec app.ServerRecord) {
	inst, ok := s.resolveServerInstance(rec)
	if !ok {
		s.Notify(ToastInfo, "请先创建一个游戏实例")
		s.openNewInstance()
		return
	}
	if s.installation.active {
		s.Notify(ToastWarning, "当前已有启动或安装任务正在进行")
		return
	}
	for _, game := range s.games.rows {
		if game.InstanceID == inst.ID && game.Running {
			s.Notify(ToastWarning, inst.Name+" 正在运行中")
			return
		}
	}
	for _, record := range s.installation.sessions {
		if !s.knownGame(record.ID) && record.InstanceID == inst.ID {
			s.Notify(ToastWarning, inst.Name+" 正在运行中")
			return
		}
	}
	if !supportedPackLoader(inst) {
		s.Notify(ToastWarning, "暂不支持该实例的加载器")
		return
	}
	if _, loggedIn := s.accounts.data.Active(); !loggedIn {
		s.Notify(ToastInfo, "请先选择或添加游戏账户")
		s.openAccounts()
		return
	}
	s.library.selectedID = inst.ID
	s.startGameWithServer(inst, rec.Address, rec.Address)
}

func (s *Shell) resolveServerInstance(rec app.ServerRecord) (instance.Instance, bool) {
	if rec.InstanceID != "" {
		if inst, ok := s.instanceByID(rec.InstanceID); ok {
			return inst, true
		}
	}
	for _, id := range rec.InstanceIDs {
		if inst, ok := s.instanceByID(id); ok {
			return inst, true
		}
	}
	if selected, ok := s.selectedInstance(); ok {
		return selected, true
	}
	if len(s.library.snapshot.Instances) > 0 {
		return s.library.snapshot.Instances[0], true
	}
	return instance.Instance{}, false
}

func (s *Shell) instanceByID(id string) (instance.Instance, bool) {
	if id == "" {
		return instance.Instance{}, false
	}
	for _, inst := range s.library.snapshot.Instances {
		if inst.ID == id {
			return inst, true
		}
	}
	return instance.Instance{}, false
}

func (s *Shell) instanceServerCount(instanceID string) int {
	count := 0
	for _, rec := range s.servers.records {
		if rec.InstanceID == instanceID || slices.Contains(rec.InstanceIDs, instanceID) {
			count++
		}
	}
	return count
}

func (s *Shell) pingingServerCount() int {
	count := 0
	for _, ping := range s.servers.pings {
		if ping.busy {
			count++
		}
	}
	return count
}

func (s *Shell) serversHeading() string {
	sv := &s.servers
	if sv.instanceFilter != "" {
		if inst, ok := s.instanceByID(sv.instanceFilter); ok {
			return inst.Name
		}
	}
	switch sv.filter {
	case "online":
		return "在线服务器"
	case "manual":
		return "全局添加"
	default:
		return "全部服务器"
	}
}

func (s *Shell) filteredServers() []app.ServerRecord {
	sv := &s.servers
	query := strings.ToLower(strings.TrimSpace(sv.search))
	var rows []app.ServerRecord
	for _, rec := range sv.records {
		if sv.instanceFilter != "" {
			if rec.InstanceID != sv.instanceFilter && !slices.Contains(rec.InstanceIDs, sv.instanceFilter) {
				continue
			}
		} else {
			switch sv.filter {
			case "online":
				ping := sv.pings[rec.Key]
				if ping == nil || !ping.hasResult || !ping.status.Online {
					continue
				}
			case "manual":
				if rec.Source != "manual" {
					continue
				}
			}
		}
		if query != "" {
			haystack := strings.ToLower(rec.Name + " " + rec.Address + " " + rec.Version + " " + rec.PlainMOTD)
			if !strings.Contains(haystack, query) {
				continue
			}
		}
		rows = append(rows, rec)
	}
	return rows
}

func formatServerPingError(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	if errors.Is(err, context.DeadlineExceeded) || strings.Contains(strings.ToLower(msg), "timeout") || strings.Contains(strings.ToLower(msg), "i/o timeout") {
		return "连接超时，服务器可能离线或不可达"
	}
	if strings.Contains(strings.ToLower(msg), "connection refused") {
		return "连接被拒绝，服务器未启动或端口未开放"
	}
	if strings.Contains(strings.ToLower(msg), "no such host") {
		return "域名解析失败，请检查服务器地址"
	}
	return msg
}

func trimmedMOTDSpans(spans []server.MOTDSpan) []server.MOTDSpan {
	return server.CleanMOTDSpans(spans)
}

func adaptMOTDColor(hexColor string, t *ui.Theme) (ui.Color, bool) {
	if len(hexColor) != 7 || hexColor[0] != '#' {
		return ui.Color{}, false
	}
	if !t.Dark && strings.EqualFold(hexColor, "#FFFFFF") {
		return t.Text, true
	}
	if t.Dark && strings.EqualFold(hexColor, "#000000") {
		return t.Text, true
	}
	r, errR := strconv.ParseUint(hexColor[1:3], 16, 8)
	g, errG := strconv.ParseUint(hexColor[3:5], 16, 8)
	b, errB := strconv.ParseUint(hexColor[5:7], 16, 8)
	if errR != nil || errG != nil || errB != nil {
		return ui.Color{}, false
	}
	rf, gf, bf := float64(r), float64(g), float64(b)
	luminance := 0.299*rf + 0.587*gf + 0.114*bf
	if !t.Dark {
		// Darken bright Minecraft colors (white, yellow, aqua, light green) on light backgrounds.
		if luminance > 142 {
			scale := 130.0 / luminance
			rf *= scale
			gf *= scale
			bf *= scale
		}
	} else {
		// Lift very dark colors (black, dark blue) so they remain legible on dark backgrounds.
		if luminance < 64 {
			boost := 64.0 - luminance
			rf = min(255, rf+boost)
			gf = min(255, gf+boost)
			bf = min(255, bf+boost)
		}
	}
	return ui.RGB(uint8(rf), uint8(gf), uint8(bf)), true
}
