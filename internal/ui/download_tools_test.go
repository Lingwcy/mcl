package ui

import (
	"mcl/internal/core/download"
	"testing"
)

func TestDownloadToolsRejectResultsAfterPreferencesChange(t *testing.T) {
	shell := &Shell{}
	var complete func([]download.ConnectivityResult, error)
	cancelled := false
	shell.ConnectDownloads(DownloadActions{Check: func(_ download.SourceMode, done func([]download.ConnectivityResult, error)) func() {
		complete = done
		return func() { cancelled = true }
	}})
	shell.checkDownloadConnectivity()
	shell.downloadPreferencesCommitted()
	complete([]download.ConnectivityResult{{Name: "old proxy", Status: 200}}, nil)
	if !cancelled || len(shell.downloadTools.results) != 0 || shell.downloadTools.operation != "" {
		t.Fatal("obsolete diagnostic overwrote the new saved preferences")
	}
}
