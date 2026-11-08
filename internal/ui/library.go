package ui

import (
	"context"
	"errors"
	"slices"

	"mcl/internal/app"
	"mcl/internal/core/instance"
	"mcl/internal/core/loader"

	"github.com/egoist/mygo/ui"
)

// LibraryActions starts work outside rendering. Completions run on the UI thread.
type LibraryActions struct {
	DeleteInstance     func(string, func(error)) func()
	Load               func(bool, func(app.LibrarySnapshot, error))
	Create             func(app.CreateInstance, func(instance.Instance, error))
	CreateTask         func(app.CreateInstance, func(instance.Instance, error)) func()
	LoadInstance       func(string, func(instance.Instance, error))
	SavePreferences    func(string, app.InstancePreferences, app.InstancePreferences, func(instance.Instance, error))
	LoaderReleases     func(string, string, func([]loader.Release, error)) func()
	PickIcon           func(func([]byte, error)) func()
	PickArchive        func(app.Settings, func(app.ArchiveImportPlan, error)) func()
	InspectArchiveFile func(string, app.Settings, func(app.ArchiveImportPlan, error)) func()
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
	sidebarList     ui.ListState
	versionList     ui.ListState
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
		snapshot.Instances = slices.DeleteFunc(snapshot.Instances, func(value instance.Instance) bool { return s.deletedInstances[value.ID] })
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

func (s *Shell) createInstance() {
	l := &s.library
	if l.busy || (l.actions.Create == nil && l.actions.CreateTask == nil) {
		return
	}
	settings := s.creationSettings()
	if !s.gameDirectoryAvailable(settings.GameDirectoryID) {
		s.Notify(ToastError, "请选择可用的游戏目录")
		return
	}
	s.gameDirectories.pickerOpen = false
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
		s.refreshGameDirectories()
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
