package loader

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type forgeAliasTransport func(*http.Request) (*http.Response, error)

func (f forgeAliasTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func forgeAliasResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}
func shortForgeRequest() Request {
	return Request{Type: "forge", GameVersion: "1.7.10", Version: "10.13.4.1614", BaseSHA1: strings.Repeat("a", 40), BaseChainSHA256: strings.Repeat("b", 64), TargetOS: "osx", TargetArch: "amd64"}
}
func TestForgePackShortBuildResolvesItsExactOfficialBranch(t *testing.T) {
	c := NewClient()
	c.HTTP.Transport = forgeAliasTransport(func(r *http.Request) (*http.Response, error) {
		return forgeAliasResponse(200, `{"1.7.10":["1.7.10-10.13.4.1614-1.7.10","1.7.10-10.13.4.9999-1.7.10"]}`), nil
	})
	requested := shortForgeRequest()
	resolved, err := c.CanonicalForgeRequest(t.Context(), requested)
	if err != nil || resolved.Version != "10.13.4.1614-1.7.10" || requested.Version != "10.13.4.1614" {
		t.Fatal(resolved, err)
	}
	if err := ValidateForgeAlias(requested, resolved); err != nil {
		t.Fatal(err)
	}
	coordinate, _, _, err := forgeInstaller(resolved)
	if err != nil || coordinate.Version != "1.7.10-10.13.4.1614-1.7.10" || !strings.HasSuffix(coordinate.Path(), "forge-1.7.10-10.13.4.1614-1.7.10-installer.jar") {
		t.Fatal(coordinate, err)
	}
}
func TestForgeAliasDoesNotGuessSuffixOrSelectOtherBuild(t *testing.T) {
	c := NewClient()
	c.HTTP.Transport = forgeAliasTransport(func(r *http.Request) (*http.Response, error) {
		return forgeAliasResponse(200, `{"1.7.10":["1.7.10-10.13.4.9999-1.7.10","1.7.10-10.13.4.1614-beta","1.7.10-10.13.4.1614-1.6.4"]}`), nil
	})
	if _, err := c.CanonicalForgeRequest(t.Context(), shortForgeRequest()); err == nil {
		t.Fatal("other branch/build accepted as fixed pack requirement")
	}
	original := shortForgeRequest()
	for _, mutate := range []func(*Request){func(r *Request) { r.Version = "10.13.4.9999-1.7.10" }, func(r *Request) { r.Version = "10.13.4.1614-1.6.4" }, func(r *Request) { r.TargetArch = "arm64" }, func(r *Request) { r.BaseSHA1 = strings.Repeat("c", 40) }, func(r *Request) { r.BaseChainSHA256 = "" }} {
		resolved := original
		resolved.Version += "-1.7.10"
		mutate(&resolved)
		if ValidateForgeAlias(original, resolved) == nil {
			t.Fatal("mapping changed fixed identity", resolved)
		}
	}
}
func TestForgeAliasOfficialMavenIndexFallbackAndExplicitIdentity(t *testing.T) {
	c := NewClient()
	calls := 0
	c.HTTP.Transport = forgeAliasTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if strings.HasSuffix(r.URL.Path, ".json") {
			return forgeAliasResponse(403, ""), nil
		}
		return forgeAliasResponse(200, `<metadata><versioning><versions><version>1.7.10-10.13.4.1614-1.7.10</version><version>1.6.4-10.13.4.1614-1.6.4</version></versions></versioning></metadata>`), nil
	})
	resolved, err := c.CanonicalForgeRequest(t.Context(), shortForgeRequest())
	if err != nil || resolved.Version != "10.13.4.1614-1.7.10" || calls != 2 {
		t.Fatal(resolved, err, calls)
	}
	calls = 0
	if same, err := c.CanonicalForgeRequest(t.Context(), resolved); err != nil || same != resolved || calls != 0 {
		t.Fatal("explicit branch should not query catalog", same, err, calls)
	}
	modern := shortForgeRequest()
	modern.GameVersion = "1.20.1"
	modern.Version = "47.4.10"
	if same, err := c.CanonicalForgeRequest(t.Context(), modern); err != nil || same != modern || calls != 0 {
		t.Fatal("modern build should retain exact identity", same, err, calls)
	}
}
func TestForgeAliasCancelStopsBeforeFallbackIndex(t *testing.T) {
	c := NewClient()
	ctx, cancel := context.WithCancel(t.Context())
	calls := 0
	c.HTTP.Transport = forgeAliasTransport(func(r *http.Request) (*http.Response, error) { calls++; cancel(); return nil, ctx.Err() })
	if _, err := c.CanonicalForgeRequest(ctx, shortForgeRequest()); !errors.Is(err, context.Canceled) || calls != 1 {
		t.Fatal(err, calls)
	}
}
