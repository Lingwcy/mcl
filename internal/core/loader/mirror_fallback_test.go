package loader

import (
	"context"
	"crypto/sha1"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"testing"

	"mcl/internal/core/download"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func textHTTPResponse(req *http.Request, status int, body string) *http.Response {
	return &http.Response{
		StatusCode:    status,
		Header:        make(http.Header),
		Body:          io.NopCloser(strings.NewReader(body)),
		ContentLength: int64(len(body)),
		Request:       req,
	}
}

func TestLoaderMirrorFallbackForNeoForgeAndFabricWithCDNRedirect(t *testing.T) {
	control, err := download.NewTransferControl(download.Preferences{Source: download.SourceAuto})
	if err != nil {
		t.Fatal(err)
	}
	client := NewClient()
	client.Transfers = control

	var visited []string
	client.HTTP.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		visited = append(visited, r.URL.String())
		switch r.URL.Host {
		case "maven.neoforged.net":
			return nil, context.DeadlineExceeded
		case "meta.fabricmc.net":
			return textHTTPResponse(r, http.StatusServiceUnavailable, "busy"), nil
		case "bmclapi2.bangbang93.com":
			if strings.HasPrefix(r.URL.Path, "/maven/net/neoforged/neoforge/maven-metadata.xml") {
				resp := textHTTPResponse(r, http.StatusFound, "")
				resp.Header.Set("Location", "https://mirrors.cernet.edu.cn/bmclapi/net/neoforged/neoforge/maven-metadata.xml")
				return resp, nil
			}
			if r.URL.Path == "/fabric-meta/v2/versions/loader/1.21.1" {
				return textHTTPResponse(r, http.StatusOK, `[{"loader":{"version":"0.16.7","stable":true}}]`), nil
			}
		case "mirrors.cernet.edu.cn":
			return textHTTPResponse(r, http.StatusOK, `<metadata><versioning><versions><version>21.1.1</version></versions></versioning></metadata>`), nil
		}
		return textHTTPResponse(r, http.StatusNotFound, "not found"), nil
	})

	neoReleases, err := client.Releases(t.Context(), "neoforge", "1.21.1")
	if err != nil {
		t.Fatalf("NeoForge mirror fallback failed: %v", err)
	}
	if len(neoReleases) != 1 || neoReleases[0].Version != "21.1.1" {
		t.Fatalf("unexpected NeoForge releases: %+v", neoReleases)
	}

	fabricReleases, err := client.Releases(t.Context(), "fabric", "1.21.1")
	if err != nil {
		t.Fatalf("Fabric mirror fallback failed: %v", err)
	}
	if len(fabricReleases) != 1 || fabricReleases[0].Version != "0.16.7" || !fabricReleases[0].Stable {
		t.Fatalf("unexpected Fabric releases: %+v", fabricReleases)
	}

	// Verify official-only mode still rejects mirrors cleanly.
	if err := control.SetPreferences(download.Preferences{Source: download.SourceOfficial}); err != nil {
		t.Fatal(err)
	}
	visited = nil
	if _, err := client.Releases(t.Context(), "fabric", "1.21.1"); err == nil {
		t.Fatal("SourceOfficial unexpectedly succeeded via mirror")
	}
	for _, u := range visited {
		if strings.Contains(u, "bmclapi2.bangbang93.com") {
			t.Fatalf("SourceOfficial hit mirror %s", u)
		}
	}
}

func TestLoaderBytesWrapsReadTimeoutWithoutLeakingDeadlineError(t *testing.T) {
	client := NewClient()
	client.HTTP.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(errReader{err: context.DeadlineExceeded}),
			Request:    r,
		}, nil
	})
	_, err := client.bytes(t.Context(), "https://meta.fabricmc.net/v2/versions/loader/1.21.1", 1024)
	if err == nil || errors.Is(err, context.DeadlineExceeded) || !strings.Contains(err.Error(), "读取加载器响应失败") {
		t.Fatalf("expected wrapped timeout error, got %v", err)
	}
	if slices.Contains([]string{""}, err.Error()) {
		t.Fatal("empty error")
	}
}

type errReader struct{ err error }

func (e errReader) Read([]byte) (int, error) { return 0, e.err }

func TestMavenSizeFallsBackToMirrorWithCDNRedirect(t *testing.T) {
	control, err := download.NewTransferControl(download.Preferences{Source: download.SourceAuto})
	if err != nil {
		t.Fatal(err)
	}
	client := NewClient()
	client.Transfers = control

	var visited []string
	client.HTTP.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		visited = append(visited, r.Method+" "+r.URL.String())
		switch r.URL.Host {
		case "maven.fabricmc.net":
			return nil, context.DeadlineExceeded
		case "bmclapi2.bangbang93.com":
			resp := textHTTPResponse(r, http.StatusFound, "")
			resp.Header.Set("Location", "https://bmclapi2.hurtmc.com:443/download/abc")
			return resp, nil
		case "bmclapi2.hurtmc.com:443", "bmclapi2.hurtmc.com":
			resp := textHTTPResponse(r, http.StatusOK, "")
			resp.ContentLength = 42
			resp.Header.Set("Content-Length", "42")
			return resp, nil
		}
		return textHTTPResponse(r, http.StatusNotFound, ""), nil
	})
	size, err := client.mavenSize(t.Context(), "https://maven.fabricmc.net/net/fabricmc/fabric-loader/0.16.7/fabric-loader-0.16.7.jar")
	if err != nil || size != 42 {
		t.Fatalf("mavenSize mirror fallback: size=%d err=%v visited=%v", size, err, visited)
	}
}

