package jobs

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"assistente/internal/commandjobactivation"
	"assistente/internal/database"
	"assistente/internal/eventctx"
)

type tasklistObservedTrigger struct {
	trigger        *TriggerContext
	hasPrivateMark bool
	payload        map[string]any
}

func tasklistDomainOriginManager(t *testing.T, identity commandjobactivation.RuntimeIdentity, current *commandjobactivation.RuntimeIdentity) (*Manager, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	m := &Manager{eventBus: NewEventBus()}
	m.cfg.CommandRuntimeIdentity = func(context.Context) (commandjobactivation.RuntimeIdentity, context.Context, func(), error) {
		calls.Add(1)
		watchCtx, cancel := context.WithCancel(context.Background())
		selected := identity
		if current != nil && calls.Load() > 1 {
			selected = *current
		}
		return selected, watchCtx, cancel, nil
	}
	t.Cleanup(m.eventBus.Close)
	return m, &calls
}

func TestTasklistDomainEventSinkCreatesAuthenticatedInternalRoot(t *testing.T) {
	identity := commandjobactivation.RuntimeIdentity{UserID: "user-a", AuthContextType: "local_session", AuthContextID: "session-a", AuthGeneration: "auth-a", SecurityGeneration: "security-a"}
	m, calls := tasklistDomainOriginManager(t, identity, nil)
	ctx := database.WithUserID(context.Background(), identity.UserID)

	received := make(chan map[string]any, 1)
	triggered := make(chan *TriggerContext, 1)
	m.eventBus.Subscribe("tasklist.task.updated", "test", func(ctx context.Context, _ string, payload map[string]any) {
		trigger := &TriggerContext{Type: TriggerEvent, Provenance: map[string]any{"command_chain_history": []any{"spoof"}}}
		inheritCommandEventOrigin(ctx, trigger)
		received <- payload
		triggered <- trigger
	})
	sink := m.TasklistDomainEventSink()
	if err := sink.PublishDomainEvent(ctx, "tasklist.task.updated", map[string]any{
		"task_id":          "task-1",
		"root_origin_type": "manual",
		"root_origin_id":   "spoof-root",
		"_source":          "job",
		"_source_job_id":   "spoof-job",
		"_chain_id":        "spoof-chain",
		"_chain_history":   []string{"spoof"},
	}); err != nil {
		t.Fatal(err)
	}
	payload := <-received
	if payload["_source"] != "user" || payload["_source_job_id"] != "" {
		t.Fatalf("proveniência pública inesperada: %#v", payload)
	}
	for _, key := range []string{"root_origin_type", "root_origin_id", "command_chain_history"} {
		if _, ok := payload[key]; ok {
			t.Fatalf("campo de autoridade aceito do payload: %q", key)
		}
	}
	if chainID, ok := payload["_chain_id"].(string); !ok || chainID == "" || payload["_chain_history"] == nil {
		t.Fatalf("cadeia privada da nova raiz ausente: %#v", payload)
	}
	trigger := <-triggered
	if trigger.RootOriginType != "internal_event" || trigger.RootOriginID == "" || trigger.ChainID != payload["_chain_id"] || len(trigger.ChainHistory) != 0 {
		t.Fatalf("raiz interna/cadeia inesperadas: %+v", trigger)
	}
	if calls.Load() != 2 {
		t.Fatalf("runtime deveria ser consultado na captura e no listener: %d", calls.Load())
	}
}

func TestTasklistDomainEventSinkRejectsEpochDriftBeforePromotingRoot(t *testing.T) {
	first := commandjobactivation.RuntimeIdentity{UserID: "user-a", AuthContextType: "local_session", AuthContextID: "session-a", AuthGeneration: "auth-a", SecurityGeneration: "security-a"}
	changed := first
	changed.SecurityGeneration = "security-b"
	m, _ := tasklistDomainOriginManager(t, first, &changed)
	ctx := database.WithUserID(context.Background(), first.UserID)
	triggered := make(chan *TriggerContext, 1)
	m.eventBus.Subscribe("tasklist.task.updated", "test", func(ctx context.Context, _ string, payload map[string]any) {
		trigger := &TriggerContext{Type: TriggerEvent}
		inheritCommandEventOrigin(ctx, trigger)
		triggered <- trigger
	})
	if err := m.TasklistDomainEventSink().PublishDomainEvent(ctx, "tasklist.task.updated", map[string]any{"task_id": "task-1"}); err != nil {
		t.Fatal(err)
	}
	trigger := <-triggered
	if trigger.RootOriginType != "unknown" || trigger.RootOriginID != "" {
		t.Fatalf("drift de epoch promoveu raiz: %+v", trigger)
	}
}

