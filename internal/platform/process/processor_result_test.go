package process

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"

	"mcl/internal/core/diagnostics"
	"mcl/internal/core/launch"
)

func TestProcessorResultsKeepCauseAndBoundedRealOutput(t *testing.T) {
	output := (launch.Plan{}).NewOutput()
	stream := output.Stream("stderr")
	var bytes atomic.Uint64
	writer := processorEvidenceWriter{ctx: t.Context(), writer: stream, bytes: &bytes}
	text := "first\nsecond\nthird\n" + strings.Repeat("x", 1000) + "\n"
	if _, err := writer.Write([]byte(text)); err != nil {
		t.Fatal(err)
	}
	stream.Close()
	if bytes.Load() != uint64(len(text)) {
		t.Fatal(bytes.Load())
	}
	failure := processorRunFailure("timeout", context.DeadlineExceeded, bytes.Load(), output)
	if !errors.Is(failure, context.DeadlineExceeded) || diagnostics.Code(failure) != "deadline" || !strings.Contains(failure.Error(), "4分钟上限") {
		t.Fatal(failure)
	}
	if strings.Contains(failure.Error(), strings.Repeat("x", 400)) {
		t.Fatal("unbounded output in error")
	}
	drain := processorRunFailure("drain", exec.ErrWaitDelay, 0, (launch.Plan{}).NewOutput())
	if !errors.Is(drain, exec.ErrWaitDelay) || !strings.Contains(drain.Error(), "未在期限内排空") || !strings.Contains(drain.Error(), "未收到标准输出或错误输出") {
		t.Fatal(drain)
	}
}
