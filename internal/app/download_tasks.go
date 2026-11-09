package app

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"

	"mcl/internal/core/archive"
	"mcl/internal/core/diagnostics"
	"mcl/internal/core/download"
	"mcl/internal/core/instance"
	"mcl/internal/core/mods"
	"mcl/internal/core/tasks"
)

type DownloadTaskRepository interface {
	Create(context.Context, tasks.Group) error
	Save(context.Context, tasks.Group) error
	Get(context.Context, string) (tasks.Group, error)
	List(context.Context) ([]tasks.Group, error)
	Lock(context.Context, string) (io.Closer, error)
	Remove(context.Context, string) error
}
type TaskReplay func(context.Context, tasks.Intent) error
type TaskService struct {
	root    context.Context
	store   DownloadTaskRepository
	mu      sync.Mutex
	live    map[string]*taskExecution
	changed chan struct{}
	replay  TaskReplay
	cached  []tasks.Group
}
type taskExecution struct {
	mu       sync.Mutex
	group    tasks.Group
	indices  map[string]int
	paths    map[string]string
	cancel   context.CancelCauseFunc
	request  string
	wake     chan struct{}
	settled  chan struct{}
	finished chan struct{}
	lastSave time.Time
	saveErr  error
}
type taskContextKey struct{}
type taskContext struct {
	service   *TaskService
	execution *taskExecution
}

