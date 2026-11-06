package launch

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
)

func TestStreamingRedactionAcrossEveryBoundary(t *testing.T) {
	secret := "synthetic-secret-1234567890"
	for split := 0; split <= len(secret); split++ {
		o := (Plan{secrets: []string{secret}}).NewOutput()
		w := o.Stream("stdout")
		fmt.Fprint(w, "before "+secret[:split])
		fmt.Fprint(w, secret[split:]+" after\n")
		w.Close()
		lines := o.Snapshot().Lines
		if len(lines) != 1 || lines[0].Text != "before [REDACTED] after" {
			t.Fatalf("split %d: %+v", split, lines)
		}
	}
	for end := 1; end < len(secret); end++ {
		o := (Plan{secrets: []string{secret}}).NewOutput()
		w := o.Stream("stderr")
		fmt.Fprint(w, "truncated "+secret[:end])
		w.Close()
		if lines := o.Snapshot().Lines; len(lines) != 1 || lines[0].Text != "truncated [REDACTED]" {
			t.Fatalf("incomplete secret at %d: %+v", end, lines)
		}
	}
}

func TestOutputDiagnosticsCannotExposeSecrets(t *testing.T) {
	secret := "synthetic-secret-1234567890"
	o := (Plan{secrets: []string{secret}}).NewOutput()
	w := o.Stream("stdout")
	fmt.Fprintln(w, secret)
	w.Close()
	data, _ := json.Marshal(o)
	for _, value := range []string{string(data), fmt.Sprintf("%+v", o), fmt.Sprintf("%#v", o)} {
		if strings.Contains(value, secret) {
			t.Fatal("output diagnostics disclosed credentials")
		}
	}
}

func TestBoundedOutputLongLinesAndConcurrentStreams(t *testing.T) {
	secret := "synthetic-secret"
	o := (Plan{secrets: []string{secret}}).NewOutput()
	var wg sync.WaitGroup
	for _, name := range []string{"stdout", "stderr"} {
		wg.Go(func() {
			w := o.Stream(name)
			defer w.Close()
			fmt.Fprint(w, strings.Repeat("x", 1<<20)+secret+"\n")
			for i := range 6000 {
				fmt.Fprintf(w, "%d %s %s\r\n", i, name, secret)
			}
		})
	}
	wg.Go(func() {
		for range 100 {
			_ = o.Snapshot()
		}
	})
	wg.Wait()
	snapshot := o.Snapshot()
	if snapshot.Dropped == 0 || len(snapshot.Lines) > MaxLogLines {
		t.Fatal("log tail not bounded")
	}
	bytes := 0
	var sequence uint64
	for _, line := range snapshot.Lines {
		bytes += len(line.Text)
		if strings.Contains(line.Text, secret) || strings.HasSuffix(line.Text, "\r") || line.Sequence <= sequence {
			t.Fatal("unsafe or unordered log line")
		}
		sequence = line.Sequence
	}
	if bytes > MaxLogBytes {
		t.Fatal("byte bound exceeded")
	}
	w := o.Stream("stdout")
	fmt.Fprint(w, strings.Repeat("z", maxLineBytes*4)+"\n")
	w.Close()
	last := o.Snapshot().Lines
	line := last[len(last)-1]
	if !line.Truncated || len(line.Text) > maxLineBytes {
		t.Fatal("long line did not truncate")
	}
}

func TestArgumentFileEscapesAndBounds(t *testing.T) {
	data, err := encodeArgumentFile([]string{"", `C:\Java Home\`, "\"quote\" # literal", "中文 空格", "a\nb\tc\r\f"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"\"\"\n", `C:\\Java Home\\`, `\"quote\" # literal`, `a\nb\tc\r\f`} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("encoding %q lacks %q", data, want)
		}
	}
	if _, err := encodeArgumentFile([]string{"bad\x00arg"}); err == nil {
		t.Fatal("NUL accepted")
	}
	if _, err := encodeArgumentFile([]string{strings.Repeat("x", 4<<20)}); err == nil {
		t.Fatal("unbounded argument file")
	}
}
