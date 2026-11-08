package download

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestRateBudgetRefundsUnusedBytesAndPreservesValidation(t *testing.T) {
	control, err := NewTransferControl(Preferences{SpeedLimitKiB: 1})
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		reader := &limitedReader{ctx: t.Context(), control: control, reader: bytes.NewReader([]byte("a"))}
		if n, err := reader.Read(make([]byte, 4096)); err != nil || n != 1 {
			t.Fatalf("short asset read: %d, %v", n, err)
		}
	}
	control.mu.Lock()
	tokens, rate := control.tokens, control.rate
	control.mu.Unlock()
	if tokens < 253 || tokens > 256 || rate != 1024 {
		t.Fatalf("unused byte allowance was lost: tokens=%f rate=%d", tokens, rate)
	}
	if err := control.SetPreferences(Preferences{SpeedLimitKiB: -1}); err == nil {
		t.Fatal("invalid speed accepted")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	reader := &limitedReader{ctx: ctx, control: control, reader: strings.NewReader("never consumed")}
	if _, err := reader.Read(make([]byte, 10)); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestRateWaitIsNotAnHTTPWallClockTimeout(t *testing.T) {
	control, err := NewTransferControl(Preferences{SpeedLimitKiB: 1})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	content := strings.Repeat("a", 1024)
	client := &http.Client{Timeout: 100 * time.Millisecond, Transport: policyRoundTripper(func(request *http.Request) (*http.Response, error) {
		return policyResponse(request, content), nil
	})}
	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://example.invalid/file", nil)
	response, err := control.Do(client, request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil || string(body) != content {
		t.Fatalf("rate wait was counted as a network stall: %v", err)
	}
}

func TestDisablingRateLimitWakesAnExistingReader(t *testing.T) {
	control, err := NewTransferControl(Preferences{SpeedLimitKiB: 1})
	if err != nil {
		t.Fatal(err)
	}
	control.mu.Lock()
	control.tokens = 0
	control.updated = time.Now()
	control.mu.Unlock()
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	result := make(chan error, 1)
	go func() {
		reader := &limitedReader{ctx: ctx, control: control, reader: strings.NewReader("ok")}
		_, err := reader.Read(make([]byte, 1024))
		result <- err
	}()
	if err := control.SetPreferences(Preferences{}); err != nil {
		t.Fatal(err)
	}
	if err := <-result; err != nil {
		t.Fatalf("live change did not wake the reader: %v", err)
	}
}

func TestCumulativeNetworkReadBudgetCannotRenewForever(t *testing.T) {
	task, cancel := context.WithCancelCause(t.Context())
	defer cancel(nil)
	timer := time.NewTimer(time.Hour)
	defer timer.Stop()
	reader := &idleReader{reader: strings.NewReader("must not read"), timer: timer, task: task, cancel: cancel, remaining: 0}
	if n, err := reader.Read(make([]byte, 16)); n != 0 || !errors.Is(err, ErrTransferStalled) || !errors.Is(context.Cause(task), ErrTransferStalled) {
		t.Fatalf("exhausted network-read budget resumed: %d, %v", n, err)
	}
}
