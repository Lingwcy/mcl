package ui

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"

	"mcl/internal/app"
	"mcl/internal/core/auth"

	"github.com/egoist/mygo/ui"
)

type AccountActions struct {
	MicrosoftAvailable bool
	Load               func(func(app.Accounts, error))
	Microsoft          func(string, func(app.Accounts, error)) func()
	Offline            func(string, func(app.Accounts, error))
	Select             func(string, func(app.Accounts, error))
	Remove             func(string, func(app.Accounts, error))
	PickSkin           func(string, func(app.Accounts, error))
	RefreshHead        func(string, bool, func([]byte, error))
}

type accountState struct {
	actions                              AccountActions
	data                                 app.Accounts
	heads                                map[string]*ui.Bitmap
	headLookups                          map[string]bool // true while a lookup is in flight
	loaded, busy, canceling, showOffline bool
	cancel                               func()
	name, message                        string
}

func (s *Shell) ConnectAccounts(actions AccountActions) {
	s.accounts.actions = actions
	s.loadAccounts()
}

func (s *Shell) loadAccounts() {
	a := &s.accounts
	if a.busy || a.actions.Load == nil {
		return
	}
	a.busy = true
	a.actions.Load(func(accounts app.Accounts, err error) {
		a.busy = false
		if err != nil {
			a.message = err.Error()
			return
		}
		s.applyAccounts(accounts)
	})
}

func (s *Shell) finishAccountAction(accounts app.Accounts, err error) {
	a := &s.accounts
	a.busy, a.canceling, a.cancel = false, false, nil
	if err != nil {
		if !errors.Is(err, context.Canceled) {
			a.message = err.Error()
		}
		return
	}
	a.showOffline, a.message, a.name = false, "", ""
	s.applyAccounts(accounts)
}

func (s *Shell) applyAccounts(accounts app.Accounts) {
	a := &s.accounts
	if !a.loaded {
		a.message = ""
	}
	a.loaded = true
	a.data = accounts
	heads := make(map[string]*ui.Bitmap, len(accounts.Profiles))
	for _, profile := range accounts.Profiles {
		id := profile.ID()
		if len(profile.Head) == 0 {
			// A snapshot read during a lookup can predate its commit.
			heads[id] = a.heads[id]
		} else if head, err := ui.DecodeBitmap(profile.Head); err == nil {
			heads[id] = head.Pixelated()
		}
	}
	a.heads = heads
	if profile, ok := a.data.Active(); ok {
		s.refreshOfflineHead(profile, false)
	}
}

func (s *Shell) refreshOfflineHead(profile auth.Profile, explicit bool) {
	a := &s.accounts
	id := profile.ID()
	pending, attempted := a.headLookups[id]
	if a.actions.RefreshHead == nil || profile.Kind != auth.Offline || pending {
		return
	}
	if explicit {
		if a.busy {
			return
		}
		a.busy, a.message = true, ""
	} else if attempted || len(profile.Head) > 0 {
		return
	} else {
		for _, busy := range a.headLookups {
			if busy {
				return
			}
		}
	}
	if a.headLookups == nil {
		a.headLookups = make(map[string]bool)
	}
	a.headLookups[id] = true
	a.actions.RefreshHead(id, explicit, func(head []byte, err error) {
		a.headLookups[id] = false
		defer func() {
			if active, ok := a.data.Active(); ok {
				s.refreshOfflineHead(active, false)
			}
		}()
		if explicit {
			a.busy = false
			if err != nil {
				a.message = err.Error()
			}
		}
		if err != nil || len(head) == 0 {
			return
		}
		for i, current := range a.data.Profiles {
			if current.ID() == id && bytes.Equal(current.Head, profile.Head) {
				if bitmap, err := ui.DecodeBitmap(head); err == nil {
					a.data.Profiles[i].Head = head
					a.heads[id] = bitmap.Pixelated()
				}
				break
			}
		}
	})
}

func (s *Shell) changeAccount(id string, action func(string, func(app.Accounts, error))) {
	a := &s.accounts
	if a.busy || action == nil {
		return
	}
	a.busy, a.message = true, ""
	action(id, s.finishAccountAction)
}

func (s *Shell) microsoftSignIn(id string) {
	a := &s.accounts
	if a.busy || a.actions.Microsoft == nil {
		return
	}
	a.busy, a.message = true, ""
	a.cancel = a.actions.Microsoft(id, s.finishAccountAction)
}

func (s *Shell) openAccounts() {
	s.page = accountsPage
}

func (s *Shell) accountAvatar(c *ui.Context, id string, size float32) {
	head := s.accounts.heads[id]
	if head == nil {
		head = steveHead
	}
	ui.Image(c, head).Size(size, size).Shrink(0).Radius(4).Fit(ui.Contain)
}

