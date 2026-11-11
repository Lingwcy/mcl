package app

import (
	"context"
	"errors"
	"slices"
	"time"

	"mcl/internal/core/diagnostics"
	"mcl/internal/core/launch"
)

const maxSessionHistory = 12

type GameWindowState uint8

const (
	GameWindowUnavailable GameWindowState = iota
	GameWindowPending
	GameWindowVisible
)

// GameSnapshot contains only public process state and already-redacted output.
// Completed snapshots remain bounded; no UI object or credential is retained.
type GameSnapshot struct {
	Evidence                  diagnostics.Snapshot
	DiagnosticFiles           []launch.DiagnosticLog
	DiagnosticFileState       string
	WindowCheckedAt           time.Time
	WindowProbeCode           string
	Warnings                  []string
	ID, InstanceID, VersionID string
	PID                       int
	StartedAt                 time.Time
	Running, Stopping         bool
	Window                    GameWindowState
	Exit                      launch.Exit
	Output                    launch.OutputSnapshot
	Error                     string
}

type managedGame struct {
	files    *sessionLogs
	record   launch.Record
	snapshot GameSnapshot
	game     launch.Game
	redact   func(string) string
}

func (s *LaunchService) Changed() <-chan struct{} { return s.changed }
func (s *LaunchService) notify() {
	select {
	case s.changed <- struct{}{}:
	default:
	}
}

func (s *LaunchService) manage(record launch.Record, game launch.Game, release func(), redact func(string) string, evidence ...diagnostics.Snapshot) {
	s.manageSession(record, game, release, redact, nil, "not-configured", evidence...)
}

func (s *LaunchService) manageSession(record launch.Record, game launch.Game, release func(), redact func(string) string, files *sessionLogs, fileCode string, evidence ...diagnostics.Snapshot) {
	snapshot := GameSnapshot{Warnings: launch.StartupWarnings(game), ID: record.ID, InstanceID: record.InstanceID, VersionID: record.VersionID, PID: game.PID(), StartedAt: time.Now().UTC(), Running: true}
	if len(evidence) > 0 && evidence[0].Validate() == nil {
		snapshot.Evidence = evidence[0].Clone()
	}
	if s.windowVisible != nil {
		snapshot.Window = GameWindowPending
	}
	snapshot.DiagnosticFileState = fileCode
	entry := &managedGame{files: files, record: record, game: game, redact: redact, snapshot: snapshot}
	s.gamesMu.Lock()
	s.games[record.ID] = entry
	s.gamesMu.Unlock()
	s.notify()
	go func() {
		changes := game.Changed()
		var ticks <-chan time.Time
		var watchDone <-chan struct{}
		var ticker *time.Ticker
		if snapshot.Window == GameWindowPending {
			ticker = time.NewTicker(300 * time.Millisecond)
			defer ticker.Stop()
			ticks, watchDone = ticker.C, s.windowWatchDone
		}
		for {
			select {
			case <-watchDone:
				ticker.Stop()
				ticks, watchDone = nil, nil
			case <-ticks:
				visible, err := s.windowVisible(record.Process)
				s.gamesMu.Lock()
				first := entry.snapshot.WindowCheckedAt.IsZero()
				entry.snapshot.WindowCheckedAt = time.Now().UTC()
				entry.snapshot.WindowProbeCode = diagnostics.Code(err)
				s.gamesMu.Unlock()
				if first {
					s.notify()
				}
				if err == nil && !visible {
					continue
				}
				ticker.Stop()
				ticks, watchDone = nil, nil
				state := GameWindowUnavailable
				if err == nil && visible {
					state = GameWindowVisible
				}
				s.gamesMu.Lock()
				entry.snapshot.Window = state
				s.gamesMu.Unlock()
				s.notify()
			case _, open := <-changes:
				if !open {
					changes = nil
					continue
				}
				s.notify()
			case <-game.Done():
				exit, output := game.Wait(), game.Output()
				s.gamesMu.Lock()
				entry.snapshot.Running, entry.snapshot.Stopping = false, false
				entry.snapshot.Exit, entry.snapshot.Output = exit, output
				if files != nil {
					entry.snapshot.DiagnosticFileState = "capturing"
				}
				s.gamesMu.Unlock()
				s.notify()
				var logs []launch.DiagnosticLog
				fileState := fileCode
				if files != nil {
					capture, cancel := context.WithTimeout(context.Background(), 2*time.Second)
					var err error
					logs, err = files.Capture(capture, record.Process.PID, redact)
					cancel()
					files.Close()
					if err != nil {
						fileState = diagnostics.Code(err)
					} else {
						fileState = "captured"
					}
				}
				release()
				s.gamesMu.Lock()
				entry.snapshot.Running, entry.snapshot.Stopping = false, false
				entry.snapshot.Exit, entry.snapshot.Output = exit, output
				entry.snapshot.DiagnosticFiles, entry.snapshot.DiagnosticFileState = logs, fileState
				entry.game, entry.redact = nil, nil
				s.history = append(s.history, record.ID)
				for len(s.history) > maxSessionHistory {
					delete(s.games, s.history[0])
					s.history = s.history[1:]
				}
				s.gamesMu.Unlock()
				s.notify()
				return
			}
		}
	}()
}

