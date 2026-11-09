package download

import (
	"context"
	"errors"
	"io"
	"mcl/internal/core/diagnostics"
	"net/http"
	"net/url"
	"time"
)

var ErrTransferStalled = diagnostics.Wrap("deadline", errors.New("下载连接长时间无响应"))

// Do preserves redirects/transport but replaces the wall-clock file timeout
// with bounded header, idle, and cumulative network-read waits. Rate waits sit
// OUTSIDE the idle reader, so a deliberately slow download is not timed out.
// The caller owns an Acquire slot; closing the returned body releases timers.
func (c *TransferControl) Do(client *http.Client, request *http.Request) (*http.Response, error) {
	task, cancel := context.WithCancelCause(request.Context())
	headerTimeout := client.Timeout
	if headerTimeout <= 0 || headerTimeout > 2*time.Minute {
		headerTimeout = 2 * time.Minute
	}
	timer := time.AfterFunc(headerTimeout, func() { cancel(ErrTransferStalled) })
	copy := *client
	copy.Timeout = 0
	response, err := copy.Do(request.WithContext(task))
	timer.Stop()
	if err != nil {
		if response != nil && response.Body != nil {
			response.Body.Close()
		}
		cause := context.Cause(task)
		cancel(nil)
		if cause != nil {
			return nil, cause
		}
		var urlError *url.Error
		if errors.As(err, &urlError) {
			return nil, urlError.Err
		}
		return nil, err
	}
	readBudget := client.Timeout
	if readBudget <= 0 || readBudget > 15*time.Minute {
		readBudget = 15 * time.Minute
	}
	body := &transferBody{body: response.Body, timer: timer, cancel: cancel}
	body.reader = &limitedReader{ctx: task, control: c, reader: &idleReader{
		reader: response.Body, timer: timer, task: task, cancel: cancel, remaining: readBudget,
	}}
	response.Body = body
	return response, nil
}

type transferBody struct {
	reader io.Reader
	body   io.ReadCloser
	timer  *time.Timer
	cancel context.CancelCauseFunc
}

func (b *transferBody) Read(buffer []byte) (int, error) { return b.reader.Read(buffer) }
func (b *transferBody) Close() error {
	b.timer.Stop()
	b.cancel(nil)
	return b.body.Close()
}

type idleReader struct {
	reader    io.Reader
	timer     *time.Timer
	task      context.Context
	cancel    context.CancelCauseFunc
	remaining time.Duration
}

func (r *idleReader) Read(buffer []byte) (int, error) {
	if r.remaining <= 0 {
		r.cancel(ErrTransferStalled)
		return 0, ErrTransferStalled
	}
	r.timer.Reset(min(45*time.Second, r.remaining))
	started := time.Now()
	n, err := r.reader.Read(buffer)
	r.timer.Stop()
	r.remaining -= time.Since(started)
	if r.remaining <= 0 {
		r.cancel(ErrTransferStalled)
	}
	if errors.Is(context.Cause(r.task), ErrTransferStalled) {
		return n, ErrTransferStalled
	}
	return n, err
}
