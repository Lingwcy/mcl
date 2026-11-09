package diagnostics

import (
	"context"
	"errors"
	"io/fs"
	"strings"
	"sync"
	"syscall"
	"testing"
)

func TestWaitingDoesNotInventWorkAndRetriesClearPreviousError(t *testing.T) {
	var snapshot Snapshot
	ctx := WithObserver(t.Context(), snapshot.Apply)
	ctx, parent := Start(ctx, "work", "libraries/a.jar")
	_, waiting := Start(ctx, "quota", "transfer-budget")
	if !snapshot.LastWorkAt.IsZero() {
		t.Fatal("waiting invented work")
	}
	if record, ok := snapshot.Current(); !ok || record.Phase != "quota" {
		t.Fatal(snapshot)
	}
	waiting.End()
	_, network := Start(ctx, "network", "libraries/a.jar")
	network.Network("forge", 403, 1)
	if record, _ := snapshot.Current(); record.Code != "forbidden" {
		t.Fatal(record)
	}
	network.Network("forge", 200, 2)
	network.Work()
	network.End()
	parent.End()
	if snapshot.LastWorkAt.IsZero() {
		t.Fatal("received bytes not recorded")
	}
	for _, r := range snapshot.Records {
		if r.Phase == "network" && (r.Code != "" || r.Attempt != 2) {
			t.Fatal(r)
		}
	}
	if err := snapshot.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestEvidenceIsBoundedAndExportDoesNotRetainSensitiveInputs(t *testing.T) {
	var snapshot Snapshot
	ctx := WithObserver(t.Context(), snapshot.Apply)
	for _, subject := range []string{"https://user:secret@example.com/file?token=secret", "/Users/private/file.jar", "../secret"} {
		_, span := Start(ctx, "network", subject)
		span.Fail(errors.New("access_token=secret"))
		span.End()
	}
	for range MaxRecords + 10 {
		_, span := Start(ctx, "verify", "libraries/a.jar")
		span.End()
	}
	if len(snapshot.Records) != MaxRecords || snapshot.Dropped != 10 {
		t.Fatal(len(snapshot.Records), snapshot.Dropped)
	}
	if strings.Contains(snapshot.Text(), "secret") || strings.Contains(snapshot.Text(), "https://") {
		t.Fatal(snapshot.Text())
	}
	if err := snapshot.Validate(); err != nil {
		t.Fatal(err)
	}
	clone := snapshot.Clone()
	clone.Records[0].Subject = "changed"
	if snapshot.Records[0].Subject == "changed" {
		t.Fatal("snapshot aliases clone")
	}
}

func TestConcurrentEvidenceAndReplacingObserverKeepIndependentParents(t *testing.T) {
	var mu sync.Mutex
	var snapshot Snapshot
	ctx := WithObserver(t.Context(), func(u Update) { mu.Lock(); defer mu.Unlock(); snapshot.Apply(u) })
	ctx, parent := Start(ctx, "processor", "tools:processor:1")
	var workers sync.WaitGroup
	for range 8 {
		workers.Go(func() {
			for range 24 {
				_, s := Start(ctx, "network", "libraries/a.jar")
				s.Work()
				s.End()
			}
		})
	}
	workers.Wait()
	parent.End()
	if err := snapshot.Validate(); err != nil {
		t.Fatal(err)
	}
	var next Snapshot
	fresh := WithObserver(ctx, next.Apply)
	_, child := Start(fresh, "java", "game-runtime")
	child.End()
	if len(next.Records) != 1 || next.Records[0].Parent != 0 {
		t.Fatal(next)
	}
}

func TestErrorsUseTypesNotSensitiveExceptionText(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want string
	}{
		{context.Canceled, "canceled"}, {context.DeadlineExceeded, "deadline"},
		{syscall.ENOSPC, "disk-full"}, {fs.ErrPermission, "permission"},
		{errors.New("HTTP 403 secret timeout checksum"), "unknown"},
		{Wrap("integrity", errors.New("secret")), "integrity"},
	} {
		if got := Code(tc.err); got != tc.want {
			t.Fatalf("%s != %s", got, tc.want)
		}
	}
	if Source("https://github.com/unrelated/project") != "other" {
		t.Fatal("unrelated GitHub project mislabeled as Temurin")
	}
}
