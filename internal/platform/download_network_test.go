package platform

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"
	"testing"

	"mcl/internal/core/download"
)

func TestDownloadProxyValidationAndTransportReplacement(t *testing.T) {
	for _, proxy := range []download.Proxy{
		{Mode: download.ProxyHTTP, Host: "user:secret@proxy", Port: 8080},
		{Mode: download.ProxyHTTP, Host: "https://proxy", Port: 8080},
		{Mode: download.ProxySOCKS5, Host: "127.0.0.1", Port: 0},
		{Mode: download.ProxyDirect, Host: "unwanted", Port: 8080},
		{Mode: download.ProxyHTTP, Host: "proxy\n", Port: 8080},
	} {
		if proxy.Validate() == nil {
			t.Errorf("invalid proxy accepted: %+v", proxy)
		}
	}
	network, err := NewDownloadNetwork(download.Proxy{Mode: download.ProxyHTTP, Host: "127.0.0.1", Port: 8080})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(network.CloseIdleConnections)
	previous := network.transport
	request, _ := http.NewRequest(http.MethodGet, "https://example.invalid", nil)
	if err := network.SetProxy(download.Proxy{Mode: download.ProxySOCKS5, Host: "::1", Port: 1080}); err != nil {
		t.Fatal(err)
	}
	oldAddress, err := previous.Proxy(request)
	if err != nil || oldAddress.String() != "http://127.0.0.1:8080" {
		t.Fatal("old request transport was mutated")
	}
	address, err := network.transport.Proxy(request)
	if err != nil || address.String() != "socks5h://[::1]:1080" {
		t.Fatalf("invalid remote-DNS SOCKS proxy: %v, %v", address, err)
	}
	if err := network.SetProxy(download.Proxy{Mode: download.ProxyDirect}); err != nil {
		t.Fatal(err)
	}
	if network.transport.Proxy != nil {
		t.Fatal("direct mode inherited environment proxy")
	}
	current := network.transport
	if err := network.SetProxy(download.Proxy{Mode: "unknown"}); err == nil || network.transport != current {
		t.Fatal("invalid proxy update replaced the working transport")
	}
}

func TestDownloadProxyErrorsDoNotExposeEnvironmentCredentials(t *testing.T) {
	network, err := NewDownloadNetwork(download.Proxy{Mode: download.ProxyDirect})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(network.CloseIdleConnections)
	// Configure the test transport before use; no DNS, proxy or socket is used.
	network.transport.DialContext = func(context.Context, string, string) (net.Conn, error) {
		return nil, errors.New("http://user:synthetic-proxy-secret@proxy.invalid")
	}
	request, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://example.invalid", nil)
	_, err = network.RoundTrip(request)
	if err == nil || strings.Contains(err.Error(), "synthetic-proxy-secret") || strings.Contains(err.Error(), "proxy.invalid") {
		t.Fatalf("unredacted proxy failure: %v", err)
	}
}

type brokenDownloadBody struct{}

func (brokenDownloadBody) Read([]byte) (int, error) {
	return 0, errors.New("signed-url?token=synthetic-response-secret")
}
func (brokenDownloadBody) Close() error { return errors.New("synthetic-response-secret") }

func TestDownloadResponseErrorsDoNotExposeRemoteText(t *testing.T) {
	body := &downloadNetworkBody{body: brokenDownloadBody{}, ctx: t.Context()}
	_, err := body.Read(make([]byte, 10))
	if err == nil || strings.Contains(err.Error(), "synthetic-response-secret") {
		t.Fatalf("unredacted response error: %v", err)
	}
	if err := body.Close(); err == nil || strings.Contains(err.Error(), "synthetic-response-secret") {
		t.Fatalf("unredacted close error: %v", err)
	}
}
