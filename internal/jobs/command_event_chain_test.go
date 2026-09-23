package jobs

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"assistente/internal/database"
)

func commandEventChainForTest(t *testing.T, count int) []any {
	t.Helper()
	chain := make([]any, count)
	for i := range chain {
		chain[i] = map[string]any{
			"command_id":    "command." + string(rune('a'+i)),
			"invocation_id": uuid7ForTest(t),
			"layer_refs":    []string{"user-layer"},
		}
	}
	return chain
}

func commandEventUserContext() context.Context {
	return database.WithUserID(context.Background(), "event-chain-user")
}

func TestCommandEventOriginCarriesValidatedPrivateChainSnapshot(t *testing.T) {
	e := &JobExecutor{}
	job := &Job{ID: "current-job"}
	chain := commandEventChainForTest(t, 16)
	chain[0].(map[string]any)["layer_refs"] = []string{}
	run := &RunLog{RunID: "run", RootOriginType: "manual", RootOriginID: "root", Provenance: map[string]any{
		"command_chain_history": chain,
	}}
	ctx := e.withCommandEventOrigin(commandEventUserContext(), job, &TriggerContext{}, run)

	// A persisted provenance map may be reused or mutated by its owner; the
	// private event context must not alias it.
	chain[0].(map[string]any)["command_id"] = "tampered"
	got := &TriggerContext{Provenance: map[string]any{
		"command_chain_history": []any{"payload-must-not-win"},
	}}
	inheritCommandEventOrigin(ctx, got)

	validated, err := e.commandRunProvenance(job, got, run)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(validated["command_chain_history"])
	if !strings.Contains(string(raw), `"layer_refs":[]`) || !strings.Contains(string(raw), `"command_id":"command.a"`) {
		t.Fatalf("private command chain was not preserved: %s", raw)
	}
}

func TestCommandEventOriginRejectsSeventeenthEntry(t *testing.T) {
	e := &JobExecutor{}
	job := &Job{ID: "current-job"}
	run := &RunLog{RunID: "run", Provenance: map[string]any{
		"command_chain_history": commandEventChainForTest(t, 17),
	}}
	ctx := e.withCommandEventOrigin(commandEventUserContext(), job, &TriggerContext{}, run)
	got := &TriggerContext{Provenance: map[string]any{"command_chain_history": []any{"payload"}}}
	inheritCommandEventOrigin(ctx, got)
	if _, err := e.commandRunProvenance(job, got, run); err == nil {
		t.Fatal("invalid seventeenth entry was authorized after inheritance")
	}
}

func TestCommandEventOriginSurvivesEventBusWithoutPayloadChainReset(t *testing.T) {
	e := &JobExecutor{}
	job := &Job{ID: "current-job"}
	run := &RunLog{RunID: "run", Provenance: map[string]any{
		"command_chain_history": commandEventChainForTest(t, 2),
	}}
	ctx := e.withCommandEventOrigin(commandEventUserContext(), job, &TriggerContext{}, run)
	eb := NewEventBus()
	defer eb.Close()
	done := make(chan *TriggerContext, 1)
	eb.Subscribe("command.hop", "test", func(ctx context.Context, _ string, payload map[string]any) {
		got := &TriggerContext{Provenance: map[string]any{"command_chain_history": payload["command_chain_history"]}}
		inheritCommandEventOrigin(ctx, got)
		done <- got
	})
	eb.Publish(ctx, "command.hop", map[string]any{
		"command_chain_history": []any{"attacker-payload"},
		"_chain_history":        []string{},
	})

	select {
	case got := <-done:
		validated, err := e.commandRunProvenance(job, got, run)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(validated["command_chain_history"])
		if !strings.Contains(string(raw), `"command_id":"command.a"`) || !strings.Contains(string(raw), `"command_id":"command.b"`) {
			t.Fatalf("EventBus hop reset or replaced command chain: %#v", got.Provenance)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for EventBus hop")
	}
}

func TestCommandEventOriginForeignMarkerCannotPromoteManualRoot(t *testing.T) {
	trigger := &TriggerContext{
		RootOriginType: "manual",
		RootOriginID:   "new-manual-root",
		ChainID:        "manual-chain",
		ChainHistory:   []string{"manual-job"},
		Provenance:     map[string]any{"command_chain_history": []any{"foreign"}},
	}
	ctx := context.WithValue(commandEventUserContext(), commandEventOriginKey{}, commandEventOrigin{
		userID:   "different-user",
		rootType: "manual",
		rootID:   "foreign-root",
		chainID:  "foreign-chain",
		history:  []string{"foreign-job"},
	})

	inheritCommandEventOrigin(ctx, trigger)
	if trigger.RootOriginType != "unknown" || trigger.RootOriginID != "" {
		t.Fatalf("foreign marker promoted root: %#v", trigger)
	}
	if trigger.ChainID != "" || len(trigger.ChainHistory) != 0 {
		t.Fatalf("foreign chain was inherited: %#v", trigger)
	}
	if _, ok := trigger.Provenance["command_chain_history"]; ok {
		t.Fatal("foreign command provenance survived")
	}
}
