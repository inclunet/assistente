package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"assistente/internal/commandcatalog"
	"assistente/internal/commandexecution"
	"assistente/internal/commandledger"
)

func newDeckFeedbackTestState(t *testing.T) (*commandProductRuntime, *commandDeckExecution, commandexecution.Versions, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	versions := commandexecution.Versions{Registry: "registry", GlobalConfig: "config", ActiveLayers: "layers", Unlocked: true}
	state := &commandDeckExecution{
		occurrences: make(map[string]commandDeckOccurrence),
		feedback:    newCommandDeckFeedbackState(),
	}
	p := &commandProductRuntime{deckExecution: state}
	state.occurrences["invocation-1"] = commandDeckOccurrence{
		ctx: ctx, identity: "streamdeck.key:deck:key:1", instanceID: "instance-1", versions: versions, generation: 7,
	}
	return p, state, versions, cancel
}

func registerDeckFeedbackTest(t *testing.T, state *commandDeckExecution, invocationID string) {
	t.Helper()
	state.mu.Lock()
	state.registerDeckFeedbackLocked(invocationID, state.occurrences[invocationID])
	state.mu.Unlock()
}

func TestCommandDeckFeedbackSnapshotTracksTerminalAfterOccurrenceRemoval(t *testing.T) {
	p, state, versions, cancel := newDeckFeedbackTestState(t)
	defer cancel()

	registerDeckFeedbackTest(t, state, "invocation-1")
	identity := "streamdeck.key:deck:key:1"
	if state, invocation := p.deckFeedbackSnapshot(identity, "instance-1", versions, 7); state != "waiting" || invocation != "invocation-1" {
		t.Fatalf("waiting: %q %q", state, invocation)
	}
	state.markDeckFeedbackRunning("invocation-1")
	if state, _ := p.deckFeedbackSnapshot(identity, "instance-1", versions, 7); state != "running" {
		t.Fatalf("running: %q", state)
	}
	state.finishDeckFeedback("invocation-1", commandledger.FullRecord{Status: commandledger.Succeeded}, nil)
	state.remove("invocation-1")
	if state, invocation := p.deckFeedbackSnapshot(identity, "instance-1", versions, 7); state != "succeeded" || invocation != "invocation-1" {
		t.Fatalf("completed after occurrence removal: %q %q", state, invocation)
	}
}

func TestCommandDeckFeedbackSnapshotRejectsStaleContextVersionsGenerationAndInstance(t *testing.T) {
	p, state, versions, cancel := newDeckFeedbackTestState(t)
	registerDeckFeedbackTest(t, state, "invocation-1")
	identity := "streamdeck.key:deck:key:1"
	if state, _ := p.deckFeedbackSnapshot(identity, "other-instance", versions, 7); state != "" {
		t.Fatalf("instance mismatch leaked %q", state)
	}
	if state, _ := p.deckFeedbackSnapshot(identity, "instance-1", versions, 8); state != "" {
		t.Fatalf("generation mismatch leaked %q", state)
	}
	otherVersions := versions
	otherVersions.ActiveLayers = "other-layers"
	if state, _ := p.deckFeedbackSnapshot(identity, "instance-1", otherVersions, 7); state != "" {
		t.Fatalf("version mismatch leaked %q", state)
	}
	cancel()
	if state, _ := p.deckFeedbackSnapshot(identity, "instance-1", versions, 7); state != "" {
		t.Fatalf("cancelled source context leaked %q", state)
	}
}

func TestCommandDeckFeedbackLatestOverlapCanOnlyFinishLatest(t *testing.T) {
	p, state, versions, cancel := newDeckFeedbackTestState(t)
	defer cancel()
	identity := "streamdeck.key:deck:key:1"
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	state.occurrences["invocation-2"] = commandDeckOccurrence{
		ctx: ctx2, identity: identity, instanceID: "instance-1", versions: versions, generation: 7,
	}
	registerDeckFeedbackTest(t, state, "invocation-1")
	registerDeckFeedbackTest(t, state, "invocation-2")
	state.finishDeckFeedback("invocation-1", commandledger.FullRecord{Status: commandledger.Succeeded}, nil)
	if state, invocation := p.deckFeedbackSnapshot(identity, "instance-1", versions, 7); state != "waiting" || invocation != "invocation-2" {
		t.Fatalf("stale overlap finished newer entry: %q %q", state, invocation)
	}
	state.finishDeckFeedback("invocation-2", commandledger.FullRecord{Status: commandledger.Failed}, nil)
	if state, invocation := p.deckFeedbackSnapshot(identity, "instance-1", versions, 7); state != "failed" || invocation != "invocation-2" {
		t.Fatalf("latest terminal result missing: %q %q", state, invocation)
	}
}

