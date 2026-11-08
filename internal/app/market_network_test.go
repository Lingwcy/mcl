package app

import (
	"net/http"
	"testing"
	"time"

	"mcl/internal/core/market"
)

func TestCurseForgeKeyReplacementKeepsInjectedNetworkClient(t *testing.T) {
	original := market.NewClient("synthetic-session-key")
	original.HTTP = &http.Client{Timeout: time.Second}
	original.ModrinthURL = "https://modrinth.example.invalid/v2"
	service := NewMarketService(original)
	if err := service.SetCurseForgeKey(""); err != nil {
		t.Fatal(err)
	}
	current := service.Client()
	if current == original || current.HTTP != original.HTTP || current.ModrinthURL != original.ModrinthURL || current.CurseForgeAvailable() || !original.CurseForgeAvailable() {
		t.Fatal("key update mutated an active client or discarded injected network policy")
	}
}
