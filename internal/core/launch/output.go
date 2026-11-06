package launch

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"sync"
	"time"
)

const MaxLogLines = 1024
const MaxLogBytes = 256 << 10
const maxLineBytes = 8 << 10

type Line struct {
	Sequence  uint64
	Time      time.Time
	Stream    string
	Text      string
	Truncated bool
}

type OutputSnapshot struct {
	Lines   []Line
	Dropped uint64
}

// Output is an in-memory bounded, already-redacted log tail. Writers returned
// by Stream belong to one reader each; Snapshot can run concurrently with both.
type Output struct {
	mu                  sync.Mutex
	lines               [MaxLogLines]Line
	start, count, bytes int
	next, dropped       uint64
	secrets             []string
	changed             chan struct{}
}

func (p Plan) NewOutput() *Output {
	return &Output{secrets: append([]string(nil), p.secrets...), changed: make(chan struct{}, 1)}
}

func (o *Output) Changed() <-chan struct{} { return o.changed }

func (o *Output) String() string               { data, _ := o.MarshalJSON(); return string(data) }
func (o *Output) GoString() string             { return o.String() }
func (o *Output) MarshalJSON() ([]byte, error) { return json.Marshal(o.Snapshot()) }

func (o *Output) Snapshot() OutputSnapshot {
	o.mu.Lock()
	defer o.mu.Unlock()
	out := OutputSnapshot{Lines: make([]Line, o.count), Dropped: o.dropped}
	for i := range o.count {
		out.Lines[i] = o.lines[(o.start+i)%MaxLogLines]
	}
	return out
}

func (o *Output) append(stream, text string, truncated bool) {
	text = strings.ToValidUTF8(strings.TrimSuffix(text, "\r"), "�")
	o.mu.Lock()
	defer o.mu.Unlock()
	for o.count > 0 && (o.count == MaxLogLines || o.bytes+len(text) > MaxLogBytes) {
		o.bytes -= len(o.lines[o.start].Text)
		o.lines[o.start] = Line{}
		o.start = (o.start + 1) % MaxLogLines
		o.count--
		o.dropped++
	}
	o.next++
	o.lines[(o.start+o.count)%MaxLogLines] = Line{Sequence: o.next, Time: time.Now().UTC(), Stream: stream, Text: text, Truncated: truncated}
	o.count++
	o.bytes += len(text)
	select {
	case o.changed <- struct{}{}:
	default:
	}
}

// Stream redacts before line truncation, including a secret split across input
// chunks. Close flushes any remaining safe output. It never persists raw text.
func (o *Output) Stream(name string) io.WriteCloser {
	if name != "stderr" {
		name = "stdout"
	}
	w := &outputWriter{output: o, name: name}
	for _, secret := range o.secrets {
		w.longest = max(w.longest, len(secret))
	}
	return w
}

type outputWriter struct {
	output            *Output
	name              string
	longest           int
	pending           []byte
	line              []byte
	truncated, closed bool
}

func (w *outputWriter) Write(p []byte) (int, error) {
	if w.closed {
		return 0, io.ErrClosedPipe
	}
	n := len(p)
	// Bound temporary redaction memory even if a caller writes a huge slice.
	for len(p) > 0 {
		part := min(len(p), 32<<10)
		w.pending = append(w.pending, p[:part]...)
		p = p[part:]
		cut := max(0, len(w.pending)-max(0, w.longest-1))
		// Session credentials cannot contain newlines, so complete lines can
		// be emitted immediately without waiting for another token-sized chunk.
		cut = max(cut, bytes.LastIndexByte(w.pending, '\n')+1)
		// Retain any full secret that straddles the safe-prefix boundary.
		for changed := true; changed; {
			changed = false
			for _, secret := range w.output.secrets {
				if secret == "" {
					continue
				}
				start := max(0, cut-len(secret)+1)
				index := bytes.Index(w.pending[start:], []byte(secret))
				if index >= 0 {
					index += start
					if index < cut && index+len(secret) > cut {
						cut = index
						changed = true
					}
				}
			}
		}
		w.emit(w.redact(string(w.pending[:cut])))
		w.pending = append(w.pending[:0], w.pending[cut:]...)
	}
	return n, nil
}

func (w *outputWriter) redact(text string) string {
	for _, secret := range w.output.secrets {
		if secret != "" {
			text = strings.ReplaceAll(text, secret, "[REDACTED]")
		}
	}
	return text
}

func (w *outputWriter) emit(text string) {
	for len(text) > 0 {
		line, rest, newline := strings.Cut(text, "\n")
		remaining := max(0, maxLineBytes-len(w.line))
		if len(line) > remaining {
			w.truncated = true
			line = line[:remaining]
		}
		w.line = append(w.line, line...)
		if newline {
			w.output.append(w.name, string(w.line), w.truncated)
			w.line = w.line[:0]
			w.truncated = false
		} else {
			return
		}
		text = rest
	}
}

func (w *outputWriter) Close() error {
	if w.closed {
		return nil
	}
	text := string(w.pending)
	// A pipe can close halfway through a credential (detach, crash or abrupt
	// exit). Conservatively hide an incomplete trailing prefix as well.
	for _, secret := range w.output.secrets {
		for n := min(len(text), len(secret)-1); n > 0; n-- {
			if strings.HasSuffix(text, secret[:n]) {
				text = text[:len(text)-n] + "[REDACTED]"
				break
			}
		}
	}
	w.emit(w.redact(text))
	w.pending = nil
	if len(w.line) > 0 || w.truncated {
		w.output.append(w.name, string(w.line), w.truncated)
	}
	w.line = nil
	w.closed = true
	return nil
}