func NewTaskService(root context.Context, store DownloadTaskRepository) *TaskService {
	return &TaskService{root: root, store: store, live: map[string]*taskExecution{}, changed: make(chan struct{}, 1)}
}
func (s *TaskService) SetReplay(replay TaskReplay) { s.mu.Lock(); s.replay = replay; s.mu.Unlock() }
func (s *TaskService) Changed() <-chan struct{}    { return s.changed }
func (s *TaskService) notify() {
	select {
	case s.changed <- struct{}{}:
	default:
	}
}
func taskScope(ctx context.Context) *taskContext {
	value, _ := ctx.Value(taskContextKey{}).(*taskContext)
	return value
}
func TaskAttempt(ctx context.Context) uint64 {
	if scope := taskScope(ctx); scope != nil {
		scope.execution.mu.Lock()
		defer scope.execution.mu.Unlock()
		return scope.execution.group.Attempt
	}
	return 0
}
func (s *TaskService) Execute(ctx context.Context, intent tasks.Intent, work func(context.Context) error) error {
	if s == nil || taskScope(ctx) != nil {
		return work(ctx)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	payload, err := safeTaskPayload(intent.Payload)
	if err != nil {
		return err
	}
	intent.Payload = payload
	group := tasks.Group{Schema: 2, ID: instance.NewID(), Intent: intent, State: "queued", Stage: "准备", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	lease, err := s.store.Lock(ctx, group.ID)
	if err != nil {
		return err
	}
	defer lease.Close()
	if err := s.store.Create(ctx, group); err != nil {
		return err
	}
	return s.execute(ctx, group, work)
}
func (s *TaskService) execute(ctx context.Context, group tasks.Group, work func(context.Context) error) error {
	execution := &taskExecution{group: group, indices: map[string]int{}, paths: map[string]string{}, wake: make(chan struct{}, 1), settled: make(chan struct{}), finished: make(chan struct{})}
	s.mu.Lock()
	if len(s.live) >= 32 {
		s.mu.Unlock()
		group.State = "failed"
		group.Reason = "同时运行或暂停的任务过多，请先处理现有任务"
		s.store.Save(context.WithoutCancel(ctx), group)
		return errors.New(group.Reason)
	}
	if s.live[group.ID] != nil {
		s.mu.Unlock()
		return errors.New("任务已在运行")
	}
	s.live[group.ID] = execution
	s.mu.Unlock()
	defer func() { s.mu.Lock(); delete(s.live, group.ID); s.mu.Unlock(); close(execution.finished); s.notify() }()
	for {
		execution.mu.Lock()
		if execution.request == "cancel" || ctx.Err() != nil {
			execution.group.State = "canceled"
			execution.group.Reason = "用户取消"
			if s.root.Err() != nil {
				execution.group.State = "interrupted"
				execution.group.Reason = "上次工作中断，可继续"
			}
			err := s.saveLocked(execution)
			execution.mu.Unlock()
			if err != nil {
				return err
			}
			return context.Canceled
		}
		execution.group.State = "running"
		execution.group.Stage = "准备"
		execution.group.Reason = ""
		execution.group.Attempt++
		execution.group.Schema = 2
		execution.group.Evidence = diagnostics.Snapshot{}
		attemptNumber := execution.group.Attempt
		execution.group.Owned = true
		execution.request = ""
		execution.indices = map[string]int{}
		execution.paths = map[string]string{}
		for index := range execution.group.Files {
			file := &execution.group.Files[index]
			file.State = "pending"
			file.Verified = 0
			execution.indices[file.Key] = index
			execution.paths[archive.Key(file.Path)] = file.Key
		}
		attempt, cancel := context.WithCancelCause(ctx)
		execution.cancel = cancel
		execution.settled = make(chan struct{})
		execution.saveErr = nil
		saveErr := s.saveLocked(execution)
		execution.mu.Unlock()
		s.notify()
		if saveErr != nil {
			cancel(saveErr)
			execution.mu.Lock()
			close(execution.settled)
			execution.mu.Unlock()
			return saveErr
		}
		attempt = context.WithValue(attempt, taskContextKey{}, &taskContext{s, execution})
		attempt = download.WithObserver(attempt, func(event download.Event) error { return s.event(execution, event, attemptNumber) })
		attempt = diagnostics.WithObserver(attempt, func(update diagnostics.Update) {
			execution.mu.Lock()
			defer execution.mu.Unlock()
			if execution.group.Attempt != attemptNumber || execution.cancel == nil {
				return
			}
			execution.group.Evidence.Apply(update)
			// Evidence follows existing durable checkpoints. Observing work must not
			// create an extra disk-failure/cancellation path or change worker admission.
			s.notify()
		})
		if admit, ok := ctx.Value(taskAdmissionKey{}).(func(error)); ok {
			admit(nil)
		}
		err := work(attempt)
		cancel(nil)
		execution.mu.Lock()
		requested := execution.request
		if execution.saveErr != nil {
			err = execution.saveErr
		}
		switch {
		case err == nil:
			execution.group.State = "completed"
			execution.group.Stage = "完成"
			execution.group.Reason = ""
		case s.root.Err() != nil:
			execution.group.State = "interrupted"
			execution.group.Reason = "启动器已退出，可继续未完成工作"
		case requested == "pause" && execution.saveErr == nil && !errors.Is(err, download.ErrCheckpoint):
			execution.group.State = "paused"
			execution.group.Reason = ""
		case requested == "cancel" || ctx.Err() != nil:
			execution.group.State = "canceled"
			execution.group.Reason = "用户取消"
		default:
			execution.group.State = "failed"
			execution.group.Reason = taskReason(err)
		}
		execution.group.Evidence.Finish(diagnostics.Code(err))
		for index := range execution.group.Files {
			file := &execution.group.Files[index]
			if file.State != "done" {
				file.State = execution.group.State
				file.Verified = 0
			}
		}
		persistErr := s.saveLocked(execution)
		state := execution.group.State
		execution.cancel = nil
		close(execution.settled)
		execution.mu.Unlock()
		s.notify()
		if persistErr != nil {
			return persistErr
		}
		if state == "completed" {
			return nil
		}
		if state != "paused" {
			return err
		}
		// Pause completes only after work has joined its children, released instance
		// leases, synced offsets, and persisted Paused. The waiting caller owns no
		// instance or transfer lease and cannot launch a replacement worker early.
		select {
		case <-ctx.Done():
			execution.mu.Lock()
			if s.root.Err() != nil {
				execution.group.State = "paused"
			} else {
				execution.group.State = "canceled"
				execution.group.Reason = "用户取消"
			}
			persistErr = s.saveLocked(execution)
			execution.mu.Unlock()
			s.notify()
			if persistErr != nil {
				return persistErr
			}
			return ctx.Err()
		case <-execution.wake:
			execution.mu.Lock()
			command := execution.request
			execution.mu.Unlock()
			if command == "cancel" {
				execution.mu.Lock()
				execution.group.State = "canceled"
				execution.group.Reason = "用户取消"
				persistErr = s.saveLocked(execution)
				execution.mu.Unlock()
				s.notify()
				if persistErr != nil {
					return persistErr
				}
				return tasks.ErrCanceled
			}
		}
	}
}
func (s *TaskService) saveLocked(execution *taskExecution) error {
	execution.group.UpdatedAt = time.Now().UTC()
	execution.lastSave = time.Now()
	err := s.store.Save(context.WithoutCancel(s.root), execution.group)
	if err != nil {
		execution.saveErr = err
		execution.group.Reason = "无法保存任务恢复记录，任务已停止"
		if execution.cancel != nil {
			execution.cancel(err)
		}
	}
	if err == nil {
		s.cacheGroup(execution.group)
	}
	return err
}
func (s *TaskService) event(execution *taskExecution, event download.Event, attempt uint64) error {
	execution.mu.Lock()
	defer execution.mu.Unlock()
	if execution.group.Attempt != attempt || execution.cancel == nil {
		return context.Canceled
	}
	if execution.saveErr != nil {
		return execution.saveErr
	}
	if event.Key == "" {
		if event.State == "checkpoint-failed" {
			execution.saveErr = download.ErrCheckpoint
			if execution.cancel != nil {
				execution.cancel(download.ErrCheckpoint)
			}
			return execution.saveErr
		}
		if event.State == "发布" && execution.group.State != "running" {
			return context.Canceled
		}
		execution.group.Stage = event.State
		if err := s.saveLocked(execution); err != nil {
			return err
		}
		s.notify()
		return nil
	}
	pathKey := archive.Key(event.Path)
	if fixed, ok := execution.paths[pathKey]; ok && fixed != event.Key {
		return errors.New("固定任务的目标文件内容身份已变化，请重新确认")
	}
	execution.paths[pathKey] = event.Key
	index, found := execution.indices[event.Key]
	if !found {
		if len(execution.group.Files) >= tasks.MaxFiles {
			return errors.New("单组下载任务超过110000个文件")
		}
		index = len(execution.group.Files)
		execution.indices[event.Key] = index
		execution.group.Files = append(execution.group.Files, tasks.File{Key: event.Key, Path: event.Path, Total: event.Total})
	}
	file := &execution.group.Files[index]
	if event.State == "pending" && file.State == "done" {
		return nil
	}
	if event.Received > file.Received || event.Verified > file.Verified || event.State == "done" && file.State != "done" {
		execution.group.Evidence.LastWorkAt = time.Now().UTC()
	}
	file.State, file.Received, file.Verified = event.State, event.Received, event.Verified
	if event.State == "downloading" {
		execution.group.Stage = "下载"
	}
	if event.State == "verifying" {
		execution.group.Stage = "校验下载文件"
	}
	if time.Since(execution.lastSave) >= time.Second {
		if err := s.saveLocked(execution); err != nil {
			return err
		}
	}
	s.notify()
	return nil
}

// TaskStage is a checkpoint, not a timer-based progress estimate. Publication
// is never claimed complete until its owning service returns success.
func TaskStage(ctx context.Context, stage string) error {
	scope := taskScope(ctx)
	if scope == nil {
		return ctx.Err()
	}
	e := scope.execution
	e.mu.Lock()
	defer e.mu.Unlock()
	if len(stage) > 160 {
		return errors.New("任务阶段名称过长")
	}
	if e.saveErr != nil {
		return e.saveErr
	}
	e.group.Stage = stage
	err := scope.service.saveLocked(e)
	scope.service.notify()
	if err != nil {
		return err
	}
	return ctx.Err()
}
func (s *TaskService) Control(ctx context.Context, id, command string) error {
	if command != "pause" && command != "cancel" {
		return errors.New("未知任务操作")
	}
	s.mu.Lock()
	execution := s.live[id]
	s.mu.Unlock()
	if execution == nil {
		if command == "pause" {
			return errors.New("此任务未在当前启动器运行")
		}
		lease, err := s.store.Lock(ctx, id)
		if err != nil {
			return errors.New("任务正在其他启动器运行，不能更改")
		}
		defer lease.Close()
		group, err := s.store.Get(ctx, id)
		if err != nil {
			return err
		}
		if group.Terminal() {
			return errors.New("任务已经结束")
		}
		group.State = "canceled"
		group.Reason = "用户取消"
		group.UpdatedAt = time.Now().UTC()
		err = s.store.Save(ctx, group)
		if err == nil {
			s.cacheGroup(group)
		}
		s.notify()
		return err
	}
	execution.mu.Lock()
	if execution.group.Terminal() {
		execution.mu.Unlock()
		return errors.New("任务已经结束")
	}
	if command == "pause" && execution.group.Stage == "发布" {
		execution.mu.Unlock()
		return errors.New("正在安全发布，请稍后再暂停")
	}
	if command == "pause" && execution.group.State != "running" {
		execution.mu.Unlock()
		return errors.New("此任务不能再次暂停")
	}
	execution.request = command
	state := "pausing"
	cause := tasks.ErrPaused
	if command == "cancel" {
		state = "canceling"
		cause = tasks.ErrCanceled
	}
	execution.group.State = state
	err := s.saveLocked(execution)
	cancel, settled := execution.cancel, execution.settled
	finished := execution.finished
	if cancel != nil {
		cancel(cause)
	} else {
		select {
		case execution.wake <- struct{}{}:
		default:
		}
	}
	execution.mu.Unlock()
	s.notify()
	if err != nil {
		return err
	}
	if cancel != nil {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-settled:
		}
		execution.mu.Lock()
		saveErr := execution.saveErr
		execution.mu.Unlock()
		if saveErr != nil {
			return saveErr
		}
	} else if command == "cancel" {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-finished:
		}
	}
	return nil
}
func (s *TaskService) Resume(ctx context.Context, id string) error {
	s.mu.Lock()
	execution := s.live[id]
	replay := s.replay
	s.mu.Unlock()
	if execution != nil {
		execution.mu.Lock()
		defer execution.mu.Unlock()
		if execution.group.State != "paused" || execution.request == "resume" {
			return errors.New("暂停尚未完成或任务已恢复")
		}
		execution.request = "resume"
		execution.group.State = "queued"
		if err := s.saveLocked(execution); err != nil {
			return err
		}
		s.notify()
		select {
		case execution.wake <- struct{}{}:
		default:
		}
		return nil
	}
	lease, err := s.store.Lock(ctx, id)
	if err != nil {
		return errors.New("此任务正在其他启动器运行")
	}
	defer lease.Close()
	group, err := s.store.Get(ctx, id)
	if err != nil {
		return err
	}
	group.Intent.Payload, err = safeTaskPayload(group.Intent.Payload)
	if err != nil {
		return err
	}
	if group.State == "completed" || group.State == "canceled" {
		return errors.New("此任务已经结束")
	}
	if replay == nil {
		return errors.New("任务恢复服务未配置")
	}
	return s.execute(ctx, group, func(task context.Context) error { return replay(task, group.Intent) })
}
func (s *TaskService) List(ctx context.Context) ([]tasks.Group, error) {
	groups, err := s.store.List(ctx)
	if err != nil {
		return nil, err
	}
	for index := range groups {
		group := &groups[index]
		s.mu.Lock()
		local := s.live[group.ID] != nil
		s.mu.Unlock()
		if local {
			continue
		}
		lease, err := s.store.Lock(ctx, group.ID)
		if err != nil {
			group.Remote = true
			continue
		}
		lease.Close()
		if !group.Terminal() && group.State != "paused" {
			group.State = "interrupted"
			group.Reason = "上次工作中断，可继续"
			group.Evidence.Finish("interrupted")
		}
	}
	s.mu.Lock()
	s.cached = groups
	s.mu.Unlock()
	return s.Snapshot(), ctx.Err()
}
func (s *TaskService) Snapshot() []tasks.Group {
	s.mu.Lock()
	groups := slices.Clone(s.cached)
	live := make(map[string]*taskExecution, len(s.live))
	for id, value := range s.live {
		live[id] = value
	}
	s.mu.Unlock()
	for index := range groups {
		groups[index].Evidence = groups[index].Evidence.Clone()
		if current := live[groups[index].ID]; current != nil {

			current.mu.Lock()
			groups[index] = current.group.Summary()
			current.mu.Unlock()
			groups[index].Owned = true
			delete(live, groups[index].ID)
		}
	}
	for _, current := range live {
		current.mu.Lock()
		group := current.group.Summary()
		current.mu.Unlock()
		group.Owned = true
		groups = append(groups, group)
	}
	slices.SortFunc(groups, func(a, b tasks.Group) int { return b.CreatedAt.Compare(a.CreatedAt) })
	return groups
}
func (s *TaskService) cacheGroup(group tasks.Group) {
	summary := group.Summary()
	summary.Owned = false
	summary.Remote = false
	s.mu.Lock()
	defer s.mu.Unlock()
	for index := range s.cached {
		if s.cached[index].ID == group.ID {
			s.cached[index] = summary
			return
		}
	}
	s.cached = append(s.cached, summary)
}
func (s *TaskService) Remove(ctx context.Context, id string) error {
	lease, err := s.store.Lock(ctx, id)
	if err != nil {
		return errors.New("任务仍被占用")
	}
	defer lease.Close()
	err = s.store.Remove(ctx, id)
	if err == nil {
		s.mu.Lock()
		s.cached = slices.DeleteFunc(s.cached, func(group tasks.Group) bool { return group.ID == id })
		s.mu.Unlock()
	}
	s.notify()
	return err
}
func cloneTaskGroup(group tasks.Group) tasks.Group {
	group.Evidence = group.Evidence.Clone()
	group.Identities = maps.Clone(group.Identities)
	group.Files = slices.Clone(group.Files)
	group.Intent.Payload = slices.Clone(group.Intent.Payload)
	return group
}
func performTask[T any](ctx context.Context, service *TaskService, intent tasks.Intent, work func(context.Context) (T, error)) (T, error) {
	var result T
	err := service.Execute(ctx, intent, func(task context.Context) error { value, err := work(task); result = value; return err })
	return result, err
}
func taskPayload(value any) json.RawMessage { data, _ := json.Marshal(value); return data }

