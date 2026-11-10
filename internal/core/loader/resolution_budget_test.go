package loader

import (
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"mcl/internal/core/diagnostics"
)

func TestMetadataBudgetSharedAcrossNestedLookups(t *testing.T) {
	ctx := WithResolutionBudget(t.Context())
	if WithResolutionBudget(ctx) != ctx {
		t.Fatal("nested lookup reset budget")
	}
	budget := ctx.Value(resolutionBudgetKey{}).(*resolutionBudget)
	budget.requests = maxMetadataRequests - 2
	c := NewClient()
	calls := 0
	c.HTTP.Transport = forgeAliasTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		deadline, ok := r.Context().Deadline()
		if !ok || time.Until(deadline) > metadataRequestTimeout {
			t.Fatal("request lacks bounded body/header context")
		}
		return forgeAliasResponse(200, "fixture"), nil
	})
	for range 2 {
		if _, err := c.bytes(ctx, "https://maven.minecraftforge.net/catalog", 64); err != nil {
			t.Fatal(err)
		}
	}
	_, err := c.bytes(ctx, "https://maven.minecraftforge.net/catalog", 64)
	if diagnostics.Code(err) != "metadata-budget" || calls != 2 {
		t.Fatal(err, calls)
	}
}
func TestMetadataRetryAllowanceCannotMultiplyPerLibrary(t *testing.T) {
	b := &resolutionBudget{}
	for range maxMetadataRetries {
		if err := b.admit(true); err != nil {
			t.Fatal(err)
		}
	}
	if err := b.admit(true); diagnostics.Code(err) != "metadata-budget" {
		t.Fatal(err)
	}
	if err := b.admit(false); err != nil {
		t.Fatal("ordinary lookup charged as retry", err)
	}
	b = &resolutionBudget{}
	var accepted atomic.Int32
	var workers sync.WaitGroup
	for range 16 {
		workers.Go(func() {
			for range 100 {
				if b.admit(false) == nil {
					accepted.Add(1)
				}
			}
		})
	}
	workers.Wait()
	if accepted.Load() != maxMetadataRequests {
		t.Fatal("concurrent admission exceeded budget", accepted.Load())
	}
}
func TestMetadataResponseBytesCannotExceedSharedBudget(t *testing.T) {
	ctx := WithResolutionBudget(t.Context())
	budget := ctx.Value(resolutionBudgetKey{}).(*resolutionBudget)
	budget.bytes = maxMetadataBytes - 4
	c := NewClient()
	c.HTTP.Transport = forgeAliasTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("0123456789")), Request: r}, nil
	})
	data, err := c.bytes(ctx, "https://maven.minecraftforge.net/catalog", 64)
	if diagnostics.Code(err) != "metadata-budget" || len(data) != 0 || budget.bytes != maxMetadataBytes {
		t.Fatal(data, err, budget.bytes)
	}
}
