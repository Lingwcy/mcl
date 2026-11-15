// Package download retrieves verified objects into a confined application root.
package download

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/text/unicode/norm"

	"mcl/internal/core/diagnostics"
	"mcl/internal/core/meta"
)

const MaxFileSize int64 = 2 << 30

type File struct {
	Path          string
	SHA512        string
	SHA256        string
	AllowedHosts  []string
	SizeTolerance int64
	meta.Download
}

func (f File) Validate() error {
	if !meta.ValidPath(f.Path) || f.Size > MaxFileSize {
		return errors.New("unsafe download path or excessive file size")
	}
	if _, err := objectForFile(f).Key(); err != nil {
		return err
	}
	if len(f.AllowedHosts) > 0 && !allowedFileURL(f.URL, f.AllowedHosts) {
		return errors.New("download host is not permitted")
	}
	if f.SHA256 != "" {
		if f.SHA1 != "" || f.Size < 0 {
			return errors.New("invalid SHA256 download")
		}
		return meta.ValidateURL(f.URL)
	}
	return f.Download.Validate()
}

// Progress counts verified bytes, including valid cached files. Callbacks are
// serialized, and never run after Run returns. Bytes in partial transfers are
// deliberately not counted as usable data.
type Progress struct {
	Files, TotalFiles, Reused      int
	Bytes, TotalBytes, Transferred int64
}

type gate struct {
	ready chan struct{}
	users int
}

// Client coordinates concurrent requests for the same target across installs.
// Configuration must be fixed before use. Use one shared client per data root.
type Client struct {
	Coordination Coordination
	Track        func(context.Context, []File, func(context.Context) error) error
	Root         string
	HTTP         *http.Client
	Concurrency  int
	Attempts     int
	Transfers    *TransferControl
	Cache        ObjectCache
	mu           sync.Mutex
	locks        map[string]*gate
}

func NewClient(root string) *Client {
	return &Client{Root: root, HTTP: &http.Client{Timeout: 2 * time.Minute}, Concurrency: 6, Attempts: 3}
}

func (c *Client) lock(ctx context.Context, name string) (func(), error) {
	c.mu.Lock()
	if c.locks == nil {
		c.locks = make(map[string]*gate)
	}
	g := c.locks[name]
	if g == nil {
		g = &gate{ready: make(chan struct{}, 1)}
		c.locks[name] = g
	}
	g.users++
	c.mu.Unlock()
	release := func(acquired bool) {
		if acquired {
			<-g.ready
		}
		c.mu.Lock()
		defer c.mu.Unlock()
		g.users--
		if g.users == 0 {
			delete(c.locks, name)
		}
	}
	select {
	case g.ready <- struct{}{}:
		return func() { release(true) }, nil
	case <-ctx.Done():
		release(false)
		return nil, ctx.Err()
	}
}

