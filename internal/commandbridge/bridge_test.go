package commandbridge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"assistente/internal/commandinput"
)

type testPort struct {
	mu          sync.Mutex
	dispatched  []Invocation
	cancelled   []CancelRequest
	ack         InvocationAck
	dispatchErr error
	cancelErr   error
	mutateProof bool
}

func testUUID7(number int) string {
	return fmt.Sprintf("01900000-0000-7000-8000-%012d", number)
}

type handoffPort struct {
	started chan struct{}
	release chan struct{}
	done    chan struct{}
}

type shutdownPort struct {
	*testPort
	shutdownCalls int
	shutdownErr   error
}

func (p *shutdownPort) Shutdown(context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.shutdownCalls++
	return p.shutdownErr
}

type blockingCancelPort struct {
	*testPort
	cancelStarted chan struct{}
	releaseCancel chan struct{}
	shutdownCalls int
}

func (p *blockingCancelPort) Cancel(ctx context.Context, _ CancelRequest) error {
	close(p.cancelStarted)
	select {
	case <-p.releaseCancel:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (p *blockingCancelPort) Shutdown(context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.shutdownCalls++
	return nil
}

func (p *handoffPort) Dispatch(_ context.Context, invocation Invocation) (InvocationAck, error) {
	close(p.started)
	go func() {
		<-p.release
		close(p.done)
	}()
	return InvocationAck{InvocationID: invocation.InvocationID, Accepted: true}, nil
}

func (p *handoffPort) Cancel(context.Context, CancelRequest) error { return nil }

func (p *testPort) Dispatch(_ context.Context, invocation Invocation) (InvocationAck, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.mutateProof && invocation.DialogProof != nil {
		invocation.DialogProof.DialogID = "mutated-by-port"
	}
	p.dispatched = append(p.dispatched, invocation)
	if p.dispatchErr != nil {
		return InvocationAck{}, p.dispatchErr
	}
	ack := p.ack
	if ack == (InvocationAck{}) {
		ack.Accepted = true
	}
	return ack, nil
}

func (p *testPort) Cancel(_ context.Context, request CancelRequest) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cancelled = append(p.cancelled, request)
	return p.cancelErr
}

func newBridgeFixture(t *testing.T) (*Bridge, *testPort, Owner, Invocation) {
	t.Helper()
	port := &testPort{}
	owner := Owner{UserID: "user-a", SessionID: "session-a", WorkspaceID: "workspace-a"}
	bridge, err := New(Config{Port: port, Capabilities: []Capability{{ID: "cap-a", CommandID: "command.a", Generation: 1, Source: SourceUIAction, Owner: owner}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := bridge.OpenSession(Session{ID: owner.SessionID, Generation: 1, Owner: owner}); err != nil {
		t.Fatal(err)
	}
	return bridge, port, owner, Invocation{SessionID: owner.SessionID, InvocationID: testUUID7(1), CommandID: "command.a", Generation: 1, CapabilityID: "cap-a", Ownership: OwnershipLocal, Source: SourceUIAction}
}

func TestBridgeDispatchesOnlyAfterCapabilityAndOwnerChecks(t *testing.T) {
	bridge, port, owner, invocation := newBridgeFixture(t)
	ack, err := bridge.Invoke(context.Background(), invocation, owner)
	if err != nil || !ack.Accepted || ack.InvocationID != invocation.InvocationID {
		t.Fatalf("ack=%+v err=%v", ack, err)
	}
	if len(port.dispatched) != 1 {
		t.Fatalf("dispatches=%d", len(port.dispatched))
	}
	if _, err := bridge.Invoke(context.Background(), invocation, owner); !errors.Is(err, ErrInvocationReplay) {
		t.Fatalf("replay err=%v", err)
	}
	wrongOwner := owner
	wrongOwner.UserID = "user-b"
	wrong := invocation
	wrong.InvocationID = testUUID7(2)
	if _, err := bridge.Invoke(context.Background(), wrong, wrongOwner); !errors.Is(err, ErrStaleGeneration) {
		t.Fatalf("owner err=%v", err)
	}
	if len(port.dispatched) != 1 {
		t.Fatalf("owner inválido chegou à porta: %d", len(port.dispatched))
	}
}

func TestBridgeCapabilityBindsSourceAndIngressRejectsPhysicalForgery(t *testing.T) {
	bridge, port, owner, invocation := newBridgeFixture(t)
	if err := bridge.ReplaceCapabilities([]Capability{{ID: "cap-a", CommandID: invocation.CommandID, Generation: 1, Source: SourceKeyboardLocal, Owner: owner}}); err != nil {
		t.Fatal(err)
	}

	forged := invocation
	forged.InvocationID = testUUID7(12)
	forged.Source = SourceStreamDeck
	if _, err := bridge.Invoke(context.Background(), forged, owner); !errors.Is(err, ErrCapabilityDenied) {
		t.Fatalf("origem diferente da capability aceita: %v", err)
	}
	if _, err := bridge.InvokeIngress(context.Background(), Invocation{
		SessionID: owner.SessionID, InvocationID: testUUID7(14), CommandID: invocation.CommandID,
		Generation: 1, CapabilityID: invocation.CapabilityID, Ownership: OwnershipLocal,
		Source: SourceKeyboardLocal,
	}, owner); !errors.Is(err, ErrCapabilityDenied) {
		t.Fatalf("ingresso físico Wails aceito: %v", err)
	}

	physical := invocation
	physical.InvocationID = testUUID7(15)
	physical.Source = SourceKeyboardLocal
	ack, err := bridge.Invoke(context.Background(), physical, owner)
	if err != nil || !ack.Accepted {
		t.Fatalf("handoff físico confiável rejeitado: ack=%+v err=%v", ack, err)
	}
	if len(port.dispatched) != 1 {
		t.Fatalf("dispatch inesperado após forged source: %d", len(port.dispatched))
	}
	if _, err := bridge.Cancel(context.Background(), CancelRequest{
		SessionID: owner.SessionID, InvocationID: physical.InvocationID, Generation: 1,
		CapabilityID: physical.CapabilityID, Owner: owner,
	}); err != nil {
		t.Fatalf("cancel do handoff físico: %v", err)
	}
	input := Input{
		SessionID: owner.SessionID, Source: "keyboard", Key: "Ctrl+N", Generation: 1,
		Kind: commandinput.KeyDown, Invocation: physical, Owner: owner,
	}
	if _, err := bridge.InputIngress(context.Background(), input); !errors.Is(err, ErrCapabilityDenied) {
		t.Fatalf("input físico Wails aceito: %v", err)
	}
}

func TestBridgeResultRequiresExactIdentityAndReleasesClaim(t *testing.T) {
	bridge, _, owner, invocation := newBridgeFixture(t)
	invocation.OccurrenceID = "8:keyboard6:Ctrl+N"
	if _, err := bridge.Invoke(context.Background(), invocation, owner); err != nil {
		t.Fatal(err)
	}
	wrong := Result{SessionID: invocation.SessionID, InvocationID: invocation.InvocationID, CommandID: invocation.CommandID, Generation: invocation.Generation, CapabilityID: invocation.CapabilityID, Ownership: invocation.Ownership, OccurrenceID: invocation.OccurrenceID, Owner: owner, Status: ResultSucceeded}
	wrong.Generation++
	if _, err := bridge.AcceptResult(wrong); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("resultado de geração err=%v", err)
	}
	wrong.Generation = invocation.Generation
	wrong.Owner.UserID = "user-b"
	if _, err := bridge.AcceptResult(wrong); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("resultado de owner err=%v", err)
	}
	wrong.Owner = owner
	wrong.SourceEventID = testUUID7(99)
	if _, err := bridge.AcceptResult(wrong); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("resultado de source event err=%v", err)
	}
	wrong.SourceEventID = ""
	ack, err := bridge.AcceptResult(wrong)
	if err != nil || !ack.Accepted {
		t.Fatalf("resultado válido ack=%+v err=%v", ack, err)
	}
	if _, err := bridge.AcceptResult(wrong); !errors.Is(err, ErrUnknownInvocation) {
		t.Fatalf("resultado repetido err=%v", err)
	}
	second := invocation
	second.InvocationID = testUUID7(3)
	if _, err := bridge.Invoke(context.Background(), second, owner); err != nil {
		t.Fatalf("claim não liberado: %v", err)
	}
}

func TestBridgeDialogProofRequiresExactRoundTrip(t *testing.T) {
	bridge, _, owner, invocation := newBridgeFixture(t)
	if err := bridge.ReplaceCapabilities([]Capability{{ID: "cap-a", CommandID: DecisionRespondCommandID, Generation: 1, Source: SourceKeyboardLocal, Owner: owner}}); err != nil {
		t.Fatal(err)
	}
	invocation.CommandID = DecisionRespondCommandID
	invocation.Source = SourceKeyboardLocal
	invocation.DialogProof = &DialogProof{
		DialogID:        "decision-a",
		Kind:            "decision",
		ScopeGeneration: 1,
		CommandID:       DecisionRespondCommandID,
		TriggerSpec:     DecisionRepeatTrigger,
	}
	if _, err := bridge.Invoke(context.Background(), invocation, owner); err != nil {
		t.Fatal(err)
	}
	wrong := Result{SessionID: invocation.SessionID, InvocationID: invocation.InvocationID, CommandID: invocation.CommandID, Generation: invocation.Generation, CapabilityID: invocation.CapabilityID, Ownership: invocation.Ownership, Owner: owner, Status: ResultSucceeded, DialogProof: &DialogProof{
		DialogID:        "decision-a",
		Kind:            "decision",
		ScopeGeneration: 2,
		CommandID:       DecisionRespondCommandID,
		TriggerSpec:     DecisionRepeatTrigger,
	}}
	if _, err := bridge.AcceptResult(wrong); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("resultado com dialogProof obsoleto err=%v", err)
	}
	valid := wrong
	valid.DialogProof = invocation.DialogProof
	ack, err := bridge.AcceptResult(valid)
	if err != nil || !ack.Accepted {
		t.Fatalf("resultado válido ack=%+v err=%v", ack, err)
	}
}

