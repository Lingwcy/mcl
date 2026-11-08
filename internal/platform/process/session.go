// Package process executes launch plans and owns their process/output lifetime.
package process

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"mcl/internal/core/java"
	"mcl/internal/core/launch"
	"mcl/internal/core/meta"
)

type Exit = launch.Exit

// Session is independent of preparation contexts. Wait is repeatable; Stop is
// explicit. The application can stop observing/detach during quit without
// terminating Java. An internal waiter reaps the child on its natural exit.
type Session struct {
	warnings             []string
	cmd                  *exec.Cmd
	output               *launch.Output
	readers              [2]*os.File
	readDone, done       chan struct{}
	root                 *os.Root
	argumentPath         string
	redact               func(string) string
	mu                   sync.Mutex
	control              controller
	stopping, detached   bool
	exit                 Exit
	stopOnce, detachOnce sync.Once
	readFailed           atomic.Bool
}

type controller interface {
	commit() error
	terminate() error
	kill() error
	close()
}

// Start writes only JVM options to a private argument file. Game arguments,
// including the access token, are passed directly to exec, never a shell.
func Start(plan launch.Plan) (*Session, error) {
	return start(plan, nil)
}

// StartTracked commits identity before Java may execute. The registration
// callback runs under the caller's instance reservation. A callback failure
// aborts the still-gated/suspended child; it never becomes an untracked JVM.
func StartTracked(plan launch.Plan, register func(launch.ProcessIdentity) error, directory ...*os.Root) (*Session, error) {
	if register == nil {
		return nil, errors.New("missing game identity registration")
	}
	return start(plan, register, directory...)
}

func start(plan launch.Plan, register func(launch.ProcessIdentity) error, directory ...*os.Root) (*Session, error) {
	if register == nil && plan.Priority() != launch.PriorityNormal {
		return nil, errors.New("自定义游戏优先级需要受控身份交接")
	}
	data, err := plan.JVMArgumentFile()
	if err != nil {
		return nil, err
	}
	if target, current := plan.Target(), meta.CurrentPlatform(); target.OS != current.OS || target.Arch != current.Arch {
		return nil, errors.New("启动计划与当前系统或架构不匹配")
	}
	if !filepath.IsAbs(plan.Executable()) || !filepath.IsAbs(plan.Directory()) {
		return nil, errors.New("invalid process paths")
	}
	root, err := os.OpenRoot(plan.DataRoot())
	if err != nil {
		return nil, fmt.Errorf("open launch data: %w", err)
	}
	committed := false
	argumentPath := path.Join(plan.InstancePath(), "runtime", "jvm-"+rand.Text()+".args")
	defer func() {
		if !committed {
			root.Remove(argumentPath)
			root.Close()
		}
	}()
	// Confine the working directory, too, before passing its absolute path to
	// exec. Preparation holds the instance exclusion against app mutations.
	var game *os.Root
	if len(directory) > 0 && directory[0] != nil {
		// The production adapter supplies an ownership-validated game root.
		game, err = directory[0].OpenRoot(".")
	} else {
		expected := filepath.Join(plan.DataRoot(), filepath.FromSlash(plan.InstancePath()), "game")
		if filepath.Clean(plan.Directory()) != expected {
			return nil, errors.New("外部游戏工作目录尚未经过所有权检查")
		}
		game, err = root.OpenRoot(path.Join(plan.InstancePath(), "game"))
	}
	if err != nil {
		return nil, err
	}
	defer game.Close()
	opened, err := game.Stat(".")
	if err != nil {
		return nil, err
	}
	current, err := os.Lstat(plan.Directory())
	if err != nil {
		return nil, err
	}
	if !current.IsDir() || current.Mode()&os.ModeSymlink != 0 || !os.SameFile(opened, current) {
		return nil, errors.New("游戏工作目录发生变化，已取消启动")
	}
	if err := root.MkdirAll(path.Dir(argumentPath), 0o700); err != nil {
		return nil, err
	}
	f, err := root.OpenFile(argumentPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return nil, err
	}
	if err := f.Close(); err != nil {
		return nil, err
	}
	args := plan.ProcessArguments(filepath.Join(plan.DataRoot(), filepath.FromSlash(argumentPath)))
	if len(args) == 0 {
		return nil, errors.New("empty process arguments")
	}
	cmd, resume, closeGate, err := startCommand(plan.Executable(), args, register != nil)
	if err != nil {
		return nil, err
	}
	defer closeGate()
	cmd.Dir, cmd.Env = plan.Directory(), java.Environment(os.Environ())
	// A self-executed debug gate must not create or replace its parent's
	// MyGo automation listener before it reaches GateMain.
	environment := cmd.Env[:0]
	for _, entry := range cmd.Env {
		if key, _, _ := strings.Cut(entry, "="); key != "MYGO_AUTOMATION" {
			environment = append(environment, entry)
		}
	}
	cmd.Env = environment
	configure(cmd)
	s := &Session{cmd: cmd, root: root, argumentPath: argumentPath, output: plan.NewOutput(), redact: plan.Redact, readDone: make(chan struct{}), done: make(chan struct{})}
	var writes [2]*os.File
	for i := range 2 {
		r, w, err := os.Pipe()
		if err != nil {
			for j := range i {
				s.readers[j].Close()
				writes[j].Close()
			}
			return nil, err
		}
		s.readers[i], writes[i] = r, w
	}
	cmd.Stdout, cmd.Stderr = writes[0], writes[1]
	if err := cmd.Start(); err != nil {
		for i := range 2 {
			s.readers[i].Close()
			writes[i].Close()
		}
		return nil, fmt.Errorf("启动 Java 失败: %s", plan.Redact(err.Error()))
	}
	for _, w := range writes {
		w.Close()
	}
	s.control, err = attach(cmd, func() error {
		warning, err := prepareChildPriority(cmd.Process.Pid, plan.Priority(), register, Inspect, setChildPriority)
		if warning != "" {
			s.warnings = []string{warning}
			writer := s.output.Stream("launcher")
			fmt.Fprintln(writer, warning)
			writer.Close()
		}
		return err
	})
	if err == nil {
		err = resume()
	}
	if err == nil {
		err = s.control.commit()
	}
	if err != nil {
		if s.control != nil {
			s.control.kill()
			s.control.close()
		}
		cmd.Process.Kill()
		cmd.Wait()
		for _, r := range s.readers {
			r.Close()
		}
		return nil, fmt.Errorf("建立进程会话失败: %s", plan.Redact(err.Error()))
	}
	s.exit.StartedAt = time.Now().UTC()
	var readers sync.WaitGroup
	for i, name := range []string{"stdout", "stderr"} {
		readers.Go(func() {
			defer s.readers[i].Close()
			writer := s.output.Stream(name)
			if _, err := io.Copy(writer, s.readers[i]); err != nil && !errors.Is(err, os.ErrClosed) {
				s.readFailed.Store(true)
			}
			writer.Close()
		})
	}
	go func() { readers.Wait(); close(s.readDone) }()
	committed = true
	go s.wait()
	return s, nil
}

