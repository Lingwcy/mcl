package ui

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"strings"

	"mcl/internal/app"
	"mcl/internal/core/instance"

	"github.com/egoist/mygo/ui"
)

// Each edit session owns its drafts and callbacks, not build-scoped elements.
// Writes are serialized per session and keep going when navigating away.
type instanceSettingsState struct {
	deleting                      bool
	id, name, javaPath, iconStyle string
	icon                          []byte
	bitmap                        *ui.Bitmap
	base                          app.InstancePreferences
	value                         instance.Instance
	loaded, loading, saving       bool
	conflict                      bool
	loadError, saveError          string
	validation                    string
	iconOpen, iconBusy            bool
	iconGeneration                uint64
	cancelIcon                    func()
}

func (state *instanceSettingsState) draft() app.InstancePreferences {
	return app.InstancePreferences{
		Name: strings.TrimSpace(state.name), JavaPath: strings.TrimSpace(state.javaPath),
		IconStyle: state.iconStyle, Icon: bytes.Clone(state.icon),
	}
}

func equalInstancePreferences(a, b app.InstancePreferences) bool {
	return a.Name == b.Name && a.JavaPath == b.JavaPath && a.IconStyle == b.IconStyle && bytes.Equal(a.Icon, b.Icon)
}

func (s *Shell) openInstanceSettings() {
	value, ok := s.selectedInstance()
	if !ok {
		return
	}
	s.page = instanceSettingsPage
	if old := s.instanceEditor; old != nil && old.id == value.ID && (old.deleting || old.saving || old.loading || old.saveError != "" || old.validation != "") {
		return
	}
	s.loadInstanceSettings(value)
}

func (s *Shell) loadInstanceSettings(value instance.Instance) {
	if old := s.instanceEditor; old != nil {
		if old.id == value.ID && (old.saving || old.deleting) {
			return
		}
		s.cancelInstanceSettingsIcon()
	}
	state := &instanceSettingsState{id: value.ID}
	s.instanceEditor = state
	if s.library.actions.LoadInstance == nil {
		state.loadError = "实例设置读取服务未配置"
		return
	}
	state.loading = true
	s.library.actions.LoadInstance(value.ID, func(current instance.Instance, err error) {
		if s.instanceEditor != state {
			return
		}
		state.loading = false
		if err != nil {
			state.loadError = err.Error()
			return
		}
		if current.ID != state.id {
			state.loadError = "读取的实例与当前设置不匹配"
			return
		}
		state.value, state.base = current, app.PreferencesForInstance(current)
		state.name, state.javaPath, state.iconStyle = current.Name, current.JavaPath, current.IconStyle
		state.icon = bytes.Clone(current.Icon)
		if len(state.icon) > 0 {
			state.bitmap, _ = ui.DecodeBitmap(state.icon)
		}
		state.loaded = true
		s.publishEditedInstance(current)
	})
}

func (s *Shell) publishEditedInstance(value instance.Instance) {
	for index := range s.library.snapshot.Instances {
		if s.library.snapshot.Instances[index].ID == value.ID {
			s.library.snapshot.Instances[index] = value
			break
		}
	}
	s.cacheInstanceIcons([]instance.Instance{value})
}

func (s *Shell) instanceEditBlockReason(id string) string {
	if s.directories.running && s.directories.instanceID == id {
		return "目录操作进行中，请从目录任务查看进度"
	}
	for _, game := range s.games.rows {
		if game.InstanceID == id && game.Running {
			return "游戏运行时不能修改实例信息"
		}
	}
	for _, record := range s.installation.sessions {
		if record.InstanceID == id && !s.knownGame(record.ID) {
			return "游戏仍在运行，结束后可修改实例信息"
		}
	}
	if task := s.market.install; task != nil && task.busy && (task.request.InstanceID == id || (task.archivePlan != nil && task.archivePlan.Instance.ID == id)) {
		return "实例安装任务进行中，完成后可修改或删除"
	}
	if (s.installation.active && s.installation.instanceID == id) || s.assetUpdateBlocked(id) ||
		(s.assets.changing && s.assets.changeID == id) || s.repairingPack(id) || s.diagnosingAssets(id) || s.identifyingAssets(id) || s.associatingAsset(id) {
		return "实例任务进行中，完成后可修改信息"
	}
	return ""
}