func (s *Shell) accountRailButton(c *ui.Context) {
	a, t := &s.accounts, s.theme
	label := "账户 · 未登录"
	if profile, ok := a.data.Active(); ok {
		label = "账户 · " + profile.Name
	}
	button := ui.ButtonBase(c.Key("rail-accounts")).Label("账户").Tooltip(label).Size(30, 30).Shrink(0).Radius(8).Center()
	fill := ui.Transparent
	if s.page == accountsPage {
		fill = colors(t).Selected
	}
	controlMotion(c, button, fill, true).OnClick(s.openAccounts).Children(func() {
		s.accountAvatar(c, a.data.ActiveID, 20)
	})
}

func (s *Shell) accountsView(c *ui.Context) {
	a := &s.accounts
	ui.Column(c.Key("accounts-content")).Fill().Padding(20, 24).Gap(8).Children(func() {
		if a.cancel != nil {
			label := "请在浏览器中完成微软登录"
			if a.canceling {
				label = "正在取消登录…"
			}
			s.accountLoadingView(c, label, true)
			return
		}
		if !a.loaded {
			if a.busy {
				s.accountLoadingView(c, "正在读取账户", false)
			} else {
				ui.Column(c.Key("account-load-error")).Fill().Center().Gap(12).Children(func() {

					if a.actions.Load != nil {
						ui.Button(c.Key("reload-accounts"), "重新读取账户").Height(28).OnClick(s.loadAccounts)
					}
				})
			}
			return
		}
		if len(a.data.Profiles) == 0 {
			if a.busy {
				s.accountLoadingView(c, "正在添加离线账户", false)
			} else {
				s.emptyAccountsView(c)
			}
			return
		}
		ui.Row(c.Key("accounts-toolbar")).FillWidth().Height(28).Shrink(0).Gap(8).Children(func() {
			ui.Spacer(c)
			ui.PrimaryButton(c.Key("add-account"), "添加账户").Height(26).Disabled(a.busy || !a.loaded).Menu(func(m *ui.Menu) {
				if m.Item("微软账户").Chosen() {
					s.microsoftSignIn("")
				}
				if m.Item("离线 ID").Chosen() {
					a.showOffline, a.message = true, ""
				}
			})
		})
		if a.showOffline {
			s.offlineAccountForm(c)
		}
		ui.Scroll(c.Key("account-list")).Grow(1).MinHeight(0).FillWidth().Children(func() {
			ui.Column(c.Key("accounts-group")).FillWidth().Children(func() {
				for index, profile := range a.data.Profiles {
					s.accountRow(c, profile)
					if index < len(a.data.Profiles)-1 {
						gameSettingsDivider(c)
					}
				}
			})
		})
	})
}

// Keep the first-account choices centered without a second toolbar or card.
func (s *Shell) emptyAccountsView(c *ui.Context) {
	a, t := &s.accounts, s.theme
	ui.Column(c.Key("accounts-empty")).Fill().Center().Children(func() {
		ui.Column(c.Key("account-welcome")).FillWidth().MaxWidth(320).AlignItems(ui.Center).Gap(16).Transition(motionReveal).Children(func() {
			ui.Column(c).AlignItems(ui.Center).Gap(12).Children(func() {
				s.accountAvatar(c, "", 48)
				ui.Text(c, "还没有账户").FontSize(15).FontWeight(600).TextColor(t.Text)
			})
			ui.Row(c.Key("first-account-actions")).FillWidth().Gap(10).Children(func() {
				ui.PrimaryButton(c.Key("first-microsoft"), "").Label("登录微软账户").Height(32).Grow(1).MinWidth(0).Gap(8).
					Disabled(a.actions.Microsoft == nil).OnClick(func() { s.microsoftSignIn("") }).Children(func() {
					ui.Icon(c, iconMicrosoft).Size(14, 14).Shrink(0)
					ui.Text(c, "登录微软账户").FontSize(12).SingleLine()
				})
				ui.Button(c.Key("first-offline"), "").Label("添加离线 ID").Height(32).Grow(1).MinWidth(0).Gap(8).
					Disabled(a.actions.Offline == nil).OnClick(func() { a.showOffline, a.message = true, "" }).Children(func() {
					ui.Icon(c, iconUser).Size(15, 15).Shrink(0)
					ui.Text(c, "添加离线 ID").FontSize(12).SingleLine()
				})
			})
			if a.showOffline {
				s.offlineAccountForm(c)
			}
			if !a.actions.MicrosoftAvailable {
				ui.Text(c, "微软登录需要配置 mcl 自有的应用 Client ID").FillWidth().TextAlign(ui.Center).FontSize(11).TextColor(t.TextMuted)
			}
		})
	})
}

