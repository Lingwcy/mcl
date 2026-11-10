package download

import (
	"context"
	"crypto/sha1"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"mcl/internal/core/meta"
)

func testFile(address, target, body string) File {
	return File{Path: target, Download: meta.Download{URL: address, SHA1: fmt.Sprintf("%x", sha1.Sum([]byte(body))), Size: int64(len(body))}}
}

func TestRepairReuseAndSharedTargets(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1); fmt.Fprint(w, "verified object") }))
	defer server.Close()
	c := NewClient(t.TempDir())
	f := testFile(server.URL, "cache/中文 空格/object", "verified object")
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			if err := c.Run(t.Context(), []File{f, f}, nil); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if requests.Load() != 1 {
		t.Fatalf("shared target fetched %d times", requests.Load())
	}
	if err := os.WriteFile(filepath.Join(c.Root, f.Path), []byte("corrupted data!"), 0o600); err != nil {
		t.Fatal(err)
	}
	var progress Progress
	if err := c.Run(t.Context(), []File{f}, func(p Progress) { progress = p }); err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 2 || progress.Files != 1 || progress.Reused != 0 || progress.Bytes != f.Size {
		t.Fatalf("repair: requests=%d progress=%+v", requests.Load(), progress)
	}
	if err := c.Run(t.Context(), []File{f}, func(p Progress) { progress = p }); err != nil {
		t.Fatal(err)
	}
	if progress.Reused != 1 || progress.Transferred != 0 {
		t.Fatalf("cache: %+v", progress)
	}
	entries, _ := os.ReadDir(filepath.Dir(filepath.Join(c.Root, f.Path)))
	if len(entries) != 1 {
		t.Fatalf("temporary files remain: %v", entries)
	}
	if len(c.locks) != 0 {
		t.Fatal("target gates leaked")
	}
}

func TestRetriesAndNoPartialPublication(t *testing.T) {
	for _, tc := range []struct {
		name     string
		status   int
		body     string
		attempts int
	}{
		{"transient", 503, "", 3}, {"permanent", 404, "", 1}, {"digest", 200, "wrong", 3}, {"size", 200, "shorter", 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			}))
			defer server.Close()
			c := NewClient(t.TempDir())
			f := testFile(server.URL, "object", "valid")
			if err := c.Run(t.Context(), []File{f}, nil); err == nil {
				t.Fatal("invalid response succeeded")
			}
			if requests.Load() != int32(tc.attempts) {
				t.Fatalf("attempts %d", requests.Load())
			}
			entries, _ := os.ReadDir(c.Root)
			if len(entries) != 0 {
				t.Fatalf("failed data published or retained: %v", entries)
			}
		})
	}
}

func TestTransientRecoveryAndConcurrencyBound(t *testing.T) {
	var running, peak, requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := running.Add(1)
		defer running.Add(-1)
		for p := peak.Load(); n > p && !peak.CompareAndSwap(p, n); p = peak.Load() {
		}
		if requests.Add(1) == 1 {
			w.WriteHeader(503)
			return
		}
		time.Sleep(10 * time.Millisecond)
		fmt.Fprint(w, "ok")
	}))
	defer server.Close()
	c := NewClient(t.TempDir())
	c.Concurrency = 2
	var files []File
	for i := range 12 {
		files = append(files, testFile(server.URL, fmt.Sprintf("file-%d", i), "ok"))
	}
	if err := c.Run(t.Context(), files, nil); err != nil {
		t.Fatal(err)
	}
	if peak.Load() > 2 || requests.Load() != 13 {
		t.Fatalf("peak=%d requests=%d", peak.Load(), requests.Load())
	}
}

func TestCancellationAndTimeout(t *testing.T) {
	for _, timeout := range []bool{false, true} {
		t.Run(fmt.Sprint(timeout), func(t *testing.T) {
			started := make(chan struct{}, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Length", "10000")
				fmt.Fprint(w, "partial")
				w.(http.Flusher).Flush()
				select {
				case started <- struct{}{}:
				default:
				}
				<-r.Context().Done()
			}))
			defer server.Close()
			c := NewClient(t.TempDir())
			c.Attempts = 1
			if timeout {
				c.HTTP = &http.Client{Timeout: 50 * time.Millisecond}
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			f := testFile(server.URL, "object", strings.Repeat("x", 10000))
			done := make(chan error, 1)
			go func() { done <- c.Run(ctx, []File{f}, nil) }()
			<-started
			if !timeout {
				cancel()
			}
			select {
			case err := <-done:
				if err == nil {
					t.Fatal("interrupted download succeeded")
				}
			case <-time.After(3 * time.Second):
				t.Fatal("download did not stop")
			}
			// An interrupted transfer keeps a verifiable partial so the task can
			// resume it; only the managed parts directory may remain, and the
			// final target must never exist.
			entries, _ := os.ReadDir(c.Root)
			for _, entry := range entries {
				if entry.Name() != partsDirectory {
					t.Fatalf("partial remains: %v", entries)
				}
			}
			if _, err := os.Stat(filepath.Join(c.Root, f.Path)); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("unverified target exists: %v", err)
			}
		})
	}
}

func TestConfinementAndConflicts(t *testing.T) {
	c := NewClient(t.TempDir())
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(c.Root, "escape")); err != nil {
		t.Fatal(err)
	}
	f := testFile("http://127.0.0.1:1", "escape/object", "ok")
	c.Attempts = 1
	if err := c.Run(t.Context(), []File{f}, nil); err == nil {
		t.Fatal("symlink escape accepted")
	}
	entries, _ := os.ReadDir(outside)
	if len(entries) != 0 {
		t.Fatal("wrote outside root")
	}
	f.Path = "../object"
	if err := c.Run(t.Context(), []File{f}, nil); err == nil {
		t.Fatal("traversal accepted")
	}
	f.Path = "Object"
	second := f
	second.Path = "object"
	if err := c.Run(t.Context(), []File{f, second}, nil); err == nil {
		t.Fatal("case alias accepted")
	}
	for _, value := range []string{"120", http.TimeFormat} {
		if value == http.TimeFormat {
			value = time.Now().Add(2 * time.Minute).UTC().Format(http.TimeFormat)
		}
		if _, ok := retryDelay(value, time.Now()); ok {
			t.Fatal("excessive Retry-After accepted")
		}
	}
}
