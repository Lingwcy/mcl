package app

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"mcl/internal/core/download"
	"mcl/internal/core/java"
	"mcl/internal/core/meta"
)

type fallbackJavaTransport func(*http.Request) (*http.Response, error)

func (f fallbackJavaTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func fallbackJavaResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}

// All binaries fail before probing. The injected metadata and transfer layers
// exercise fallback and cancellation without a Java executable or public HTTP.
func TestEnsureFallsBackAfterBinaryFailureAndDoesNotHideCancellation(t *testing.T) {
	for _, cancelFirst := range []bool{false, true} {
		t.Run(map[bool]string{false: "download-failure", true: "user-cancel"}[cancelFirst], func(t *testing.T) {
			catalog := java.NewCatalogClient()
			catalog.HTTP.Transport = fallbackJavaTransport(func(r *http.Request) (*http.Response, error) {
				if r.URL.Host == "api.adoptium.net" {
					if strings.HasSuffix(r.URL.Path, "available_releases") {
						return fallbackJavaResponse(200, `{"available_releases":[8]}`), nil
					}
					if r.URL.Query().Get("image_type") == "jre" {
						return fallbackJavaResponse(200, "[]"), nil
					}
					return fallbackJavaResponse(200, `[{"binary":{"architecture":"aarch64","os":"mac","image_type":"jdk","package":{"checksum":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","link":"https://github.com/adoptium/temurin8-binaries/releases/download/v/OpenJDK8U-jdk_aarch64_mac_hotspot_8u512b09.tar.gz","size":1024}},"version":{"semver":"8.0.512"}}]`), nil
				}
				if strings.HasSuffix(r.URL.Path, "11111111-1111-1111-1111-111111111111") {
					return fallbackJavaResponse(200, `{"package_uuid":"11111111-1111-1111-1111-111111111111","product":"zulu","os":"macos","arch":"arm","hw_bitness":64,"java_package_type":"jdk","archive_type":"tar.gz","release_status":"ga","availability_type":"CA","java_version":[8,0,512],"download_url":"https://cdn.azul.com/zulu/bin/zulu8.98.0.111-ca-jdk8.0.512-macosx_aarch64.tar.gz","sha256_hash":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","size":1024}`), nil
				}
				if r.URL.Query().Get("java_package_type") == "jre" {
					return fallbackJavaResponse(200, "[]"), nil
				}
				return fallbackJavaResponse(200, `[{"package_uuid":"11111111-1111-1111-1111-111111111111"}]`), nil
			})
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			seen := []string{}
			skipped := 0
			client := download.NewClient(t.TempDir())
			client.Attempts = 1
			client.HTTP.Transport = fallbackJavaTransport(func(r *http.Request) (*http.Response, error) {
				seen = append(seen, r.URL.Host)
				if cancelFirst {
					cancel()
					return nil, ctx.Err()
				}
				return fallbackJavaResponse(503, ""), nil
			})
			service := NewJavaService(selectionFailureStore{}, func(context.Context) ([]string, error) { return nil, nil }, &java.Installer{Downloads: client, Directory: t.TempDir()}, catalog)
			task := download.WithObserver(ctx, func(event download.Event) error {
				if event.State == "skipped" {
					skipped++
				}
				return nil
			})
			_, err := service.Ensure(task, 8, meta.Platform{OS: "osx", Arch: "arm64"}, nil, nil)
			if cancelFirst {
				if !errors.Is(err, context.Canceled) || len(seen) != 1 || skipped != 0 {
					t.Fatal(err, seen, skipped)
				}
				return
			}
			if err == nil || len(seen) != 2 || seen[0] != "github.com" || seen[1] != "cdn.azul.com" || skipped != 2 {
				t.Fatal("binary failure did not try next vendor or damaged task progress", err, seen, skipped)
			}
		})
	}
}
