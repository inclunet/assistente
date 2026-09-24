package commandexecution

import (
	"context"
	"errors"
	"testing"

	"assistente/internal/commandcontract"
)

func TestExternalAuthorizationPreservesCancellationDuringCachedValidation(t *testing.T) {
	for _, phase := range []struct {
		name string
		call int
	}{{"resolution", 1}, {"authorization", 2}} {
		t.Run(phase.name, func(t *testing.T) {
			h := newExternalHarness(t, nil)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			bound, release, err := h.service.requestContext(ctx, "token-a", nil)
			if err != nil {
				t.Fatal(err)
			}
			defer release()
			ports := h.service.executor.config.Envelope.Identity
			identity, err := ports.Authenticate(bound, "token-a")
			if err != nil {
				t.Fatal(err)
			}
			calls := 0
			h.verifier.mu.Lock()
			h.verifier.onCached = func(check context.Context) error {
				calls++
				if calls == phase.call {
					cancel()
					return context.Canceled
				}
				return check.Err()
			}
			h.verifier.mu.Unlock()
			definition, ok := h.service.executor.config.Registry.Lookup("generic.read")
			if !ok {
				t.Fatal("comando de teste ausente")
			}
			err = ports.Authorize(bound, identity.Ownership, commandcontract.Envelope{}, definition)
			if calls != phase.call || !errors.Is(err, context.Canceled) || errors.Is(err, ErrDenied) {
				t.Fatalf("cancelamento convertido em recusa: calls=%d err=%v", calls, err)
			}
		})
	}
}
