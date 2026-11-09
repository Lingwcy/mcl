package download

import (
	"context"
	"io"
	"math"
	"time"

	"mcl/internal/core/diagnostics"
)

func burst(rate int64) float64 { return float64(max(0, min(rate/4, 64<<10))) }

// refill is called under mu and uses monotonic elapsed time.
func (c *TransferControl) refill(now time.Time) {
	if c.rate > 0 {
		c.tokens = min(burst(c.rate), c.tokens+max(0, now.Sub(c.updated).Seconds())*float64(c.rate))
	} else {
		c.tokens = 0
	}
	c.updated = now
}

func (c *TransferControl) take(ctx context.Context, wanted int) (int, uint64, error) {
	var waiting *diagnostics.Span
	defer func() {
		if waiting != nil {
			waiting.End()
		}
	}()
	for {
		if err := ctx.Err(); err != nil {
			return 0, 0, err
		}
		c.mu.Lock()
		if c.rate == 0 {
			epoch := c.rateEpoch
			c.mu.Unlock()
			return min(wanted, 32<<10), epoch, nil
		}
		c.refill(time.Now())
		quantum := min(wanted, 32<<10, int(max(1, c.rate/10)))
		if c.tokens >= float64(quantum) {
			grant := min(wanted, 32<<10, int(c.tokens))
			c.tokens -= float64(grant)
			epoch := c.rateEpoch
			c.mu.Unlock()
			return grant, epoch, nil
		}
		wait := time.Duration(math.Ceil((float64(quantum) - c.tokens) / float64(c.rate) * float64(time.Second)))
		changed := c.changed
		c.mu.Unlock()
		if waiting == nil {
			_, waiting = diagnostics.Start(ctx, "rate", "shared-rate-limit")
		}
		timer := time.NewTimer(max(wait, time.Microsecond))
		select {
		case <-ctx.Done():
			timer.Stop()
			return 0, 0, ctx.Err()
		case <-changed:
			timer.Stop()
		case <-timer.C:
		}
	}
}

func (c *TransferControl) refund(unused int, epoch uint64) {
	if unused <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.rate > 0 && epoch == c.rateEpoch {
		c.refill(time.Now())
		c.tokens = min(burst(c.rate), c.tokens+float64(unused))
		c.notify()
	}
}

type limitedReader struct {
	ctx     context.Context
	control *TransferControl
	reader  io.Reader
}

func (r *limitedReader) Read(buffer []byte) (int, error) {
	if len(buffer) == 0 {
		return 0, nil
	}
	grant, epoch, err := r.control.take(r.ctx, len(buffer))
	if err != nil {
		return 0, err
	}
	n, err := r.reader.Read(buffer[:grant])
	if n > 0 {
		diagnostics.Work(r.ctx)
	}
	// Small assets and EOF must not consume a full buffer's unused quota.
	r.control.refund(grant-n, epoch)
	return n, err
}