func (s *LaunchService) Sessions() []GameSnapshot {
	s.gamesMu.Lock()
	defer s.gamesMu.Unlock()
	result := make([]GameSnapshot, 0, len(s.games))
	for _, entry := range s.games {
		value := entry.snapshot
		value.Evidence = value.Evidence.Clone()
		value.Warnings = slices.Clone(value.Warnings)
		value.DiagnosticFiles = cloneDiagnosticLogs(value.DiagnosticFiles)
		if entry.game != nil {
			value.Output = entry.game.Output()
		} else {
			value.Output.Lines = slices.Clone(value.Output.Lines)
		}
		if len(value.Output.Lines) > 0 {
			if at := value.Output.Lines[len(value.Output.Lines)-1].Time; at.After(value.Evidence.LastWorkAt) {
				value.Evidence.LastWorkAt = at
			}
		}
		result = append(result, value)
	}
	slices.SortFunc(result, func(a, b GameSnapshot) int { return b.StartedAt.Compare(a.StartedAt) })
	return result
}

func (s *LaunchService) Stop(ctx context.Context, id string) error {
	s.gamesMu.Lock()
	entry := s.games[id]
	if entry == nil || entry.game == nil {
		s.gamesMu.Unlock()
		return errors.New("会话已经结束或不由当前启动器管理")
	}
	game, redact := entry.game, entry.redact
	entry.snapshot.Stopping = true
	s.gamesMu.Unlock()
	s.notify()
	err := game.Stop(ctx)
	if err != nil {
		s.gamesMu.Lock()
		entry.snapshot.Error = redact(err.Error())
		s.gamesMu.Unlock()
		s.notify()
		return errors.New(redact(err.Error()))
	}
	return nil
}

// DetachAll stops observing stdout/stderr during launcher shutdown. It does
// not issue Stop or wait for the JVM's natural lifetime.
func (s *LaunchService) DetachAll() {
	s.stopWindowWatch()
	s.gamesMu.Lock()
	var games []launch.Game
	var sources []*sessionLogs
	for _, entry := range s.games {
		if entry.game != nil {
			games = append(games, entry.game)
			if entry.files != nil {
				sources = append(sources, entry.files)
			}
		}
	}
	s.gamesMu.Unlock()
	for _, game := range games {
		game.Detach()
	}
	for _, source := range sources {
		source.Close()
	}
}

func cloneDiagnosticLogs(logs []launch.DiagnosticLog) []launch.DiagnosticLog {
	result := slices.Clone(logs)
	for i := range result {
		result[i].Lines = slices.Clone(result[i].Lines)
	}
	return result
}

// Diagnose reads only the exact managed session, never a PID guessed from an
// instance or a later default directory. Completed sessions use redacted copies.
func (s *LaunchService) Diagnose(ctx context.Context, id string) (launch.RuntimeDiagnosis, error) {
	s.gamesMu.Lock()
	entry := s.games[id]
	if entry == nil {
		s.gamesMu.Unlock()
		return launch.RuntimeDiagnosis{}, errors.New("会话已不在保留范围")
	}
	snapshot := entry.snapshot
	if entry.game != nil {
		snapshot.Output = entry.game.Output()
	}
	logs := cloneDiagnosticLogs(snapshot.DiagnosticFiles)
	files, redact, record := entry.files, entry.redact, entry.record
	s.gamesMu.Unlock()
	fileState := snapshot.DiagnosticFileState
	if snapshot.Running && files != nil {
		var err error
		logs, err = files.Capture(ctx, record.Process.PID, redact)
		if err != nil {
			if ctx.Err() != nil {
				return launch.RuntimeDiagnosis{}, ctx.Err()
			}
			fileState = diagnostics.Code(err)
		} else {
			fileState = "captured"
		}
	}
	if err := ctx.Err(); err != nil {
		return launch.RuntimeDiagnosis{}, err
	}
	// Exit may have completed while a live file snapshot was being read.
	s.gamesMu.Lock()
	if current := s.games[id]; current == entry {
		snapshot = current.snapshot
		if current.game != nil {
			snapshot.Output = current.game.Output()
		}
		if !snapshot.Running {
			logs = cloneDiagnosticLogs(snapshot.DiagnosticFiles)
			fileState = snapshot.DiagnosticFileState
		}
	}
	s.gamesMu.Unlock()
	window := "unsupported"
	if !snapshot.WindowCheckedAt.IsZero() {
		window = "unavailable"
	}
	if snapshot.Window == GameWindowPending {
		window = "pending"
	} else if snapshot.Window == GameWindowVisible {
		window = "visible"
	}
	result := launch.DiagnoseRuntime(snapshot.Output, snapshot.Exit, snapshot.Running, window, logs)
	if fileState == "" {
		fileState = "not-configured"
	}
	result.Observations = append(result.Observations, launch.RuntimeObservation{Code: "diagnostic-files", Value: fileState})
	checked := "unobserved"
	if !snapshot.WindowCheckedAt.IsZero() {
		checked = snapshot.WindowCheckedAt.UTC().Format(time.RFC3339Nano)
	}
	result.Observations = append(result.Observations, launch.RuntimeObservation{Code: "window-checked-at", Value: checked})
	return result, nil
}
