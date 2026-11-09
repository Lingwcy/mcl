package app

import (
	"context"
	"mcl/internal/core/diagnostics"
	"mcl/internal/core/java"
	"testing"
)

func TestTaskResumeRejectsEvidenceFromPreviousAttempt(t *testing.T) {
	service := NewTaskService(t.Context(), newTaskMemory())
	attempts := make(chan context.Context, 2)
	finish := make(chan struct{})
	result := make(chan error, 1)
	go func() {
		result <- service.Execute(t.Context(), taskTestIntent(), func(ctx context.Context) error {
			diagnostics.SetFacts(ctx, diagnostics.Facts{Game: "1.12.2"})
			_, span := diagnostics.Start(ctx, "processor", "forge:processor:1")
			defer span.End()
			attempts <- ctx
			if TaskAttempt(ctx) == 1 {
				<-ctx.Done()
				return ctx.Err()
			}
			<-finish
			return nil
		})
	}()
	old := taskReceive(t, attempts)
	groups, err := service.List(t.Context())
	if err != nil || len(groups) != 1 {
		t.Fatal(groups, err)
	}
	id := groups[0].ID
	if err := service.Control(t.Context(), id, "pause"); err != nil {
		t.Fatal(err)
	}
	paused, err := service.Details(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := paused.Evidence.Current(); ok {
		t.Fatal("pause reported before diagnostic scopes drained")
	}
	if err := service.Resume(t.Context(), id); err != nil {
		t.Fatal(err)
	}
	taskReceive(t, attempts)
	diagnostics.SetFacts(old, diagnostics.Facts{Game: "stale"})
	current, err := service.Details(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	if current.Attempt != 2 || current.Evidence.Facts.Game != "1.12.2" {
		t.Fatal(current)
	}
	close(finish)
	if err := taskReceive(t, result); err != nil {
		t.Fatal(err)
	}
}

func TestProcessorJavaEvidenceNeverOverwritesGameRuntime(t *testing.T) {
	var evidence diagnostics.Snapshot
	ctx := diagnostics.WithObserver(t.Context(), evidence.Apply)
	reportRuntimeFacts(ctx, java.Runtime{Version: "1.8.0_402", Arch: "amd64", VM: "hotspot"})
	tool := context.WithValue(ctx, runtimePurposeKey{}, true)
	reportRuntimeFacts(tool, java.Runtime{Version: "17.0.12", Arch: "amd64", VM: "hotspot"})
	if evidence.Facts.JavaVersion != "1.8.0_402" || evidence.Facts.ToolJavaVersion != "17.0.12" {
		t.Fatal(evidence.Facts)
	}
}
