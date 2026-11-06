package process_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGateRejectsInvalidHandoffWithoutExecuting(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(t.TempDir(), "executed")
	data, _ := json.Marshal(map[string]any{"Executable": executable, "Arguments": []string{"anything"}, "Unknown": syntheticToken})
	for _, payload := range [][]byte{nil, []byte("{broken " + syntheticToken), data, bytes.Repeat([]byte("x"), (1<<20)+1), []byte(`{"Executable":"java","Arguments":["-version"]}`)} {
		cmd := exec.Command(executable, "--mcl-launch-gate")
		cmd.Env = append(os.Environ(), "MCL_TEST_PROCESS_MODE=marker", "MCL_TEST_EXECUTED_FILE="+marker)
		cmd.Stdin = bytes.NewReader(payload)
		output, err := cmd.CombinedOutput()
		var exit *exec.ExitError
		if !errors.As(err, &exit) || exit.ExitCode() != 125 {
			t.Fatalf("invalid handoff exit: %v", err)
		}
		if strings.Contains(string(output), syntheticToken) {
			t.Fatal("invalid protocol disclosed credential data")
		}
		if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("invalid handoff executed target")
		}
	}
}