// A valid edit is persisted off-thread. While a write is in flight, newer
// drafts stay editable and are coalesced into the next write on completion.
func (s *Shell) saveInstancePreferences(state *instanceSettingsState) {
	if state.deleting || !state.loaded || state.loading || state.conflict || s.library.actions.SavePreferences == nil {
		return
	}
	desired := state.draft()
	state.validation = ""
	if err := instance.ValidateName(desired.Name); err != nil {
		state.validation = err.Error()
		return
	}
	if err := (app.Settings{JavaPath: desired.JavaPath}).Validate(); err != nil {
		state.validation = err.Error()
		return
	}
	if state.saving {
		return
	}
	if equalInstancePreferences(desired, state.base) {
		state.saveError = ""
		return
	}
	if reason := s.instanceEditBlockReason(state.id); reason != "" {
		state.saveError = reason
		return
	}
	expected := state.base
	expected.Icon = bytes.Clone(expected.Icon)
	state.saving, state.saveError = true, ""
	s.library.actions.SavePreferences(state.id, expected, desired, func(updated instance.Instance, err error) {
		state.saving = false
		if err != nil {
			state.saveError = err.Error()
			state.conflict = errors.Is(err, app.ErrInstanceSettingsChanged)
			return
		}
		if updated.ID != state.id {
			state.saveError = "保存结果与当前实例不匹配"
			return
		}
		// Normalization must not overwrite a newer icon choice or cause an
		// unchanged custom PNG to be submitted in a normalization loop.
		if state.iconStyle == desired.IconStyle && bytes.Equal(state.icon, desired.Icon) {
			state.iconStyle, state.icon = updated.IconStyle, bytes.Clone(updated.Icon)
			state.bitmap = nil
			if len(state.icon) > 0 {
				state.bitmap, _ = ui.DecodeBitmap(state.icon)
			}
		}
		state.value, state.base = updated, app.PreferencesForInstance(updated)
		s.publishEditedInstance(updated)
		s.saveInstancePreferences(state)
	})
}

func (s *Shell) pickInstanceSettingsIcon(state *instanceSettingsState) {
	if state.iconBusy || s.library.actions.PickIcon == nil {
		return
	}
	state.iconGeneration++
	generation := state.iconGeneration
	state.iconBusy, state.saveError = true, ""
	state.iconOpen = false
	state.cancelIcon = s.library.actions.PickIcon(func(data []byte, err error) {
		if state.iconGeneration != generation {
			return
		}
		state.iconBusy, state.cancelIcon = false, nil
		if s.instanceEditor != state || s.page != instanceSettingsPage || s.library.selectedID != state.id {
			return
		}
		if err != nil {
			if !errors.Is(err, context.Canceled) {
				state.saveError = err.Error()
			}
			return
		}
		if len(data) == 0 {
			return
		}
		bitmap, err := ui.DecodeBitmap(data)
		if err != nil {
			state.saveError = "无法读取实例图标"
			return
		}
		state.icon, state.bitmap, state.iconStyle = bytes.Clone(data), bitmap, ""
		s.saveInstancePreferences(state)
	})
}

func (s *Shell) cancelInstanceSettingsIcon() {
	if state := s.instanceEditor; state != nil {
		state.iconGeneration++
		if state.cancelIcon != nil {
			state.cancelIcon()
		}
		state.cancelIcon, state.iconBusy, state.iconOpen = nil, false, false
	}
}

func (s *Shell) chooseInstanceSettingsIcon(state *instanceSettingsState, style string) {
	state.iconGeneration++
	if state.cancelIcon != nil {
		state.cancelIcon()
	}
	state.cancelIcon, state.iconBusy = nil, false
	state.iconStyle, state.icon, state.bitmap, state.iconOpen = style, nil, nil, false
	s.saveInstancePreferences(state)
}

func instanceSettingsIconStyle(state *instanceSettingsState) string {
	if state.iconStyle != "" {
		return state.iconStyle
	}
	if required, ok := state.value.RequiredLoader(); ok {
		return required.Type
	}
	return "grass"
}

func (s *Shell) deleteSelectedInstance() {
	state := s.instanceEditor
	if state == nil || !state.loaded || state.loading || state.saving || state.deleting || state.iconBusy || s.library.actions.DeleteInstance == nil || s.instanceEditBlockReason(state.id) != "" {
		return
	}
	state.deleting = true
	s.library.actions.DeleteInstance(state.id, func(err error) {
		state.deleting = false
		if err != nil {
			if !errors.Is(err, context.Canceled) {
				s.Notify(ToastError, err.Error())
			}
			return
		}
		id := state.id
		if s.deletedInstances == nil {
			s.deletedInstances = make(map[string]bool)
		}
		s.deletedInstances[id] = true
		s.library.snapshot.Instances = slices.DeleteFunc(s.library.snapshot.Instances, func(value instance.Instance) bool { return value.ID == id })
		delete(s.library.icons, id)
		delete(s.installation.receipts, id)
		if s.assets.instanceID == id {
			if s.assets.cancel != nil {
				s.assets.cancel()
				s.assets.cancel = nil
			}
			s.assets.generation++
			s.cancelAssetArtwork()
			s.assets.instanceID = ""
			s.assets.snapshot = app.AssetSnapshot{}
		}
		if current := s.market.install; current != nil && current.request.InstanceID == id && !current.busy {
			s.market.install = nil
			s.market.showInstall = false
		}
		if s.instanceEditor == state {
			s.cancelInstanceSettingsIcon()
			s.instanceEditor = nil
		}
		if s.library.selectedID == id {
			s.library.selectedID = ""
			if len(s.library.snapshot.Instances) > 0 {
				s.library.selectedID = s.library.snapshot.Instances[0].ID
			}
			s.openHome()
		}
		s.refreshGameDirectories()
		s.refreshSessions()
		s.Notify(ToastSuccess, "实例及其全部数据已移入废纸篓")
	})
}
