package loader

import (
	"context"
	"crypto/sha1"
	"errors"
	"fmt"
	"os"
	"path"
	"reflect"
	"strings"
	"testing"

	"mcl/internal/core/download"
	"mcl/internal/core/meta"
)

func mcpPlanFixture(t *testing.T) (*os.Root, map[string]meta.Download, preparedProcessor) {
	t.Helper()
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { root.Close() })
	payload := legacyTestZip(t, map[string][]byte{"config.json": []byte("{\"data\":{\"patches\":\"patches/\"}}"), "patches/a.patch": []byte("fixed-a"), "patches/nested/b.patch": []byte("fixed-b"), "unrelated.txt": []byte("not selected")})
	known := map[string]meta.Download{"libraries/mcp/config.zip": stageFixed(t, root, "libraries/mcp/config.zip", payload)}
	p, err := prepareForgeProcessor(forgeProcessor{Position: 3, Total: 8, Jar: "net.minecraftforge:installertools:1.4.0", Args: []string{"--task", "MCP_DATA", "--input", "{INPUT}", "--key", "patches", "--output", "{OUTPUT}"}}, map[string]forgeValue{"INPUT": {"libraries/mcp/config.zip", true}, "OUTPUT": {"libraries/patches", true}}, root.Name())
	if err != nil {
		t.Fatal(err)
	}
	return root, known, p
}

func TestMCPDirectoryMaterializesExactMembersBeforeWriting(t *testing.T) {
	root, known, p := mcpPlanFixture(t)
	source := map[string]string{"libraries/patches/a.patch": "fixed-a", "libraries/patches/nested/b.patch": "fixed-b"}
	plan, err := materializeProcessorPlan(t.Context(), root, known, []preparedProcessor{p}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan) != 1 || len(plan[0].outputs) != 2 || plan[0].extraction == nil {
		t.Fatal("concrete member plan missing")
	}
	if len(p.outputs) != 1 || p.extraction != nil || len(known) != 1 {
		t.Fatal("read-only planning mutated source plan/provenance")
	}
	if _, err := root.Lstat("libraries/patches"); !os.IsNotExist(err) {
		t.Fatal("planning extracted output before execution", err)
	}
	for name, data := range source {
		want := fmt.Sprintf("%x", sha1.Sum([]byte(data)))
		if plan[0].outputs[name] != want || plan[0].extraction.files[name].Size != int64(len(data)) {
			t.Fatal("member identity not derived from pinned archive", name)
		}
	}
	installer := NewForgeInstaller(nil, nil, func(context.Context, string, string, []string) error {
		t.Fatal("MCP extraction launched Java")
		return nil
	})
	if err := installer.runProcessors(t.Context(), root, root, root.Name(), "", meta.CurrentPlatform(), meta.Version{}, plan, known, func(download.Progress) {}, nil); err != nil {
		t.Fatal(err)
	}
	for name, data := range source {
		got, err := root.ReadFile(name)
		if err != nil || string(got) != data {
			t.Fatal("extraction differs from fixed member", err)
		}
	}
	if err := verifyProcessorDirectory(t.Context(), root, known, "libraries/patches"); err != nil {
		t.Fatal(err)
	}
	// Exact source archive and member identities also permit safe second reuse.
	if err := installer.runProcessors(t.Context(), root, root, root.Name(), "", meta.CurrentPlatform(), meta.Version{}, plan, known, func(download.Progress) {}, nil); err != nil {
		t.Fatal("complete pinned output set was not reusable", err)
	}
}

func TestMaterializedPlanRejectsImaginaryDescendantsAndRuntimeFiles(t *testing.T) {
	root, known, p := mcpPlanFixture(t)
	next := preparedProcessor{position: 7, total: 8, coordinate: "net.minecraftforge:binarypatcher:1.2.0", nativeTask: "EXTRACT_FILES", inputs: []string{"libraries/patches/never-existed.patch"}}
	if _, err := materializeProcessorPlan(t.Context(), root, known, []preparedProcessor{p, next}, nil); err == nil {
		t.Fatal("directory declaration implied any descendant")
	}
	next.inputs = []string{"libraries/patches"}
	if _, err := materializeProcessorPlan(t.Context(), root, known, []preparedProcessor{p, next}, nil); err != nil {
		t.Fatal("actual directory input rejected", err)
	}
	next.inputs = []string{"libraries/patches/a.patch/child"}
	if _, err := materializeProcessorPlan(t.Context(), root, known, []preparedProcessor{p, next}, nil); err == nil {
		t.Fatal("ordinary output file treated as directory")
	}
	c, _ := meta.ParseCoordinate("example:missing:1")
	if _, err := materializeProcessorPlan(t.Context(), root, known, []preparedProcessor{p}, []meta.Library{{Name: c.String()}}); err == nil {
		t.Fatal("unproduced runtime file accepted")
	}
}

