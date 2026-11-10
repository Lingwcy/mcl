package loader

import (
	"context"
	"io"
	"sync"
	"time"

	"mcl/internal/core/diagnostics"
)

const (
	maxMetadataRequests    = 1024
	maxMetadataRetries     = 64
	maxMetadataBytes       = 32 << 20
	metadataRequestTimeout = 30 * time.Second
)

type resolutionBudget struct {
	mu                sync.Mutex
	requests, retries int
	bytes             int64
}
type resolutionBudgetKey struct{}

// WithResolutionBudget joins nested catalog/identity lookups into one budget.
// It has no whole-install wall clock: downloads may wait on deliberate rate
// limits and processors have their separate lifecycle and existing deadline.
func WithResolutionBudget(ctx context.Context) context.Context {
	if _, ok := ctx.Value(resolutionBudgetKey{}).(*resolutionBudget); ok {
		return ctx
	}
	return context.WithValue(ctx, resolutionBudgetKey{}, &resolutionBudget{})
}
func (b *resolutionBudget) admit(retry bool) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.requests >= maxMetadataRequests || retry && b.retries >= maxMetadataRetries || b.bytes >= maxMetadataBytes {
		return diagnostics.Wrap("metadata-budget", errMetadataBudget)
	}
	b.requests++
	if retry {
		b.retries++
	}
	return nil
}
func (b *resolutionBudget) reserveRead(wanted int) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	remaining := maxMetadataBytes - b.bytes
	if remaining <= 0 {
		return 0, diagnostics.Wrap("metadata-budget", errMetadataBudget)
	}
	grant := min(wanted, 32<<10, int(remaining))
	b.bytes += int64(grant)
	return grant, nil
}
func (b *resolutionBudget) refundRead(unused int) {
	b.mu.Lock()
	b.bytes -= int64(unused)
	b.mu.Unlock()
}

type metadataBody struct {
	io.ReadCloser
	budget *resolutionBudget
	cancel context.CancelFunc
}

func (b *metadataBody) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	grant, err := b.budget.reserveRead(len(p))
	if err != nil {
		b.cancel()
		return 0, err
	}
	n, err := b.ReadCloser.Read(p[:grant])
	b.budget.refundRead(grant - n)
	return n, err
}
func (b *metadataBody) Close() error { defer b.cancel(); return b.ReadCloser.Close() }
