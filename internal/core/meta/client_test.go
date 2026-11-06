package meta

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
)

func TestClientCacheIntegrityAndOfflineFallback(t *testing.T) {
	t.Parallel()
	body := fixture(t, "1.21.1.json")
	digest := sha1.Sum(body)
	var calls atomic.Int32
	var unavailable atomic.Bool
	var corrupt atomic.Bool
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if unavailable.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		if r.URL.Path == "/version" {
			if corrupt.Load() {
				w.Write([]byte(`{"id":"corrupt"}`))
			} else {
				w.Write(body)
			}
			return
		}
		json.NewEncoder(w).Encode(Manifest{Versions: []ManifestEntry{{ID: "1.21.1", Type: "release", SHA1: hex.EncodeToString(digest[:]), URL: server.URL + "/version"}}})
	}))
	defer server.Close()
	client := NewClient(t.TempDir())
	client.HTTP, client.ManifestURL = server.Client(), server.URL+"/manifest"
	catalog, err := client.Manifest(t.Context(), false)
	if err != nil || catalog.Cached || calls.Load() != 1 {
		t.Fatalf("initial fetch: %+v, %v", catalog, err)
	}
	if catalog, err = client.Manifest(t.Context(), false); err != nil || !catalog.Cached || catalog.Stale || calls.Load() != 1 {
		t.Fatalf("fresh cache: %+v, %v", catalog, err)
	}
	entry := catalog.Manifest.Versions[0]
	if version, err := client.Version(t.Context(), entry); err != nil || version.Java.Major != 21 {
		t.Fatalf("version download: Java %+v, %v", version.Java, err)
	}
	versionPath := filepath.Join(client.CacheDir, "versions", entry.SHA1+".json")
	if err := os.WriteFile(versionPath, []byte("bad cache"), 0o600); err != nil {
		t.Fatal(err)
	}
	corrupt.Store(true)
	if _, err := client.Version(t.Context(), entry); err == nil {
		t.Fatal("corrupt remote version must fail integrity verification")
	}
	corrupt.Store(false)
	if _, err := client.Version(t.Context(), entry); err != nil {
		t.Fatalf("repair corrupted version cache: %v", err)
	}
	unavailable.Store(true)
	catalog, err = client.Manifest(t.Context(), true)
	if err != nil || !catalog.Cached || !catalog.Stale || catalog.Warning == nil {
		t.Fatalf("explicit offline fallback: %+v, %v", catalog, err)
	}
	before := calls.Load()
	if _, err := client.Version(t.Context(), entry); err != nil || calls.Load() != before {
		t.Fatalf("immutable verified cache should work offline: %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := client.Manifest(ctx, true); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation must not become an offline success: %v", err)
	}
}

func TestOfficialFixtureDigests(t *testing.T) {
	t.Parallel()
	manifest, err := ParseManifest(fixture(t, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range manifest.Versions {
		digest := sha1.Sum(fixture(t, entry.ID+".json"))
		if hex.EncodeToString(digest[:]) != entry.SHA1 {
			t.Fatalf("official fixture %s differs from the recorded metadata digest", entry.ID)
		}
	}
}
