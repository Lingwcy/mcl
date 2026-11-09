package download

import (
	"errors"
	"net"
	"net/url"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/net/idna"
)

type SourceMode string

const (
	SourceOfficial      SourceMode = ""
	SourceAuto          SourceMode = "auto"
	SourceOfficialFirst SourceMode = "official-first"
	SourceMirrorFirst   SourceMode = "mirror-first"
)

type ProxyMode string

const (
	ProxyEnvironment ProxyMode = ""
	ProxyDirect      ProxyMode = "direct"
	ProxyHTTP        ProxyMode = "http"
	ProxySOCKS5      ProxyMode = "socks5"
)

// Proxy contains no credentials. Environment proxy credentials remain in the
// process environment, never in settings or diagnostic output.
type Proxy struct {
	Mode ProxyMode `json:"mode,omitempty"`
	Host string    `json:"host,omitempty"`
	Port int       `json:"port,omitempty"`
}

func (p Proxy) Validate() error {
	switch p.Mode {
	case ProxyEnvironment, ProxyDirect:
		if p.Host != "" || p.Port != 0 {
			return errors.New("环境代理或直连模式不能包含自定义代理地址")
		}
		return nil
	case ProxyHTTP, ProxySOCKS5:
	default:
		return errors.New("不支持的下载代理类型")
	}
	if p.Port < 1 || p.Port > 65535 {
		return errors.New("代理端口应为 1–65535")
	}
	host := strings.TrimSpace(p.Host)
	if host == "" || len(p.Host) > 512 || len(host) > 253 || !utf8.ValidString(p.Host) || strings.IndexFunc(p.Host, unicode.IsControl) >= 0 || strings.ContainsAny(host, "/\\@?#[] %") {
		return errors.New("代理地址只填写主机名或 IP，不包含协议、凭据或端口")
	}
	if net.ParseIP(host) != nil {
		return nil
	}
	ascii, err := idna.Lookup.ToASCII(host)
	if err != nil || len(ascii) > 253 {
		return errors.New("代理主机名无效")
	}
	for _, part := range strings.Split(strings.TrimSuffix(ascii, "."), ".") {
		if part == "" || len(part) > 63 || part[0] == '-' || part[len(part)-1] == '-' {
			return errors.New("代理主机名无效")
		}
		for _, r := range part {
			if r != '-' && !(r >= 'a' && r <= 'z') && !(r >= 'A' && r <= 'Z') && !(r >= '0' && r <= '9') {
				return errors.New("代理主机名无效")
			}
		}
	}
	return nil
}

func (p Proxy) URL() (*url.URL, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}
	if p.Mode != ProxyHTTP && p.Mode != ProxySOCKS5 {
		return nil, nil
	}
	host := strings.TrimSpace(p.Host)
	if net.ParseIP(host) == nil {
		host, _ = idna.Lookup.ToASCII(host)
	}
	scheme := "http"
	if p.Mode == ProxySOCKS5 {
		scheme = "socks5h"
	}
	return &url.URL{Scheme: scheme, Host: net.JoinHostPort(host, strconv.Itoa(p.Port))}, nil
}

// Zero concurrency is automatic; zero speed is unlimited. Units are KiB/s.
type Preferences struct {
	CacheDirectory string     `json:"cache_directory,omitempty"`
	Concurrency    int        `json:"concurrency,omitempty"`
	SpeedLimitKiB  int        `json:"speed_limit_kib,omitempty"`
	Source         SourceMode `json:"source,omitempty"`
	Proxy          Proxy      `json:"proxy,omitempty"`
}

func (p Preferences) Validate() error {
	if p.CacheDirectory != "" && (!filepath.IsAbs(p.CacheDirectory) || len(p.CacheDirectory) > 4096 || !utf8.ValidString(p.CacheDirectory) || strings.IndexFunc(p.CacheDirectory, unicode.IsControl) >= 0) {
		return errors.New("下载缓存目录应为有效的绝对路径")
	}
	if p.Concurrency < 0 || p.Concurrency > 128 {
		return errors.New("下载并发数应为 1–128，0 表示自动选择")
	}
	if p.SpeedLimitKiB < 0 || p.SpeedLimitKiB > 1048576 {
		return errors.New("下载限速应为 1–1048576 KiB/s，0 表示不限速")
	}
	switch p.Source {
	case SourceOfficial, SourceAuto, SourceOfficialFirst, SourceMirrorFirst:
	default:
		return errors.New("不支持的下载源策略")
	}
	return p.Proxy.Validate()
}

func (p Preferences) Limit(parallelism int) int {
	if p.Concurrency != 0 {
		return p.Concurrency
	}
	return max(1, min(parallelism, 32))
}

func automaticLimit(p Preferences) int { return p.Limit(runtime.GOMAXPROCS(0)) }
