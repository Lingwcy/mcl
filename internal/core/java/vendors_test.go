package java

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"mcl/internal/core/meta"
)

type vendorTestTransport func(*http.Request) (*http.Response, error)

func (f vendorTestTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func vendorTestResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}
}

const vendorTestUUID = "11111111-1111-1111-1111-111111111111"

func vendorTestDetail() string {
	return `{"package_uuid":"11111111-1111-1111-1111-111111111111","product":"zulu","os":"macos","arch":"arm","hw_bitness":64,"java_package_type":"jdk","archive_type":"tar.gz","release_status":"ga","availability_type":"CA","java_version":[8,0,512],"download_url":"https://cdn.azul.com/zulu/bin/zulu8.98.0.111-ca-jdk8.0.512-macosx_aarch64.tar.gz","sha256_hash":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","size":1024}`
}
func TestCatalogFallsBackFromAdoptiumFailureToPinnedZuluARM8(t *testing.T) {
	requests := 0
	c := NewCatalogClient()
	c.HTTP.Transport = vendorTestTransport(func(r *http.Request) (*http.Response, error) {
		requests++
		if r.URL.Host == "api.adoptium.net" {
			return vendorTestResponse(403, ""), nil
		}
		if r.URL.Host != "api.azul.com" {
			t.Fatal("unexpected metadata host", r.URL.Host)
		}
		if strings.HasSuffix(r.URL.Path, vendorTestUUID) {
			return vendorTestResponse(200, vendorTestDetail()), nil
		}
		if r.URL.Query().Get("java_package_type") == "jre" {
			return vendorTestResponse(200, "[]"), nil
		}
		if r.URL.Query().Get("arch") != "aarch64" || r.URL.Query().Get("java_version") != "8" {
			t.Fatal("wrong target query", r.URL.Query())
		}
		return vendorTestResponse(200, `[{"package_uuid":"11111111-1111-1111-1111-111111111111"}]`), nil
	})
	result, err := c.Catalog(t.Context(), 8, meta.Platform{OS: "osx", Arch: "arm64"})
	if err != nil || len(result.Packages) != 1 || result.Packages[0].Vendor() != "zulu" || result.Packages[0].Size != 1024 || requests != 4 {
		t.Fatal(result, err, requests)
	}
}
func TestZuluRejectsWrongPlatformEAAndMissingIdentity(t *testing.T) {
	for _, change := range []struct{ from, to string }{
		{`"hw_bitness":64`, `"hw_bitness":32`}, {`"arch":"arm"`, `"arch":"x86"`}, {`"release_status":"ga"`, `"release_status":"ea"`}, {`"java_version":[8,0,512]`, `"java_version":[17,0,1]`}, {`"availability_type":"CA"`, `"availability_type":"SA"`}, {`"size":1024`, `"size":0`}, {"cdn.azul.com", "evil.invalid"}, {strings.Repeat("a", 64), "bad"},
	} {
		c := NewCatalogClient()
		c.HTTP.Transport = vendorTestTransport(func(r *http.Request) (*http.Response, error) {
			if strings.HasSuffix(r.URL.Path, vendorTestUUID) {
				return vendorTestResponse(200, strings.Replace(vendorTestDetail(), change.from, change.to, 1)), nil
			}
			return vendorTestResponse(200, `[{"package_uuid":"11111111-1111-1111-1111-111111111111"}]`), nil
		})
		result, _ := c.CatalogFor(t.Context(), "zulu", 8, meta.Platform{OS: "osx", Arch: "arm64"})
		if len(result.Packages) != 0 {
			t.Fatal("invalid official identity accepted", change, result)
		}
	}
}
func TestVendorCatalogCancellationDoesNotTryAnotherSource(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	calls := 0
	c := NewCatalogClient()
	c.HTTP.Transport = vendorTestTransport(func(r *http.Request) (*http.Response, error) { calls++; cancel(); return nil, ctx.Err() })
	_, err := c.Catalog(ctx, 8, meta.Platform{OS: "osx", Arch: "arm64"})
	if !errors.Is(err, context.Canceled) || calls != 1 {
		t.Fatal(err, calls)
	}
}
func TestZuluPackageCannotChangeOriginPlatformOrArchiveKind(t *testing.T) {
	p := Package{Provider: "zulu", Major: 8, Version: "8.0.512", Kind: "jdk", OS: "osx", Arch: "arm64", URL: "https://cdn.azul.com/zulu/bin/zulu8.98.0.111-ca-jdk8.0.512-macosx_aarch64.tar.gz", SHA256: strings.Repeat("a", 64), Size: 1024}
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"https://cdn.azul.com/zulu/bin/other/file.tar.gz", strings.Replace(p.URL, "aarch64", "x64", 1), p.URL + "?signature=private", strings.Replace(p.URL, "https:", "http:", 1)} {
		copy := p
		copy.URL = bad
		if copy.Validate() == nil {
			t.Fatal("invalid pinned package accepted", bad)
		}
	}
}

func TestTemurinPackageAcceptsOfficialEscapedReleaseTag(t *testing.T) {
	p := Package{Provider: "temurin", Name: "Eclipse Temurin", Version: "21.0.9+10", Major: 21, Kind: "jre", OS: "osx", Arch: "arm64", URL: "https://github.com/adoptium/temurin21-binaries/releases/download/jdk-21.0.9%2B10/OpenJDK21U-jre_aarch64_mac_hotspot_21.0.9_10.tar.gz", SHA256: strings.Repeat("a", 64), Size: 1024}
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
}
