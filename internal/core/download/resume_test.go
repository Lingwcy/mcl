package download

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"mcl/internal/core/meta"
)

func resumeFile(address string, data []byte) File {
	sum := sha1.Sum(data)
	return File{Path: "cache/resume.bin", Download: meta.Download{URL: address, SHA1: hex.EncodeToString(sum[:]), Size: int64(len(data))}}
}
func seedPartial(t *testing.T, root string, file File, data []byte) {
	t.Helper()
	r, err := os.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	p, err := openPartial(t.Context(), r, file, file.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer p.close()
	p.state.Source = shaText(file.URL)
	p.state.ETag = "\"fixed-v1\""
	if _, err := p.file.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := p.checkpoint(t.Context()); err != nil {
		t.Fatal(err)
	}
}
func TestResumeUsesExactRangeThenHashesWholeFile(t *testing.T) {
	data := bytes.Repeat([]byte("fixed-content-"), 1024)
	half := len(data) / 2
	var observed string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		observed = r.Header.Get("Range")
		if observed != fmt.Sprintf("bytes=%d-", half) || r.Header.Get("If-Range") != "\"fixed-v1\"" {
			t.Error("missing range/validator", observed)
		}
		w.Header().Set("ETag", "\"fixed-v1\"")
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", half, len(data)-1, len(data)))
		w.Header().Set("Content-Length", fmt.Sprint(len(data)-half))
		w.WriteHeader(206)
		w.Write(data[half:])
	}))
	defer server.Close()
	root := t.TempDir()
	file := resumeFile(server.URL, data)
	seedPartial(t, root, file, data[:half])
	client := NewClient(root)
	client.Attempts = 1
	if err := client.Run(t.Context(), []File{file}, nil); err != nil {
		t.Fatal(err)
	}
	actual, err := os.ReadFile(filepath.Join(root, file.Path))
	if err != nil || !bytes.Equal(actual, data) {
		t.Fatal("resumed content invalid", err)
	}
	if observed == "" {
		t.Fatal("resume silently restarted")
	}
}
func TestResumeRejectsWrongRangeAndFallsBackWithoutAppending(t *testing.T) {
	for _, mode := range []string{"ignored", "wrong-offset", "wrong-total", "changed-etag", "unsatisfied", "unsupported"} {
		t.Run(mode, func(t *testing.T) {
			data := []byte("0123456789abcdefghij")
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				if r.Header.Get("Range") != "" && mode != "ignored" {
					if mode == "unsupported" {
						w.WriteHeader(400)
						return
					}
					if mode == "unsatisfied" {
						w.Header().Set("Content-Range", "bytes */20")
						w.WriteHeader(416)
						return
					}
					start, total := 10, 20
					if mode == "wrong-offset" {
						start = 9
					}
					if mode == "wrong-total" {
						total = 21
					}
					tag := "\"fixed-v1\""
					if mode == "changed-etag" {
						tag = "\"new-version\""
					}
					w.Header().Set("ETag", tag)
					w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-19/%d", start, total))
					w.WriteHeader(206)
					w.Write(data[10:])
					return
				}
				w.Header().Set("Content-Length", "20")
				w.Write(data)
			}))
			defer server.Close()
			root := t.TempDir()
			file := resumeFile(server.URL, data)
			seedPartial(t, root, file, data[:10])
			client := NewClient(root)
			client.Attempts = 1
			if err := client.Run(t.Context(), []File{file}, nil); err != nil {
				t.Fatal(err)
			}
			actual, _ := os.ReadFile(filepath.Join(root, file.Path))
			if !bytes.Equal(actual, data) {
				t.Fatal("mixed partial/full body published")
			}
			if mode != "ignored" && requests != 2 {
				t.Fatal("missing safe full fallback", requests)
			}
		})
	}
}
func TestInterruptedTransferPreservesRealOffsetForNewClient(t *testing.T) {
	data := bytes.Repeat([]byte("chunk"), 2000)
	var mu sync.Mutex
	calls := 0
	var ranges []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls++
		number := calls
		ranges = append(ranges, r.Header.Get("Range"))
		mu.Unlock()
		w.Header().Set("ETag", "\"fixed-v1\"")
		if number == 1 {
			w.Header().Set("Content-Length", fmt.Sprint(len(data)))
			w.Write(data[:4000])
			return
		}
		if r.Header.Get("Range") != "bytes=4000-" {
			t.Error("wrong durable offset", r.Header.Get("Range"))
		}
		w.Header().Set("Content-Range", fmt.Sprintf("bytes 4000-%d/%d", len(data)-1, len(data)))
		w.Header().Set("Content-Length", fmt.Sprint(len(data)-4000))
		w.WriteHeader(206)
		w.Write(data[4000:])
	}))
	defer server.Close()
	root := t.TempDir()
	file := resumeFile(server.URL, data)
	first := NewClient(root)
	first.Attempts = 1
	if err := first.Run(t.Context(), []File{file}, nil); err == nil {
		t.Fatal("short response accepted")
	}
	second := NewClient(root)
	second.Attempts = 1
	if err := second.Run(t.Context(), []File{file}, nil); err != nil {
		t.Fatal(err)
	}
	actual, _ := os.ReadFile(filepath.Join(root, file.Path))
	if !bytes.Equal(actual, data) {
		t.Fatal("restarted client corrupted file")
	}
}
func TestResumeCorruptPrefixAndDigestNeverPublish(t *testing.T) {
	data := []byte("fixed-original-content")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(bytes.Repeat([]byte("X"), len(data))) }))
	defer server.Close()
	root := t.TempDir()
	file := resumeFile(server.URL, data)
	seedPartial(t, root, file, data[:8])
	name := filepath.Join(root, partsDirectory, FileKey(file)+".part")
	f, err := os.OpenFile(name, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteAt([]byte("bad"), 0)
	f.Close()
	client := NewClient(root)
	client.Attempts = 1
	if err := client.Run(t.Context(), []File{file}, nil); err == nil {
		t.Fatal("wrong digest accepted")
	}
	if _, err := os.Stat(filepath.Join(root, file.Path)); !os.IsNotExist(err) {
		t.Fatal("invalid bytes published")
	}
}
func TestResumeRefusesSymlinkTargetsAndMalformedRanges(t *testing.T) {
	for _, value := range []string{"bytes 5-4/10", "bytes 0-10/10", "bytes -1-2/10", "bytes 0-1/*", "bytes 0-1/10 extra", "items 0-1/10"} {
		if _, _, _, err := parseContentRange(value); err == nil {
			t.Fatal("bad range accepted", value)
		}
	}
	root, outside := t.TempDir(), t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "cache")); err != nil {
		t.Skip(err)
	}
	client := NewClient(root)
	if err := client.Run(context.Background(), []File{resumeFile("https://example.com/object", []byte("x"))}, nil); err == nil {
		t.Fatal("linked cache target accepted")
	}
	entries, _ := os.ReadDir(outside)
	if len(entries) != 0 {
		t.Fatal("foreign directory mutated")
	}
	if strongETag("W/\"weak\"") != "" || strongETag(strings.Repeat("x", 300)) != "" {
		t.Fatal("invalid validator accepted")
	}
}
