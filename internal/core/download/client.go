// Package download retrieves verified objects into a confined application root.
package download

import (
	"context"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"mcl/internal/core/meta"
)

const MaxFileSize int64 = 2 << 30

type File struct {
	Path         string
	SHA512       string
	AllowedHosts []string
	meta.Download
}

func (f File) Validate() error {
	if !meta.ValidPath(f.Path) || f.Size > MaxFileSize {
		return errors.New("unsafe download path or excessive file size")
	}
	if f.SHA512 != "" {
		decoded, err := hex.DecodeString(f.SHA512)
		if err != nil || len(decoded) != sha512.Size {
			return errors.New("invalid SHA-512 digest")
		}
	}
	if len(f.AllowedHosts) > 0 && !allowedFileURL(f.URL, f.AllowedHosts) {
		return errors.New("download host is not permitted")
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
	Root        string
	HTTP        *http.Client
	Concurrency int
	Attempts    int
	mu          sync.Mutex
	locks       map[string]*gate
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
		key := strings.ToLower(f.Path)
		if previous, ok := seen[key]; ok {
			if previous.Path != f.Path || previous.Size != f.Size || !strings.EqualFold(previous.SHA1, f.SHA1) || !strings.EqualFold(previous.SHA512, f.SHA512) {
				return fmt.Errorf("conflicting download target %s", f.Path)
			}
			continue
		}
		seen[key] = f
		unique = append(unique, f)
		p.TotalBytes += f.Size
	}
	p.TotalFiles = len(unique)
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
	work, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	jobs := make(chan File)
	var wg sync.WaitGroup
	var progressMu sync.Mutex
	if report != nil {
		report(p)
	}
	for range c.Concurrency {
		wg.Go(func() {
			for f := range jobs {
				reused, err := c.get(work, root, f)
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
	return context.Cause(work)
}

func (c *Client) get(ctx context.Context, root *os.Root, f File) (bool, error) {
	unlock, err := c.lock(ctx, strings.ToLower(f.Path))
	if err != nil {
		return false, err
	}
	defer unlock()
	if err := Verify(ctx, root, f); err == nil {
		return true, nil
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if err := root.MkdirAll(path.Dir(f.Path), 0o700); err != nil {
		return false, err
	}
	for attempt := range c.Attempts {
		retry, delay, err := c.transfer(ctx, root, f)
		if err == nil {
			return false, nil
		}
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
		if !retry || attempt+1 == c.Attempts {
			return false, err
		}
		if delay == 0 {
			delay = (200 * time.Millisecond) << attempt
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return false, ctx.Err()
		case <-timer.C:
		}
	}
	return false, errors.New("download attempts exhausted")
}

// Verify always hashes a regular file; matching size or an old success marker
// alone is insufficient to reuse it. Root prevents traversal through symlinks.
func Verify(ctx context.Context, root *os.Root, f File) error {
	if err := f.Validate(); err != nil {
		return err
	}
	file, err := root.Open(f.Path)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() != f.Size {
		return errors.New("cached file size or type mismatch")
	}
	h := sha1.New()
	h512 := sha512.New()
	var writer io.Writer = h
	if f.SHA512 != "" {
		writer = io.MultiWriter(h, h512)
	}
	if _, err := io.Copy(writer, &contextReader{ctx, io.LimitReader(file, f.Size+1)}); err != nil {
		return err
	}
	if !strings.EqualFold(hex.EncodeToString(h.Sum(nil)), f.SHA1) {
		return errors.New("cached file SHA-1 mismatch")
	}
	if f.SHA512 != "" && !strings.EqualFold(hex.EncodeToString(h512.Sum(nil)), f.SHA512) {
		return errors.New("cached file SHA-512 mismatch")
	}
	return ctx.Err()
}

func (c *Client) transfer(ctx context.Context, root *os.Root, f File) (bool, time.Duration, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.URL, nil)
	if err != nil {
		return false, 0, err
	}
	req.Header.Set("User-Agent", "mcl/0.1.0")
	req.Header.Set("Accept-Encoding", "identity")
	client := c.HTTP
	if len(f.AllowedHosts) > 0 {
		client = &http.Client{Transport: c.HTTP.Transport, Jar: c.HTTP.Jar, Timeout: c.HTTP.Timeout, CheckRedirect: func(r *http.Request, via []*http.Request) error {
			if len(via) >= 5 || !allowedFileURL(r.URL.String(), f.AllowedHosts) {
				return errors.New("download redirect host is not permitted")
			}
			if c.HTTP.CheckRedirect != nil {
				return c.HTTP.CheckRedirect(r, via)
			}
			return nil
		}}
	}
	response, err := client.Do(req)
	if err != nil {
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			err = urlErr.Err
		}
		return true, 0, fmt.Errorf("request failed: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		retry := response.StatusCode == 408 || response.StatusCode == 429 || response.StatusCode == 500 || response.StatusCode == 502 || response.StatusCode == 503 || response.StatusCode == 504
		delay, acceptable := retryDelay(response.Header.Get("Retry-After"), time.Now())
		return retry && acceptable, delay, fmt.Errorf("server returned HTTP %d", response.StatusCode)
	}
	if response.ContentLength >= 0 && response.ContentLength != f.Size {
		return true, 0, errors.New("response size does not match metadata")
	}
	temporary := path.Join(path.Dir(f.Path), ".mcl-"+rand.Text()+".tmp")
	file, err := root.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return false, 0, err
	}
	defer root.Remove(temporary)
	defer file.Close()
	h := sha1.New()
	h512 := sha512.New()
	var writer io.Writer = io.MultiWriter(file, h)
	if f.SHA512 != "" {
		writer = io.MultiWriter(file, h, h512)
	}
	n, err := io.Copy(writer, &contextReader{ctx, io.LimitReader(response.Body, f.Size+1)})
	if err != nil {
		return true, 0, fmt.Errorf("transfer failed: %w", err)
	}
	if n != f.Size || !strings.EqualFold(hex.EncodeToString(h.Sum(nil)), f.SHA1) || (f.SHA512 != "" && !strings.EqualFold(hex.EncodeToString(h512.Sum(nil)), f.SHA512)) {
		return true, 0, errors.New("download size or SHA-1 mismatch")
	}
	if err := file.Sync(); err != nil {
		return false, 0, err
	}
	if err := file.Close(); err != nil {
		return false, 0, err
	}
	if err := ctx.Err(); err != nil {
		return false, 0, err
	}
	return false, 0, root.Rename(temporary, f.Path)
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
