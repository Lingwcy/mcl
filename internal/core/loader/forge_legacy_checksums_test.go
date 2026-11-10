package loader

import (
	"bytes"
	"context"
	"crypto/sha1"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"

	"mcl/internal/core/download"
	"mcl/internal/core/meta"
)

type legacyChecksumTransport struct {
	body                      []byte
	sidecarStatus, headStatus int
	sidecar, size             string
	gets, heads               int
	probes, sidecars          int
	failDownload              bool
}

func (t *legacyChecksumTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	status, body := http.StatusOK, t.body
	h := make(http.Header)
	switch {
	case strings.HasSuffix(r.URL.Path, ".sha1"):
		t.sidecars++
		status, body = t.sidecarStatus, []byte(t.sidecar)
	case r.Method == http.MethodHead:
		t.heads++
		status, body = t.headStatus, nil
		h.Set("Content-Length", t.size)
	case r.Header.Get("Range") == "bytes=0-0":
		t.probes++
		status, body = 206, t.body[:1]
		h.Set("Content-Length", "1")
		h.Set("Content-Range", "bytes 0-0/"+strconv.Itoa(len(t.body)))
	default:
		t.gets++
		if t.failDownload {
			return nil, errors.New("fixture transport failure")
		}
		h.Set("Content-Length", strconv.Itoa(len(body)))
	}
	return &http.Response{StatusCode: status, Header: h, Body: io.NopCloser(bytes.NewReader(body)), ContentLength: int64(len(body)), Request: r}, nil
}
func legacyChecksumFixture(t *testing.T) (*ForgeInstaller, *legacyChecksumTransport, legacyForgeLibrary, string) {
	t.Helper()
	transport := &legacyChecksumTransport{body: []byte("synthetic Scala artifact, not executable"), sidecarStatus: 404, headStatus: 200}
	transport.size = strconv.Itoa(len(transport.body))
	pin := fmt.Sprintf("%x", sha1.Sum(transport.body))
	catalog := NewClient()
	catalog.HTTP = &http.Client{Transport: transport}
	client := download.NewClient(t.TempDir())
	client.HTTP = catalog.HTTP
	client.Attempts = 1
	installer := NewForgeInstaller(catalog, client, nil)
	lib := legacyForgeLibrary{Library: meta.Library{Name: "org.scala-lang:scala-parser-combinators_2.11:1.0.1"}, URL: "https://maven.minecraftforge.net/", Checksums: []string{pin}}
	return installer, transport, lib, pin
}
func TestLegacyForgeMissingSidecarUsesFixedInstallerChecksum(t *testing.T) {
	for _, missing := range []int{404, 410} {
		s, tr, lib, pin := legacyChecksumFixture(t)
		tr.sidecarStatus = missing
		fixed, err := s.legacyForgeLibraryIdentity(t.Context(), lib, meta.Version{})
		if err != nil || fixed.SHA1 != pin || fixed.Size != int64(len(tr.body)) || tr.gets != 1 {
			t.Fatal("pinned JAR with missing sidecar rejected", fixed, err, tr.gets)
		}
		if !strings.HasPrefix(fixed.URL, "https://maven.minecraftforge.net/org/scala-lang/") {
			t.Fatal("coordinate/repository was substituted", fixed.URL)
		}
		root, err := os.OpenRoot(s.downloads.Root)
		if err != nil {
			t.Fatal(err)
		}
		if err := download.Verify(t.Context(), root, forgeInput(fixed)); err != nil {
			root.Close()
			t.Fatal("unverified file selected", err)
		}
		root.Close()
	}
}
func TestLegacyForgeOnlyTriesInstallerPinnedVariants(t *testing.T) {
	s, tr, lib, pin := legacyChecksumFixture(t)
	lib.Checksums = []string{strings.Repeat("0", 40), pin, strings.ToUpper(pin)}
	skipped := 0
	ctx := download.WithObserver(t.Context(), func(e download.Event) error {
		if e.State == "skipped" {
			skipped++
		}
		return nil
	})
	fixed, err := s.legacyForgeLibraryIdentity(ctx, lib, meta.Version{})
	if err != nil || fixed.SHA1 != pin || tr.gets != 2 || skipped != 1 {
		t.Fatal("did not choose a verified allowed variant", fixed, err, tr.gets, skipped)
	}
}
func TestLegacyForgeCannotBypassChecksumAndMetadataFailures(t *testing.T) {
	for _, scenario := range []string{"no-pins", "malformed-pin", "too-many", "unauthorized", "forbidden", "invalid-sidecar", "conflicting-sidecar", "missing-jar", "bad-size", "unmatched-bytes", "transport"} {
		t.Run(scenario, func(t *testing.T) {
			s, tr, lib, pin := legacyChecksumFixture(t)
			wantGets := 0
			switch scenario {
			case "no-pins":
				lib.Checksums = nil
			case "malformed-pin":
				lib.Checksums = []string{"bad"}
			case "too-many":
				lib.Checksums = make([]string, 17)
				for i := range lib.Checksums {
					lib.Checksums[i] = pin
				}
			case "unauthorized":
				tr.sidecarStatus = 401
			case "forbidden":
				tr.sidecarStatus = 403
			case "invalid-sidecar":
				tr.sidecarStatus = 200
				tr.sidecar = "not a digest"
			case "conflicting-sidecar":
				tr.sidecarStatus = 200
				tr.sidecar = strings.Repeat("0", 40)
			case "missing-jar":
				tr.headStatus = 404
			case "bad-size":
				tr.size = "0"
			case "unmatched-bytes":
				lib.Checksums = []string{strings.Repeat("0", 40)}
				wantGets = 1
			case "transport":
				lib.Checksums = []string{strings.Repeat("0", 40), pin}
				tr.failDownload = true
				wantGets = 1
			}
			_, err := s.legacyForgeLibraryIdentity(t.Context(), lib, meta.Version{})
			if err == nil || tr.gets != wantGets {
				t.Fatal("unsafe fallback or unrelated failure treated as hash alternative", err, tr.gets)
			}
			if scenario == "unmatched-bytes" && !errors.Is(err, download.ErrContentMismatch) {
				t.Fatal("digest error identity lost", err)
			}
		})
	}
}
func TestLegacyForgeSidecarSuccessDoesNotDownloadDuringIdentityLookup(t *testing.T) {
	s, tr, lib, pin := legacyChecksumFixture(t)
	tr.sidecarStatus = 200
	tr.sidecar = pin
	fixed, err := s.legacyForgeLibraryIdentity(t.Context(), lib, meta.Version{})
	if err != nil || fixed.SHA1 != pin || tr.gets != 0 {
		t.Fatal("normal metadata path changed", fixed, err, tr.gets)
	}
}
func TestLegacyForgeChecksumFallbackHonorsCancellation(t *testing.T) {
	s, tr, lib, _ := legacyChecksumFixture(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := s.legacyForgeLibraryIdentity(ctx, lib, meta.Version{})
	if !errors.Is(err, context.Canceled) || tr.gets != 0 || tr.heads != 0 {
		t.Fatal("cancellation continued into fallback", err, tr.gets, tr.heads)
	}
}

func TestLegacyForgeChecksumFallbackRespectsRemainingInstallBudget(t *testing.T) {
	s, tr, lib, _ := legacyChecksumFixture(t)
	_, err := s.legacyForgeLibraryIdentityWithin(t.Context(), lib, meta.Version{}, int64(len(tr.body)-1))
	if err == nil || tr.gets != 0 {
		t.Fatal("download exceeded remaining group budget", err, tr.gets)
	}
}

func TestLegacyPinnedChecksumAndHEADCapabilityFallbackCompose(t *testing.T) {
	for _, sidecar := range []int{200, 404, 410} {
		for _, head := range []int{405, 501} {
			s, tr, lib, pin := legacyChecksumFixture(t)
			tr.sidecarStatus, tr.headStatus, tr.sidecar = sidecar, head, pin
			fixed, err := s.legacyForgeLibraryIdentity(t.Context(), lib, meta.Version{})
			wantGets := 0
			if sidecar != 200 {
				wantGets = 1
			}
			if err != nil || fixed.SHA1 != pin || fixed.Size != int64(len(tr.body)) || tr.gets != wantGets || tr.probes != 1 || tr.heads != 1 {
				t.Fatal(sidecar, head, fixed, err, tr.gets, tr.probes, tr.heads)
			}
		}
	}
}

func TestMissingArtifactIsNotConfusedWithMissingChecksum(t *testing.T) {
	s, tr, lib, pin := legacyChecksumFixture(t)
	tr.sidecarStatus, tr.sidecar, tr.headStatus = 200, pin, 404
	_, err := s.legacyForgeLibraryIdentity(t.Context(), lib, meta.Version{})
	if err == nil || tr.sidecars != 1 || tr.heads != 1 || tr.probes != 0 || tr.gets != 0 {
		t.Fatal(err, tr.sidecars, tr.heads, tr.probes, tr.gets)
	}
}

func TestLegacySidecarPathHonorsRemainingBudget(t *testing.T) {
	s, tr, lib, pin := legacyChecksumFixture(t)
	tr.sidecarStatus, tr.sidecar = 200, pin
	_, err := s.legacyForgeLibraryIdentityWithin(t.Context(), lib, meta.Version{}, int64(len(tr.body)-1))
	if err == nil || tr.gets != 0 {
		t.Fatal(err, tr.gets)
	}
}
