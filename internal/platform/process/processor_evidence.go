package process

import (
	"context"
	"io"
	"mcl/internal/core/diagnostics"
	"sync/atomic"
)

type processorEvidenceWriter struct {
	ctx    context.Context
	writer io.Writer
	bytes  *atomic.Uint64
}

func (w processorEvidenceWriter) Write(p []byte) (int, error) {
	n, err := w.writer.Write(p)
	if n > 0 {
		if w.bytes != nil {
			w.bytes.Add(uint64(n))
		}
		diagnostics.Work(w.ctx)
	}
	return n, err
}
