package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"sync"

	"mcl/internal/app"
	"mcl/internal/core/instance"
	"mcl/internal/core/launch"
	"mcl/internal/fileio"
	"mcl/internal/platform/instancelock"
)

type sessionDocument struct {
	Schema  int           `json:"schema"`
	Session launch.Record `json:"session"`
}
type ProcessRunning func(launch.ProcessIdentity) (bool, error)
type SessionStore struct {
	dir     string
	running ProcessRunning
}

func NewSessionStore(dir string, running ProcessRunning) *SessionStore {
	return &SessionStore{dir: dir, running: running}
}

func (s *SessionStore) Reserve(ctx context.Context, id string) (app.SessionLease, error) {
	return s.reserve(ctx, id, false, false, false)
}

func (s *SessionStore) ReserveForDeletion(ctx context.Context, id string) (app.SessionLease, error) {
	return s.reserve(ctx, id, false, true, false)
}

func (s *SessionStore) ReserveForRecovery(ctx context.Context, id string) (app.SessionLease, error) {
	return s.reserve(ctx, id, true, false, false)
}

func (s *SessionStore) ReserveForDirectoryTransfer(ctx context.Context, id string) (app.SessionLease, error) {
	return s.reserve(ctx, id, false, false, true)
}

func (s *SessionStore) reserve(ctx context.Context, id string, recovery, deleting, directoryTransfer bool) (app.SessionLease, error) {
	if !instance.ValidID(id) {
		return nil, errors.New("invalid instance ID")
	}
	if s.running == nil {
		return nil, errors.New("进程身份核验服务未配置")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(s.dir)
	if err != nil {
		return nil, err
	}
	lock, err := instancelock.Acquire(root, id)
	if err != nil {
		root.Close()
		return nil, fmt.Errorf("实例正被其他启动器操作占用: %w", err)
	}
	fail := func(err error) (app.SessionLease, error) { lock.Close(); root.Close(); return nil, err }
	if !directoryTransfer {
		if _, err := root.Lstat(path.Join("instances", id, directoryPending)); err == nil {
			return fail(errors.New("此实例有未完成的目录操作，请在目录任务中恢复"))
		} else if !errors.Is(err, os.ErrNotExist) {
			return fail(err)
		}
	}
	if !deleting {
		if _, err := root.Lstat(path.Join("instances", id, instanceDeleteMarker)); err == nil {
			return fail(errors.New("此实例有未完成的删除，请在实例设置中继续删除"))
		} else if !errors.Is(err, os.ErrNotExist) {
			return fail(err)
		}
	}
	if !recovery {
		if _, err := root.Lstat(path.Join("instances", id, updatePendingFile)); err == nil {
			return fail(errors.New("此实例有未完成的资产更新，请先在实例页恢复"))
		} else if !errors.Is(err, os.ErrNotExist) {
			return fail(err)
		}
	}
	record, err := readSession(ctx, root, id)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fail(fmt.Errorf("游戏会话记录无法读取，原文件已保留: %w", err))
	}
	if err == nil {
		alive, err := s.running(record.Process)
		if err != nil {
			return fail(fmt.Errorf("无法核验上次游戏会话，暂不能修改实例: %w", err))
		}
		if alive {
			return fail(fmt.Errorf("此实例的游戏仍在运行（PID %d）", record.Process.PID))
		}
		if err := root.Remove(sessionPath(id)); err != nil {
			return fail(err)
		}
	}
	if err := ctx.Err(); err != nil {
		return fail(err)
	}
	// No live session and no competing preparation can use these files now.
	// A launcher crash may have skipped the normal process-exit cleanup.
	if err := cleanupJVMFiles(root, id); err != nil {
		return fail(err)
	}
	return &sessionLease{root: root, lock: lock, id: id, running: s.running}, nil
}