func TestTasklistDomainEventSinkDoesNotPromotePublicEventctxOrExistingMarker(t *testing.T) {
	identity := commandjobactivation.RuntimeIdentity{UserID: "user-a", AuthContextType: "local_session", AuthContextID: "session-a", AuthGeneration: "auth-a", SecurityGeneration: "security-a"}
	m, calls := tasklistDomainOriginManager(t, identity, nil)
	ctx := database.WithUserID(context.Background(), identity.UserID)
	ctx = eventctx.With(ctx, eventctx.Provenance{Source: "job", SourceJobID: "job-a", ChainID: "chain-public", ChainHistory: []string{"job-a"}})
	received := make(chan tasklistObservedTrigger, 3)
	m.eventBus.Subscribe("tasklist.task.updated", "test", func(ctx context.Context, _ string, payload map[string]any) {
		trigger := &TriggerContext{Type: TriggerEvent}
		inheritCommandEventOrigin(ctx, trigger)
		_, hasPrivateMark := ctx.Value(commandEventOriginKey{}).(commandEventOrigin)
		received <- tasklistObservedTrigger{trigger: trigger, hasPrivateMark: hasPrivateMark, payload: payload}
	})
	if err := m.TasklistDomainEventSink().PublishDomainEvent(ctx, "tasklist.task.updated", map[string]any{
		"_source": "job", "_source_job_id": "spoof", "_chain_id": "spoof", "_chain_history": []string{"spoof"},
	}); err != nil {
		t.Fatal(err)
	}
	first := receiveTasklistOrigin(t, received)
	rootType, rootID := runRoot(first.trigger, "run-public-eventctx")
	if first.hasPrivateMark || rootType != "unknown" || rootID != "" || calls.Load() != 0 || first.payload["_source"] != "job" || first.payload["_source_job_id"] != "job-a" || first.payload["_chain_id"] != "chain-public" {
		t.Fatalf("eventctx público promoveu raiz: observed=%+v root=(%q,%q) runtimeCalls=%d", first, rootType, rootID, calls.Load())
	}

	markerCtx := context.WithValue(database.WithUserID(context.Background(), identity.UserID), commandEventOriginKey{}, commandEventOrigin{userID: identity.UserID, rootType: "unknown"})
	if err := m.TasklistDomainEventSink().PublishDomainEvent(markerCtx, "tasklist.task.updated", map[string]any{
		"_source": "job", "_chain_id": "spoof-unknown", "_chain_history": []string{"spoof"},
	}); err != nil {
		t.Fatal(err)
	}
	second := receiveTasklistOrigin(t, received)
	if !second.hasPrivateMark || second.trigger.RootOriginType != "unknown" || second.trigger.RootOriginID != "" || second.payload["_source"] != "user" {
		t.Fatalf("marcador unknown não foi preservado: %+v", second)
	}
	foreignCtx := context.WithValue(database.WithUserID(context.Background(), identity.UserID), commandEventOriginKey{}, commandEventOrigin{
		userID: "foreign-user", rootType: "internal_event", rootID: "foreign-root", chainID: "foreign-chain", history: []string{"foreign-job"},
	})
	if err := m.TasklistDomainEventSink().PublishDomainEvent(foreignCtx, "tasklist.task.updated", map[string]any{"_chain_id": "spoof-foreign"}); err != nil {
		t.Fatal(err)
	}
	third := receiveTasklistOrigin(t, received)
	if !third.hasPrivateMark || third.trigger.RootOriginType != "unknown" || third.trigger.RootOriginID != "" || third.payload["_source"] != "user" {
		t.Fatalf("marcador foreign promoveu origem ou proveniência: %+v", third)
	}
}

