package commandtoolbridge

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"assistente/internal/commandexecution"
	"assistente/internal/commandledger"
	"assistente/internal/database"
	"assistente/internal/tools/invocationctx"
)

type bridgeContextValueKey struct{}

func TestBridgePrepareContextRunsOnWorkerAndPreservesMetadata(t *testing.T) {
	bridge, _, tool, userID, invocationID := newBridgeFixture(t)
	entered := make(chan struct{})
	allow := make(chan struct{})
	var releases atomic.Int32
	var callbackSnapshot commandexecution.Invocation

	route := bridge.routes[bridgeCommandID]
	route.PrepareContext = func(ctx context.Context, snapshot commandexecution.Invocation) (context.Context, func(), error) {
		callbackSnapshot = snapshot
		if got, ok := database.UserIDFromContext(ctx); !ok || got != userID {
			t.Errorf("owner no contexto de preparação = %q, ok=%v", got, ok)
		}
		if _, ok := invocationctx.Get(ctx); !ok {
			t.Error("metadados de invocação ausentes no contexto de preparação")
		}
		close(entered)
		<-allow
		return context.WithValue(ctx, bridgeContextValueKey{}, "prepared"), func() { releases.Add(1) }, nil
	}
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
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("PrepareContext não foi executado no worker")
	}
	select {
	case <-tool.started:
		t.Fatal("tool iniciou enquanto PrepareContext estava bloqueado")
	default:
	}
	close(allow)

	select {
	case toolCtx := <-tool.contexts:
		if value := toolCtx.Value(bridgeContextValueKey{}); value != "prepared" {
			t.Fatalf("valor preparado = %#v", value)
		}
		if _, ok := invocationctx.Get(toolCtx); !ok {
			t.Fatal("metadados de invocação desapareceram no contexto retornado")
		}
	case <-time.After(time.Second):
		t.Fatal("tool não recebeu contexto")
	}
	close(tool.release)
	assertBridgeOutcome(t, handle, commandledger.Succeeded)
	if releases.Load() != 1 {
		t.Fatalf("release chamado %d vezes, want 1", releases.Load())
	}
	if callbackSnapshot.ID != invocationID {
		t.Fatalf("snapshot recebido = %q, want %q", callbackSnapshot.ID, invocationID)
	}
}

func TestBridgePrepareContextPreservesOriginalCancellation(t *testing.T) {
	bridge, _, tool, userID, invocationID := newBridgeFixture(t)
	prepared := make(chan struct{})
	allow := make(chan struct{})
	var releases atomic.Int32
	route := bridge.routes[bridgeCommandID]
	route.PrepareContext = func(ctx context.Context, _ commandexecution.Invocation) (context.Context, func(), error) {
		close(prepared)
		<-allow
		return database.WithUserID(context.Background(), "foreign-owner"), func() { releases.Add(1) }, nil
	}
	bridge.routes[bridgeCommandID] = route

	parent, cancel := context.WithTimeout(context.Background(), time.Second)
	handler, _ := bridge.Handler(bridgeCommandID)
	handle, err := handler.Start(parent, commandInvocation(userID, invocationID))
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	select {
	case <-prepared:
	case <-time.After(time.Second):
		t.Fatal("PrepareContext não iniciou")
	}
	close(allow)
	select {
	case toolCtx := <-tool.contexts:
		if got, ok := database.UserIDFromContext(toolCtx); !ok || got != userID {
			t.Fatalf("owner propagado = %q, ok=%v; want %q", got, ok, userID)
		}
		if _, ok := toolCtx.Deadline(); !ok {
			t.Fatal("deadline original desapareceu no contexto combinado")
		}
	case <-time.After(time.Second):
		t.Fatal("tool não recebeu contexto após preparação")
	}
	select {
	case <-tool.started:
	case <-time.After(time.Second):
		t.Fatal("tool não iniciou após preparação")
	}
	cancel()
	select {
	case outcome := <-handle.Done:
		if outcome.Status != commandledger.OutcomeUnknown {
			t.Fatalf("outcome=%s, want outcome_unknown", outcome.Status)
		}
	case <-time.After(time.Second):
		t.Fatal("handler não respeitou cancelamento original")
	}
	if releases.Load() != 1 {
		t.Fatalf("release chamado %d vezes, want 1", releases.Load())
	}
}

