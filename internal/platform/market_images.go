package platform

import (
	"bytes"
	"context"
	"errors"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"mcl/internal/core/thumbnail"

	_ "golang.org/x/image/webp"
)

const (
	maxDescriptionMediaBytes = 4 << 20
	maxDescriptionSVGBytes   = 512 << 10
	maxDescriptionImageDim   = 4096
)

// MarketImage fetches a bounded public thumbnail. It accepts only the two
// providers' image CDNs, and never forwards credentials or arbitrary URLs.
func MarketImage(ctx context.Context, address string) (image.Image, error) {
	data, _, err := fetchMarketBytes(ctx, address, thumbnail.MaxBytes, func(value string) bool {
		u, err := url.Parse(value)
		if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" {
			return false
		}
		switch strings.ToLower(u.Hostname()) {
		case "cdn.modrinth.com", "media.forgecdn.net", "mediafilez.forgecdn.net":
			return true
		}
		return false
	})
	if err != nil {
		return nil, err
	}
	decoded, err := thumbnail.Decode(data)
	if err != nil {
		return nil, err
	}
	return decoded, ctx.Err()
}

// MarketMedia fetches a bounded public HTTPS image or SVG embedded in a
// project description or changelog without forwarding credentials.
func MarketMedia(ctx context.Context, address string) ([]byte, bool, error) {
	data, contentType, err := fetchMarketBytes(ctx, address, maxDescriptionMediaBytes, func(value string) bool {
		u, err := url.Parse(value)
		return err == nil && u.Scheme == "https" && u.Hostname() != "" && u.User == nil && u.Port() == ""
	})
	if err != nil {
		return nil, false, err
	}
	if looksLikeSVG(contentType, data) {
		if len(data) > maxDescriptionSVGBytes {
			return nil, false, errors.New("image size limit")
		}
		return data, true, ctx.Err()
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || config.Width < 1 || config.Height < 1 || config.Width > maxDescriptionImageDim || config.Height > maxDescriptionImageDim {
		return nil, false, errors.New("unsupported image dimensions")
	}
	return data, false, ctx.Err()
}

func fetchMarketBytes(ctx context.Context, address string, maxBytes int64, allowed func(string) bool) ([]byte, string, error) {
	if !allowed(address) {
		return nil, "", errors.New("unsupported image host")
	}
	client := &http.Client{Timeout: 12 * time.Second, CheckRedirect: func(r *http.Request, via []*http.Request) error {
		if len(via) > 3 || !allowed(r.URL.String()) {
			return errors.New("unsupported image redirect")
		}
		return nil
	}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", "mcl/0.1.0")
	response, err := client.Do(req)
	if err != nil {
		return nil, "", errors.New("image request failed")
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return nil, "", errors.New("image unavailable")
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxBytes+1))
	if err != nil || int64(len(data)) > maxBytes || len(data) == 0 {
		return nil, "", errors.New("image size limit")
	}
	return data, response.Header.Get("Content-Type"), nil
}

func looksLikeSVG(contentType string, data []byte) bool {
	if strings.Contains(strings.ToLower(contentType), "image/svg") {
		return true
	}
	head := bytes.TrimSpace(data[:min(len(data), 512)])
	lower := bytes.ToLower(head)
	return (bytes.HasPrefix(lower, []byte("<svg")) || bytes.HasPrefix(lower, []byte("<?xml"))) && bytes.Contains(lower, []byte("<svg"))
}
