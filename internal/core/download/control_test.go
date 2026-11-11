package download

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestTransferPreferencesValidationAndAutomaticBounds(t *testing.T) {
	for _, value := range []int{-1, 129} {
		if (Preferences{Concurrency: value}).Validate() == nil {
			t.Errorf("accepted invalid concurrency %d", value)
		}
	}
	for _, cpus := range []int{0, 1, 8, 128} {
		if got := (Preferences{}).Limit(cpus); got < 1 || got > 32 {
			t.Errorf("unbounded automatic concurrency %d", got)
		}
	}
	if got := (Preferences{Concurrency: 128}).Limit(1); got != 128 {
		t.Fatal("manual concurrency was not honored")
	}
}

func TestTransferControlLiveChangesCancellationAndIdempotentRelease(t *testing.T) {
	control, err := NewTransferControl(Preferences{Concurrency: 1})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	first, err := control.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer first()
	waiting, stop := context.WithCancel(ctx)
	result := make(chan error, 1)
	go func() {
		release, err := control.Acquire(waiting)
		if release != nil {
			release()
		}
		result <- err
	}()
	stop()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("waiting acquisition was not cancelled: %v", err)
	}
	if err := control.SetPreferences(Preferences{Concurrency: 2}); err != nil {
		t.Fatal(err)
	}
	second, err := control.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer second()
	if err := control.SetPreferences(Preferences{Concurrency: 1}); err != nil {
		t.Fatal(err)
	}
	control.mu.Lock()
	limit, active := control.limit, control.active
	control.mu.Unlock()
	if limit != 1 || active != 2 {
		t.Fatalf("existing transfers were not allowed to drain: limit=%d active=%d", limit, active)
	}
	first()
	first()
	second()
	control.mu.Lock()
	active = control.active
	control.mu.Unlock()
	if active != 0 {
		t.Fatalf("release leaked slots or ran twice: %d", active)
	}
	if err := control.SetPreferences(Preferences{Concurrency: 129}); err == nil || control.Limit() != 1 {
		t.Fatal("invalid live update changed the budget")
	}
}

func TestTransferControlIsSharedAcrossClientsAndSkipsVerifiedCache(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	const total = 6
	entered, permit := make(chan struct{}, total), make(chan struct{}, total)
	var active, requests atomic.Int32
	var exceeded atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if active.Add(1) > 2 {
			exceeded.Store(true)
		}
		defer active.Add(-1)
		entered <- struct{}{}
		select {
		case <-permit:
			fmt.Fprint(w, "verified object")
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(server.Close)
	t.Cleanup(cancel)
	control, err := NewTransferControl(Preferences{Concurrency: 2})
	if err != nil {
		t.Fatal(err)
	}
	first, second := NewClient(t.TempDir()), NewClient(t.TempDir())
	first.Transfers, second.Transfers = control, control
	files := make([]File, total/2)
	for i := range files {
		files[i] = testFile(server.URL, fmt.Sprintf("cache/%d", i), "verified object")
	}
	results := make(chan error, 2)
	for _, client := range []*Client{first, second} {
		go func() { results <- client.Run(ctx, files, nil) }()
	}
	// Fill both slots before releasing either response, then allow the rest.
	for range 2 {
		select {
		case <-entered:
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	permit <- struct{}{}
	permit <- struct{}{}
	for range total - 2 {
		select {
		case <-entered:
			permit <- struct{}{}
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	for range 2 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	if exceeded.Load() || requests.Load() != total {
		t.Fatalf("clients did not share the budget: exceeded=%v requests=%d", exceeded.Load(), requests.Load())
	}
	for range 2 {
		release, err := control.Acquire(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer release()
	}
	if err := first.Run(ctx, files, nil); err != nil || requests.Load() != total {
		t.Fatalf("verified cache incorrectly waited for a network slot: %v", err)
	}
}
