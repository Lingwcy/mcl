package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"mcl/internal/core/download"
	"mcl/internal/core/tasks"
)

type taskMemory struct {
	mu        sync.Mutex
	groups    map[string]tasks.Group
	locks     map[string]bool
	failState string
}
type taskCloser struct {
	once  sync.Once
	close func()
}

func (c *taskCloser) Close() error { c.once.Do(c.close); return nil }
func newTaskMemory() *taskMemory {
	return &taskMemory{groups: map[string]tasks.Group{}, locks: map[string]bool{}}
}
func (m *taskMemory) Lock(ctx context.Context, id string) (io.Closer, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.locks[id] {
		return nil, errors.New("locked")
	}
	m.locks[id] = true
	return &taskCloser{close: func() { m.mu.Lock(); delete(m.locks, id); m.mu.Unlock() }}, nil
}
func (m *taskMemory) Save(ctx context.Context, g tasks.Group) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if g.State == m.failState {
		return errors.New("disk full")
	}
	if err := g.Validate(); err != nil {
		return err
	}
	m.groups[g.ID] = cloneTaskGroup(g)
	return nil
}
func (m *taskMemory) Create(ctx context.Context, g tasks.Group) error { return m.Save(ctx, g) }
func (m *taskMemory) Get(ctx context.Context, id string) (tasks.Group, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	g, ok := m.groups[id]
	if !ok {
		return g, errors.New("missing")
	}
	return cloneTaskGroup(g), nil
}
func (m *taskMemory) List(context.Context) ([]tasks.Group, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []tasks.Group
	for _, g := range m.groups {
		out = append(out, g.Summary())
	}
	return out, nil
}
func (m *taskMemory) Remove(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.groups, id)
	return nil
}
func taskReceive[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case result := <-ch:
		return result
	case <-time.After(3 * time.Second):
		t.Fatal("task did not settle")
		var zero T
		return zero
	}
}
func taskTestIntent() tasks.Intent {
	return tasks.Intent{Kind: "files", Name: "fixture", Payload: taskPayload([]download.File{})}
}
func TestDownloadTaskPauseWaitsForWorkersBeforeResume(t *testing.T) {
	store := newTaskMemory()
	service := NewTaskService(t.Context(), store)
	started, draining := make(chan struct{}, 2), make(chan struct{}, 1)
	release := make(chan struct{})
	result := make(chan error, 1)
	var active, peak atomic.Int32
	go func() {
		result <- service.Execute(t.Context(), taskTestIntent(), func(ctx context.Context) error {
			n := active.Add(1)
			defer active.Add(-1)
			for old := peak.Load(); n > old && !peak.CompareAndSwap(old, n); old = peak.Load() {
			}
			started <- struct{}{}
			if TaskAttempt(ctx) == 1 {
				<-ctx.Done()
				draining <- struct{}{}
				<-release
				return ctx.Err()
			}
			return nil
		})
	}()
	taskReceive(t, started)
	groups, err := service.List(t.Context())
	if err != nil || len(groups) != 1 {
		t.Fatal(groups, err)
	}
	id := groups[0].ID
	paused := make(chan error, 1)
	go func() { paused <- service.Control(t.Context(), id, "pause") }()
	taskReceive(t, draining)
	if err := service.Resume(t.Context(), id); err == nil {
		t.Fatal("resume accepted while old worker draining")
	}
	select {
	case <-paused:
		t.Fatal("pause acknowledged before workers exited")
	default:
	}
	close(release)
	if err := taskReceive(t, paused); err != nil {
		t.Fatal(err)
	}
	saved, _ := store.Get(t.Context(), id)
	if saved.State != "paused" {
		t.Fatal("paused was not durable", saved.State)
	}
	if err := service.Resume(t.Context(), id); err != nil {
		t.Fatal(err)
	}
	taskReceive(t, started)
	if err := taskReceive(t, result); err != nil {
		t.Fatal(err)
	}
	if peak.Load() != 1 {
		t.Fatal("old and replacement workers overlapped")
	}
	saved, _ = store.Get(t.Context(), id)
	if saved.State != "completed" || saved.Attempt != 2 {
		t.Fatal(saved)
	}
}
func TestDownloadTaskRestartDoesNotAutoReplayAndKeepsStableID(t *testing.T) {
	store := newTaskMemory()
	root, cancel := context.WithCancel(context.Background())
	first := NewTaskService(root, store)
	started := make(chan struct{}, 1)
	done := make(chan error, 1)
	go func() {
		done <- first.Execute(root, taskTestIntent(), func(ctx context.Context) error { started <- struct{}{}; <-ctx.Done(); return ctx.Err() })
	}()
	taskReceive(t, started)
	cancel()
	taskReceive(t, done)
	second := NewTaskService(t.Context(), store)
	calls := 0
	second.SetReplay(func(context.Context, tasks.Intent) error { calls++; return nil })
	groups, err := second.List(t.Context())
	if err != nil || len(groups) != 1 || groups[0].State != "interrupted" {
		t.Fatal(groups, err)
	}
	if calls != 0 {
		t.Fatal("restart auto-replayed work")
	}
	id := groups[0].ID
	if err := second.Resume(t.Context(), id); err != nil {
		t.Fatal(err)
	}
	saved, _ := store.Get(t.Context(), id)
	if saved.ID != id || saved.State != "completed" || calls != 1 {
		t.Fatal(saved, calls)
	}
}
func TestDownloadTaskCheckpointFailureCannotBecomePaused(t *testing.T) {
	store := newTaskMemory()
	service := NewTaskService(t.Context(), store)
	started := make(chan struct{}, 1)
	done := make(chan error, 1)
	go func() {
		done <- service.Execute(t.Context(), taskTestIntent(), func(ctx context.Context) error {
			started <- struct{}{}
			<-ctx.Done()
			download.Observe(ctx, download.Event{State: "checkpoint-failed"})
			return ctx.Err()
		})
	}()
	taskReceive(t, started)
	groups, _ := service.List(t.Context())
	if err := service.Control(t.Context(), groups[0].ID, "pause"); !errors.Is(err, download.ErrCheckpoint) {
		t.Fatal("pause hid failed checkpoint", err)
	}
	if err := taskReceive(t, done); !errors.Is(err, download.ErrCheckpoint) {
		t.Fatal(err)
	}
	saved, _ := store.Get(t.Context(), groups[0].ID)
	if saved.State != "failed" {
		t.Fatal("checkpoint failure reported as paused", saved.State)
	}
}
func TestTaskPayloadStripsSignedURLsPreservesLargeIntegersAndRejectsSecrets(t *testing.T) {
	raw := json.RawMessage(`{"size":9223372036854775806,"URL":"https://cdn.modrinth.com/object?token=private","good":"https://cdn.modrinth.com/file.jar"}`)
	actual, err := safeTaskPayload(raw)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(actual, []byte("private")) || !bytes.Contains(actual, []byte("9223372036854775806")) || !bytes.Contains(actual, []byte("https://cdn.modrinth.com/file.jar")) {
		t.Fatal(string(actual))
	}
	for _, key := range []string{"access_token", "api_key", "password", "Authorization"} {
		raw := taskPayload(map[string]string{key: "secret"})
		if _, err := safeTaskPayload(raw); err == nil {
			t.Fatal("secret accepted", key)
		}
	}
}
func TestDownloadTaskPublishIsNotPausableAndHistoryDoesNotStoreErrors(t *testing.T) {
	store := newTaskMemory()
	service := NewTaskService(t.Context(), store)
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- service.Execute(t.Context(), taskTestIntent(), func(ctx context.Context) error {
			if err := TaskStage(ctx, "发布"); err != nil {
				return err
			}
			started <- struct{}{}
			<-release
			return errors.New("https://private.example/file?token=secret")
		})
	}()
	taskReceive(t, started)
	groups, _ := service.List(t.Context())
	if err := service.Control(t.Context(), groups[0].ID, "pause"); err == nil {
		t.Fatal("pause entered publication")
	}
	close(release)
	taskReceive(t, done)
	saved, _ := store.Get(t.Context(), groups[0].ID)
	data, _ := json.Marshal(saved)
	if bytes.Contains(data, []byte("secret")) {
		t.Fatal("raw error persisted")
	}
}

