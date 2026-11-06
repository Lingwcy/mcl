package process

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const gateFlag = "--mcl-launch-gate"
const maxGateBytes = 1 << 20

type gateMessage struct {
	Executable string
	Arguments  []string
}

// GateMain must run before normal flag/UI initialization in the executable.
// A new gate cannot execute Java until its parent has persisted OS identity
// and delivered a complete bounded handoff through its anonymous stdin pipe.
// Parent death/EOF/timeout before handoff simply exits this unlaunched gate.
func GateMain(arguments []string) (handled bool, code int) {
	if len(arguments) == 0 || (arguments[0] != gateFlag && arguments[0] != "--grun-launch-gate") {
		return false, 0
	}
	if len(arguments) != 1 {
		return true, 125
	}
	timer := time.AfterFunc(10*time.Second, func() { os.Exit(125) })
	defer timer.Stop()
	data, err := io.ReadAll(io.LimitReader(os.Stdin, maxGateBytes+1))
	if err != nil || len(data) > maxGateBytes {
		return true, 125
	}
	var message gateMessage
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(&message); err != nil {
		return true, 125
	}
	if err := d.Decode(new(any)); !errors.Is(err, io.EOF) {
		return true, 125
	}
	if !filepath.IsAbs(message.Executable) || len(message.Arguments) == 0 || strings.ContainsRune(message.Executable, '\x00') {
		return true, 125
	}
	for _, arg := range message.Arguments {
		if strings.ContainsRune(arg, '\x00') {
			return true, 125
		}
	}
	if err := gateExec(message.Executable, message.Arguments); err != nil {
		fmt.Fprintln(os.Stderr, "启动进程交接失败")
	}
	return true, 125 // successful exec never returns
}