func TestQuiltReleasesSortsNewestFirstAndMarksStable(t *testing.T) {
	client := NewClient()
	client.HTTP.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return textHTTPResponse(r, http.StatusOK, `[
			{"loader":{"version":"0.20.0-beta.9"}},
			{"loader":{"version":"0.24.0"}},
			{"loader":{"version":"0.30.2"}},
			{"loader":{"version":"0.31.0-beta.6"}}
		]`), nil
	})
	got, err := client.Releases(t.Context(), "quilt", "26.3")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 || got[0].Version != "0.31.0-beta.6" || got[0].Stable || got[1].Version != "0.30.2" || !got[1].Stable {
		t.Fatalf("unexpected quilt order/stability: %+v", got)
	}
}

func TestLegacyForgePinnedMetadataFallsBackToPinsOnTimeoutWithMirror(t *testing.T) {
	control, err := download.NewTransferControl(download.Preferences{Source: download.SourceAuto})
	if err != nil {
		t.Fatal(err)
	}

	payload := "akka-actor-jar-bytes"
	sum := fmt.Sprintf("%x", sha1.Sum([]byte(payload)))
	relative := "com/typesafe/akka/akka-actor_2.11/2.3.3/akka-actor_2.11-2.3.3.jar"
	address := "https://maven.minecraftforge.net/" + relative

	var visited []string
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		visited = append(visited, r.Method+" "+r.URL.String())
		switch r.URL.Host {
		case "maven.minecraftforge.net":
			return nil, context.DeadlineExceeded
		case "bmclapi2.bangbang93.com":
			if strings.HasSuffix(r.URL.Path, ".sha1") {
				return textHTTPResponse(r, http.StatusNotFound, ""), nil
			}
			resp := textHTTPResponse(r, http.StatusOK, payload)
			resp.Header.Set("Content-Length", strconv.Itoa(len(payload)))
			if r.Method == http.MethodHead {
				resp.Body = io.NopCloser(strings.NewReader(""))
			}
			return resp, nil
		}
		return textHTTPResponse(r, http.StatusNotFound, ""), nil
	})

	catalog := NewClient()
	catalog.HTTP = &http.Client{Transport: transport}
	catalog.Transfers = control
	downloads := download.NewClient(t.TempDir())
	downloads.HTTP = &http.Client{Transport: transport}
	downloads.Transfers = control
	installer := NewForgeInstaller(catalog, downloads, nil)

	fixed, err := installer.legacyForgePinnedMetadata(t.Context(), address, relative, []string{sum, "0000000000000000000000000000000000000000"}, 1<<20)
	if err != nil {
		t.Fatalf("pinned timeout fallback failed: %v (visited=%v)", err, visited)
	}
	if fixed.SHA1 != sum || fixed.Size != int64(len(payload)) || fixed.URL != address {
		t.Fatalf("unexpected resolved download: %+v", fixed)
	}

	// HTTP 401 must still refuse pin fallback.
	unauth := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return textHTTPResponse(r, http.StatusUnauthorized, ""), nil
	})
	catalog.HTTP = &http.Client{Transport: unauth}
	if _, err := installer.legacyForgePinnedMetadata(t.Context(), address, relative, []string{sum}, 1<<20); err == nil {
		t.Fatal("expected HTTP 401 to reject pin fallback")
	}
}

func TestLegacyForgeUnpinnedCentralMetadataFallsBackToMavenCentral(t *testing.T) {
	payload := "continuations-plugin-bytes"
	sum := fmt.Sprintf("%x", sha1.Sum([]byte(payload)))
	relative := "org/scala-lang/plugins/scala-continuations-plugin_2.11.1/1.0.2/scala-continuations-plugin_2.11.1-1.0.2.jar"
	forgeURL := "https://maven.minecraftforge.net/" + relative
	centralURL := "https://repo.maven.apache.org/maven2/" + relative

	catalog := NewClient()
	catalog.HTTP = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Host {
		case "maven.minecraftforge.net":
			return nil, context.DeadlineExceeded
		case "repo.maven.apache.org":
			if strings.HasSuffix(r.URL.Path, ".sha1") {
				return textHTTPResponse(r, http.StatusOK, sum), nil
			}
			resp := textHTTPResponse(r, http.StatusOK, "")
			resp.ContentLength = int64(len(payload))
			resp.Header.Set("Content-Length", strconv.Itoa(len(payload)))
			return resp, nil
		}
		return textHTTPResponse(r, http.StatusNotFound, ""), nil
	})}
	installer := NewForgeInstaller(catalog, download.NewClient(t.TempDir()), nil)

	fixed, err := installer.legacyForgePinnedMetadata(t.Context(), forgeURL, relative, nil, 1<<20)
	if err != nil {
		t.Fatalf("central fallback failed: %v", err)
	}
	if fixed.URL != centralURL || fixed.SHA1 != sum || fixed.Size != int64(len(payload)) {
		t.Fatalf("unexpected central metadata: %+v", fixed)
	}

	// net/minecraftforge coordinates must not fall back to Maven Central.
	forgeRel := "net/minecraftforge/forge/1.8.9-11.15.1.1902-1.8.9/forge-1.8.9-11.15.1.1902-1.8.9.jar"
	if _, err := installer.legacyForgePinnedMetadata(t.Context(), "https://maven.minecraftforge.net/"+forgeRel, forgeRel, nil, 1<<20); err == nil {
		t.Fatal("expected net/minecraftforge artifact not to fall back to Maven Central")
	}
}
