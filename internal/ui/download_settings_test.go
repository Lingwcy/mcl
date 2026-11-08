package ui

import (
	"testing"

	"mcl/internal/core/download"
)

func TestAutomaticConcurrencyKeepsRateSourceAndProxy(t *testing.T) {
	preferences := download.Preferences{CacheDirectory: t.TempDir(), SpeedLimitKiB: 2048, Source: download.SourceMirrorFirst, Proxy: download.Proxy{Mode: download.ProxyHTTP, Host: "127.0.0.1", Port: 7890}}
	draft := newDownloadSettingsState(preferences)
	got, err := draft.preferences()
	if err != nil || got != preferences {
		t.Fatalf("automatic concurrency discarded another setting: %+v, %v", got, err)
	}
	draft.speed = "0"
	if _, err := draft.preferences(); err == nil {
		t.Fatal("manual zero silently disabled rate limiting")
	}
	draft.speedLimited = false
	draft.proxyMode = download.ProxyDirect
	draft.proxyPort = "invalid hidden draft"
	got, err = draft.preferences()
	if err != nil || got.SpeedLimitKiB != 0 || got.Proxy != (download.Proxy{Mode: download.ProxyDirect}) || got.Source != preferences.Source {
		t.Fatalf("inactive controls blocked a valid setting: %+v, %v", got, err)
	}
}
