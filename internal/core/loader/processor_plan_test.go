package loader

import (
	"errors"
	"os"
	"path"
	"strings"
	"testing"

	"mcl/internal/core/archive"
	"mcl/internal/core/meta"
)

func TestOfficialClientProcessorPreflightKeepsOriginalPositions(t *testing.T) {
	var sample officialSample
	for _, candidate := range officialMetadataSamples(t) {
		if candidate.Game == "1.20.4" {
			sample = candidate
		}
	}
	profile := officialProjection(t, sample.Profile, sample.ProfileProjectionSHA256)
	version := officialProjection(t, sample.Version, sample.VersionProjectionSHA256)
	files := map[string][]byte{"install_profile.json": profile, "version.json": version, "data/client.lzma": []byte("metadata-only placeholder, not a real patch")}
	z := officialMetadataArchive(t, files)
	request := Request{Type: "forge", GameVersion: sample.Game, Version: sample.Build, BaseSHA1: strings.Repeat("a", 40)}
	spec, runtime, err := readForgeSpec(t.Context(), z, request)
	if err != nil {
		t.Fatal(err)
	}
	values, resources, err := forgeData(spec)
	if err != nil {
		t.Fatal(err)
	}
	var processors []preparedProcessor
	positions := []int{3, 4, 5, 7, 8}
	for index, entry := range spec.Processors {
		p, err := prepareForgeProcessor(entry, values, t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		if p.position != positions[index] || p.total != 8 {
			t.Fatal(p.position, p.total)
		}
		processors = append(processors, p)
	}
	libraries, err := forgeLibraries(spec.Libraries, runtime.Libraries)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateProcessorPlan(z, libraries, processors, resources, runtime.Libraries); err != nil {
		t.Fatal(err)
	}
	// Merely recognizing metadata is not download, processing or installation.
}
func processorPlanFixture(t *testing.T) (*archive.Zip, map[string]forgeLibrary, []preparedProcessor) {
	t.Helper()
	coordinate, _ := meta.ParseCoordinate("net.minecraftforge:binarypatcher:1.2.0")
	library := forgeLibrary{name: coordinate.String(), file: meta.Download{Path: coordinate.Path(), URL: "https://maven.minecraftforge.net/" + coordinate.Path()}}
	tool := path.Join("libraries", coordinate.Path())
	steps := []preparedProcessor{
		{coordinate: coordinate.String(), position: 3, total: 8, jar: tool, classpath: []string{tool}, inputs: []string{"work/client.jar"}, outputs: map[string]string{"libraries/intermediate.jar": ""}},
		{coordinate: coordinate.String(), position: 7, total: 8, jar: tool, classpath: []string{tool}, inputs: []string{"libraries/intermediate.jar"}, outputs: map[string]string{"libraries/result.jar": ""}},
	}
	return officialMetadataArchive(t, map[string][]byte{"fixture": []byte("inventory")}), map[string]forgeLibrary{coordinate.Path(): library}, steps
}
func TestProcessorPlanRejectsMissingInputsForwardReferencesAndToolWrites(t *testing.T) {
	for _, name := range []string{"missing-tool", "missing-input", "forward-input", "tool-output", "tool-parent-output", "empty-directory-is-not-file"} {
		t.Run(name, func(t *testing.T) {
			z, libraries, steps := processorPlanFixture(t)
			switch name {
			case "missing-tool":
				clear(libraries)
			case "missing-input":
				steps[0].inputs = []string{"libraries/missing.jar"}
			case "forward-input":
				steps[0].inputs = []string{"libraries/result.jar"}
			case "tool-output":
				steps[0].outputs = map[string]string{steps[0].jar: ""}
			case "tool-parent-output":
				steps[0].outputs = map[string]string{path.Dir(steps[0].jar): ""}
			case "empty-directory-is-not-file":
				steps[0].nativeTask = "CREATE_DIR"
				steps[0].outputs = nil
				steps[0].inputs = nil
				steps[0].nativeValues = []forgeValue{{text: "--task"}, {text: "CREATE_DIR"}, {text: "--target"}, {text: "libraries/empty", file: true}}
				steps[1].inputs = []string{"libraries/empty/unproduced.jar"}
			}
			err := validateProcessorPlan(z, libraries, steps, nil, nil)
			var detail *ProcessorError
			if !errors.As(err, &detail) || detail.Position == 0 || !strings.HasPrefix(detail.Tool, "net.minecraftforge:") {
				t.Fatal("missing precise failure", err)
			}
		})
	}
}
func TestProcessorPlanAllowsOrderedPrivateDataRewrite(t *testing.T) {
	z, libraries, steps := processorPlanFixture(t)
	steps[1].outputs = map[string]string{"libraries/intermediate.jar": ""}
	if err := validateProcessorPlan(z, libraries, steps, nil, nil); err != nil {
		t.Fatal(err)
	}
}
func TestProcessorOutputSetIsCommittedOnlyWhenComplete(t *testing.T) {
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	known := map[string]meta.Download{"libraries/old.jar": stageFixed(t, root, "libraries/old.jar", []byte("old"))}
	first := stageFixed(t, root, "libraries/a.jar", []byte("first"))
	stageFixed(t, root, "libraries/b.jar", []byte("second"))
	p := preparedProcessor{outputs: map[string]string{"libraries/a.jar": first.SHA1, "libraries/b.jar": strings.Repeat("0", 40)}}
	err = commitProcessorOutputs(t.Context(), root, known, p)
	if !errors.Is(err, errForgeDigestMismatch) || len(known) != 1 {
		t.Fatal("partial output set accepted", known, err)
	}
}
