package commandtoolbridge

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"assistente/internal/commandexecution"
	"assistente/internal/commandledger"
)

func TestBridgeAuthorizationRunsOnWorkerWithDetachedInvocation(t *testing.T) {
	bridge, _, tool, userID, invocationID := newBridgeFixture(t)
	authorized := make(chan commandexecution.Invocation, 1)
	authorizeRelease := make(chan struct{})
	route := bridge.routes[bridgeCommandID]
	route.Authorize = func(_ context.Context, snapshot commandexecution.Invocation) error {
		authorized <- snapshot
		<-authorizeRelease
		return nil
	}
	bridge.routes[bridgeCommandID] = route

	handler, ok := bridge.Handler(bridgeCommandID)
	if !ok {
		t.Fatal("handler não encontrado")
	}
	invocation := commandInvocation(userID, invocationID)
	arguments := json.RawMessage(`{"binding":"original"}`)
	invocation.Envelope.Arguments = &arguments
	invocation.Envelope.BindingIDs = []string{"binding.original"}

	handle, err := handler.Start(context.Background(), invocation)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	var snapshot commandexecution.Invocation
	select {
	case snapshot = <-authorized:
	case <-time.After(time.Second):
		t.Fatal("Authorize não foi executado no worker")
	}
	if snapshot.Envelope == nil {
		t.Fatal("snapshot sem envelope")
	}
	// Mutação posterior do chamador não pode alcançar a cópia entregue ao
	// callback nem a execução da tool.
	*invocation.Envelope.Arguments = []byte(`{"binding":"mutated"}`)
	invocation.Envelope.BindingIDs[0] = "binding.mutated"
	close(authorizeRelease)
	select {
	case <-tool.started:
	case <-time.After(time.Second):
		t.Fatal("tool não iniciou após autorização")
	}
	close(tool.release)

	if snapshot.Envelope.BindingIDs[0] != "binding.original" || string(*snapshot.Envelope.Arguments) != `{"binding":"original"}` {
		t.Fatalf("snapshot sofreu alias do chamador: %+v", snapshot.Envelope)
	}
	select {
	case outcome := <-handle.Done:
		if outcome.Status != commandledger.Succeeded {
			t.Fatalf("outcome=%s", outcome.Status)
		}
	case <-time.After(time.Second):
		t.Fatal("handler não terminou")
	}
}

func TestBridgeAuthorizationRejectsBeforeTool(t *testing.T) {
	bridge, _, tool, userID, invocationID := newBridgeFixture(t)
	denied := errors.New("grant recusado")
	route := bridge.routes[bridgeCommandID]
	route.Authorize = func(context.Context, commandexecution.Invocation) error { return denied }
	bridge.routes[bridgeCommandID] = route
	handler, ok := bridge.Handler(bridgeCommandID)
	if !ok {
		t.Fatal("handler não encontrado")
	}
	handle, err := handler.Start(context.Background(), commandInvocation(userID, invocationID))
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	select {
	case <-tool.started:
		t.Fatal("tool iniciou antes da autorização")
	case <-time.After(100 * time.Millisecond):
	}
	select {
	case outcome := <-handle.Done:
		if outcome.Status != commandledger.Failed {
			t.Fatalf("outcome=%s, want failed", outcome.Status)
		}
	case <-time.After(time.Second):
		t.Fatal("handler não concluiu a recusa")
	}
}

func TestBridgeWithoutAuthorizationPreservesLegacyExecution(t *testing.T) {
	bridge, _, tool, userID, invocationID := newBridgeFixture(t)
	handler, ok := bridge.Handler(bridgeCommandID)
	if !ok {
		t.Fatal("handler não encontrado")
	}
	handle, err := handler.Start(context.Background(), commandInvocation(userID, invocationID))
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	select {
	case <-tool.started:
	case <-time.After(time.Second):
		t.Fatal("tool legada não iniciou")
	}
	close(tool.release)
	select {
	case outcome := <-handle.Done:
		if outcome.Status != commandledger.Succeeded {
			t.Fatalf("outcome=%s", outcome.Status)
		}
	case <-time.After(time.Second):
		t.Fatal("handler legado não terminou")
	}
}