func TestMCPPlanRejectsExpandedAliasesAndSourceOverwrite(t *testing.T) {
	for _, name := range []string{"member-overwrites-source", "case-parent-alias", "unicode-parent-alias", "source-modified", "missing-archive"} {
		t.Run(name, func(t *testing.T) {
			root, known, p := mcpPlanFixture(t)
			processors := []preparedProcessor{p}
			switch name {
			case "member-overwrites-source":
				// Prior Java work may not rewrite an archive used to materialize MCP data.
				processors = append([]preparedProcessor{{position: 1, total: 8, coordinate: "net.minecraftforge:binarypatcher:1.2.0", nativeTask: "EXTRACT_FILES", inputs: []string{p.inputs[0]}, outputs: map[string]string{p.inputs[0]: ""}}}, p)
			case "case-parent-alias":
				known["libraries/Patches/other.txt"] = stageFixed(t, root, "libraries/Patches/other.txt", []byte("fixed"))
			case "unicode-parent-alias":
				// Same normalized parent, distinct physical spelling. Use only a plan
				// identity here; the filesystem's normalization behavior is irrelevant.
				known["libraries/e\u0301/other.txt"] = meta.Download{Path: "libraries/e\u0301/other.txt", SHA1: strings.Repeat("a", 40), Size: 1}
				p.nativeValues[len(p.nativeValues)-1] = forgeValue{"libraries/é", true}
				p.outputs = map[string]string{"libraries/é": ""}
				processors = []preparedProcessor{p}
			case "source-modified":
				if err := root.WriteFile(p.inputs[0], []byte("changed"), 0o600); err != nil {
					t.Fatal(err)
				}
			case "missing-archive":
				clear(known)
			}
			before := make(map[string]string)
			for k, v := range p.outputs {
				before[k] = v
			}
			if result, err := materializeProcessorPlan(t.Context(), root, known, processors, nil); err == nil || result != nil {
				t.Fatal("invalid expanded plan accepted")
			}
			if !reflect.DeepEqual(before, p.outputs) {
				t.Fatal("failed materialization altered caller outputs")
			}
		})
	}
}

func TestDirectoryInputsRequireEveryKnownMember(t *testing.T) {
	root, known, _ := mcpPlanFixture(t)
	name := "libraries/patches/a.patch"
	known[name] = stageFixed(t, root, name, []byte("fixed-a"))
	if err := verifyProcessorDirectory(t.Context(), root, known, path.Dir(name)); err != nil {
		t.Fatal(err)
	}
	if err := root.Remove(name); err != nil {
		t.Fatal(err)
	}
	if err := verifyProcessorDirectory(t.Context(), root, known, path.Dir(name)); err == nil {
		t.Fatal("directory silently lost one fixed member")
	}
	stageFixed(t, root, name, []byte("fixed-a"))
	stageFixed(t, root, "libraries/patches/unrecorded.patch", []byte("foreign"))
	if err := verifyProcessorDirectory(t.Context(), root, known, path.Dir(name)); err == nil {
		t.Fatal("unrecorded directory member accepted")
	}
}

func TestMCPPlanningCancellationCannotProduceMembers(t *testing.T) {
	root, known, p := mcpPlanFixture(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if plan, err := materializeProcessorPlan(ctx, root, known, []preparedProcessor{p}, nil); !errors.Is(err, context.Canceled) || plan != nil {
		t.Fatal("materialization lost cancellation", err)
	}
	if _, err := root.Lstat("libraries/patches"); !os.IsNotExist(err) {
		t.Fatal("cancelled planning wrote files", err)
	}
	if len(p.outputs) != 1 || p.extraction != nil {
		t.Fatal("cancelled plan mutated outputs")
	}
}

func TestSingleMCPMemberBindsDeclaredHashAndReadBudget(t *testing.T) {
	root, known, p := mcpPlanFixture(t)
	member := []byte("fixed single mapping")
	payload := legacyTestZip(t, map[string][]byte{"config.json": []byte("{\"data\":{\"patches\":\"mapping.tsrg\"}}"), "mapping.tsrg": member})
	known[p.inputs[0]] = stageFixed(t, root, p.inputs[0], payload)
	want := fmt.Sprintf("%x", sha1.Sum(member))
	p.outputs = map[string]string{"libraries/patches": want}
	exact, err := resolveMCPProcessor(t.Context(), root, known, p)
	if err != nil || len(exact.outputs) != 1 || exact.outputs["libraries/patches"] != want || exact.extraction.directory != "" {
		t.Fatal("single-member binding changed", err)
	}
	p.outputs["libraries/patches"] = strings.Repeat("0", 40)
	if _, err := resolveMCPProcessor(t.Context(), root, known, p); err == nil {
		t.Fatal("fixed output hash ignored in read-only preflight")
	}
	p.outputs["libraries/patches"] = want
	budget := int64(len(member) - 1)
	if _, err := resolveMCPProcessor(t.Context(), root, known, p, &budget); err == nil {
		t.Fatal("insufficient aggregate read budget accepted")
	}
	if _, err := root.Lstat("libraries/patches"); !os.IsNotExist(err) {
		t.Fatal("failed preflight wrote an output", err)
	}
	changed := known[p.inputs[0]]
	changed.SHA1 = strings.Repeat("1", 40)
	known[p.inputs[0]] = changed
	if _, err := resolveMCPProcessor(t.Context(), root, known, exact); err == nil {
		t.Fatal("materialized source identity was replaced")
	}
}
