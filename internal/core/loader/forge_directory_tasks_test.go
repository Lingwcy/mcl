package loader

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"mcl/internal/core/download"
	"mcl/internal/core/meta"
)

func TestMcpDirectoryOutputsAreIndividuallyPinned(t *testing.T) {
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	data := legacyTestZip(t, map[string][]byte{"config.json": []byte(`{"data":{"client":"client/"}}`), "client/a.txt": []byte("first"), "client/nested/b.txt": []byte("second")})
	known := map[string]meta.Download{"libraries/config.zip": stageFixed(t, root, "libraries/config.zip", data)}
	values := map[string]forgeValue{"INPUT": {"libraries/config.zip", true}, "OUTPUT": {"libraries/config-data", true}}
	p, err := prepareForgeProcessor(forgeProcessor{Jar: "net.minecraftforge:installertools:1.0", Args: []string{"--task", "MCP_DATA", "--input", "{INPUT}", "--key", "client", "--output", "{OUTPUT}"}}, values, root.Name())
	if err != nil {
		t.Fatal(err)
	}
	installer := NewForgeInstaller(nil, nil, nil)
	if err := installer.runProcessors(t.Context(), root, root, root.Name(), "", meta.CurrentPlatform(), meta.Version{}, []preparedProcessor{p}, known, func(download.Progress) {}, nil); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"libraries/config-data/a.txt", "libraries/config-data/nested/b.txt"} {
		if _, ok := known[name]; !ok {
			t.Fatal("extracted file lacks provenance", name)
		}
	}
	if err := verifyProcessorInputs(t.Context(), root, known, []string{"libraries/config-data"}); err != nil {
		t.Fatal(err)
	}
	stageFixed(t, root, "libraries/config-data/unknown.txt", []byte("foreign"))
	if err := verifyProcessorInputs(t.Context(), root, known, []string{"libraries/config-data"}); err == nil {
		t.Fatal("directory admitted an unpinned file")
	}
}

func TestProcessorPathAliasesAndNativeOptionsAreRejected(t *testing.T) {
	p := preparedProcessor{inputs: []string{"libraries/Library.jar"}, outputs: map[string]string{"libraries/library.jar": ""}}
	if err := validateProcessorPaths(nil, []preparedProcessor{p}); err == nil {
		t.Fatal("case alias crossed processor steps")
	}
	var spec forgeSpec
	if err := json.Unmarshal([]byte(`{"minecraft":"1.20.4","data":{"OUT":{"client":"{LIBRARY_DIR}/output.txt"}}}`), &spec); err != nil {
		t.Fatal(err)
	}
	values, _, err := forgeData(spec)
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"--task", "EXTRACT_FILES", "--archive", "{INSTALLER}", "--from", "../bad", "--to", "{OUT}"},
		{"--task", "EXTRACT_FILES", "--archive", "{INSTALLER}", "--from", "data/good", "--to", "{OUT}", "--execs", "{OUT}"},
		{"--task", "DOWNLOAD_MOJMAPS", "--version", "1.20.4", "--side", "client", "--output", "{OUT}", "--unknown", "yes"},
	} {
		if _, err := prepareForgeProcessor(forgeProcessor{Jar: "net.minecraftforge:installertools:1.0", Args: args}, values, t.TempDir()); err == nil {
			t.Fatal("unsafe native options accepted", strings.Join(args, " "))
		}
	}
}