func (s *Shell) accountLoadingView(c *ui.Context, label string, cancelable bool) {
	a := &s.accounts
	ui.Column(c.Key("account-waiting")).Fill().Center().Gap(12).Children(func() {
		ui.Column(c.Key("account-loading")).FillWidth().MaxWidth(360).Height(132).Shrink(0).Children(func() {
			loadingStatus(c, label)
		})
		if cancelable {
			ui.Button(c.Key("cancel-sign-in"), "取消登录").Height(28).Disabled(a.canceling).OnClick(func() {
				if a.cancel != nil {
					a.canceling = true
					a.cancel()
				}
			})
		}
	})
}

func (s *Shell) offlineAccountForm(c *ui.Context) {
	a := &s.accounts
	save := func() { s.changeAccount(a.name, a.actions.Offline) }
	ui.Row(c.Key("offline-account-form")).FillWidth().MaxWidth(360).Gap(6).Children(func() {
		ui.TextInput(c.Key("offline-name"), &a.name).Label("离线玩家名称").Placeholder("玩家名称").Grow(1).MinWidth(0).Height(28).Disabled(a.busy).OnSubmit(save)
		ui.PrimaryButton(c.Key("save-offline"), "添加").Height(28).Disabled(a.busy || a.name == "").OnClick(save)
		iconButton(c, "cancel-offline", "关闭离线账户表单", iconClose, func() { a.showOffline, a.name, a.message = false, "", "" })
	})
}

func (s *Shell) accountRow(c *ui.Context, profile auth.Profile) {
	a, t := &s.accounts, s.theme
	id := profile.ID()
	active := id == a.data.ActiveID
	provider := "离线"
	if profile.Kind == auth.Microsoft {
		provider = "微软"
	}
	ui.Row(c.Key(id)).FillWidth().Height(44).Shrink(0).Padding(0, 6).Gap(4).Children(func() {
		button := ui.ButtonBase(c.Key("select")).Label("切换到 "+profile.Name).Grow(1).MinWidth(0).Height(38).Radius(7).Padding(0, 8).Gap(8).Justify(ui.Start).Disabled(a.busy)
		rowMotion(c, button, ui.Transparent).OnClick(func() {
			if !active {
				s.changeAccount(id, a.actions.Select)
			}
		}).Children(func() {
			s.accountAvatar(c, id, 24)
			ui.Text(c, profile.Name).FontSize(13).FontWeight(500).MinWidth(0).MaxLines(1).Ellipsis("…")
			marketTag(c, provider, false)
			ui.Spacer(c)
			if active {
				ui.Icon(c, iconCheck).Label("当前账户").Size(15, 15).TextColor(colors(t).AccentInk)
			}
		})
		ui.ButtonBase(c.Key("options")).Label(profile.Name+" 的账户选项").Tooltip("账户选项").Size(28, 28).Radius(5).Center().Disabled(a.busy).Menu(func(m *ui.Menu) {
			if profile.Kind == auth.Microsoft {
				if m.Item("重新登录").Chosen() {
					s.microsoftSignIn(id)
				}
			} else {
				if m.Item("使用同名正版头像").Disabled(a.headLookups[id]).Chosen() {
					s.refreshOfflineHead(profile, true)
				}
				if m.Item("从本地皮肤设置头像").Chosen() {
					s.changeAccount(id, a.actions.PickSkin)
				}
			}
			if m.Item("移除账户").Chosen() {
				s.changeAccount(id, a.actions.Remove)
			}
		}).Children(func() { ui.Text(c, "···").FontSize(16).FontWeight(600) })
	})
}

var steveHead = func() *ui.Bitmap {
	colors := map[byte]color.NRGBA{
		'h': {R: 66, G: 45, B: 33, A: 255},
		'd': {R: 83, G: 57, B: 40, A: 255},
		's': {R: 185, G: 125, B: 91, A: 255},
		'l': {R: 208, G: 149, B: 113, A: 255},
		'w': {R: 228, G: 220, B: 211, A: 255},
		'e': {R: 78, G: 64, B: 123, A: 255},
		'n': {R: 160, G: 100, B: 70, A: 255},
		'm': {R: 105, G: 63, B: 48, A: 255},
	}
	pixels := [...]string{"hhdhhhdh", "hhhdhhhh", "hsslllsh", "slsslsls", "swellews", "slsnnlss", "slmnnmss", "ssmmmmss"}
	img := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	for y, row := range pixels {
		for x := range 8 {
			img.SetNRGBA(x, y, colors[row[x]])
		}
	}
	return ui.NewBitmap(img).Pixelated()
}()
