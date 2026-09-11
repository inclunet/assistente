package llm

import (
	"context"
	"testing"
	"time"
)

func TestStreamWatchdog_Stop_NaoBloqueia(t *testing.T) {
	ctx := context.Background()
	_, wd := startStreamWatchdog(ctx, 10*time.Millisecond, func() { time.Sleep(5 * time.Second) })
	start := time.Now()
	wd.Stop()
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("Stop bloqueou %v, esperado <3s", elapsed)
	}
}
