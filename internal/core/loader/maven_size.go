package loader

import (
	"context"
	"net/http"
	"strconv"
	"strings"
)

const maxMavenArtifactSize = int64(512 << 20)

// A discovered size bounds transfer only. It cannot replace the independent
// SHA-1. The shared downloader verifies the full representation before publish.
func (c *Client) mavenSize(ctx context.Context, address string) (int64, error) {
	ctx = WithResolutionBudget(ctx)
	if !validMavenAddress(address) {
		return 0, identityError("Maven长度来源不受支持")
	}
	response, err := c.request(ctx, http.MethodHead, address)
	if err != nil {
		return 0, err
	}
	status := response.StatusCode
	var size int64
	missing := false
	if status == http.StatusOK {
		size, missing, err = mavenLength(response.Header)
		if err == nil && response.Header.Get("Content-Range") != "" {
			err = identityError("Maven HEAD返回意外范围")
		}
		if err == nil && encodedMavenResponse(response) {
			err = identityError("Maven长度响应包含内容编码")
		}
	} else if status != http.StatusMethodNotAllowed && status != http.StatusNotImplemented {
		err = &metadataStatusError{status: status}
	}
	response.Body.Close()
	if err != nil {
		return 0, err
	}
	if status == http.StatusOK && !missing {
		return size, ctx.Err()
	}
	return c.mavenRangeSize(ctx, address)
}
func mavenLength(header http.Header) (int64, bool, error) {
	values := header.Values("Content-Length")
	if len(values) == 0 {
		return 0, true, nil
	}
	if len(values) != 1 {
		return 0, false, identityError("Maven长度响应存在多个声明")
	}
	text := values[0]
	if text == "" {
		return 0, true, nil
	}
	if strings.IndexFunc(text, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
		return 0, false, identityError("Maven文件大小格式无效")
	}
	size, err := strconv.ParseInt(text, 10, 64)
	if err != nil || size <= 0 || size > maxMavenArtifactSize {
		return 0, false, identityError("Maven文件大小超出支持边界")
	}
	return size, false, nil
}
func encodedMavenResponse(response *http.Response) bool {
	value := response.Header.Get("Content-Encoding")
	return response.Uncompressed || value != "" && !strings.EqualFold(value, "identity")
}
func (c *Client) mavenRangeSize(ctx context.Context, address string) (int64, error) {
	headers := http.Header{"Range": []string{"bytes=0-0"}}
	response, err := c.requestHeaders(ctx, http.MethodGet, address, headers)
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()
	if encodedMavenResponse(response) {
		return 0, identityError("Maven长度探测响应包含内容编码")
	}
	switch response.StatusCode {
	case http.StatusOK:
		// If Range is ignored, do not read an unbounded body. Only explicit
		// Content-Length is usable, and actual bytes remain untrusted until hash.
		if response.Header.Get("Content-Range") != "" {
			return 0, identityError("Maven完整响应混入范围声明")
		}
		size, missing, err := mavenLength(response.Header)
		if err != nil {
			return 0, err
		}
		if missing {
			return 0, identityError("Maven未提供可验证的文件大小")
		}
		return size, ctx.Err()
	case http.StatusPartialContent:
		ranges := response.Header.Values("Content-Range")
		if len(ranges) != 1 || !strings.HasPrefix(ranges[0], "bytes 0-0/") {
			return 0, identityError("Maven长度探测未返回精确单字节范围")
		}
		text := strings.TrimPrefix(ranges[0], "bytes 0-0/")
		if text == "" || strings.IndexFunc(text, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
			return 0, identityError("Maven范围总大小无效")
		}
		size, err := strconv.ParseInt(text, 10, 64)
		if err != nil || size <= 0 || size > maxMavenArtifactSize {
			return 0, identityError("Maven范围总大小超出支持边界")
		}
		if values := response.Header.Values("Content-Length"); len(values) > 1 || len(values) == 1 && values[0] != "1" {
			return 0, identityError("Maven单字节范围长度不一致")
		}
		// Headers alone discover a bound. No body is consumed or promoted into a
		// verified file; actual Range/size/digest enforcement stays in downloads.
		return size, ctx.Err()
	default:
		return 0, &metadataStatusError{status: response.StatusCode}
	}
}
