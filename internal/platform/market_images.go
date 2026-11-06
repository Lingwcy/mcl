package platform

import (
	"context"
	"errors"
	"image"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"mcl/internal/core/thumbnail"
)

// MarketImage fetches a bounded public thumbnail. It accepts only the two
// providers' image CDNs, and never forwards credentials or arbitrary URLs.
func MarketImage(ctx context.Context, address string) (image.Image, error) {
	allowed := func(value string) bool {
		u, err := url.Parse(value)
		if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" {
			return false
		}
		switch strings.ToLower(u.Hostname()) {
		case "cdn.modrinth.com", "media.forgecdn.net", "mediafilez.forgecdn.net":
			return true
		}
		return false
	}
	if !allowed(address) {
		return nil, errors.New("unsupported icon host")
	}
	client := &http.Client{Timeout: 12 * time.Second, CheckRedirect: func(r *http.Request, via []*http.Request) error {
		if len(via) > 3 || !allowed(r.URL.String()) {
			return errors.New("unsupported icon redirect")
		}
		return nil
	}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "mcl/0.1.0")
	response, err := client.Do(req)
	if err != nil {
		return nil, errors.New("icon request failed")
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return nil, errors.New("icon unavailable")
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, (2<<20)+1))
	if err != nil || len(data) > 2<<20 {
		return nil, errors.New("icon size limit")
	}
	decoded, err := thumbnail.Decode(data)
	if err != nil {
		return nil, err
	}
	return decoded, ctx.Err()
}
