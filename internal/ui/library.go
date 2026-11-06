package ui

import (
	"context"
	"errors"
	"strings"

	"mcl/internal/app"
	"mcl/internal/core/instance"
	"mcl/internal/core/loader"

	"github.com/egoist/mygo/ui"
)

// LibraryActions starts work outside rendering. Completions run on the UI thread.
type LibraryActions struct {
	Load           func(bool, func(app.LibrarySnapshot, error))
	Create         func(app.CreateInstance, func(instance.Instance, error))
	CreateTask     func(app.CreateInstance, func(instance.Instance, error)) func()
	Rename         func(string, string, func(instance.Instance, error))
	SaveRuntime    func(string, app.Settings, func(instance.Instance, error))
	LoaderReleases func(string, string, func([]loader.Release, error)) func()
	PickIcon       func(func([]byte, error)) func()
	PickArchive    func(app.Settings, func(app.ArchiveImportPlan, error)) func()
}

type libraryState struct {
	actions         LibraryActions
	snapshot        app.LibrarySnapshot
	selected        string
	selectedID      string
	name            string
	search          instanceSearchState
	versionSearch   string
	newDetails      bool
	busy            bool
	message         string
	failed          bool
	editing         string
	editName        string
	editJava        string
	sidebarList     ui.ListState
	versionList     ui.ListState
	pickerList      ui.ListState
	icons           map[string]*ui.Bitmap
	cancelCreate    func()
	cancelingCreate bool
}

func (s *Shell) ConnectLibrary(actions LibraryActions) {
	s.library.actions = actions
	s.refreshLibrary(false)
}

func (s *Shell) refreshLibrary(refresh bool) {
	l := &s.library
	if l.busy || l.actions.Load == nil {
		return
	}
	l.busy, l.message, l.failed = true, "", false
	l.actions.Load(refresh, func(snapshot app.LibrarySnapshot, err error) {
		l.busy = false
		if err == nil || snapshot.InstancesLoaded {
			l.snapshot.Instances = snapshot.Instances
			s.cacheInstanceIcons(snapshot.Instances)
		}
		if _, found := s.selectedInstance(); !found && len(l.snapshot.Instances) > 0 {
			l.selectedID = l.snapshot.Instances[0].ID
		}
		s.ensureAssets()
		if err != nil {
			l.message, l.failed = err.Error(), true
			return
		}
		l.snapshot, l.message = snapshot, snapshot.Warning
		if s.page == newInstancePage && l.selected == "" && len(snapshot.Releases) > 0 {
			l.selected = snapshot.Releases[0].ID
			s.updateCreationName()
			s.loadCreationReleases()
		}
	})
}

func (s *Shell) selectedInstance() (instance.Instance, bool) {
	for _, value := range s.library.snapshot.Instances {
		if value.ID == s.library.selectedID {
			return value, true
		}
	}
	return instance.Instance{}, false
}

func (s *Shell) openNewInstance() {
	if !s.library.failed {
		s.library.message = ""
	}
	s.library.newDetails = false
	s.page, s.sidebarCollapsed = newInstancePage, false
	if s.library.selected == "" && len(s.library.snapshot.Releases) > 0 {
		s.library.selected = s.library.snapshot.Releases[0].ID
	}
	s.updateCreationName()
	s.loadCreationReleases()
}

func (s *Shell) pickVersion(id string) {
	l := &s.library
	for _, entry := range l.snapshot.Releases {
		if entry.ID == id {
			if l.name == "" || l.name == l.selected {
				l.name = id
			}
			l.selected, l.newDetails, l.message, l.failed = id, true, "", false
			s.creation.versionOpen = false
			s.updateCreationName()
			s.loadCreationReleases()
			s.page = newInstancePage
			return
		}
	}
	l.message, l.failed = "请选择列表中的正式版本", true
}

func (s *Shell) openInstanceSettings() {
	value, ok := s.selectedInstance()
	if !ok {
		s.page = settingsPage
		return
	}
	s.library.editing, s.library.editName = "", value.Name
	s.library.editJava = value.JavaPath
	s.page = instanceSettingsPage
}

func (s *Shell) saveInstanceRuntime() {
	l := &s.library
	value, ok := s.selectedInstance()
	if !ok || l.busy || l.actions.SaveRuntime == nil {
		return
	}
	settings := app.Settings{JavaPath: strings.TrimSpace(l.editJava), MaxMemoryMiB: value.MaxMemoryMiB}
	if err := settings.Validate(); err != nil {
		l.message, l.failed = err.Error(), true
		return
	}
	l.busy, l.failed, l.message = true, false, ""
	l.actions.SaveRuntime(value.ID, settings, func(updated instance.Instance, err error) {
		l.busy = false
		if err != nil {
			l.message, l.failed = err.Error(), true
			return
		}
		for index := range l.snapshot.Instances {
			if l.snapshot.Instances[index].ID == updated.ID {
				l.snapshot.Instances[index] = updated
			}
		}
		l.message, l.failed = "实例运行设置已保存", false
	})
}

func (s *Shell) createInstance() {
	l := &s.library
	if l.busy || (l.actions.Create == nil && l.actions.CreateTask == nil) {
		return
	}
	settings := s.settings
	l.busy, l.message, l.failed = true, "", false
	draft := &s.creation
	request := app.CreateInstance{Name: l.name, VersionID: l.selected, Settings: settings, Icon: append([]byte(nil), draft.icon...), IconStyle: draft.iconStyle}
	if draft.kind != "" {
		if draft.loaderVersion == "" || draft.busy {
			l.busy = false
			l.message, l.failed = "请选择可用的加载器版本", true
			return
		}
		request.Loader = &instance.Loader{Type: draft.kind, Version: draft.loaderVersion}
	}
	installNow := draft.installNow
	done := func(value instance.Instance, err error) {
		l.busy = false
		l.cancelCreate, l.cancelingCreate = nil, false
		if err != nil {
			if errors.Is(err, context.Canceled) {
				l.message, l.failed = "已取消创建", false
				return
			}
			l.message, l.failed = err.Error(), true
			return
		}
		l.snapshot.Instances = append([]instance.Instance{value}, l.snapshot.Instances...)
		s.cacheInstanceIcons([]instance.Instance{value})
		l.selectedID, l.name, l.message, l.newDetails = value.ID, "", "实例已创建，游戏目录与其他实例隔离。", false
		s.page = libraryPage
		s.ensureAssets()
		if installNow {
			s.startInstallation()
		}
	}
	if l.actions.CreateTask != nil {
		l.cancelCreate = l.actions.CreateTask(request, done)
	} else {
		l.actions.Create(request, done)
	}
}

func (s *Shell) renameInstance() {
	l := &s.library
	if l.busy || l.actions.Rename == nil || l.editing == "" {
		return
	}
	id, name := l.editing, l.editName
	l.busy, l.message, l.failed = true, "", false
	l.actions.Rename(id, name, func(value instance.Instance, err error) {
		l.busy = false
		if err != nil {
			l.message, l.failed = err.Error(), true
			return
		}
		for index := range l.snapshot.Instances {
			if l.snapshot.Instances[index].ID == value.ID {
				l.snapshot.Instances[index] = value
			}
		}
		l.editing, l.message = "", "实例名称已更新"
	})
}
