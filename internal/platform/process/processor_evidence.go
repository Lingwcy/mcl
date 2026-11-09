package process

import (
	"context"
	"io"
	"mcl/internal/core/diagnostics"
)

type processorEvidenceWriter struct {
	ctx    context.Context
	writer io.Writer
}

func (w processorEvidenceWriter) Write(p []byte) (int, error) {
	n, err := w.writer.Write(p)
	if n > 0 {
		diagnostics.Work(w.ctx)
	}
	return n, err
}
