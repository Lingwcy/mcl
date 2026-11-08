package download

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

type connectivityTransport func(*http.Request) (*http.Response, error)

func (f connectivityTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestConnectivityUsesAnonymousHeadersAndReportsForbiddenAsResponse(t *testing.T) {
	var mirror atomic.Bool
	client := &http.Client{Transport: connectivityTransport(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodHead || r.URL.Scheme != "https" {
			t.Error("diagnostic downloaded a body or downgraded TLS")
		}
		for _, name := range []string{"Authorization", "X-API-Key", "Cookie", "Proxy-Authorization"} {
			if r.Header.Get(name) != "" {
				t.Error("diagnostic copied a credential")
			}
		}
		if r.URL.Host == "bmclapi2.bangbang93.com" {
			mirror.Store(true)
		}
		return &http.Response{StatusCode: http.StatusForbidden, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header), Request: r}, nil
	})}
	results, err := CheckConnectivity(t.Context(), client, SourceOfficial)
	if err != nil || len(results) == 0 || mirror.Load() {
		t.Fatalf("unexpected official diagnostic: %v", err)
	}
	for _, result := range results {
		if result.Status != 403 || result.Failure != "" {
			t.Fatal("HTTP denial was presented as a transport failure")
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := CheckConnectivity(ctx, client, SourceAuto); err != context.Canceled {
		t.Fatal("cancelled diagnostic was reported complete")
	}
}
