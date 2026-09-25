package commandexecution

import (
	"context"
	"testing"
)

func TestHostInteractivePresentationDoesNotAuthorizeCommands(t *testing.T) {
	host, _, _, _ := hostStateFixture(t)
	ctx := context.Background()
	if ready, err := host.InteractiveSessionReady(ctx); err != nil || ready {
		t.Fatalf("unknown OS session admitted presentation: %v %v", ready, err)
	}
	if err := host.SetOSSessionState(ctx, true, false); err != nil {
		t.Fatal(err)
	}
	if ready, err := host.InteractiveSessionReady(ctx); err != nil || !ready {
		t.Fatalf("interactive OS session blocked presentation: %v %v", ready, err)
	}
	if ready, err := host.SourceSecurityReady(ctx); err != nil || ready {
		t.Fatalf("presentation readiness authorized source with vault locked: %v %v", ready, err)
	}
	if err := host.SetOSSessionState(ctx, true, true); err != nil {
		t.Fatal(err)
	}
	if ready, err := host.InteractiveSessionReady(ctx); err != nil || ready {
		t.Fatalf("locked OS session admitted presentation: %v %v", ready, err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if ready, err := host.InteractiveSessionReady(cancelled); ready || err == nil {
		t.Fatalf("cancelled presentation: %v %v", ready, err)
	}
}