func TestTasklistDomainEventSinkPreservesSameUserEmptyChainID(t *testing.T) {
	identity := commandjobactivation.RuntimeIdentity{UserID: "user-a", AuthContextType: "local_session", AuthContextID: "session-a", AuthGeneration: "auth-a", SecurityGeneration: "security-a"}
	m, _ := tasklistDomainOriginManager(t, identity, nil)
	received := make(chan map[string]any, 1)
	m.eventBus.Subscribe("tasklist.task.updated", "test", func(_ context.Context, _ string, payload map[string]any) { received <- payload })
	ctx := context.WithValue(database.WithUserID(context.Background(), identity.UserID), commandEventOriginKey{}, commandEventOrigin{
		userID: identity.UserID, rootType: "unknown", chainID: "private-chain", history: []string{},
	})
	if err := m.TasklistDomainEventSink().PublishDomainEvent(ctx, "tasklist.task.updated", map[string]any{"_chain_id": "spoof"}); err != nil {
		t.Fatal(err)
	}
	payload := receiveTasklistPayload(t, received)
	if payload["_source"] != "user" || payload["_chain_id"] != "private-chain" {
		t.Fatalf("chain privado do mesmo usuário foi perdido: %#v", payload)
	}
	history, ok := payload["_chain_history"].([]string)
	if !ok || len(history) != 0 {
		t.Fatalf("histórico vazio não preservado: %#v", payload["_chain_history"])
	}
}

func TestTasklistDomainEventSinkRejectsExternalRuntimeIdentity(t *testing.T) {
	identity := commandjobactivation.RuntimeIdentity{UserID: "user-a", AuthContextType: "external_token", AuthContextID: "issuer-a", AuthGeneration: "auth-a", SecurityGeneration: "security-a"}
	m, _ := tasklistDomainOriginManager(t, identity, nil)
	m.eventBus.Subscribe("tasklist.task.updated", "test", func(context.Context, string, map[string]any) { t.Error("evento externo publicado como internal_event") })
	ctx := database.WithUserID(context.Background(), identity.UserID)
	if err := m.TasklistDomainEventSink().PublishDomainEvent(ctx, "tasklist.task.updated", nil); err == nil {
		t.Fatal("identidade externa deveria ser recusada")
	}
}

func TestCommandEventRuntimeMatchesFailsClosedWithoutGuard(t *testing.T) {
	identity := commandjobactivation.RuntimeIdentity{UserID: "user-a", AuthContextType: "local_session", AuthContextID: "session-a", AuthGeneration: "auth-a", SecurityGeneration: "security-a"}
	ctx := context.WithValue(context.Background(), commandEventOriginKey{}, commandEventOrigin{runtimeIdentity: &identity})
	if commandEventRuntimeMatches(ctx, identity) {
		t.Fatal("marcador com runtimeIdentity sem guard não pode ser aceito")
	}
	withGuard := context.WithValue(context.Background(), commandEventOriginKey{}, commandEventOrigin{runtimeIdentity: &identity, runtimeGuard: func(context.Context, commandjobactivation.RuntimeIdentity) bool { return true }})
	if !commandEventRuntimeMatches(withGuard, identity) {
		t.Fatal("marcador com identidade e guard correspondentes deveria passar")
	}
}

func receiveTasklistPayload(t *testing.T, received <-chan map[string]any) map[string]any {
	t.Helper()
	select {
	case payload := <-received:
		return payload
	case <-time.After(2 * time.Second):
		t.Fatal("timeout aguardando payload de tasklist")
		return nil
	}
}

func receiveTasklistOrigin(t *testing.T, received <-chan tasklistObservedTrigger) tasklistObservedTrigger {
	t.Helper()
	select {
	case observed := <-received:
		return observed
	case <-time.After(2 * time.Second):
		t.Fatal("timeout aguardando evento de tasklist")
		return tasklistObservedTrigger{}
	}
}
