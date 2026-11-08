package platform

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"mcl/internal/core/download"
)

// DownloadNetwork swaps immutable transports. Active requests retain their old
// transport; only idle connections are closed when a proxy changes. It is not
// installed on authentication, thumbnail or the game's own network clients.
type DownloadNetwork struct {
	mu        sync.RWMutex
	proxy     download.Proxy
	transport *http.Transport
}

func NewDownloadNetwork(proxy download.Proxy) (*DownloadNetwork, error) {
	network := &DownloadNetwork{}
	if err := network.SetProxy(proxy); err != nil {
		return nil, err
	}
	return network, nil
}

func (n *DownloadNetwork) SetProxy(proxy download.Proxy) error {
	address, err := proxy.URL()
	if err != nil {
		return err
	}
	n.mu.Lock()
	if n.transport != nil && n.proxy == proxy {
		n.mu.Unlock()
		return nil
	}
	transport := &http.Transport{
		DialContext:       (&net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		ForceAttemptHTTP2: true, MaxIdleConns: 128, MaxIdleConnsPerHost: 8,
		IdleConnTimeout: 90 * time.Second, TLSHandshakeTimeout: 10 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second, ExpectContinueTimeout: time.Second,
	}
	switch proxy.Mode {
	case download.ProxyEnvironment:
		transport.Proxy = http.ProxyFromEnvironment
	case download.ProxyHTTP, download.ProxySOCKS5:
		transport.Proxy = http.ProxyURL(address)
	case download.ProxyDirect:
		// A nil proxy deliberately ignores HTTP(S)_PROXY and ALL_PROXY.
	}
	previous := n.transport
	n.proxy, n.transport = proxy, transport
	n.mu.Unlock()
	if previous != nil {
		previous.CloseIdleConnections()
	}
	return nil
}

func (n *DownloadNetwork) RoundTrip(request *http.Request) (*http.Response, error) {
	n.mu.RLock()
	transport := n.transport
	n.mu.RUnlock()
	if transport == nil {
		return nil, errors.New("下载网络尚未初始化")
	}
	response, err := transport.RoundTrip(request)
	if err == nil {
		response.Body = &downloadNetworkBody{body: response.Body, ctx: request.Context()}
		return response, nil
	}
	if response != nil && response.Body != nil {
		response.Body.Close()
	}
	if request.Context().Err() != nil {
		return nil, request.Context().Err()
	}
	var timeout net.Error
	if errors.As(err, &timeout) && timeout.Timeout() {
		return nil, context.DeadlineExceeded
	}
	// Environment proxy URLs can contain credentials. Never format the original
	// transport error, proxy URL, request URL, or a CONNECT response body.
	return nil, errors.New("下载网络连接失败，请检查网络及代理设置")
}

type downloadNetworkBody struct {
	body io.ReadCloser
	ctx  context.Context
}

func (b *downloadNetworkBody) Read(buffer []byte) (int, error) {
	n, err := b.body.Read(buffer)
	if err == nil || err == io.EOF {
		return n, err
	}
	if b.ctx.Err() != nil {
		return n, b.ctx.Err()
	}
	return n, errors.New("下载响应连接中断")
}

func (b *downloadNetworkBody) Close() error {
	if err := b.body.Close(); err != nil {
		return errors.New("关闭下载连接失败")
	}
	return nil
}

// Snapshot pins the successful saved proxy for an explicit diagnostic batch.
func (n *DownloadNetwork) Snapshot() http.RoundTripper {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return &DownloadNetwork{transport: n.transport, proxy: n.proxy}
}

func (n *DownloadNetwork) CloseIdleConnections() {
	n.mu.RLock()
	transport := n.transport
	n.mu.RUnlock()
	if transport != nil {
		transport.CloseIdleConnections()
	}
}
