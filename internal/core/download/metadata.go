package download

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"mcl/internal/core/diagnostics"
	"mcl/internal/core/meta"
)

// FetchMetadata only accepts a digest obtained independently (the official
// manifest). The mutable, unpinned manifest itself never enters this path.
// Metadata is bounded and uses the proxy/source policy, not file rate slots.
func (c *Client) FetchMetadata(ctx context.Context, address, digest string, limit int64) ([]byte, error) {
	if c.HTTP == nil || c.Attempts < 1 || c.Attempts > 5 || !meta.ValidSHA1(digest) || limit < 1 || limit > 16<<20 {
		return nil, errors.New("有摘要的元数据请求配置无效")
	}
	if err := meta.ValidateURL(address); err != nil {
		return nil, err
	}
	sources := c.Transfers.candidates(address)
	var last error
	for attempt := range c.Attempts {
		candidate := sources[attempt%len(sources)]
		requestCtx, evidence := diagnostics.Start(ctx, "metadata", digest)
		evidence.Network(diagnostics.Source(candidate), 0, attempt+1)
		data, retry, delay, err := c.metadataAttempt(requestCtx, address, candidate, digest, limit, len(sources) > 1)
		if err != nil {
			evidence.Fail(err)
		} else {
			evidence.Work()
		}
		evidence.End()
		if err == nil {
			c.Transfers.rememberSource(address, candidate)
			return data, nil
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		last = err
		if !retry || attempt+1 == c.Attempts {
			break
		}
		if delay == 0 {
			delay = 200 * time.Millisecond << attempt
		}
		_, waiting := diagnostics.Start(ctx, "backoff", digest)
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			waiting.End()
			return nil, ctx.Err()
		case <-timer.C:
			waiting.End()
		}
	}
	return nil, last
}

func (c *Client) metadataAttempt(ctx context.Context, original, candidate, digest string, limit int64, alternate bool) ([]byte, bool, time.Duration, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, candidate, nil)
	if err != nil {
		return nil, false, 0, errors.New("元数据请求地址无效")
	}
	request.Header.Set("User-Agent", "mcl/0.1.0")
	request.Header.Set("Accept", "application/json")
	client := c.requestClient(original, nil, candidate != original)
	if client.Timeout <= 0 || client.Timeout > 25*time.Second {
		client.Timeout = 25 * time.Second
	}
	response, err := client.Do(request)
	if err != nil {
		code := diagnostics.Code(err)
		if code == "unknown" {
			code = "network"
		}
		return nil, true, 0, diagnostics.Wrap(code, errors.New("无法读取版本元数据，请检查下载代理和网络设置"))
	}
	defer response.Body.Close()
	diagnostics.Response(ctx, candidate, response.StatusCode)
	if response.StatusCode != http.StatusOK {
		status := response.StatusCode
		retry := status == 408 || status == 429 || status == 500 || status == 502 || status == 503 || status == 504 || alternate && (status == 403 || status == 404)
		delay, allowed := retryDelay(response.Header.Get("Retry-After"), time.Now())
		return nil, retry && allowed, delay, diagnostics.Wrap(diagnostics.HTTPCode(status), fmt.Errorf("版本元数据返回 HTTP %d", status))
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		code := diagnostics.Code(err)
		if code == "unknown" {
			code = "network"
		}
		return nil, true, 0, diagnostics.Wrap(code, errors.New("读取版本元数据失败"))
	}
	if int64(len(data)) > limit {
		return nil, true, 0, errors.New("版本元数据超过大小限制")
	}
	sum := sha1.Sum(data)
	if !strings.EqualFold(hex.EncodeToString(sum[:]), digest) {
		return nil, true, 0, diagnostics.Wrap("integrity", errors.New("版本元数据 SHA-1 与官方清单不符"))
	}
	return data, false, 0, ctx.Err()
}
