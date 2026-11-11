package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"slices"
	"strings"
	"sync"
	"time"

	"mcl/internal/core/launch"
)

const diagnosticTailBytes = 64 << 10

type sessionLogs struct {
	mu       sync.Mutex
	root     *os.Root
	began    time.Time
	baseline map[string]os.FileInfo
}

func newSessionLogs(ctx context.Context, root *os.Root) (*sessionLogs, error) {
	source := &sessionLogs{root: root, began: time.Now().UTC(), baseline: make(map[string]os.FileInfo)}
	candidates, err := sessionLogCandidates(ctx, root, 0, true)
	if err != nil {
		return nil, err
	}
	for _, name := range candidates {
		info, err := sessionLogStat(root, name)
		if err == nil {
			source.baseline[name] = info
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}
	return source, ctx.Err()
}

func sessionLogStat(root *os.Root, name string) (os.FileInfo, error) {
	if dir := path.Dir(name); dir != "." {
		info, err := root.Lstat(dir)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			return nil, errors.New("诊断目录不是普通目录")
		}
	}
	info, err := root.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("诊断来源不是普通文件")
	}
	return info, nil
}

func sessionLogCandidates(ctx context.Context, root *os.Root, pid int, baseline bool) ([]string, error) {
	names := []string{"logs/latest.log", "logs/debug.log"}
	if pid > 0 {
		names = append(names, fmt.Sprintf("hs_err_pid%d.log", pid))
	}
	for _, directory := range []string{"crash-reports", "."} {
		if directory == "." && !baseline {
			continue
		}
		if directory != "." {
			info, err := root.Lstat(directory)
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			if err != nil {
				return nil, err
			}
			if !info.IsDir() {
				return nil, errors.New("诊断目录包含链接或其他类型")
			}
		}
		dir, err := root.Open(directory)
		if err != nil {
			return nil, err
		}
		count := 0
		scanErr := func() error {
			defer dir.Close()
			for {
				if err := ctx.Err(); err != nil {
					return err
				}
				entries, readErr := dir.ReadDir(32)
				for _, entry := range entries {
					count++
					if count > 256 {
						return errors.New("诊断目录候选超过256项")
					}
					name := entry.Name()
					selected := directory == "crash-reports" && strings.HasPrefix(name, "crash-") && strings.HasSuffix(name, "-client.txt") || directory == "." && strings.HasPrefix(name, "hs_err_pid") && strings.HasSuffix(name, ".log")
					if selected && entry.Type().IsRegular() {
						names = append(names, path.Join(directory, name))
					}
				}
				if errors.Is(readErr, io.EOF) {
					return nil
				}
				if readErr != nil {
					return readErr
				}
			}
		}()
		if scanErr != nil {
			return nil, scanErr
		}
	}
	return names, ctx.Err()
}

func (s *sessionLogs) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.root != nil {
		s.root.Close()
		s.root = nil
	}
}

// Capture returns sanitized copies only. The root belongs to the exact launch
// reservation; it is never reconstructed from a later default directory or log.
func (s *sessionLogs) Capture(ctx context.Context, pid int, redact func(string) string) ([]launch.DiagnosticLog, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.root == nil {
		return nil, errors.New("会话诊断来源已经关闭")
	}
	names, err := sessionLogCandidates(ctx, s.root, pid, false)
	if err != nil {
		return nil, err
	}
	type candidate struct {
		name string
		info os.FileInfo
	}
	var candidates []candidate
	for _, name := range names {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		info, err := sessionLogStat(s.root, name)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		old, existed := s.baseline[name]
		unchanged := existed && os.SameFile(old, info) && old.Size() == info.Size() && old.ModTime().Equal(info.ModTime())
		if unchanged || info.ModTime().Before(s.began.Add(-2*time.Second)) {
			continue
		}
		candidates = append(candidates, candidate{name, info})
	}
	slices.SortStableFunc(candidates, func(a, b candidate) int { return b.info.ModTime().Compare(a.info.ModTime()) })
	result := make([]launch.DiagnosticLog, 0, 4)
	kinds := make(map[string]bool)
	for _, item := range candidates {
		kind := "crash-report"
		switch item.name {
		case "logs/latest.log":
			kind = "latest"
		case "logs/debug.log":
			kind = "debug"
		default:
			if strings.HasPrefix(item.name, "hs_err_pid") {
				kind = "jvm-error"
			}
		}
		if kinds[kind] {
			continue
		}
		text, truncated, err := readSessionLogTail(ctx, s.root, item.name, item.info)
		if err != nil {
			return nil, err
		}
		association := "changed-in-session"
		if kind == "jvm-error" {
			association = "pid-and-session"
		}
		lines := strings.Split(text, "\n")
		if len(lines) > 256 {
			lines = lines[len(lines)-256:]
			truncated = true
		}
		for i, line := range lines {
			lines[i] = launch.DiagnosticText(line, redact)
		}
		result = append(result, launch.DiagnosticLog{Kind: kind, Association: association, Lines: lines, Truncated: truncated})
		kinds[kind] = true
	}
	return result, ctx.Err()
}

func readSessionLogTail(ctx context.Context, root *os.Root, name string, before os.FileInfo) (string, bool, error) {
	if err := ctx.Err(); err != nil {
		return "", false, err
	}
	file, err := root.Open(name)
	if err != nil {
		return "", false, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		return "", false, err
	}
	if !os.SameFile(before, opened) || before.Size() != opened.Size() || !before.ModTime().Equal(opened.ModTime()) {
		return "", false, errors.New("诊断文件在读取前改变")
	}
	start := max(int64(0), before.Size()-diagnosticTailBytes)
	if _, err := file.Seek(start, io.SeekStart); err != nil {
		return "", false, err
	}
	data, err := io.ReadAll(io.LimitReader(&sessionLogReader{ctx, file}, before.Size()-start+1))
	if err != nil {
		return "", false, err
	}
	current, err := sessionLogStat(root, name)
	if err != nil {
		return "", false, err
	}
	after, err := file.Stat()
	if err != nil {
		return "", false, err
	}
	if !os.SameFile(before, current) || !os.SameFile(before, after) || after.Size() != before.Size() || int64(len(data)) != before.Size()-start || !after.ModTime().Equal(before.ModTime()) {
		return "", false, errors.New("诊断文件正在变化，请稍后重新读取")
	}
	text := string(data)
	if start > 0 {
		if _, tail, ok := strings.Cut(text, "\n"); ok {
			text = tail
		} else {
			text = ""
		}
	} // never expose a partial leading credential/command line
	return text, start > 0, ctx.Err()
}

type sessionLogReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *sessionLogReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}
