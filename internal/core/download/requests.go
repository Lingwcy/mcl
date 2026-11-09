package download

import (
	"errors"
	"net/http"
	"net/url"
)

func (c *Client) requestClient(original string, allowedHosts []string, mirrored bool) *http.Client {
	copy := *c.HTTP
	copy.Jar = nil
	origin, _ := url.Parse(original)
	copy.CheckRedirect = func(request *http.Request, via []*http.Request) error {
		if len(via) >= 5 || request.URL.User != nil {
			return errors.New("下载重定向过多或包含凭据")
		}
		if mirrored {
			if !safeMirrorRedirect(request.URL) {
				return errors.New("镜像只能重定向到公开 HTTPS 下载节点")
			}
		} else {
			if origin.Scheme == "https" && request.URL.Scheme != "https" {
				return errors.New("拒绝将 HTTPS 下载降级为明文")
			}
			if len(allowedHosts) > 0 && !allowedFileURL(request.URL.String(), allowedHosts) {
				return errors.New("download redirect host is not permitted")
			}
		}
		if c.HTTP.CheckRedirect != nil {
			if err := c.HTTP.CheckRedirect(request, via); err != nil {
				return err
			}
		}
		stripTransferSecrets(request)
		return nil
	}
	return &copy
}