func TestDownloadTaskRejectsChangedSourceIdentityOnColdResume(t *testing.T) {
	store := newTaskMemory()
	root, stop := context.WithCancel(context.Background())
	service := NewTaskService(root, store)
	started := make(chan struct{}, 1)
	done := make(chan error, 1)
	first := string(bytes.Repeat([]byte("a"), 64))
	second := string(bytes.Repeat([]byte("b"), 64))
	go func() {
		done <- service.Execute(root, taskTestIntent(), func(ctx context.Context) error {
			if err := TaskIdentity(ctx, "mojang:fixture", first); err != nil {
				return err
			}
			started <- struct{}{}
			<-ctx.Done()
			return ctx.Err()
		})
	}()
	taskReceive(t, started)
	stop()
	taskReceive(t, done)
	restarted := NewTaskService(t.Context(), store)
	restarted.SetReplay(func(ctx context.Context, intent tasks.Intent) error {
		return TaskIdentity(ctx, "mojang:fixture", second)
	})
	groups, err := restarted.List(t.Context())
	if err != nil || len(groups) != 1 {
		t.Fatal(groups, err)
	}
	if err := restarted.Resume(t.Context(), groups[0].ID); err == nil {
		t.Fatal("fixed official chain changed silently")
	}
	saved, _ := store.Get(t.Context(), groups[0].ID)
	if saved.Identities["mojang:fixture"] != first || saved.State != "failed" {
		t.Fatal("source identity overwritten", saved)
	}
}
