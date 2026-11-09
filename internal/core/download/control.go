package download

import (
	"context"
	"sync"
	"time"

	"mcl/internal/core/diagnostics"
)

// TransferControl bounds actual network transfers across all task groups and
// file clients, including Java downloads. It does not own persistent task state.
// Construct it with NewTransferControl. A reduction drains existing transfers
// naturally instead of killing them.
type TransferControl struct {
	mu          sync.Mutex
	limit       int
	active      int
	changed     chan struct{}
	preferences Preferences
	rate        int64
	tokens      float64
	updated     time.Time
	rateEpoch   uint64
	preferred   map[string]sourceHealth
}

func NewTransferControl(preferences Preferences) (*TransferControl, error) {
	if err := preferences.Validate(); err != nil {
		return nil, err
	}
	rate := int64(preferences.SpeedLimitKiB) * 1024
	return &TransferControl{
		limit: automaticLimit(preferences), changed: make(chan struct{}), preferences: preferences,
		rate: rate, updated: time.Now(), tokens: burst(rate), rateEpoch: 1,
		preferred: make(map[string]sourceHealth),
	}, nil
}

func (c *TransferControl) SetPreferences(preferences Preferences) error {
	if err := preferences.Validate(); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.preferences == preferences {
		return nil
	}
	if c.preferences.SpeedLimitKiB != preferences.SpeedLimitKiB {
		c.refill(time.Now())
		c.rate = int64(preferences.SpeedLimitKiB) * 1024
		c.tokens = min(c.tokens, burst(c.rate))
		c.rateEpoch++
	}
	if c.preferences.Source != preferences.Source || c.preferences.Proxy != preferences.Proxy {
		clear(c.preferred)
	}
	c.preferences = preferences
	c.limit = automaticLimit(preferences)
	c.notify()
	return nil
}

func (c *TransferControl) Limit() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.limit
}

func (c *TransferControl) notify() {
	close(c.changed)
	c.changed = make(chan struct{})
}

// Acquire is cancellable and returns an idempotent release. Waiters do not
// consume workers beyond their caller's already bounded file/task group.
func (c *TransferControl) Acquire(ctx context.Context) (func(), error) {
	var waiting *diagnostics.Span
	defer func() {
		if waiting != nil {
			waiting.End()
		}
	}()
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		c.mu.Lock()
		if c.active < c.limit {
			c.active++
			c.mu.Unlock()
			var once sync.Once
			return func() {
				once.Do(func() {
					c.mu.Lock()
					defer c.mu.Unlock()
					c.active--
					c.notify()
				})
			}, nil
		}
		changed := c.changed
		c.mu.Unlock()
		if waiting == nil {
			_, waiting = diagnostics.Start(ctx, "quota", "transfer-budget")
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-changed:
		}
	}
}
