package ui

import (
	"context"
	"errors"
	"slices"

	"github.com/egoist/mygo/ui"
	"mcl/internal/app"
	"mcl/internal/core/discovery"
	"mcl/internal/core/instance"
	"mcl/internal/core/transfer"
)

type DirectoryActions struct {
	Open          func(app.DirectoryOperation, func(error)) func()
	List          func(func([]app.DirectoryOperation, error)) func()
	InspectImport func(app.DirectoryImportRequest, func(transfer.Progress), func(app.DirectoryPlan, error)) func()
	InspectMove   func(string, string, func(transfer.Progress), func(app.DirectoryPlan, error)) func()
	Confirm       func(app.DirectoryPlan, func(bool, error)) func()
	Run           func(app.DirectoryPlan, func(transfer.Progress), func(instance.Instance, error)) func()
	Resume        func(app.DirectoryOperation, func(transfer.Progress), func(instance.Instance, error)) func()
	Abort, Clean  func(app.DirectoryOperation, func(error)) func()
}
type directoryState struct {
	actions                          DirectoryActions
	mode, operation, instanceID      string
	request                          app.DirectoryImportRequest
	candidate                        discovery.Candidate
	groups                           map[string]bool
	to                               string
	plan                             *app.DirectoryPlan
	operations                       []app.DirectoryOperation
	progress                         transfer.Progress
	busy, running, canceling, closed bool
	cancel                           func()
	generation                       uint64
	returnPage                       page
	pickerOpen, groupsOpen           bool
	files, tasks                     ui.ListState
}