func TestBridgeInputValidatesBeforeTransitionIncludingRelease(t *testing.T) {
	bridge, _, owner, invocation := newBridgeFixture(t)
	input := Input{
		SessionID: owner.SessionID, Source: "keyboard", Key: "Ctrl+N", Generation: 1,
		Kind: commandinput.KeyDown, Invocation: invocation, Owner: owner,
	}
	wrongOwner := owner
	wrongOwner.UserID = "user-b"
	if _, err := bridge.Input(context.Background(), Input{SessionID: input.SessionID, Source: input.Source, Key: input.Key, Generation: input.Generation, Kind: input.Kind, Invocation: input.Invocation, Owner: wrongOwner}); !errors.Is(err, ErrStaleGeneration) {
		t.Fatalf("down com owner inválido err=%v", err)
	}
	if ack, err := bridge.Input(context.Background(), input); err != nil || !ack.Accepted {
		t.Fatalf("down válido após owner inválido ack=%+v err=%v", ack, err)
	}

	badRelease := input
	badRelease.Kind = commandinput.KeyUp
	badRelease.Owner = wrongOwner
	if _, err := bridge.Input(context.Background(), badRelease); !errors.Is(err, ErrStaleGeneration) {
		t.Fatalf("release com owner inválido err=%v", err)
	}
	if _, err := bridge.AcceptResult(Result{
		SessionID: owner.SessionID, InvocationID: invocation.InvocationID, CommandID: invocation.CommandID,
		Generation: 1, CapabilityID: invocation.CapabilityID, Ownership: invocation.Ownership,
		OccurrenceID: normalizeOccurrence(input.Source, input.Key), Owner: owner, Status: ResultSucceeded,
	}); err != nil {
		t.Fatalf("resultado do down inicial: %v", err)
	}
	secondDown := input
	secondDown.Invocation.InvocationID = testUUID7(90)
	if ack, err := bridge.Input(context.Background(), secondDown); err != nil || ack.Accepted {
		t.Fatalf("release inválido consumiu down ack=%+v err=%v", ack, err)
	}
}