// Active does not modify records while another owner may be updating them.
// Dead records are reclaimed only by Reserve under the instance OS lock.
func (s *SessionStore) Active(ctx context.Context) ([]launch.Record, error) {
	if s.running == nil {
		return nil, errors.New("进程身份核验服务未配置")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(s.dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer root.Close()
	dir, err := root.Open("instances")
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	entries, err := dir.ReadDir(-1)
	if err != nil {
		return nil, err
	}
	var result []launch.Record
	var failures []error
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		if !instance.ValidID(entry.Name()) {
			continue
		}
		record, err := readSession(ctx, root, entry.Name())
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			failures = append(failures, err)
			continue
		}
		alive, err := s.running(record.Process)
		if err != nil {
			failures = append(failures, err)
			continue
		}
		if alive {
			result = append(result, record)
		}
	}
	return result, errors.Join(failures...)
}

type sessionLease struct {
	mu      sync.Mutex
	root    *os.Root
	lock    *instancelock.Lock
	id      string
	running ProcessRunning
	tracked *launch.Record
	closed  bool
	err     error
}

func (l *sessionLease) Track(ctx context.Context, record launch.Record) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed || l.tracked != nil {
		return errors.New("instance reservation already settled")
	}
	if err := record.Validate(); err != nil {
		return err
	}
	if record.InstanceID != l.id {
		return errors.New("session belongs to another instance")
	}
	if _, err := readSession(ctx, l.root, l.id); err == nil {
		return errors.New("session record appeared during reservation; preserving it")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	data, err := json.MarshalIndent(sessionDocument{Schema: 1, Session: record}, "", "  ")
	if err != nil {
		return err
	}
	if err := fileio.WriteAt(ctx, l.root, sessionPath(l.id), append(data, '\n'), 0o600); err != nil {
		return err
	}
	l.tracked = &record
	return nil
}

func (l *sessionLease) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return l.err
	}
	l.closed = true
	if l.tracked != nil {
		alive, err := l.running(l.tracked.Process)
		if err != nil {
			l.err = err
		} else if !alive {
			current, err := readSession(context.Background(), l.root, l.id)
			if err == nil && current.ID == l.tracked.ID {
				l.err = l.root.Remove(sessionPath(l.id))
			} else if err != nil && !errors.Is(err, os.ErrNotExist) {
				l.err = err
			}
		}
	}
	l.err = errors.Join(l.err, l.lock.Close(), l.root.Close())
	return l.err
}

func sessionPath(id string) string { return path.Join("instances", id, "session.json") }

func cleanupJVMFiles(root *os.Root, id string) error {
	directory := path.Join("instances", id, "runtime")
	f, err := root.Open(directory)
	if err != nil {
		return err
	}
	defer f.Close()
	entries, err := f.ReadDir(-1)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		name := entry.Name()
		extension := ".args"
		if strings.HasSuffix(name, ".jar") {
			extension = ".jar"
		}
		if !strings.HasPrefix(name, "jvm-") || !strings.HasSuffix(name, extension) || len(name) != len("jvm-")+26+len(extension) {
			continue
		}
		nonce := strings.TrimSuffix(strings.TrimPrefix(name, "jvm-"), extension)
		if strings.ContainsFunc(nonce, func(r rune) bool { return !(r >= 'A' && r <= 'Z' || r >= '2' && r <= '7') }) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode().IsRegular() {
			if err := root.Remove(path.Join(directory, name)); err != nil {
				return err
			}
		}
	}
	return nil
}

func readSession(ctx context.Context, root *os.Root, id string) (launch.Record, error) {
	if err := ctx.Err(); err != nil {
		return launch.Record{}, err
	}
	f, err := root.Open(sessionPath(id))
	if err != nil {
		return launch.Record{}, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, (64<<10)+1))
	if err != nil {
		return launch.Record{}, err
	}
	if len(data) > 64<<10 {
		return launch.Record{}, errors.New("session record exceeds 64 KiB")
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	var doc sessionDocument
	if err := d.Decode(&doc); err != nil {
		return launch.Record{}, err
	}
	if err := d.Decode(new(any)); !errors.Is(err, io.EOF) {
		return launch.Record{}, errors.New("session record has trailing content")
	}
	if doc.Schema != 1 || doc.Session.InstanceID != id {
		return launch.Record{}, errors.New("session schema or instance mismatch")
	}
	if err := doc.Session.Validate(); err != nil {
		return launch.Record{}, err
	}
	return doc.Session, ctx.Err()
}
