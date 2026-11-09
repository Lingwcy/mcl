package download

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

func (c *Client) transfer(ctx context.Context, root *os.Root, f File, address string, alternate bool) (bool, time.Duration, error) {
	p, err := openPartial(ctx, root, f, address, c.Coordination)
	if err != nil {
		return false, 0, err
	}
	defer p.close()
	if !p.reset && p.offset == f.Size {
		return p.publish(ctx, root, f)
	}
	if c.Transfers != nil {
		release, e := c.Transfers.Acquire(ctx)
		if e != nil {
			return false, 0, e
		}
		defer release()
	}
	for fallback := 0; fallback < 2; fallback++ {
		requested := p.offset
		if p.reset {
			requested = 0
		}
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
		if err != nil {
			return false, 0, errors.New("下载地址无效")
		}
		request.Header.Set("User-Agent", "mcl/0.1.0")
		request.Header.Set("Accept-Encoding", "identity")
		if requested > 0 {
			request.Header.Set("Range", fmt.Sprintf("bytes=%d-", requested))
			if p.state.ETag != "" {
				request.Header.Set("If-Range", p.state.ETag)
			} else if p.state.Modified != "" {
				request.Header.Set("If-Range", p.state.Modified)
			}
		}
		client := c.requestClient(f.URL, f.AllowedHosts, address != f.URL)
		var response *http.Response
		if c.Transfers != nil {
			response, err = c.Transfers.Do(client, request)
		} else {
			response, err = client.Do(request)
		}
		if err != nil {
			if ctx.Err() != nil {
				return false, 0, ctx.Err()
			}
			return true, 0, errors.New("下载请求失败，请检查网络及代理")
		}
		status := response.StatusCode
		if status == 416 && requested > 0 {
			response.Body.Close()
			if response.Header.Get("Content-Range") == fmt.Sprintf("bytes */%d", f.Size) && requested == f.Size {
				return p.publish(ctx, root, f)
			}
			p.reset = true
			continue
		}
		length, start := f.Size, int64(0)
		if status == 206 {
			var end, total int64
			start, end, total, err = parseContentRange(response.Header.Get("Content-Range"))
			changed := p.state.ETag != "" && response.Header.Get("ETag") != "" && response.Header.Get("ETag") != p.state.ETag
			if p.state.ETag == "" && p.state.Modified != "" && response.Header.Get("Last-Modified") != "" && response.Header.Get("Last-Modified") != p.state.Modified {
				changed = true
			}
			if err != nil || start != requested || total != f.Size || requested > 0 && changed {
				response.Body.Close()
				if requested == 0 {
					return true, 0, errors.New("服务器返回了不匹配的续传范围")
				}
				p.reset = true
				continue
			}
			length = end - start + 1
		} else if status != 200 {
			response.Body.Close()
			if requested > 0 && (status == 400 || status == 405 || status == 501) {
				p.reset = true
				continue
			}
			retry := status == 408 || status == 429 || status >= 500 && status <= 599 || alternate && (status == 403 || status == 404)
			delay, allowed := retryDelay(response.Header.Get("Retry-After"), time.Now())
			return retry && allowed, delay, fmt.Errorf("服务器返回 HTTP %d", status)
		}
		if response.Header.Get("Content-Encoding") != "" && response.Header.Get("Content-Encoding") != "identity" || response.ContentLength >= 0 && response.ContentLength != length {
			response.Body.Close()
			return true, 0, errors.New("下载响应大小或编码与固定元数据不符")
		}
		if status == 200 || start == 0 {
			start = 0
			if err := p.file.Truncate(0); err != nil {
				response.Body.Close()
				return false, 0, err
			}
		}
		if _, err := p.file.Seek(start, 0); err != nil {
			response.Body.Close()
			return false, 0, err
		}
		p.state.Source = shaText(address)
		p.state.ETag = strongETag(response.Header.Get("ETag"))
		p.state.Modified = ""
		if date := response.Header.Get("Last-Modified"); len(date) <= 64 {
			if _, e := http.ParseTime(date); e == nil {
				p.state.Modified = date
			}
		}
		p.state.Offset = 0
		p.state.PrefixSHA256 = ""
		if err := savePartial(ctx, p.root, p.marker, p.state); err != nil {
			response.Body.Close()
			return false, 0, err
		}
		writer := &transferWriter{ctx: ctx, file: p.file, expected: f, offset: start}
		n, copyErr := io.Copy(writer, io.LimitReader(response.Body, length+1))
		response.Body.Close()
		if n > length {
			if err := p.file.Truncate(start); err != nil {
				return false, 0, err
			}
			copyErr = errors.New("下载响应超出固定范围")
		}
		if info, e := p.file.Stat(); e == nil {
			if eventErr := Observe(ctx, eventFor(f, "downloading", min(info.Size(), f.Size))); eventErr != nil && copyErr == nil {
				copyErr = eventErr
			}
		}
		if err := p.checkpoint(ctx); err != nil {
			Observe(ctx, Event{State: "checkpoint-failed"})
			return false, 0, err
		}
		if copyErr != nil || n != length {
			if ctx.Err() != nil {
				return false, 0, ctx.Err()
			}
			return true, 0, errors.New("下载中断，已保留可校验的分片")
		}
		p.offset = start + n
		if p.offset < f.Size {
			return true, 0, errors.New("服务器返回部分区段，继续下载剩余内容")
		}
		return p.publish(ctx, root, f)
	}
	return true, 0, errors.New("服务器拒绝续传，已保留原分片，可重试完整下载")
}
func parseContentRange(value string) (int64, int64, int64, error) {
	bad := errors.New("Content-Range无效")
	if !strings.HasPrefix(value, "bytes ") {
		return 0, 0, 0, bad
	}
	parts := strings.Split(strings.TrimPrefix(value, "bytes "), "/")
	if len(parts) != 2 {
		return 0, 0, 0, bad
	}
	rangeParts := strings.Split(parts[0], "-")
	if len(rangeParts) != 2 {
		return 0, 0, 0, bad
	}
	for _, number := range []string{rangeParts[0], rangeParts[1], parts[1]} {
		if number == "" || strings.ContainsFunc(number, func(r rune) bool { return r < '0' || r > '9' }) {
			return 0, 0, 0, bad
		}
	}
	a, e := strconv.ParseInt(rangeParts[0], 10, 64)
	b, e2 := strconv.ParseInt(rangeParts[1], 10, 64)
	total, e3 := strconv.ParseInt(parts[1], 10, 64)
	if e != nil || e2 != nil || e3 != nil || a < 0 || b < a || total <= b {
		return 0, 0, 0, bad
	}
	return a, b, total, nil
}
func strongETag(value string) string {
	if len(value) < 2 || len(value) > 256 || strings.HasPrefix(value, "W/") || value[0] != '"' || value[len(value)-1] != '"' || strings.ContainsAny(value, "\r\n\x00") {
		return ""
	}
	return value
}

type transferWriter struct {
	ctx      context.Context
	file     *os.File
	expected File
	offset   int64
	last     time.Time
}

func (w *transferWriter) Write(buffer []byte) (int, error) {
	if err := w.ctx.Err(); err != nil {
		return 0, err
	}
	n, err := w.file.Write(buffer)
	w.offset += int64(n)
	if time.Since(w.last) >= 150*time.Millisecond {
		w.last = time.Now()
		if e := Observe(w.ctx, eventFor(w.expected, "downloading", min(w.offset, w.expected.Size))); e != nil {
			return n, e
		}
	}
	return n, err
}
