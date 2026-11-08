package download

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

type policyRoundTripper func(*http.Request) (*http.Response, error)

func (f policyRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func policyResponse(request *http.Request, content string) *http.Response {
	return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(content)), ContentLength: int64(len(content)), Request: request}
}

func TestMirrorScopeAndExplicitSourceOrder(t *testing.T) {
	for _, address := range []string{
		"https://api.curseforge.com/v1/mods/1/files/2", "https://cdn.modrinth.com/data/abc/versions/def/mod.jar",
		"https://github.com/adoptium/temurin21-binaries/releases/download/file",
		"https://piston-data.mojang.com/v1/object?token=secret", "https://piston-data.mojang.com/v1/object?",
		"https://user:secret@piston-data.mojang.com/v1/object", "https://piston-data.mojang.com/v1/../object",
		"https://piston-meta.mojang.com/mc/game/version_manifest_v2.json",
		"https://example.com/maven/file.jar", "http://libraries.minecraft.net/file.jar",
	} {
		if got := mirrorURL(address); got != "" {
			t.Errorf("out-of-scope URL acquired a mirror: %s", got)
		}
	}
	original := "https://libraries.minecraft.net/org/example/test/1/test-1.jar"
	mirror := mirrorOrigin + "/maven/org/example/test/1/test-1.jar"
	control, err := NewTransferControl(Preferences{Source: SourceMirrorFirst})
	if err != nil {
		t.Fatal(err)
	}
	if got := control.candidates(original); !slices.Equal(got, []string{mirror, original}) {
		t.Fatal(got)
	}
	if err := control.SetPreferences(Preferences{Source: SourceAuto}); err != nil {
		t.Fatal(err)
	}
	control.rememberSource(original, mirror)
	if control.candidates(original)[0] != mirror {
		t.Fatal("automatic policy forgot a verified source")
	}
	control.mu.Lock()
	control.preferred["libraries.minecraft.net"] = sourceHealth{mirror: true, until: time.Now().Add(-time.Second)}
	control.mu.Unlock()
	if control.candidates(original)[0] != original {
		t.Fatal("expired source preference was reused")
	}
	if err := control.SetPreferences(Preferences{}); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(control.candidates(original), []string{original}) {
		t.Fatal("default enabled a third-party mirror")
	}
}

func TestCorruptMirrorFallsBackWithoutChangingPinnedFileIdentity(t *testing.T) {
	const good, bad = "object", "tamper"
	address := "https://piston-data.mojang.com/v1/objects/pinned/client.jar"
	control, err := NewTransferControl(Preferences{Concurrency: 1, Source: SourceMirrorFirst})
	if err != nil {
		t.Fatal(err)
	}
	client := NewClient(t.TempDir())
	client.Transfers = control
	var requests []string
	client.HTTP.Transport = policyRoundTripper(func(request *http.Request) (*http.Response, error) {
		requests = append(requests, request.URL.Host)
		body := good
		if request.URL.Host == "bmclapi2.bangbang93.com" {
			body = bad
		}
		return policyResponse(request, body), nil
	})
	file := testFile(address, "cache/client.jar", good)
	file.AllowedHosts = []string{"piston-data.mojang.com"}
	if err := client.Run(t.Context(), []File{file}, nil); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(client.Root, file.Path))
	if err != nil || string(data) != good {
		t.Fatalf("unverified mirror data published: %q, %v", data, err)
	}
	if !slices.Equal(requests, []string{"bmclapi2.bangbang93.com", "piston-data.mojang.com"}) {
		t.Fatal(requests)
	}
	// The same digest check is required before delivering metadata to its cache.
	requests = nil
	data, err = client.FetchMetadata(t.Context(), address, file.SHA1, 1024)
	if err != nil || string(data) != good || len(requests) != 2 {
		t.Fatalf("metadata mirror bypassed verification: %v", err)
	}
}

func TestMirrorRedirectsDropSecretsAndRejectLocalOrPlaintextTargets(t *testing.T) {
	client := NewClient(t.TempDir()).requestClient("https://libraries.minecraft.net/file.jar", nil, true)
	request, _ := http.NewRequest(http.MethodGet, "https://cdn.example.com/file.jar", nil)
	for _, key := range []string{"Authorization", "X-API-Key", "Cookie", "Proxy-Authorization", "Referer"} {
		request.Header.Set(key, "synthetic-secret")
	}
	if err := client.CheckRedirect(request, []*http.Request{request}); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"Authorization", "X-API-Key", "Cookie", "Proxy-Authorization", "Referer"} {
		if request.Header.Get(key) != "" {
			t.Fatalf("redirect retained %s", key)
		}
	}
	for _, address := range []string{"http://cdn.example.com/file", "https://localhost/file", "https://127.0.0.1/file", "https://[::1]/file", "https://10.0.0.1/file", "https://user:pass@cdn.example.com/file"} {
		u, _ := url.Parse(address)
		if safeMirrorRedirect(u) {
			t.Errorf("accepted unsafe redirect %s", address)
		}
	}
}

func TestPinnedMetadataCancellationStopsFallback(t *testing.T) {
	client := NewClient(t.TempDir())
	client.Transfers, _ = NewTransferControl(Preferences{Source: SourceAuto})
	ctx, cancel := context.WithCancel(t.Context())
	client.HTTP.Transport = policyRoundTripper(func(request *http.Request) (*http.Response, error) { cancel(); return nil, errors.New("unavailable") })
	file := testFile("https://piston-meta.mojang.com/v1/packages/pinned/test.json", "unused", "object")
	_, err := client.FetchMetadata(ctx, file.URL, file.SHA1, 1024)
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
