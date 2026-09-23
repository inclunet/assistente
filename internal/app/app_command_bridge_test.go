package app

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"assistente/internal/commandbridge"
	"assistente/internal/commandinput"
)

type appCommandBridgePort struct {
	mu            sync.Mutex
	dispatched    []commandbridge.Invocation
	cancelled     []commandbridge.CancelRequest
	shutdownCalls int
}

func (p *appCommandBridgePort) Dispatch(_ context.Context, invocation commandbridge.Invocation) (commandbridge.InvocationAck, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.dispatched = append(p.dispatched, invocation)
	return commandbridge.InvocationAck{InvocationID: invocation.InvocationID, Accepted: true}, nil
}

func (p *appCommandBridgePort) Cancel(_ context.Context, request commandbridge.CancelRequest) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cancelled = append(p.cancelled, request)
	return nil
}

func (p *appCommandBridgePort) Shutdown(context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.shutdownCalls++
	return nil
}

func appCommandBridgeUUID(number int) string {
	return fmt.Sprintf("01900000-0000-7000-8000-%012d", number)
}

func appCommandBridgeFixture(t *testing.T) (*App, *appCommandBridgePort, commandbridge.Owner, commandbridge.Invocation) {
	t.Helper()
	owner := commandbridge.Owner{UserID: "user-a", SessionID: "session-a", WorkspaceID: "workspace-a"}
	port := &appCommandBridgePort{}
	bridge, err := commandbridge.New(commandbridge.Config{
		Port:         port,
		Capabilities: []commandbridge.Capability{{ID: "cap-a", CommandID: "command.a", Generation: 1, Source: commandbridge.SourceUIAction, Owner: owner}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := bridge.OpenSession(commandbridge.Session{ID: owner.SessionID, Generation: 1, Owner: owner}); err != nil {
		t.Fatal(err)
	}
	app := &App{ctx: context.Background()}
	app.authMu.Lock()
	app.currentUserID = owner.UserID
	app.currentAuthUser = &AuthUser{UserID: owner.UserID, SessionID: owner.SessionID}
	app.authMu.Unlock()
	if err := ConfigureCommandBridge(app, bridge); err != nil {
		t.Fatal(err)
	}
	invocation := commandbridge.Invocation{SessionID: owner.SessionID, InvocationID: appCommandBridgeUUID(1), CommandID: "command.a", Generation: 1, CapabilityID: "cap-a", Ownership: commandbridge.OwnershipLocal, Source: commandbridge.SourceUIAction}
	return app, port, owner, invocation
}

func TestAppCommandBridgeRequiresMountedBridgeAndCurrentAuth(t *testing.T) {
	app := &App{}
	owner := commandbridge.Owner{UserID: "user-a", SessionID: "session-a", WorkspaceID: "workspace-a"}
	invocation := commandbridge.Invocation{SessionID: owner.SessionID, InvocationID: appCommandBridgeUUID(10), CommandID: "command.a", Generation: 1, CapabilityID: "cap-a", Ownership: commandbridge.OwnershipLocal, Source: commandbridge.SourceUIAction}
	if _, err := app.CommandBridgeInvoke(invocation, owner); !errors.Is(err, commandbridge.ErrInvalidConfiguration) {
		t.Fatalf("invoke sem ponte=%v", err)
	}

	app, port, owner, invocation := appCommandBridgeFixture(t)
	wrongUser := owner
	wrongUser.UserID = "user-b"
	if _, err := app.CommandBridgeInvoke(invocation, wrongUser); !errors.Is(err, commandbridge.ErrSessionUnavailable) {
		t.Fatalf("owner da UI trocou usuário: %v", err)
	}
	wrongSession := owner
	wrongSession.SessionID = "session-b"
	if _, err := app.CommandBridgeInvoke(invocation, wrongSession); !errors.Is(err, commandbridge.ErrSessionUnavailable) {
		t.Fatalf("owner da UI trocou sessão: %v", err)
	}
	if len(port.dispatched) != 0 {
		t.Fatalf("payload não autenticado chegou à porta: %d", len(port.dispatched))
	}
}

func TestAppCommandBridgeIngressRejectsForgedPhysicalSources(t *testing.T) {
	app, port, owner, invocation := appCommandBridgeFixture(t)
	physical := invocation
	physical.InvocationID = appCommandBridgeUUID(11)
	physical.Source = commandbridge.SourceKeyboardGlobal
	if _, err := app.CommandBridgeInvoke(physical, owner); !errors.Is(err, commandbridge.ErrCapabilityDenied) {
		t.Fatalf("origem física forjada aceita: %v", err)
	}
	input := commandbridge.Input{
		SessionID: owner.SessionID, Source: "keyboard", Key: "Ctrl+N", Generation: 1,
		Kind: commandinput.KeyDown, Invocation: physical, Owner: owner,
	}
	if _, err := app.CommandBridgeInput(input); !errors.Is(err, commandbridge.ErrCapabilityDenied) {
		t.Fatalf("input físico forjado aceito: %v", err)
	}
	if len(port.dispatched) != 0 {
		t.Fatalf("origem física forjada chegou à porta: %d", len(port.dispatched))
	}
}

func TestAppCommandBridgeLifecycleIngressRequiresCurrentAuthForEveryEvent(t *testing.T) {
	app, port, owner, invocation := appCommandBridgeFixture(t)
	otherSession := owner
	otherSession.SessionID = "session-b"
	otherSession.UserID = "user-b"
	events := []commandbridge.LifecycleEvent{
		{Kind: commandbridge.LifecycleGeneration, SessionID: otherSession.SessionID, Generation: 2},
		{Kind: commandbridge.LifecycleBlur, SessionID: otherSession.SessionID, Generation: 1},
		{Kind: commandbridge.LifecycleRepeat, SessionID: otherSession.SessionID, Generation: 1},
		{Kind: commandbridge.LifecycleRelease, SessionID: otherSession.SessionID, Generation: 1},
		{Kind: commandbridge.LifecycleLock, SessionID: otherSession.SessionID, Generation: 1},
		{Kind: commandbridge.LifecycleLogout, SessionID: otherSession.SessionID, Generation: 1},
	}
	for _, event := range events {
		if err := app.CommandBridgeLifecycle(event); !errors.Is(err, commandbridge.ErrSessionUnavailable) {
			t.Fatalf("evento %q aceitou sessão não autenticada: %v", event.Kind, err)
		}
	}
	physical := invocation
	physical.InvocationID = appCommandBridgeUUID(12)
	physical.Source = commandbridge.SourceKeyboardGlobal
	if err := app.CommandBridgeLifecycle(commandbridge.LifecycleEvent{
		Kind: commandbridge.LifecycleRepeat, SessionID: owner.SessionID, Generation: 1,
		Input: &commandbridge.Input{SessionID: "forged-session", Source: "keyboard", Key: "Ctrl+N", Generation: 1, Invocation: physical, Owner: owner},
	}); !errors.Is(err, commandbridge.ErrCapabilityDenied) {
		t.Fatalf("repeat físico nested aceito: %v", err)
	}
	if len(port.dispatched) != 0 {
		t.Fatalf("repeat físico nested alterou o handoff: %d", len(port.dispatched))
	}
}

func TestAppCommandBridgeDispatchResultCancelAndLifecycleUseMountedBridge(t *testing.T) {
	app, port, owner, invocation := appCommandBridgeFixture(t)
	ack, err := app.CommandBridgeInvoke(invocation, owner)
	if err != nil || !ack.Accepted || ack.InvocationID != invocation.InvocationID {
		t.Fatalf("ack=%+v err=%v", ack, err)
	}
	if _, err := app.CommandBridgeAcceptResult(commandbridge.Result{SessionID: invocation.SessionID, InvocationID: invocation.InvocationID, CommandID: invocation.CommandID, Generation: invocation.Generation, CapabilityID: invocation.CapabilityID, Ownership: invocation.Ownership, Owner: owner, Status: commandbridge.ResultSucceeded}); err != nil {
		t.Fatalf("resultado válido falhou: %v", err)
	}
	second := invocation
	second.InvocationID = appCommandBridgeUUID(2)
	second.OccurrenceID = "8:keyboard6:Ctrl+N"
	if _, err := app.CommandBridgeInput(commandbridge.Input{SessionID: owner.SessionID, Source: "keyboard", Key: "Ctrl+N", Generation: 1, Kind: commandinput.KeyDown, Invocation: second, Owner: owner}); err != nil {
		t.Fatalf("input válido falhou: %v", err)
	}
	if _, err := app.CommandBridgeCancel(commandbridge.CancelRequest{SessionID: owner.SessionID, InvocationID: second.InvocationID, Generation: 1, CapabilityID: second.CapabilityID, Owner: owner}); err != nil {
		t.Fatalf("cancel válido falhou: %v", err)
	}
	if err := app.CommandBridgeLifecycle(commandbridge.LifecycleEvent{Kind: commandbridge.LifecycleLock, SessionID: owner.SessionID, Generation: 1}); err != nil {
		t.Fatalf("lock válido falhou: %v", err)
	}
	if len(port.dispatched) != 2 || len(port.cancelled) != 1 {
		t.Fatalf("dispatch/cancel inesperados: dispatch=%d cancel=%d", len(port.dispatched), len(port.cancelled))
	}
}

func TestAppCommandBridgeShutdownIsTerminalAndClosesAdmission(t *testing.T) {
	app, port, _, _ := appCommandBridgeFixture(t)
	if err := app.shutdownCommandBridgeIfConfigured(context.Background()); err != nil {
		t.Fatal(err)
	}
	if port.shutdownCalls != 1 {
		t.Fatalf("shutdown calls=%d", port.shutdownCalls)
	}
	if bridge, ok := loadCommandBridge(app); ok || bridge != nil {
		t.Fatal("ponte permaneceu montada")
	}
	bridge, err := commandbridge.New(commandbridge.Config{
		Port:         port,
		Capabilities: []commandbridge.Capability{{ID: "cap-a", CommandID: "command.a", Generation: 1, Source: commandbridge.SourceUIAction, Owner: commandbridge.Owner{UserID: "user-a", SessionID: "session-a", WorkspaceID: "workspace-a"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := ConfigureCommandBridge(app, bridge); !errors.Is(err, commandbridge.ErrBridgeClosed) {
		t.Fatalf("montagem após shutdown=%v", err)
	}
}