func (s *Session) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return fmt.Sprintf("ProcessSession{pid:%d,finished:%t}", s.PID(), !s.exit.FinishedAt.IsZero())
}
func (s *Session) GoString() string { return s.String() }

func (s *Session) StartupWarnings() []string { return slices.Clone(s.warnings) }

func (s *Session) PID() int                      { return s.cmd.Process.Pid }
func (s *Session) Output() launch.OutputSnapshot { return s.output.Snapshot() }
func (s *Session) Done() <-chan struct{}         { return s.done }
func (s *Session) Changed() <-chan struct{}      { return s.output.Changed() }

func (s *Session) Wait() Exit {
	<-s.done
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.exit
}

func (s *Session) wait() {
	err := s.cmd.Wait()
	timer := time.NewTimer(2 * time.Second)
	incomplete := false
	select {
	case <-s.readDone:
		timer.Stop()
	case <-timer.C:
		incomplete = true
		for _, r := range s.readers {
			r.Close()
		}
		<-s.readDone
	}
	s.mu.Lock()
	if s.stopping {
		s.control.kill()
	}
	s.control.close()
	s.control = nil
	s.exit.Code = s.cmd.ProcessState.ExitCode()
	if err != nil {
		s.exit.Error = s.redact(err.Error())
	}
	s.exit.FinishedAt, s.exit.Stopped, s.exit.Detached, s.exit.OutputIncomplete = time.Now().UTC(), s.stopping, s.detached, incomplete || s.detached || s.readFailed.Load()
	s.mu.Unlock()
	s.root.Remove(s.argumentPath)
	s.root.Close()
	close(s.done)
}

// Stop requests group termination once, escalates after two seconds and waits
// for reaping/output cleanup. A canceled wait also escalates but does not undo
// the stop request. It never uses a preparation context as a session lifetime.
func (s *Session) Stop(ctx context.Context) error {
	s.stopOnce.Do(func() {
		s.mu.Lock()
		if s.control == nil {
			s.mu.Unlock()
			return
		}
		s.stopping = true
		s.control.terminate()
		s.mu.Unlock()
		go func() {
			timer := time.NewTimer(2 * time.Second)
			defer timer.Stop()
			select {
			case <-s.done:
				return
			case <-timer.C:
				s.forceKill()
			}
		}()
	})
	select {
	case <-s.done:
		return nil
	case <-ctx.Done():
		s.forceKill()
		return ctx.Err()
	}
}

func (s *Session) forceKill() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.control != nil {
		s.control.kill()
	}
}

// Detach closes this launcher's log readers without killing the JVM. The
// application must not wait for the game's natural exit during its own quit.
// Session.Wait remains available for callers that continue observing it.
func (s *Session) Detach() {
	s.detachOnce.Do(func() {
		s.mu.Lock()
		s.detached = true
		s.mu.Unlock()
		for _, r := range s.readers {
			r.Close()
		}
		<-s.readDone
	})
}