func TestBridgeDetachesDialogProofFromCallerAndPort(t *testing.T) {
	port := &testPort{mutateProof: true}
	owner := Owner{UserID: "user-a", SessionID: "session-a", WorkspaceID: "workspace-a"}
	bridge, err := New(Config{Port: port, Capabilities: []Capability{{ID: "cap-a", CommandID: DecisionRespondCommandID, Generation: 1, Source: SourceKeyboardLocal, Owner: owner}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := bridge.OpenSession(Session{ID: owner.SessionID, Generation: 1, Owner: owner}); err != nil {
		t.Fatal(err)
	}
	proof := &DialogProof{DialogID: "decision-a", Kind: "decision", ScopeGeneration: 1, CommandID: DecisionRespondCommandID, TriggerSpec: DecisionRepeatTrigger}
	invocation := Invocation{SessionID: owner.SessionID, InvocationID: testUUID7(91), CommandID: DecisionRespondCommandID, Generation: 1, CapabilityID: "cap-a", Ownership: OwnershipLocal, Source: SourceKeyboardLocal, DialogProof: proof}
	if _, err := bridge.Invoke(context.Background(), invocation, owner); err != nil {
		t.Fatal(err)
	}
	proof.DialogID = "mutated-by-caller"
	result := Result{SessionID: invocation.SessionID, InvocationID: invocation.InvocationID, CommandID: invocation.CommandID, Generation: invocation.Generation, CapabilityID: invocation.CapabilityID, Ownership: invocation.Ownership, Owner: owner, Status: ResultSucceeded, DialogProof: &DialogProof{DialogID: "decision-a", Kind: "decision", ScopeGeneration: 1, CommandID: DecisionRespondCommandID, TriggerSpec: DecisionRepeatTrigger}}
	if ack, err := bridge.AcceptResult(result); err != nil || !ack.Accepted {
		t.Fatalf("roundtrip da prova detached ack=%+v err=%v", ack, err)
	}
	if port.dispatched[0].DialogProof.DialogID != "mutated-by-port" {
		t.Fatalf("porta não recebeu cópia mutável isolada: %+v", port.dispatched[0].DialogProof)
	}
}

func TestBridgeLocalGlobalOwnershipIsExclusive(t *testing.T) {
	bridge, _, owner, invocation := newBridgeFixture(t)
	invocation.OccurrenceID = "8:keyboard6:Ctrl+N"
	if _, err := bridge.Invoke(context.Background(), invocation, owner); err != nil {
		t.Fatal(err)
	}
	global := invocation
	global.InvocationID = testUUID7(4)
	global.Ownership = OwnershipGlobal
	global.OccurrenceID = invocation.OccurrenceID
	if _, err := bridge.Invoke(context.Background(), global, owner); !errors.Is(err, ErrOwnershipConflict) {
		t.Fatalf("local/global não conflitantes: %v", err)
	}
}

func TestBridgeDirectInvocationsDoNotSharePhysicalClaim(t *testing.T) {
	bridge, port, owner, invocation := newBridgeFixture(t)
	if _, err := bridge.Invoke(context.Background(), invocation, owner); err != nil {
		t.Fatal(err)
	}
	second := invocation
	second.InvocationID = testUUID7(5)
	if _, err := bridge.Invoke(context.Background(), second, owner); err != nil {
		t.Fatalf("invocações diretas legítimas conflitantes: %v", err)
	}
	if len(port.dispatched) != 2 {
		t.Fatalf("dispatches=%d", len(port.dispatched))
	}
}

func TestBridgePhysicalClaimIsScopedByOwnerWorkspaceOccurrenceAndGeneration(t *testing.T) {
	bridge, _, owner, invocation := newBridgeFixture(t)
	physical := invocation
	physical.InvocationID = testUUID7(6)
	physical.OccurrenceID = "8:keyboard6:Ctrl+N"
	if _, err := bridge.Invoke(context.Background(), physical, owner); err != nil {
		t.Fatal(err)
	}
	otherOwner := owner
	otherOwner.UserID = "user-b"
	otherOwner.SessionID = "session-b"
	otherOwner.WorkspaceID = "workspace-b"
	otherSession := Session{ID: otherOwner.SessionID, Generation: 1, Owner: otherOwner}
	if err := bridge.OpenSession(otherSession); err != nil {
		t.Fatal(err)
	}
	if err := bridge.ReplaceCapabilities([]Capability{
		{ID: "cap-a", CommandID: "command.a", Generation: 1, Source: SourceUIAction, Owner: owner},
		{ID: "cap-b", CommandID: "command.a", Generation: 1, Source: SourceUIAction, Owner: otherOwner},
	}); err != nil {
		t.Fatal(err)
	}
	other := physical
	other.SessionID, other.InvocationID, other.CapabilityID = otherOwner.SessionID, testUUID7(7), "cap-b"
	if _, err := bridge.Invoke(context.Background(), other, otherOwner); err != nil {
		t.Fatalf("owner/workspace distinto compartilhou claim: %v", err)
	}
}

func TestBridgeGenerationRequiresCapabilitySnapshotOfNewGeneration(t *testing.T) {
	bridge, _, owner, invocation := newBridgeFixture(t)
	if err := bridge.AdvanceGeneration(context.Background(), owner.SessionID, 2); err != nil {
		t.Fatal(err)
	}
	next := invocation
	next.InvocationID = testUUID7(8)
	next.Generation = 2
	if _, err := bridge.Invoke(context.Background(), next, owner); !errors.Is(err, ErrCapabilityDenied) {
		t.Fatalf("capability antiga aceita: %v", err)
	}
	if err := bridge.ReplaceCapabilities([]Capability{{ID: "cap-next", CommandID: "command.a", Generation: 2, Source: SourceUIAction, Owner: owner}}); err != nil {
		t.Fatal(err)
	}
	next.CapabilityID = "cap-next"
	if _, err := bridge.Invoke(context.Background(), next, owner); err != nil {
		t.Fatalf("capability nova rejeitada: %v", err)
	}
}

func TestBridgePortDispatchIsAQuickHandoffAndLogoutDoesNotWaitForExecution(t *testing.T) {
	port := &handoffPort{started: make(chan struct{}), release: make(chan struct{}), done: make(chan struct{})}
	owner := Owner{UserID: "user-a", SessionID: "session-a", WorkspaceID: "workspace-a"}
	bridge, err := New(Config{Port: port, Capabilities: []Capability{{ID: "cap-a", CommandID: "command.a", Generation: 1, Source: SourceUIAction, Owner: owner}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := bridge.OpenSession(Session{ID: owner.SessionID, Generation: 1, Owner: owner}); err != nil {
		t.Fatal(err)
	}
	invocation := Invocation{SessionID: owner.SessionID, InvocationID: testUUID7(13), CommandID: "command.a", Generation: 1, CapabilityID: "cap-a", Ownership: OwnershipLocal, Source: SourceUIAction}
	started := time.Now()
	if _, err := bridge.Invoke(context.Background(), invocation, owner); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(started); elapsed > 100*time.Millisecond {
		t.Fatalf("handoff bloqueou por %s", elapsed)
	}
	select {
	case <-port.started:
	default:
		t.Fatal("a porta não recebeu o handoff")
	}
	logoutStarted := time.Now()
	if err := bridge.Lifecycle(context.Background(), LifecycleEvent{Kind: LifecycleLogout, SessionID: owner.SessionID, Generation: 1}); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(logoutStarted); elapsed > 100*time.Millisecond {
		t.Fatalf("logout aguardou execução por %s", elapsed)
	}
	close(port.release)
	select {
	case <-port.done:
	case <-time.After(time.Second):
		t.Fatal("handoff não terminou após release externo")
	}
}

func TestBridgeWireUsesTaggedCamelCaseAndStringGeneration(t *testing.T) {
	value, err := json.Marshal(Result{
		SessionID: "session-a", InvocationID: testUUID7(1), CommandID: "command.a", Generation: 9007199254740993,
		CapabilityID: "cap-a", Ownership: OwnershipLocal, OccurrenceID: "8:keyboard6:Ctrl+N",
		Owner: Owner{UserID: "user-a", SessionID: "session-a", WorkspaceID: "workspace-a"}, Status: ResultSucceeded,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"sessionId":"session-a","invocationId":"01900000-0000-7000-8000-000000000001","commandId":"command.a","generation":"9007199254740993","capabilityId":"cap-a","ownership":"local","occurrenceId":"8:keyboard6:Ctrl+N","owner":{"userId":"user-a","sessionId":"session-a","workspaceId":"workspace-a"},"status":"succeeded"}`
	if string(value) != want {
		t.Fatalf("wire = %s\nwant = %s", value, want)
	}
	inputValue, err := json.Marshal(Input{
		SessionID: "session-a", Source: "keyboard", Key: "Ctrl+N", Generation: 9007199254740993, Kind: commandinput.KeyDown,
		Invocation: Invocation{SessionID: "session-a", InvocationID: testUUID7(2), CommandID: "command.a", Generation: 9007199254740993, CapabilityID: "cap-a", Ownership: OwnershipLocal, Source: SourceKeyboardLocal},
		Owner:      Owner{UserID: "user-a", SessionID: "session-a", WorkspaceID: "workspace-a"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(inputValue), `"kind":"down"`) || !strings.Contains(string(inputValue), `"generation":"9007199254740993"`) {
		t.Fatalf("input wire = %s", inputValue)
	}
	var decoded Input
	if err := json.Unmarshal(inputValue, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Kind != commandinput.KeyDown || decoded.Generation != 9007199254740993 {
		t.Fatalf("input decoded = %+v", decoded)
	}
	lifecycleValue, err := json.Marshal(LifecycleEvent{Kind: LifecycleBlur, SessionID: "session-a", Generation: 9007199254740993})
	if err != nil {
		t.Fatal(err)
	}
	if string(lifecycleValue) != `{"kind":"blur","sessionId":"session-a","generation":"9007199254740993"}` {
		t.Fatalf("lifecycle wire = %s", lifecycleValue)
	}
	var decodedLifecycle LifecycleEvent
	if err := json.Unmarshal(lifecycleValue, &decodedLifecycle); err != nil {
		t.Fatal(err)
	}
	if decodedLifecycle.Kind != LifecycleBlur || decodedLifecycle.Generation != 9007199254740993 {
		t.Fatalf("lifecycle decoded = %+v", decodedLifecycle)
	}
}

func TestBridgeSourceEnumAndUUID7Semantics(t *testing.T) {
	bridge, _, owner, invocation := newBridgeFixture(t)
	invalidSource := invocation
	invalidSource.InvocationID = testUUID7(20)
	invalidSource.Source = Source("untrusted.source")
	if _, err := bridge.Invoke(context.Background(), invalidSource, owner); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("source fora da enum aceita: %v", err)
	}
	invalidInvocation := invocation
	invalidInvocation.InvocationID = "not-a-uuid"
	if _, err := bridge.Invoke(context.Background(), invalidInvocation, owner); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("invocation ID não UUID7 aceito: %v", err)
	}
	event := invocation
	event.InvocationID = testUUID7(21)
	event.Source = SourceEvent
	event.EventID = testUUID7(22)
	if err := bridge.ReplaceCapabilities([]Capability{{ID: "cap-a", CommandID: "command.a", Generation: 1, Source: SourceEvent, Owner: owner}}); err != nil {
		t.Fatal(err)
	}
	if _, err := bridge.Invoke(context.Background(), event, owner); err != nil {
		t.Fatalf("evento UUID7 rejeitado: %v", err)
	}
	missingEventID := invocation
	missingEventID.InvocationID = testUUID7(23)
	missingEventID.Source = SourceEvent
	if _, err := bridge.Invoke(context.Background(), missingEventID, owner); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("evento sem EventID aceito: %v", err)
	}
	if err := bridge.ReplaceCapabilities([]Capability{{ID: "cap-a", CommandID: "command.a", Generation: 1, Source: SourceUIAction, Owner: owner}}); err != nil {
		t.Fatal(err)
	}
	opaqueOccurrence := invocation
	opaqueOccurrence.InvocationID = testUUID7(24)
	opaqueOccurrence.OccurrenceID = "deck-A/button-7/v2"
	if _, err := bridge.Invoke(context.Background(), opaqueOccurrence, owner); err != nil {
		t.Fatalf("occurrence física opaca rejeitada: %v", err)
	}
	streamDeck := invocation
	streamDeck.InvocationID = testUUID7(25)
	streamDeck.Source = SourceStreamDeck
	if string(streamDeck.Source) != "streamdeck.key" {
		t.Fatalf("origem Stream Deck = %q", streamDeck.Source)
	}
	if err := bridge.ReplaceCapabilities([]Capability{{ID: "cap-a", CommandID: "command.a", Generation: 1, Source: SourceStreamDeck, Owner: owner}}); err != nil {
		t.Fatal(err)
	}
	if _, err := bridge.Invoke(context.Background(), streamDeck, owner); err != nil {
		t.Fatalf("origem Stream Deck rejeitada: %v", err)
	}
	withSourceEvent := invocation
	withSourceEvent.InvocationID = testUUID7(27)
	withSourceEvent.Source = SourceKeyboardLocal
	withSourceEvent.SourceEventID = testUUID7(28)
	if err := bridge.ReplaceCapabilities([]Capability{{ID: "cap-a", CommandID: "command.a", Generation: 1, Source: SourceKeyboardLocal, Owner: owner}}); err != nil {
		t.Fatal(err)
	}
	if _, err := bridge.Invoke(context.Background(), withSourceEvent, owner); err != nil {
		t.Fatalf("source event UUIDv7 rejeitado: %v", err)
	}
	badSourceEvent := invocation
	badSourceEvent.InvocationID = testUUID7(29)
	badSourceEvent.SourceEventID = "not-a-uuid"
	if _, err := bridge.Invoke(context.Background(), badSourceEvent, owner); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("source event inválido aceito: %v", err)
	}
	legacyStreamDeck := invocation
	legacyStreamDeck.InvocationID = testUUID7(30)
	legacyStreamDeck.Source = Source("streamdeck")
	if _, err := bridge.Invoke(context.Background(), legacyStreamDeck, owner); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("origem legada Stream Deck aceita: %v", err)
	}
}

func TestBridgeInputRepeatReleaseBlurAndGeneration(t *testing.T) {
	bridge, port, owner, invocation := newBridgeFixture(t)
	input := Input{SessionID: owner.SessionID, Source: "keyboard", Key: "Ctrl+N", Generation: 1, Kind: commandinput.KeyDown, Invocation: Invocation{SessionID: invocation.SessionID, InvocationID: invocation.InvocationID, CommandID: invocation.CommandID, Generation: invocation.Generation, CapabilityID: invocation.CapabilityID, Ownership: invocation.Ownership, Source: invocation.Source, OccurrenceID: "8:keyboard6:Ctrl+N"}, Owner: owner}
	if ack, err := bridge.Input(context.Background(), input); err != nil || !ack.Accepted {
		t.Fatalf("down ack=%+v err=%v", ack, err)
	}
	input.Repeat = true
	if ack, err := bridge.Input(context.Background(), input); err != nil || ack.Accepted {
		t.Fatalf("repeat ack=%+v err=%v", ack, err)
	}
	input.Kind, input.Repeat = commandinput.KeyUp, false
	if ack, err := bridge.Input(context.Background(), input); err != nil || ack.Accepted {
		t.Fatalf("up ack=%+v err=%v", ack, err)
	}
	if _, err := bridge.AcceptResult(Result{SessionID: invocation.SessionID, InvocationID: invocation.InvocationID, CommandID: invocation.CommandID, Generation: 1, CapabilityID: invocation.CapabilityID, Ownership: invocation.Ownership, OccurrenceID: "8:keyboard6:Ctrl+N", Owner: owner, Status: ResultSucceeded}); err != nil {
		t.Fatal(err)
	}
	input.Kind = commandinput.KeyDown
	input.Invocation.InvocationID = testUUID7(9)
	if ack, err := bridge.Input(context.Background(), input); err != nil || !ack.Accepted {
		t.Fatalf("down após up ack=%+v err=%v", ack, err)
	}
	if _, err := bridge.AcceptResult(Result{SessionID: invocation.SessionID, InvocationID: testUUID7(9), CommandID: invocation.CommandID, Generation: 1, CapabilityID: invocation.CapabilityID, Ownership: invocation.Ownership, OccurrenceID: "8:keyboard6:Ctrl+N", Owner: owner, Status: ResultSucceeded}); err != nil {
		t.Fatal(err)
	}
	if err := bridge.Lifecycle(context.Background(), LifecycleEvent{Kind: LifecycleBlur, SessionID: owner.SessionID, Generation: 1}); err != nil {
		t.Fatal(err)
	}
	third := invocation
	third.InvocationID = testUUID7(10)
	if ack, err := bridge.Input(context.Background(), Input{SessionID: owner.SessionID, Source: "keyboard", Key: "Ctrl+N", Generation: 1, Kind: commandinput.KeyDown, Invocation: third, Owner: owner}); err != nil || !ack.Accepted {
		t.Fatalf("down após blur ack=%+v err=%v", ack, err)
	}
	if err := bridge.AdvanceGeneration(context.Background(), owner.SessionID, 2); err != nil {
		t.Fatal(err)
	}
	if len(port.cancelled) != 1 {
		t.Fatalf("cancelamentos da troca de geração=%d", len(port.cancelled))
	}
	stale := third
	stale.InvocationID = testUUID7(11)
	stale.Generation = 1
	if _, err := bridge.Invoke(context.Background(), stale, owner); !errors.Is(err, ErrStaleGeneration) {
		t.Fatalf("invocação velha err=%v", err)
	}
}

func TestBridgeLifecycleResetBlurAndReleaseClearAdapterState(t *testing.T) {
	bridge, port, owner, _ := newBridgeFixture(t)
	occurrenceID := "8:keyboard6:Ctrl+N"

	first := Input{SessionID: owner.SessionID, Source: "keyboard", Key: "Ctrl+N", Generation: 1, Kind: commandinput.KeyDown, Invocation: Invocation{SessionID: owner.SessionID, InvocationID: testUUID7(30), CommandID: "command.a", Generation: 1, CapabilityID: "cap-a", Ownership: OwnershipLocal, Source: SourceUIAction, OccurrenceID: occurrenceID}, Owner: owner}
	if ack, err := bridge.Input(context.Background(), first); err != nil || !ack.Accepted {
		t.Fatalf("down inicial ack=%+v err=%v", ack, err)
	}
	repeat := first
	repeat.Kind = commandinput.KeyDown
	repeat.Repeat = true
	if err := bridge.Lifecycle(context.Background(), LifecycleEvent{Kind: LifecycleRepeat, SessionID: owner.SessionID, Generation: 1, Input: &repeat}); err != nil {
		t.Fatalf("repeat falhou: %v", err)
	}
	if len(port.dispatched) != 1 {
		t.Fatalf("repeat disparou despacho: %d", len(port.dispatched))
	}
	release := first
	if err := bridge.Lifecycle(context.Background(), LifecycleEvent{Kind: LifecycleRelease, SessionID: owner.SessionID, Generation: 1, Input: &release}); err != nil {
		t.Fatalf("release: %v", err)
	}
	if _, err := bridge.AcceptResult(Result{SessionID: owner.SessionID, InvocationID: first.Invocation.InvocationID, CommandID: "command.a", Generation: 1, CapabilityID: "cap-a", Ownership: OwnershipLocal, OccurrenceID: occurrenceID, Owner: owner, Status: ResultSucceeded}); err != nil {
		t.Fatalf("resultado inicial: %v", err)
	}

	second := first
	second.Invocation.InvocationID = testUUID7(31)
	second.Repeat = false
	if ack, err := bridge.Input(context.Background(), second); err != nil || !ack.Accepted {
		t.Fatalf("down após release ack=%+v err=%v", ack, err)
	}
	if err := bridge.Lifecycle(context.Background(), LifecycleEvent{Kind: LifecycleBlur, SessionID: owner.SessionID, Generation: 1}); err != nil {
		t.Fatalf("blur: %v", err)
	}
	if _, err := bridge.AcceptResult(Result{SessionID: owner.SessionID, InvocationID: second.Invocation.InvocationID, CommandID: "command.a", Generation: 1, CapabilityID: "cap-a", Ownership: OwnershipLocal, OccurrenceID: occurrenceID, Owner: owner, Status: ResultSucceeded}); err != nil {
		t.Fatalf("resultado após blur: %v", err)
	}

	third := second
	third.Invocation.InvocationID = testUUID7(32)
	if ack, err := bridge.Input(context.Background(), third); err != nil || !ack.Accepted {
		t.Fatalf("down após blur ack=%+v err=%v", ack, err)
	}
	if err := bridge.Lifecycle(context.Background(), LifecycleEvent{Kind: LifecycleGeneration, SessionID: owner.SessionID, Generation: 2}); err != nil {
		t.Fatalf("reset de geração: %v", err)
	}
	if len(port.cancelled) != 1 || port.cancelled[0].InvocationID != third.Invocation.InvocationID {
		t.Fatalf("reset não cancelou a pendência: %+v", port.cancelled)
	}
	if err := bridge.ReplaceCapabilities([]Capability{{ID: "cap-next", CommandID: "command.a", Generation: 2, Source: SourceUIAction, Owner: owner}}); err != nil {
		t.Fatalf("capability da nova geração: %v", err)
	}
	fourth := third
	fourth.Invocation.InvocationID = testUUID7(33)
	fourth.Invocation.Generation = 2
	fourth.Invocation.CapabilityID = "cap-next"
	fourth.Generation = 2
	if ack, err := bridge.Input(context.Background(), fourth); err != nil || !ack.Accepted {
		t.Fatalf("down após reset ack=%+v err=%v", ack, err)
	}
}

func TestBridgeShutdownCancelsAllSessionsAndReleasesAdapterOnce(t *testing.T) {
	base := &testPort{}
	port := &shutdownPort{testPort: base}
	owner := Owner{UserID: "user-a", SessionID: "session-a", WorkspaceID: "workspace-a"}
	other := Owner{UserID: "user-b", SessionID: "session-b", WorkspaceID: "workspace-b"}
	bridge, err := New(Config{Port: port, Capabilities: []Capability{{ID: "cap-a", CommandID: "command.a", Generation: 1, Source: SourceUIAction, Owner: owner}, {ID: "cap-b", CommandID: "command.a", Generation: 1, Source: SourceUIAction, Owner: other}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := bridge.OpenSession(Session{ID: owner.SessionID, Generation: 1, Owner: owner}); err != nil {
		t.Fatal(err)
	}
	if err := bridge.OpenSession(Session{ID: other.SessionID, Generation: 1, Owner: other}); err != nil {
		t.Fatal(err)
	}
	first := Invocation{SessionID: owner.SessionID, InvocationID: testUUID7(34), CommandID: "command.a", Generation: 1, CapabilityID: "cap-a", Ownership: OwnershipLocal, Source: SourceUIAction}
	second := first
	second.SessionID, second.InvocationID, second.CapabilityID = other.SessionID, testUUID7(35), "cap-b"
	if _, err := bridge.Invoke(context.Background(), first, owner); err != nil {
		t.Fatal(err)
	}
	if _, err := bridge.Invoke(context.Background(), second, other); err != nil {
		t.Fatal(err)
	}
	if err := bridge.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(base.cancelled) != 2 {
		t.Fatalf("pendências canceladas=%d, esperado 2", len(base.cancelled))
	}
	if port.shutdownCalls != 1 {
		t.Fatalf("shutdown da porta=%d, esperado 1", port.shutdownCalls)
	}
	if err := bridge.Shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown idempotente: %v", err)
	}
	if len(base.cancelled) != 2 || port.shutdownCalls != 1 {
		t.Fatalf("shutdown repetido alterou estado: cancel=%d close=%d", len(base.cancelled), port.shutdownCalls)
	}
	if _, err := bridge.Invoke(context.Background(), first, owner); !errors.Is(err, ErrBridgeClosed) {
		t.Fatalf("invoke após shutdown err=%v", err)
	}
	if err := bridge.OpenSession(Session{ID: "session-c", Generation: 1, Owner: Owner{UserID: "user-c", SessionID: "session-c", WorkspaceID: "workspace-c"}}); !errors.Is(err, ErrBridgeClosed) {
		t.Fatalf("open após shutdown err=%v", err)
	}
}

func TestBridgeShutdownCallsPortAfterCancelError(t *testing.T) {
	cancelErr := errors.New("cancelamento não confirmado")
	shutdownErr := errors.New("adapter não fechou")
	base := &testPort{cancelErr: cancelErr}
	port := &shutdownPort{testPort: base, shutdownErr: shutdownErr}
	owner := Owner{UserID: "user-a", SessionID: "session-a", WorkspaceID: "workspace-a"}
	bridge, err := New(Config{Port: port, Capabilities: []Capability{{ID: "cap-a", CommandID: "command.a", Generation: 1, Source: SourceUIAction, Owner: owner}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := bridge.OpenSession(Session{ID: owner.SessionID, Generation: 1, Owner: owner}); err != nil {
		t.Fatal(err)
	}
	invocation := Invocation{SessionID: owner.SessionID, InvocationID: testUUID7(36), CommandID: "command.a", Generation: 1, CapabilityID: "cap-a", Ownership: OwnershipLocal, Source: SourceUIAction}
	if _, err := bridge.Invoke(context.Background(), invocation, owner); err != nil {
		t.Fatal(err)
	}
	if err := bridge.Shutdown(context.Background()); !errors.Is(err, cancelErr) {
		t.Fatalf("erro de cancelamento não preservado: %v", err)
	}
	if port.shutdownCalls != 1 {
		t.Fatalf("ShutdownPort não chamado após erro de cancelamento: %d", port.shutdownCalls)
	}
	if err := bridge.Shutdown(context.Background()); !errors.Is(err, cancelErr) {
		t.Fatalf("erro estável do shutdown = %v", err)
	}
}

func TestBridgeConcurrentShutdownWithCancelledContextDoesNotDeadlock(t *testing.T) {
	base := &testPort{cancelErr: context.Canceled}
	port := &shutdownPort{testPort: base}
	owner := Owner{UserID: "user-a", SessionID: "session-a", WorkspaceID: "workspace-a"}
	bridge, err := New(Config{Port: port, Capabilities: []Capability{{ID: "cap-a", CommandID: "command.a", Generation: 1, Source: SourceUIAction, Owner: owner}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := bridge.OpenSession(Session{ID: owner.SessionID, Generation: 1, Owner: owner}); err != nil {
		t.Fatal(err)
	}
	invocation := Invocation{SessionID: owner.SessionID, InvocationID: testUUID7(37), CommandID: "command.a", Generation: 1, CapabilityID: "cap-a", Ownership: OwnershipLocal, Source: SourceUIAction}
	if _, err := bridge.Invoke(context.Background(), invocation, owner); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	results := make(chan error, 2)
	for range 2 {
		go func() { results <- bridge.Shutdown(ctx) }()
	}
	deadline := time.NewTimer(time.Second)
	defer deadline.Stop()
	for range 2 {
		select {
		case err := <-results:
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("shutdown concorrente err=%v", err)
			}
		case <-deadline.C:
			t.Fatal("shutdown concorrente ficou bloqueado")
		}
	}
	if port.shutdownCalls != 1 {
		t.Fatalf("ShutdownPort concorrente=%d, esperado 1", port.shutdownCalls)
	}
}

func TestBridgeConcurrentShutdownContextMayLeaveFirstOwnerBlocked(t *testing.T) {
	base := &testPort{}
	port := &blockingCancelPort{testPort: base, cancelStarted: make(chan struct{}), releaseCancel: make(chan struct{})}
	owner := Owner{UserID: "user-a", SessionID: "session-a", WorkspaceID: "workspace-a"}
	bridge, err := New(Config{Port: port, Capabilities: []Capability{{ID: "cap-a", CommandID: "command.a", Generation: 1, Source: SourceUIAction, Owner: owner}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := bridge.OpenSession(Session{ID: owner.SessionID, Generation: 1, Owner: owner}); err != nil {
		t.Fatal(err)
	}
	invocation := Invocation{SessionID: owner.SessionID, InvocationID: testUUID7(38), CommandID: "command.a", Generation: 1, CapabilityID: "cap-a", Ownership: OwnershipLocal, Source: SourceUIAction}
	if _, err := bridge.Invoke(context.Background(), invocation, owner); err != nil {
		t.Fatal(err)
	}
	firstResult := make(chan error, 1)
	go func() { firstResult <- bridge.Shutdown(context.Background()) }()
	select {
	case <-port.cancelStarted:
	case <-time.After(time.Second):
		t.Fatal("cancelamento do primeiro shutdown não começou")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	started := time.Now()
	if err := bridge.Shutdown(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("shutdown concorrente com contexto cancelado = %v", err)
	}
	if elapsed := time.Since(started); elapsed > 100*time.Millisecond {
		t.Fatalf("shutdown concorrente ignorou cancelamento por %s", elapsed)
	}
	if port.shutdownCalls != 0 {
		t.Fatal("ShutdownPort foi liberado antes do cancelamento terminar")
	}

	close(port.releaseCancel)
	select {
	case err := <-firstResult:
		if err != nil {
			t.Fatalf("primeiro shutdown = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("primeiro shutdown não terminou após liberação")
	}
	if port.shutdownCalls != 1 {
		t.Fatalf("ShutdownPort=%d após cancelamento, esperado 1", port.shutdownCalls)
	}
}

func TestBridgeLockLogoutAndCancelFailClosed(t *testing.T) {
	bridge, port, owner, invocation := newBridgeFixture(t)
	if _, err := bridge.Invoke(context.Background(), invocation, owner); err != nil {
		t.Fatal(err)
	}
	port.cancelErr = errors.New("cancel não confirmado")
	if err := bridge.Lifecycle(context.Background(), LifecycleEvent{Kind: LifecycleLock, SessionID: owner.SessionID, Generation: 1}); err == nil {
		t.Fatal("lock não reportou falha da porta")
	}
	if _, err := bridge.Invoke(context.Background(), Invocation{SessionID: owner.SessionID, InvocationID: testUUID7(14), CommandID: invocation.CommandID, Generation: 1, CapabilityID: invocation.CapabilityID, Ownership: OwnershipLocal, Source: SourceUIAction}, owner); !errors.Is(err, ErrSessionUnavailable) {
		t.Fatalf("lock permitiu invocação: %v", err)
	}
	if _, err := bridge.Cancel(context.Background(), CancelRequest{SessionID: owner.SessionID, InvocationID: invocation.InvocationID, Generation: 1, CapabilityID: invocation.CapabilityID, Owner: owner}); !errors.Is(err, ErrUnknownInvocation) {
		t.Fatalf("cancel após lock err=%v", err)
	}
	if err := bridge.Lifecycle(context.Background(), LifecycleEvent{Kind: LifecycleLogout, SessionID: owner.SessionID, Generation: 1}); err != nil {
		// logout é idempotente quanto ao estado, mas a porta continua sendo
		// observável; neste caso não há pendência restante a cancelar.
		t.Fatalf("logout: %v", err)
	}
}

func TestBridgeCancelKeepsPendingWhenPortDoesNotConfirm(t *testing.T) {
	bridge, port, owner, invocation := newBridgeFixture(t)
	if _, err := bridge.Invoke(context.Background(), invocation, owner); err != nil {
		t.Fatal(err)
	}
	port.cancelErr = errors.New("indisponível")
	if _, err := bridge.Cancel(context.Background(), CancelRequest{SessionID: owner.SessionID, InvocationID: invocation.InvocationID, Generation: 1, CapabilityID: invocation.CapabilityID, Owner: owner}); err == nil {
		t.Fatal("cancelamento sem confirmação aceito")
	}
	port.cancelErr = nil
	if ack, err := bridge.Cancel(context.Background(), CancelRequest{SessionID: owner.SessionID, InvocationID: invocation.InvocationID, Generation: 1, CapabilityID: invocation.CapabilityID, Owner: owner}); err != nil || !ack.Accepted {
		t.Fatalf("segundo cancel ack=%+v err=%v", ack, err)
	}
}