func TestCommandDeckFeedbackExpiryAndLimit(t *testing.T) {
	p, state, versions, cancel := newDeckFeedbackTestState(t)
	defer cancel()
	registerDeckFeedbackTest(t, state, "invocation-1")
	state.finishDeckFeedback("invocation-1", commandledger.FullRecord{Status: commandledger.Succeeded}, nil)
	state.mu.Lock()
	state.feedback.completed["streamdeck.key:deck:key:1"] = commandDeckFeedbackEntry{
		ctx: context.Background(), identity: "streamdeck.key:deck:key:1", instanceID: "instance-1", versions: versions, generation: 7,
		invocationID: "invocation-1", state: "succeeded", expiresAt: time.Now().Add(-time.Second),
	}
	state.mu.Unlock()
	if state, _ := p.deckFeedbackSnapshot("streamdeck.key:deck:key:1", "instance-1", versions, 7); state != "" {
		t.Fatalf("expired terminal entry leaked %q", state)
	}

	for i := 0; i < commandDeckFeedbackLimit; i++ {
		id := "identity-" + string(rune('a'+i))
		invocation := "invocation-" + string(rune('a'+i))
		ctx := context.Background()
		state.occurrences[invocation] = commandDeckOccurrence{ctx: ctx, identity: id, instanceID: "instance-1", versions: versions, generation: 7}
		registerDeckFeedbackTest(t, state, invocation)
	}
	state.occurrences["invocation-over-limit"] = commandDeckOccurrence{ctx: context.Background(), identity: "identity-over-limit", instanceID: "instance-1", versions: versions, generation: 7}
	registerDeckFeedbackTest(t, state, "invocation-over-limit")
	if state, _ := p.deckFeedbackSnapshot("identity-over-limit", "instance-1", versions, 7); state != "" {
		t.Fatalf("entry over limit was tracked as %q", state)
	}
}

func TestCommandDeckFeedbackLateOlderStartCannotResurrectExpiredLatest(t *testing.T) {
	p, state, versions, cancel := newDeckFeedbackTestState(t)
	defer cancel()
	identity := "streamdeck.key:deck:key:1"
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	state.occurrences["invocation-2"] = commandDeckOccurrence{ctx: ctx2, identity: identity, instanceID: "instance-1", versions: versions, generation: 7}
	registerDeckFeedbackTest(t, state, "invocation-1")
	registerDeckFeedbackTest(t, state, "invocation-2")
	state.finishDeckFeedback("invocation-2", commandledger.FullRecord{Status: commandledger.Succeeded}, nil)
	state.mu.Lock()
	state.feedback.completed[identity] = commandDeckFeedbackEntry{ctx: ctx2, identity: identity, instanceID: "instance-1", versions: versions, generation: 7, invocationID: "invocation-2", state: "succeeded", expiresAt: time.Now().Add(-time.Second)}
	state.mu.Unlock()
	state.markDeckFeedbackRunning("invocation-1")
	state.finishDeckFeedback("invocation-1", commandledger.FullRecord{Status: commandledger.Succeeded}, nil)
	if state, invocation := p.deckFeedbackSnapshot(identity, "instance-1", versions, 7); state != "" || invocation != "" {
		t.Fatalf("old delayed start resurrected feedback: %q %q", state, invocation)
	}
}

func TestCommandDeckFeedbackMapsEveryLedgerOutcome(t *testing.T) {
	tests := []struct {
		status commandledger.Status
		want   string
	}{
		{commandledger.Evaluating, "outcome_unknown"}, {commandledger.Queued, "outcome_unknown"}, {commandledger.Running, "outcome_unknown"},
		{commandledger.Succeeded, "succeeded"}, {commandledger.Failed, "failed"}, {commandledger.Denied, "denied"},
		{commandledger.Cancelled, "cancelled"}, {commandledger.CancelledStale, "cancelled"}, {commandledger.TimedOut, "timed_out"},
		{commandledger.OutcomeUnknown, "outcome_unknown"}, {commandledger.Suppressed, "denied"}, {commandledger.RejectedStale, "cancelled"},
	}
	for _, test := range tests {
		if got := commandDeckFeedbackResult(test.status, nil); got != test.want {
			t.Errorf("status %q: got %q, want %q", test.status, got, test.want)
		}
	}
	if got := commandDeckFeedbackResult("", nil); got != "outcome_unknown" {
		t.Fatalf("empty status became %q", got)
	}
	if got := commandDeckFeedbackResult("", context.DeadlineExceeded); got != "timed_out" {
		t.Fatalf("deadline error became %q", got)
	}
	if got := commandDeckFeedbackResult("", errors.New("unclassified")); got != "outcome_unknown" {
		t.Fatalf("unclassified error became %q", got)
	}
}

func TestCopyDeckFeedbackHandlersDoesNotMutateSharedHandlers(t *testing.T) {
	state := &commandDeckExecution{feedback: newCommandDeckFeedbackState()}
	called := false
	handlers := map[string]commandexecution.Handler{
		"fixture.read": {
			Contract: commandcatalog.HandlerContract{Effect: commandcatalog.Read},
			Start: func(context.Context, commandexecution.Invocation) (commandexecution.ExecutionHandle, error) {
				called = true
				return commandexecution.ExecutionHandle{}, nil
			},
		},
		"fixture.invalid": {Contract: commandcatalog.HandlerContract{Effect: commandcatalog.Read}},
	}
	copyHandlers := copyDeckFeedbackHandlers(state, handlers)
	if handlers["fixture.read"].Start == nil || copyHandlers["fixture.read"].Start == nil {
		t.Fatal("handler copy lost Start")
	}
	if copyHandlers["fixture.invalid"].Start != nil {
		t.Fatal("nil Start was wrapped")
	}
	if _, err := copyHandlers["fixture.read"].Start(context.Background(), commandexecution.Invocation{ID: "missing"}); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("wrapped Start did not call original")
	}
}
