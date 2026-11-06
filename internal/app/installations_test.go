package app_test

import (
	"context"
	"crypto/sha1"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"mcl/internal/app"
	"mcl/internal/core/download"
	"mcl/internal/core/install"
	"mcl/internal/core/instance"
	"mcl/internal/core/launch"
	"mcl/internal/core/meta"
	"mcl/internal/storage"
)

type assetTransport struct {
	base      string
	transport http.RoundTripper
}

func (a assetTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	copy := request.Clone(request.Context())
	if copy.URL.Host == "resources.download.minecraft.net" {
		base, _ := url.Parse(a.base)
		copy.URL.Scheme, copy.URL.Host = base.Scheme, base.Host
	}
	return a.transport.RoundTrip(copy)
}

type installFixture struct {
	service  *app.InstallationService
	store    *storage.InstallationStore
	root     string
	value    instance.Instance
	requests *atomic.Int32
	block    *atomic.Bool
	running  *atomic.Bool
}

func newInstallFixture(t *testing.T) installFixture {
	t.Helper()
	var requests atomic.Int32
	var block atomic.Bool
	asset := "test asset object"
	hash := fmt.Sprintf("%x", sha1.Sum([]byte(asset)))
	index, _ := json.Marshal(install.AssetIndex{Objects: map[string]install.AssetObject{"sounds/test.ogg": {Hash: hash, Size: int64(len(asset))}}})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if block.Load() {
			<-r.Context().Done()
			return
		}
		switch r.URL.Path {
		case "/client":
			fmt.Fprint(w, "client jar")
		case "/index":
			w.Write(index)
		case "/" + hash[:2] + "/" + hash:
			fmt.Fprint(w, asset)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	desc := func(relative string, data []byte) meta.Download {
		return meta.Download{URL: server.URL + relative, Size: int64(len(data)), SHA1: fmt.Sprintf("%x", sha1.Sum(data))}
	}
	data, err := os.ReadFile("../core/meta/testdata/1.20.4.json")
	if err != nil {
		t.Fatal(err)
	}
	v, err := meta.ParseVersion(data)
	if err != nil {
		t.Fatal(err)
	}
	v.Libraries = nil
	v.Logging.Client = nil
	v.Downloads["client"] = desc("/client", []byte("client jar"))
	v.AssetIndex.Download = desc("/index", index)
	root := t.TempDir()
	instances := storage.NewInstanceStore(filepath.Join(root, "instances"))
	value := instance.Instance{ID: instance.NewID(), Name: "中文 空格", Version: meta.ManifestEntry{ID: v.ID, Type: "release", URL: server.URL + "/version", SHA1: strings.Repeat("a", 40)}, RequiredJava: 17, MaxMemoryMiB: 4096, CreatedAt: time.Now().UTC()}
	if err := instances.Create(t.Context(), value); err != nil {
		t.Fatal(err)
	}
	client := download.NewClient(root)
	client.Attempts = 1
	client.HTTP = &http.Client{Transport: assetTransport{base: server.URL, transport: http.DefaultTransport}, Timeout: 5 * time.Second}
	store := storage.NewInstallationStore(root)
	running := &atomic.Bool{}
	registry := storage.NewSessionStore(root, func(launch.ProcessIdentity) (bool, error) { return running.Load(), nil })
	service := app.NewInstallationService(fixedCatalog{version: v}, instances, store, client, meta.CurrentPlatform(), registry, nil)
	return installFixture{service: service, store: store, root: root, value: value, requests: &requests, block: &block, running: running}
}

func TestInstallRepairReloadAndReceipt(t *testing.T) {
	f := newInstallFixture(t)
	receipt, err := f.service.Run(t.Context(), f.value.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !receipt.Matches(f.value, meta.CurrentPlatform()) || f.requests.Load() != 3 {
		t.Fatalf("receipt %+v requests %d", receipt, f.requests.Load())
	}
	reloaded, err := f.service.Load(t.Context())
	if err != nil || reloaded[f.value.ID] != receipt {
		t.Fatalf("reload %v %v", reloaded, err)
	}
	client := filepath.Join(f.root, "cache/versions/1.20.4/1.20.4.jar")
	if err := os.WriteFile(client, []byte("broken jar"), 0o600); err != nil {
		t.Fatal(err)
	}
	var files, assets download.Progress
	updated, err := f.service.Run(t.Context(), f.value.ID, func(p install.Progress) {
		if p.Stage == "files" {
			files = p.Progress
		}
		if p.Stage == "assets" {
			assets = p.Progress
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if f.requests.Load() != 4 || files.Reused != 1 || assets.Reused != 1 {
		t.Fatalf("repair requests=%d files=%+v assets=%+v", f.requests.Load(), files, assets)
	}
	data, _ := os.ReadFile(client)
	if string(data) != "client jar" {
		t.Fatal("repair failed")
	}
	if updated.NativeDirectory == receipt.NativeDirectory {
		t.Fatal("native generation not replaced")
	}
	if _, err := os.Stat(filepath.Join(f.root, receipt.NativeDirectory)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("old native generation retained")
	}
	if _, err := os.Stat(filepath.Join(f.root, updated.NativeDirectory)); err != nil {
		t.Fatal(err)
	}
	var assetFile string
	err = filepath.WalkDir(filepath.Join(f.root, "cache/assets/objects"), func(name string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			assetFile = name
		}
		return nil
	})
	if err != nil || assetFile == "" {
		t.Fatalf("find asset: %v", err)
	}
	if err := os.Remove(assetFile); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Run(t.Context(), f.value.ID, nil); err != nil {
		t.Fatal(err)
	}
	if f.requests.Load() != 5 {
		t.Fatalf("deleted asset not repaired with exactly one request: %d", f.requests.Load())
	}
	if _, err := os.Stat(assetFile); err != nil {
		t.Fatal(err)
	}
}

func TestInstallCancelExcludesDuplicateAndNeverSucceeds(t *testing.T) {
	f := newInstallFixture(t)
	f.block.Store(true)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	started := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		_, err := f.service.Run(ctx, f.value.ID, func(p install.Progress) {
			if p.Stage == "metadata" {
				close(started)
			}
		})
		done <- err
	}()
	<-started
	if _, err := f.service.Run(t.Context(), f.value.ID, nil); err == nil {
		t.Fatal("duplicate instance accepted")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancel %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("workers did not stop")
	}
	if _, err := f.store.Load(t.Context(), f.value.ID); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("canceled receipt exists: %v", err)
	}
	f.block.Store(false)
	if _, err := f.service.Run(t.Context(), f.value.ID, nil); err != nil {
		t.Fatalf("retry after cancellation: %v", err)
	}
}

func TestCorruptReceiptPreserved(t *testing.T) {
	f := newInstallFixture(t)
	name := filepath.Join(f.root, "instances", f.value.ID, "installation.json")
	if err := os.WriteFile(name, []byte(`{"schema":99}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Run(t.Context(), f.value.ID, nil); err == nil {
		t.Fatal("corrupt receipt overwritten")
	}
	data, _ := os.ReadFile(name)
	if string(data) != `{"schema":99}` || f.requests.Load() != 0 {
		t.Fatal("corrupt receipt changed or network started")
	}
}
