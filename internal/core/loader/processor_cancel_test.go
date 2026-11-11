package loader

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"mcl/internal/core/diagnostics"
	"mcl/internal/core/download"
	"mcl/internal/core/meta"
)

func TestCancelledReuseCannotDeleteTrackedOutput(t *testing.T) {
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	fixed := stageFixed(t, root, "libraries/output.jar", []byte("keep"))
	known := map[string]meta.Download{fixed.Path: fixed}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = prepareProcessorOutputs(ctx, root, known, preparedProcessor{outputs: map[string]string{fixed.Path: strings.Repeat("b", 40)}})
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if data, err := root.ReadFile(fixed.Path); err != nil || string(data) != "keep" {
		t.Fatal(string(data), err)
	}
}
func TestProcessorFailureClosesOnlyCurrentOriginalStep(t *testing.T) {
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	fixed := stageFixed(t, root, "libraries/ready.jar", []byte("ready"))
	known := map[string]meta.Download{fixed.Path: fixed}
	tool := "net.minecraftforge:binarypatcher:1.2.0"
	steps := []preparedProcessor{
		{position: 3, total: 8, coordinate: tool, outputs: map[string]string{fixed.Path: fixed.SHA1}},
		{position: 7, total: 8, coordinate: tool, inputs: []string{"libraries/missing.jar"}, outputs: map[string]string{"libraries/not-published.jar": ""}},
	}
	var snapshot diagnostics.Snapshot
	ctx := diagnostics.WithObserver(t.Context(), snapshot.Apply)
	installer := NewForgeInstaller(nil, nil, nil)
	err = installer.runProcessors(ctx, root, root, root.Name(), "", meta.CurrentPlatform(), meta.Version{}, steps, known, func(download.Progress) {}, nil)
	var detail *ProcessorError
	if !errors.As(err, &detail) || detail.Position != 7 || detail.Stage != "输入核验" || detail.Tool != tool {
		t.Fatal(err)
	}
	for _, record := range snapshot.Records {
		if record.EndedAt.IsZero() {
			t.Fatal("step remained open", record)
		}
		if record.Step == 3 && record.Code != "" {
			t.Fatal("later failure changed completed step", record)
		}
		if record.Step == 7 && (record.Code == "" || record.Subject != tool) {
			t.Fatal(record)
		}
	}
}
func TestObsoleteAdapterRequestsAreRejected(t *testing.T) {
	for _, policy := range []string{"official-installer-v2", "official-installer-v3", "unknown"} {
		t.Run(policy, func(t *testing.T) {
			request := shortForgeRequest()
			request.AdapterPolicy = policy
			if request.Validate() == nil {
				t.Fatal("obsolete adapter request accepted")
			}
			if _, err := request.WithPlatform(meta.Platform{OS: request.TargetOS, Arch: request.TargetArch}); err == nil {
				t.Fatal("obsolete request silently upgraded")
			}
			canonical := request
			canonical.Version += "-1.7.10"
			if ValidateForgeAlias(request, canonical) == nil {
				t.Fatal("obsolete request authorized alias reuse")
			}
		})
	}
	// Empty policy belongs to a fresh builder; WithPlatform binds the one
	// current policy. There is no old-policy migration or read compatibility.
	request := shortForgeRequest()
	request.AdapterPolicy = ""
	current, err := request.WithPlatform(meta.Platform{OS: request.TargetOS, Arch: request.TargetArch})
	if err != nil || current.AdapterPolicy != ForgeAdapterPolicy {
		t.Fatal("fresh request did not bind current policy", err)
	}
}
