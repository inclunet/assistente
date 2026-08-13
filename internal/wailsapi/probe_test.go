package wailsapi_test

import (
	"assistente/internal/wailsapi"
	"testing"
)

func TestStranglerFigProbe(t *testing.T) {
	p := wailsapi.NewProbe()
	if got := p.StranglerFigProbe(); got != "aep-0088-probe-ok" {
		t.Fatalf("StranglerFigProbe() = %q, want aep-0088-probe-ok", got)
	}
}