// Run validates the whole input before mutating the cache, bounds its workers,
// cancels siblings on failure and joins every worker before returning.
func (c *Client) Run(ctx context.Context, files []File, report func(Progress)) error {
	if c.Track != nil && ctx.Value(observerKey{}) == nil {
		return c.Track(ctx, append([]File(nil), files...), func(next context.Context) error { return c.Run(next, files, report) })
	}
	if c.Concurrency < 1 || c.Concurrency > 32 || c.Attempts < 1 || c.Attempts > 5 || c.HTTP == nil {
		return errors.New("invalid download client configuration")
	}
	unique := make([]File, 0, len(files))
	seen := make(map[string]File)
	p := Progress{}
	for _, f := range files {
		if err := f.Validate(); err != nil {
			return fmt.Errorf("download %s: %w", f.Path, err)
		}
		key := strings.ToLower(norm.NFC.String(f.Path))
		if previous, ok := seen[key]; ok {
			if previous.Path != f.Path || previous.Size != f.Size || previous.SizeTolerance != f.SizeTolerance || !strings.EqualFold(previous.SHA1, f.SHA1) || !strings.EqualFold(previous.SHA512, f.SHA512) || !strings.EqualFold(previous.SHA256, f.SHA256) {
				return fmt.Errorf("conflicting download target %s", f.Path)
			}
			continue
		}
		seen[key] = f
		unique = append(unique, f)
		p.TotalBytes += f.Size
	}
	p.TotalFiles = len(unique)
	for _, f := range unique {
		if err := Observe(ctx, eventFor(f, "pending", 0)); err != nil {
			return err
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.MkdirAll(c.Root, 0o700); err != nil {
		return err
	}
	root, err := os.OpenRoot(c.Root)
	if err != nil {
		return err
	}
	defer root.Close()
	var cache CacheLease
	if c.Cache != nil {
		cache, err = c.Cache.Acquire(ctx)
		if err != nil {
			return err
		}
		defer cache.Close()
	}
	work, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	jobs := make(chan File)
	var wg sync.WaitGroup
	var progressMu sync.Mutex
	if report != nil {
		report(p)
	}
	concurrency := c.Concurrency
	if c.Transfers != nil {
		concurrency = c.Transfers.Limit()
	}
	for range concurrency {
		wg.Go(func() {
			for f := range jobs {
				reused, err := c.get(work, root, f, cache)
				if err != nil {
					cancel(fmt.Errorf("%s: %w", f.Path, err))
					return
				}
				progressMu.Lock()
				p.Files++
				p.Bytes += f.Size
				if reused {
					p.Reused++
				} else {
					p.Transferred += f.Size
				}
				if report != nil {
					report(p)
				}
				progressMu.Unlock()
			}
		})
	}
dispatch:
	for _, f := range unique {
		select {
		case jobs <- f:
		case <-work.Done():
			break dispatch
		}
	}
	close(jobs)
	wg.Wait()
	pruneParts(ctx, root, c.Coordination)
	return context.Cause(work)
}

func (c *Client) get(ctx context.Context, root *os.Root, f File, cache CacheLease) (bool, error) {
	ctx, activity := diagnostics.Start(ctx, "work", f.Path)
	defer activity.End()
	lockCtx, waiting := diagnostics.Start(ctx, "lock", f.Path)
	unlock, err := c.lock(lockCtx, strings.ToLower(norm.NFC.String(f.Path)))
	if err != nil {
		waiting.Fail(err)
		waiting.End()
		return false, err
	}
	defer unlock()
	release, err := c.targetLease(lockCtx, f.Path)
	waiting.End()
	if err != nil {
		return false, err
	}
	defer release()
	if err := Observe(ctx, eventFor(f, "checking", 0)); err != nil {
		return false, err
	}
	if err := Verify(ctx, root, f); err == nil {
		if err := Observe(ctx, eventFor(f, "done", f.Size)); err != nil {
			return true, err
		}
		return true, nil
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	directory, err := targetDirectory(root, path.Dir(f.Path), true)
	if err != nil {
		return false, err
	}
	directory.Close()

	if cache != nil {
		reused, err := restoreCachedFile(ctx, cache, root, f)
		if err != nil {
			return reused, err
		}
		if reused {
			return true, Observe(ctx, eventFor(f, "done", f.Size))
		}
	}
	sources := c.Transfers.candidates(f.URL)
	remaining := slices.Clone(sources)
	sourceAttempts := make(map[string]int, len(sources))
	maxSteps := len(sources) * max(c.Attempts, 1)
	idx := 0
	for step := 0; len(remaining) > 0 && step < maxSteps; step++ {
		pos := idx % len(remaining)
		address := remaining[pos]
		attemptNum := sourceAttempts[address] + 1
		sourceAttempts[address] = attemptNum
		retry, delay, err := c.transfer(ctx, root, f, address, len(remaining) > 1, attemptNum)
		if err == nil {
			c.Transfers.rememberSource(f.URL, address)
			if cache != nil {
				source, openErr := root.Open(f.Path)
				if openErr != nil {
					return false, openErr
				}
				cacheErr := cache.Store(ctx, objectForFile(f), source)
				source.Close()
				if cacheErr != nil {
					return false, fmt.Errorf("保存下载副本: %w", cacheErr)
				}
			}
			return false, Observe(ctx, eventFor(f, "done", f.Size))
		}
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
		if !retry {
			return false, err
		}
		code := diagnostics.Code(err)
		// Drop a source immediately if it definitively lacks the artifact (403/404)
		// so remaining attempts focus on surviving candidates without sleeping.
		if code == "forbidden" || code == "not-found" {
			remaining = append(remaining[:pos], remaining[pos+1:]...)
			if len(remaining) == 0 {
				return false, err
			}
			idx = pos % len(remaining)
			continue
		}
		if attemptNum >= c.Attempts {
			remaining = append(remaining[:pos], remaining[pos+1:]...)
			if len(remaining) == 0 {
				return false, err
			}
			idx = pos % len(remaining)
		} else {
			idx = (pos + 1) % len(remaining)
		}
		// Skip backoff when failing over immediately to an untried candidate.
		if delay == 0 && sourceAttempts[remaining[idx]] == 0 {
			continue
		}
		if delay == 0 {
			delay = (200 * time.Millisecond) << (sourceAttempts[remaining[idx]] - 1)
		}
		_, backoff := diagnostics.Start(ctx, "backoff", f.Path)
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			backoff.End()
			return false, ctx.Err()
		case <-timer.C:
			backoff.End()
		}
	}
	return false, errors.New("download attempts exhausted")
}

// Verify always hashes a regular file; matching size or an old success marker
// alone is insufficient to reuse it. Root prevents traversal through symlinks.
func Verify(ctx context.Context, root *os.Root, f File) error {
	ctx, evidence := diagnostics.Start(ctx, "verify", f.Path)
	defer evidence.End()
	if err := f.Validate(); err != nil {
		return err
	}
	directory, err := targetDirectory(root, path.Dir(f.Path), false)
	if err != nil {
		return err
	}
	defer directory.Close()
	before, err := directory.Lstat(path.Base(f.Path))
	if err != nil {
		return err
	}
	if !before.Mode().IsRegular() {
		return errors.New("cached target is not a regular file")
	}
	file, err := directory.Open(path.Base(f.Path))
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || !objectForFile(f).AcceptsSize(info.Size()) {
		return errors.New("cached file size or type mismatch")
	}
	if !os.SameFile(before, info) {
		return errors.New("cached file identity changed")
	}
	if err := objectForFile(f).Copy(ctx, io.Discard, file); err != nil {
		return err
	}
	after, err := file.Stat()
	if err != nil {
		return err
	}
	current, err := directory.Lstat(path.Base(f.Path))
	if err != nil {
		return err
	}
	if !current.Mode().IsRegular() || !os.SameFile(before, current) || !os.SameFile(before, after) || current.Size() != before.Size() || after.Size() != before.Size() || !before.ModTime().Equal(current.ModTime()) || !before.ModTime().Equal(after.ModTime()) {
		return errors.New("cached file changed while verifying")
	}
	evidence.Work()
	return ctx.Err()
}

func allowedFileURL(address string, hosts []string) bool {
	u, err := url.Parse(address)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" {
		return false
	}
	for _, host := range hosts {
		if strings.EqualFold(u.Hostname(), host) {
			return true
		}
	}
	return false
}

// Do not retry earlier than a server asks. An excessive wait becomes a visible
// failure instead of parking a worker for an unbounded duration.
func retryDelay(value string, now time.Time) (time.Duration, bool) {
	if value == "" {
		return 0, true
	}
	var delay time.Duration
	if seconds, err := strconv.ParseInt(value, 10, 32); err == nil {
		delay = time.Duration(seconds) * time.Second
	} else if date, err := http.ParseTime(value); err == nil {
		delay = date.Sub(now)
	}
	if delay < 0 {
		delay = 0
	}
	return delay, delay <= 30*time.Second
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}
