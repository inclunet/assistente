package commandtoolbridge

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"

	"assistente/internal/commandexecution"
	"assistente/internal/commandledger"
)

func TestBridgeInputAdapterPreservesAuthenticatedEnvelope(t *testing.T) {
	bridge, _, tool, owner, invocationID := newBridgeFixture(t)
	invocation := commandInvocation(owner, invocationID)
	before, err := json.Marshal(invocation.Envelope)
	if err != nil {
		t.Fatal(err)
	}
	route := bridge.routes[bridgeCommandID]
	route.InputAdapter = func(raw json.RawMessage) (json.RawMessage, error) {
		// Mesmo um adapter que modifica seu buffer não altera o envelope.
		raw[0] = '!'
		return json.RawMessage(`{"secret":"adapted-secret"}`), nil
	}
	bridge.routes[bridgeCommandID] = route
	handler, _ := bridge.Handler(bridgeCommandID)
	close(tool.release)
	handle, err := handler.Start(context.Background(), invocation)
	if err != nil {
		t.Fatal(err)
	}
	assertBridgeOutcome(t, handle, commandledger.Succeeded)
	if got := string(<-tool.received); got != `{"secret":"adapted-secret"}` {
		t.Fatalf("argumentos entregues à tool = %s", got)
	}
	after, err := json.Marshal(invocation.Envelope)
	if err != nil || string(after) != string(before) {
		t.Fatalf("envelope autenticado foi alterado: %s / %s; %v", before, after, err)
	}
}

func TestBridgeInputAdapterFailureReleasesContextWithoutExecuting(t *testing.T) {
	for _, scenario := range []string{"error", "panic", "empty", "invalid_json"} {
		t.Run(scenario, func(t *testing.T) {
			bridge, _, tool, owner, invocationID := newBridgeFixture(t)
			var releases atomic.Int32
			route := bridge.routes[bridgeCommandID]
			route.PrepareContext = func(ctx context.Context, _ commandexecution.Invocation) (context.Context, func(), error) {
				return ctx, func() { releases.Add(1) }, nil
			}
			route.InputAdapter = func(json.RawMessage) (json.RawMessage, error) {
				switch scenario {
				case "error":
					return nil, errors.New("invalid arguments")
				case "panic":
					panic("adapter failed")
				case "invalid_json":
					return json.RawMessage(`{"broken"`), nil
				default:
					return nil, nil
				}
			}
			bridge.routes[bridgeCommandID] = route
			handler, _ := bridge.Handler(bridgeCommandID)
			handle, err := handler.Start(context.Background(), commandInvocation(owner, invocationID))
			if err != nil {
				t.Fatal(err)
			}
			assertBridgeOutcome(t, handle, commandledger.Failed)
			if releases.Load() != 1 {
				t.Fatalf("liberações de contexto = %d, esperado 1", releases.Load())
			}
			select {
			case <-tool.started:
				t.Fatal("tool executou após recusa do adapter")
			default:
			}
		})
	}
}