type taskAdmissionKey struct{}

func (s *TaskService) ResumeWithStart(ctx context.Context, id string, started func(error)) error {
	var once sync.Once
	admit := func(err error) { once.Do(func() { started(err) }) }
	ctx = context.WithValue(ctx, taskAdmissionKey{}, admit)
	err := s.Resume(ctx, id)
	admit(err)
	return err
}
func (s *TaskService) TrackDownloads(ctx context.Context, files []download.File, work func(context.Context) error) error {
	return s.Execute(ctx, tasks.Intent{Kind: "files", Name: "下载资源文件", Payload: taskPayload(files)}, work)
}
func (s *TaskService) Details(ctx context.Context, id string) (tasks.Group, error) {
	s.mu.Lock()
	execution := s.live[id]
	s.mu.Unlock()
	if execution != nil {
		execution.mu.Lock()
		group := cloneTaskGroup(execution.group)
		execution.mu.Unlock()
		group.Intent.Payload = nil
		return group, nil
	}
	group, err := s.store.Get(ctx, id)
	if err == nil && !group.Terminal() && group.State != "paused" {
		if lease, lockErr := s.store.Lock(ctx, id); lockErr == nil {
			lease.Close()
			group.State = "interrupted"
			group.Reason = "上次工作中断，可继续"
			group.Evidence.Finish("interrupted")
		} else {
			group.Remote = true
		}
	}
	group.Intent.Payload = nil
	return group, err
}
func (s *TaskService) ClearHistory(ctx context.Context) error {
	groups, err := s.List(ctx)
	if err != nil {
		return err
	}
	for _, group := range groups {
		if group.Terminal() && !group.Remote {
			if err := s.Remove(ctx, group.ID); err != nil {
				return err
			}
		}
	}
	return nil
}
func taskReason(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, mods.ErrUnsafeChange) {
		return "模组声明检查未通过，未应用文件变更；请核对模组文件与依赖后重试"
	}
	if errors.Is(err, download.ErrCheckpoint) {
		return "下载检查点未能保存，请检查磁盘空间后重试；任务尚未安全暂停"
	}
	switch diagnostics.Code(err) {
	case "deadline":
		return "等待预算已到期，请核对诊断中的当前步骤后重试"
	case "unauthorized":
		return "来源授权未通过（HTTP 401），请核对授权后重试"
	case "forbidden":
		return "来源拒绝访问（HTTP 403），请核对来源与网络设置"
	case "not-found":
		return "固定对象或校验元数据不存在（HTTP 404/410），请核对任务诊断"
	case "rate-limited":
		return "来源请求频率受限，请稍后重试"
	case "integrity":
		return "文件校验未通过，未发布无效内容"
	case "disk-full":
		return "磁盘空间不足，未完成内容已保留供恢复"
	case "permission":
		return "访问被拒绝，请核对受管目录权限"
	}
	value := err.Error()
	switch {
	case strings.Contains(value, "Key") || strings.Contains(value, "api key") || strings.Contains(value, "API key"):
		return "来源授权不可用，请重新配置会话Key后继续"
	case strings.Contains(value, "未完成的资产更新"):
		return "请先在实例资产页恢复未完成的更新"
	case strings.Contains(value, "目录操作") || strings.Contains(value, "删除"):
		return "请先处理实例的目录或删除事务"
	case strings.Contains(value, "已改变") || strings.Contains(value, "已变化"):
		return "固定计划或目标已变化，请返回原功能重新确认"
	case strings.Contains(value, "SHA") || strings.Contains(value, "摘要") || strings.Contains(value, "校验"):
		return "文件校验未通过，未发布无效内容"
	case strings.Contains(value, "分片") || strings.Contains(value, "续传") || strings.Contains(value, "Content-Range"):
		return "分片状态不符，请核对或清理分片后重试"
	case strings.Contains(value, "HTTP") || strings.Contains(value, "网络") || strings.Contains(value, "request") || strings.Contains(value, "timeout"):
		return "网络请求未完成，可重试"
	default:
		return "执行未完成，请重试或核对对应实例"
	}
}

func TaskIdentity(ctx context.Context, key, value string) error {
	scope := taskScope(ctx)
	if scope == nil {
		return ctx.Err()
	}
	e := scope.execution
	e.mu.Lock()
	defer e.mu.Unlock()
	if old, ok := e.group.Identities[key]; ok {
		if old != value {
			return errors.New("固定任务的官方继承链或来源身份已变化，请重新确认")
		}
		return ctx.Err()
	}
	if e.group.Identities == nil {
		e.group.Identities = map[string]string{}
	}
	e.group.Identities[key] = value
	return scope.service.saveLocked(e)
}