func TestBridgePrepareContextReleaseOnErrorAndNilContext(t *testing.T) {
	cases := []struct {
		name       string
		prepare    func(*atomic.Int32) func(context.Context, commandexecution.Invocation) (context.Context, func(), error)
		wantStatus commandledger.Status
	}{
		{
			name: "error",
			prepare: func(releases *atomic.Int32) func(context.Context, commandexecution.Invocation) (context.Context, func(), error) {
				return func(context.Context, commandexecution.Invocation) (context.Context, func(), error) {
					return context.Background(), func() { releases.Add(1) }, errors.New("prepare failed")
				}
			},
			wantStatus: commandledger.Failed,
		},
		{
			name: "nil context",
			prepare: func(releases *atomic.Int32) func(context.Context, commandexecution.Invocation) (context.Context, func(), error) {
				return func(context.Context, commandexecution.Invocation) (context.Context, func(), error) {
					return nil, func() { releases.Add(1) }, nil
				}
			},
			wantStatus: commandledger.OutcomeUnknown,
		},
		{
			name: "panic",
			prepare: func(*atomic.Int32) func(context.Context, commandexecution.Invocation) (context.Context, func(), error) {
				return func(context.Context, commandexecution.Invocation) (context.Context, func(), error) {
					panic("prepare panic")
				}
			},
			wantStatus: commandledger.Failed,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bridge, _, tool, userID, invocationID := newBridgeFixture(t)
			var releases atomic.Int32
			route := bridge.routes[bridgeCommandID]
			route.PrepareContext = tc.prepare(&releases)
			bridge.routes[bridgeCommandID] = route
			handler, _ := bridge.Handler(bridgeCommandID)
			handle, err := handler.Start(context.Background(), commandInvocation(userID, invocationID))
			if err != nil {
				t.Fatalf("Start: %v", err)
			}
			assertBridgeOutcome(t, handle, tc.wantStatus)
			select {
			case <-tool.started:
				t.Fatal("tool iniciou após falha de PrepareContext")
			default:
			}
			wantRelease := int32(0)
			if tc.name != "panic" {
				wantRelease = 1
			}
			if releases.Load() != wantRelease {
				t.Fatalf("release chamado %d vezes, want %d", releases.Load(), wantRelease)
			}
		})
	}
}

func TestBridgePrepareContextReleasePanicIsUnknownWithoutSecondSend(t *testing.T) {
	bridge, _, tool, userID, invocationID := newBridgeFixture(t)
	route := bridge.routes[bridgeCommandID]
	route.PrepareContext = func(context.Context, commandexecution.Invocation) (context.Context, func(), error) {
		return context.Background(), func() { panic("release panic") }, nil
	}
	bridge.routes[bridgeCommandID] = route
	handler, _ := bridge.Handler(bridgeCommandID)
	handle, err := handler.Start(context.Background(), commandInvocation(userID, invocationID))
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	select {
	case <-tool.started:
	case <-time.After(time.Second):
		t.Fatal("tool não iniciou")
	}
	close(tool.release)
	assertBridgeOutcome(t, handle, commandledger.OutcomeUnknown)
}

func assertBridgeOutcome(t *testing.T, handle commandexecution.ExecutionHandle, want commandledger.Status) {
	t.Helper()
	select {
	case outcome := <-handle.Done:
		if outcome.Status != want {
			t.Fatalf("outcome=%s, want %s", outcome.Status, want)
		}
	case <-time.After(time.Second):
		t.Fatal("handler não terminou")
	}
}
