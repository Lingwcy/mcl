package loader

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"testing"

	"mcl/internal/core/diagnostics"
)

type metadataUnreadBody struct{ reads, closes int }

func (b *metadataUnreadBody) Read([]byte) (int, error) {
	b.reads++
	return 0, errors.New("probe must not consume artifact body")
}
func (b *metadataUnreadBody) Close() error { b.closes++; return nil }

func TestMavenSizeFallbackNeverConsumesArtifact(t *testing.T) {
	for _, head := range []int{200, 405, 501} {
		for _, rangeStatus := range []int{200, 206} {
			c := NewClient()
			requests := 0
			var bodies []*metadataUnreadBody
			c.HTTP.Transport = forgeAliasTransport(func(r *http.Request) (*http.Response, error) {
				requests++
				body := &metadataUnreadBody{}
				bodies = append(bodies, body)
				response := &http.Response{Header: make(http.Header), Body: body, Request: r}
				if r.Method == http.MethodHead {
					response.StatusCode = head
					return response, nil
				}
				if r.Method != http.MethodGet || r.Header.Get("Range") != "bytes=0-0" || r.Header.Get("Accept-Encoding") != "identity" {
					t.Fatal(r.Method, r.Header)
				}
				response.StatusCode = rangeStatus
				if rangeStatus == 206 {
					response.Header.Set("Content-Range", "bytes 0-0/419701")
					response.Header.Set("Content-Length", "1")
				} else {
					response.Header.Set("Content-Length", "419701")
				}
				return response, nil
			})
			size, err := c.mavenSize(t.Context(), "https://maven.minecraftforge.net/org/example/lib/1/lib-1.jar")
			if err != nil || size != 419701 || requests != 2 {
				t.Fatal(head, rangeStatus, size, err, requests)
			}
			for _, body := range bodies {
				if body.reads != 0 || body.closes != 1 {
					t.Fatal("probe consumed or leaked body", *body)
				}
			}
		}
	}
}
func TestMavenSizeDoesNotFallbackOnAuthMissingArtifactOrBadLength(t *testing.T) {
	for _, tc := range []struct {
		status int
		length string
	}{{401, ""}, {403, ""}, {404, ""}, {410, ""}, {200, "0"}, {200, "-1"}, {200, "512,512"}, {200, strconv.FormatInt(maxMavenArtifactSize+1, 10)}} {
		c := NewClient()
		calls := 0
		c.HTTP.Transport = forgeAliasTransport(func(r *http.Request) (*http.Response, error) {
			calls++
			h := make(http.Header)
			if tc.length != "" {
				h.Set("Content-Length", tc.length)
			}
			return &http.Response{StatusCode: tc.status, Header: h, Body: io.NopCloser(&emptyMetadataReader{}), Request: r}, nil
		})
		if _, err := c.mavenSize(t.Context(), "https://maven.minecraftforge.net/org/example/lib/1/lib-1.jar"); err == nil || calls != 1 {
			t.Fatal(tc, err, calls)
		}
	}
}

type emptyMetadataReader struct{}

func (*emptyMetadataReader) Read([]byte) (int, error) { return 0, io.EOF }
func TestMavenRangeRejectsInconsistentOrUnknownSize(t *testing.T) {
	for _, tc := range []struct {
		status                       int
		rangeValue, length, encoding string
	}{
		{206, "bytes 1-1/419701", "1", ""}, {206, "bytes 0-1/419701", "2", ""}, {206, "bytes 0-0/*", "1", ""},
		{206, "bytes 0-0/0", "1", ""}, {206, "bytes 0-0/419701", "2", ""}, {206, "bytes 0-0/419701", "1", "gzip"},
		{200, "", "", ""}, {200, "bytes 0-0/419701", "419701", ""},
	} {
		c := NewClient()
		c.HTTP.Transport = forgeAliasTransport(func(r *http.Request) (*http.Response, error) {
			h := make(http.Header)
			status := 405
			if r.Method != http.MethodHead {
				status = tc.status
				if tc.rangeValue != "" {
					h.Set("Content-Range", tc.rangeValue)
				}
				if tc.length != "" {
					h.Set("Content-Length", tc.length)
				}
				if tc.encoding != "" {
					h.Set("Content-Encoding", tc.encoding)
				}
			}
			return &http.Response{StatusCode: status, Header: h, Body: io.NopCloser(&emptyMetadataReader{}), Request: r}, nil
		})
		_, err := c.mavenSize(t.Context(), "https://maven.minecraftforge.net/org/example/lib/1/lib-1.jar")
		if err == nil || diagnostics.Code(err) != "artifact-identity" {
			t.Fatal(tc, err)
		}
	}
}