func (s *Shell) ConnectDirectories(actions DirectoryActions) { s.directories.actions = actions }
func (s *Shell) cancelDirectoryPreview() {
	d := &s.directories
	if d.running {
		return
	}
	d.generation++
	if d.cancel != nil {
		d.cancel()
	}
	d.cancel = nil
	d.busy = false
	d.operation = ""
}
func (s *Shell) stopDirectoryTask() {
	d := &s.directories
	if d.busy && d.cancel != nil && !d.canceling {
		d.canceling = true
		d.cancel()
	}
}
func (s *Shell) openDirectoryImport(candidate discovery.Candidate) {
	d := &s.directories
	if d.running {
		s.openDirectoryTasks()
		return
	}
	s.cancelDirectoryPreview()
	d.returnPage = existingGamesPage
	d.mode = "import"
	d.candidate = candidate
	d.plan = nil
	d.instanceID = ""
	d.groups = map[string]bool{}
	for _, group := range transfer.ImportGroups {
		d.groups[group] = group != "config"
	}
	d.request = app.DirectoryImportRequest{SourceID: s.existing.sourceID, CandidateID: candidate.ID, Name: candidate.Folder, DirectoryID: s.settings.GameDirectoryID}
	if len(candidate.GamePaths) == 1 {
		d.request.DataPath = candidate.GamePaths[0]
	}
	d.to = d.request.DirectoryID
	s.page = directoryPage
	s.refreshGameDirectories()
}
func (s *Shell) openDirectoryMove() {
	d := &s.directories
	if d.running {
		s.openDirectoryTasks()
		return
	}
	value, ok := s.selectedInstance()
	if !ok {
		return
	}
	if reason := s.instanceEditBlockReason(value.ID); reason != "" {
		s.Notify(ToastWarning, reason)
		return
	}
	s.cancelDirectoryPreview()
	d.mode = "move"
	d.returnPage = instanceSettingsPage
	d.instanceID = value.ID
	d.plan = nil
	d.to = ""
	if value.GameDirectoryID == "" {
		d.to = s.settings.GameDirectoryID
	}
	d.request = app.DirectoryImportRequest{Name: value.Name}
	s.page = directoryPage
	s.refreshGameDirectories()
}
func (s *Shell) openDirectoryTasks() {
	d := &s.directories
	if s.page != directoryPage {
		d.returnPage = s.page
	}
	s.page = directoryPage
	if d.running {
		return
	}
	s.cancelDirectoryPreview()
	d.mode = "tasks"
	d.plan = nil
	s.loadDirectoryOperations()
}
func (s *Shell) closeDirectoryView() {
	d := &s.directories
	if !d.running {
		s.cancelDirectoryPreview()
	}
	destination := d.returnPage
	if destination == "" || destination == directoryPage {
		destination = activityPage
	}
	s.page = destination
}
func (s *Shell) directoryProgress(generation uint64) func(transfer.Progress) {
	return func(progress transfer.Progress) {
		d := &s.directories
		if d.closed || d.generation != generation || !d.busy {
			return
		}
		d.progress = progress
	}
}
func (s *Shell) startDirectoryOperation(operation string, running bool) uint64 {
	s.cancelDirectoryPreview()
	d := &s.directories
	d.generation++
	d.busy = true
	d.running = running
	d.canceling = false
	d.operation = operation
	d.progress = transfer.Progress{}
	return d.generation
}
func (s *Shell) finishDirectoryOperation(generation uint64, err error) bool {
	d := &s.directories
	if d.closed || d.generation != generation {
		return false
	}
	canceled := d.canceling
	wasRunning := d.running
	d.busy = false
	d.running = false
	d.canceling = false
	d.cancel = nil
	d.operation = ""
	if err != nil {
		if !errors.Is(err, context.Canceled) {
			s.Notify(ToastError, err.Error())
		} else if canceled && wasRunning {
			s.Notify(ToastWarning, "目录操作已停止；已开始的复制可从目录任务继续或撤销")
		}
		return false
	}
	return true
}
func (s *Shell) previewDirectory() {
	d := &s.directories
	if d.busy {
		return
	}
	d.plan = nil
	generation := s.startDirectoryOperation("preview", false)
	done := func(plan app.DirectoryPlan, err error) {
		if s.finishDirectoryOperation(generation, err) {
			d.plan = &plan
		}
	}
	if d.mode == "import" {
		request := d.request
		request.DirectoryID = d.to
		request.Groups = nil
		for _, group := range transfer.ImportGroups {
			if d.groups[group] {
				request.Groups = append(request.Groups, group)
			}
		}
		if d.actions.InspectImport == nil {
			s.finishDirectoryOperation(generation, errors.New("目录导入服务未配置"))
			return
		}
		d.cancel = d.actions.InspectImport(request, s.directoryProgress(generation), done)
	} else {
		if d.actions.InspectMove == nil {
			s.finishDirectoryOperation(generation, errors.New("目录迁移服务未配置"))
			return
		}
		d.cancel = d.actions.InspectMove(d.instanceID, d.to, s.directoryProgress(generation), done)
	}
}
func (s *Shell) confirmDirectoryPlan() {
	d := &s.directories
	if d.busy || d.plan == nil || d.actions.Confirm == nil {
		return
	}
	plan := *d.plan
	generation := s.startDirectoryOperation("confirm", false)
	d.cancel = d.actions.Confirm(plan, func(okay bool, err error) {
		if !s.finishDirectoryOperation(generation, err) || !okay {
			return
		}
		s.runDirectoryPlan(plan)
	})
}
func (s *Shell) runDirectoryPlan(plan app.DirectoryPlan) {
	d := &s.directories
	if d.busy || d.actions.Run == nil {
		return
	}
	generation := s.startDirectoryOperation("run", true)
	d.instanceID = plan.After.ID
	d.plan = &plan
	d.cancel = d.actions.Run(plan, s.directoryProgress(generation), func(value instance.Instance, err error) {
		s.directoryCompleted(generation, value, err, plan.Kind == "import")
	})
}
func (s *Shell) directoryCompleted(generation uint64, value instance.Instance, err error, imported bool) {
	d := &s.directories
	if d.closed || d.generation != generation {
		return
	}
	okay := s.finishDirectoryOperation(generation, err)
	s.refreshLibrary(false)
	s.refreshSessions()
	if okay {
		s.publishEditedInstance(value)
		if s.library.selectedID == value.ID {
			s.cancelAssetArtwork()
			s.refreshAssets()
		}
		message := "迁移完成，旧副本已保留，可在目录任务中单独清理"
		if imported {
			message = "游戏资料已复制；运行环境仍需通过实例安装或修复补齐"
		}
		s.Notify(ToastInfo, message)
		if s.instanceEditor != nil && s.instanceEditor.id == value.ID {
			s.loadInstanceSettings(value)
		}
	}
	d.plan = nil
	d.mode = "tasks"
	if s.page == directoryPage {
		s.loadDirectoryOperations()
	}
}
func (s *Shell) loadDirectoryOperations() {
	d := &s.directories
	if d.busy || d.actions.List == nil {
		return
	}
	generation := s.startDirectoryOperation("list", false)
	d.cancel = d.actions.List(func(rows []app.DirectoryOperation, err error) {
		if s.finishDirectoryOperation(generation, err) {
			d.operations = slices.Clone(rows)
		}
	})
}
func (s *Shell) resumeDirectory(operation app.DirectoryOperation) {
	d := &s.directories
	if d.busy || d.actions.Resume == nil {
		return
	}
	generation := s.startDirectoryOperation("resume", true)
	d.instanceID = operation.InstanceID
	d.cancel = d.actions.Resume(operation, s.directoryProgress(generation), func(value instance.Instance, err error) {
		s.directoryCompleted(generation, value, err, operation.Kind == "import")
	})
}
func (s *Shell) directoryMaintenance(operation app.DirectoryOperation, clean bool) {
	d := &s.directories
	if d.busy {
		return
	}
	action := d.actions.Abort
	if clean {
		action = d.actions.Clean
	}
	if action == nil {
		return
	}
	generation := s.startDirectoryOperation("maintenance", true)
	d.instanceID = operation.InstanceID
	d.cancel = action(operation, func(err error) {
		if d.closed || d.generation != generation {
			return
		}
		okay := s.finishDirectoryOperation(generation, err)
		if okay {
			s.Notify(ToastInfo, "目录任务已处理；文件仅移入系统废纸篓")
			s.refreshLibrary(false)
		}
		if s.page == directoryPage {
			s.loadDirectoryOperations()
		}
	})
}

func (s *Shell) openDirectoryBackup(operation app.DirectoryOperation) {
	d := &s.directories
	if d.busy || d.actions.Open == nil {
		return
	}
	generation := s.startDirectoryOperation("open-backup", false)
	d.cancel = d.actions.Open(operation, func(err error) { s.finishDirectoryOperation(generation, err) })
}
