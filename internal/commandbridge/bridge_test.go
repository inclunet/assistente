package commandbridge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
}

func testUUID7(number int) string {
	return fmt.Sprintf("01900000-0000-7000-8000-%012d", number)
}

type handoffPort struct {
	started chan struct{}
	release chan struct{}
	done    chan struct{}
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
	bridge, err := New(Config{Port: port, Capabilities: []Capability{{ID: "cap-a", CommandID: "command.a", Generation: 1, Owner: owner}}})
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
		{ID: "cap-a", CommandID: "command.a", Generation: 1, Owner: owner},
		{ID: "cap-b", CommandID: "command.a", Generation: 1, Owner: otherOwner},
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
	if err := bridge.ReplaceCapabilities([]Capability{{ID: "cap-next", CommandID: "command.a", Generation: 2, Owner: owner}}); err != nil {
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
	bridge, err := New(Config{Port: port, Capabilities: []Capability{{ID: "cap-a", CommandID: "command.a", Generation: 1, Owner: owner}}})
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
	if _, err := bridge.Invoke(context.Background(), event, owner); err != nil {
		t.Fatalf("evento UUID7 rejeitado: %v", err)
	}
	missingEventID := invocation
	missingEventID.InvocationID = testUUID7(23)
	missingEventID.Source = SourceEvent
	if _, err := bridge.Invoke(context.Background(), missingEventID, owner); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("evento sem EventID aceito: %v", err)
	}
	opaqueOccurrence := invocation
	opaqueOccurrence.InvocationID = testUUID7(24)
	opaqueOccurrence.OccurrenceID = "deck-A/button-7/v2"
	if _, err := bridge.Invoke(context.Background(), opaqueOccurrence, owner); err != nil {
		t.Fatalf("occurrence física opaca rejeitada: %v", err)
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
