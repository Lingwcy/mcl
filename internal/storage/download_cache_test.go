package storage_test

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"io"
	"mcl/internal/core/download"
	"mcl/internal/storage"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
)

func TestDownloadCacheLeaseBlocksClearAndKeepsUnmanagedFiles(t *testing.T) {
	appRoot := t.TempDir()
	store := storage.NewDownloadCache(appRoot, "")
	data := []byte("verified transfer copy")
	hash := sha1.Sum(data)
	object := download.Object{SHA1: hex.EncodeToString(hash[:]), Size: int64(len(data))}
	lease, err := store.Acquire(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err = lease.Store(t.Context(), object, bytes.NewReader(data)); err != nil {
		lease.Close()
		t.Fatal(err)
	}
	other := storage.NewDownloadCache(appRoot, "")
	if _, err = other.Clear(t.Context(), store.Directory()); err == nil {
		lease.Close()
		t.Fatal("exclusive clear ignored a shared job lease")
	}
	var restored bytes.Buffer
	if found, err := lease.Restore(t.Context(), object, &restored); err != nil || !found || !bytes.Equal(restored.Bytes(), data) {
		lease.Close()
		t.Fatalf("cache did not verify/restore: %v", err)
	}
	lease.Close()
	unowned := filepath.Join(store.Directory(), "personal.txt")
	if err := os.WriteFile(unowned, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	runtime := filepath.Join(appRoot, "runtime.jar")
	if err := os.WriteFile(runtime, data, 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := other.Clear(t.Context(), store.Directory())
	if err != nil || info.Files != 1 || info.Skipped != 1 {
		t.Fatalf("unexpected bounded clear: %+v, %v", info, err)
	}
	for _, name := range []string{unowned, runtime, filepath.Join(store.Directory(), "owner"), filepath.Join(store.Directory(), "cache.lock")} {
		if _, err := os.Stat(name); err != nil {
			t.Fatalf("cleared a protected file: %s", name)
		}
	}
}
func TestDownloadCacheRejectsUnownedAndRuntimeLocations(t *testing.T) {
	root := t.TempDir()
	store := storage.NewDownloadCache(root, "")
	for _, base := range []string{filepath.Join(root, "instances", "game"), filepath.Join(root, "java")} {
		if err := store.Prepare(t.Context(), base); err == nil {
			t.Fatal("cache accepted a protected app subtree")
		}
		if _, err := os.Stat(base); !os.IsNotExist(err) {
			t.Fatal("rejected cache setting created protected directories")
		}
	}
	external := t.TempDir()
	child := filepath.Join(external, "mcl-download-cache-v1")
	if err := os.Mkdir(child, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := store.Prepare(t.Context(), external); err == nil {
		t.Fatal("adopted an existing unowned directory")
	}
	if _, err := os.Stat(filepath.Join(child, "owner")); !os.IsNotExist(err) {
		t.Fatal("silently wrote ownership into user data")
	}
}
func TestCorruptDownloadCopyIsAMissWithoutWritingDestination(t *testing.T) {
	store := storage.NewDownloadCache(t.TempDir(), "")
	data := []byte("original")
	hash := sha1.Sum(data)
	object := download.Object{SHA1: hex.EncodeToString(hash[:]), Size: int64(len(data))}
	lease, err := store.Acquire(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	if err := lease.Store(t.Context(), object, bytes.NewReader(data)); err != nil {
		t.Fatal(err)
	}
	key, _ := object.Key()
	if err := os.WriteFile(filepath.Join(store.Directory(), key), []byte("modified"), 0o600); err != nil {
		t.Fatal(err)
	}
	var destination bytes.Buffer
	found, err := lease.Restore(t.Context(), object, &destination)
	if err != nil || found || destination.Len() != 0 {
		t.Fatalf("corrupt copy was trusted: %v, %v", found, err)
	}
}

type cacheFixtureTransport func(*http.Request) (*http.Response, error)

func (f cacheFixtureTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestFileDownloadsReuseIndependentVerifiedCopyAfterRuntimeCacheDeletion(t *testing.T) {
	root := t.TempDir()
	cache := storage.NewDownloadCache(root, "")
	data := []byte("offline restoration fixture")
	hash := sha1.Sum(data)
	var calls atomic.Int32
	client := download.NewClient(root)
	client.Cache = cache
	client.HTTP.Transport = cacheFixtureTransport(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		return &http.Response{StatusCode: 200, Header: make(http.Header), ContentLength: int64(len(data)), Body: io.NopCloser(bytes.NewReader(data)), Request: r}, nil
	})
	file := download.File{Path: "cache/libraries/demo.jar"}
	file.URL = "https://example.invalid/demo.jar"
	file.SHA1 = hex.EncodeToString(hash[:])
	file.Size = int64(len(data))
	if err := client.Run(t.Context(), []download.File{file}, nil); err != nil {
		t.Fatal(err)
	}
	runtimePath := filepath.Join(root, filepath.FromSlash(file.Path))
	if err := os.Remove(runtimePath); err != nil {
		t.Fatal(err)
	}
	if err := client.Run(t.Context(), []download.File{file}, nil); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Fatal("verified transfer copy was not reused")
	}
	if _, err := cache.Clear(t.Context(), cache.Directory()); err != nil {
		t.Fatal(err)
	}
	restored, err := os.ReadFile(runtimePath)
	if err != nil || !bytes.Equal(restored, data) {
		t.Fatal("clearing download copies touched runtime data")
	}
}
