package app

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"assistente/internal/commandbridge"
	"assistente/internal/commandledger"
	"assistente/internal/database"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func commandProductBridgeInvocation(p *commandProductRuntime) commandbridge.Invocation {
	return commandbridge.Invocation{
		SessionID:    p.principal.SessionID,
		InvocationID: uuid.Must(uuid.NewV7()).String(),
		CommandID:    p.capability.CommandID,
		Generation:   p.capability.Generation,
		CapabilityID: p.capability.ID,
		Ownership:    commandbridge.OwnershipLocal,
		Source:       p.capability.Source,
	}
}

func waitCommandProductWorker(t *testing.T, p *commandProductRuntime) {
	t.Helper()
	// workers inclui serviços permanentes (como expiração de camadas).
	// pending só é removido após a execução e AcceptResult da invocação.
	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		p.mu.Lock()
		pending := len(p.pending)
		p.mu.Unlock()
		if pending == 0 {
			return
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf("invocações do produto não encerraram: %d pendentes", pending)
		}
	}
}

func TestCommandProductBridgeDispatchesRealOperationToLedger(t *testing.T) {
	a := readyCommandProduct(t)
	p := a.commandProduct.Load()
	if p == nil || p.bridge == nil {
		t.Fatal("produto/ponte não montados")
	}
	owner := p.owner()
	invocation := commandProductBridgeInvocation(p)

	ack, err := a.CommandBridgeInvoke(invocation, owner)
	if err != nil || !ack.Accepted || ack.InvocationID != invocation.InvocationID {
		t.Fatalf("dispatch real: ack=%+v err=%v", ack, err)
	}
	waitCommandProductWorker(t, p)

	record, err := a.GetPaletteInvocation(invocation.InvocationID)
	if err != nil || record.Status != string(commandledger.Succeeded) {
		t.Fatalf("ledger após dispatch: %+v err=%v", record, err)
	}
}

func TestCommandProductBridgeDispatchPreservesOccurrenceIdentityOnCompletion(t *testing.T) {
	a := readyCommandProduct(t)
	p := a.commandProduct.Load()
	invocation := commandProductBridgeInvocation(p)
	invocation.OccurrenceID = "palette/workspace.list/edge-1"
	invocation.SourceEventID = uuid.Must(uuid.NewV7()).String()

	if ack, err := a.CommandBridgeInvoke(invocation, p.owner()); err != nil || !ack.Accepted {
		t.Fatalf("dispatch com identidade de ocorrência: ack=%+v err=%v", ack, err)
	}
	waitCommandProductWorker(t, p)
	if result, err := a.GetPaletteInvocation(invocation.InvocationID); err != nil || result.Status != "succeeded" {
		t.Fatalf("operação não concluiu: result=%+v err=%v", result, err)
	}
	// Lookup sozinho não comprova que AcceptResult liberou a pendência.
	// Uma nova invocação da mesma ocorrência deve conseguir adquirir o claim.
	invocation.InvocationID = uuid.Must(uuid.NewV7()).String()
	if ack, err := a.CommandBridgeInvoke(invocation, p.owner()); err != nil || !ack.Accepted {
		t.Fatalf("claim permaneceu preso após conclusão: ack=%+v err=%v", ack, err)
	}
	waitCommandProductWorker(t, p)
}

func TestCommandProductBridgeRejectsPhysicalAndSpoofedIngress(t *testing.T) {
	a := readyCommandProduct(t)
	p := a.commandProduct.Load()
	if p == nil || p.bridge == nil {
		t.Fatal("produto/ponte não montados")
	}
	owner := p.owner()

	physical := commandProductBridgeInvocation(p)
	physical.Source = commandbridge.SourceKeyboardLocal
	if _, err := a.CommandBridgeInvoke(physical, owner); !errors.Is(err, commandbridge.ErrCapabilityDenied) {
		t.Fatalf("ingresso físico aceito: %v", err)
	}

	spoofedOwner := owner
	spoofedOwner.UserID = "01900000-0000-7000-8000-000000000099"
	spoofed := commandProductBridgeInvocation(p)
	if _, err := a.CommandBridgeInvoke(spoofed, spoofedOwner); !errors.Is(err, commandbridge.ErrSessionUnavailable) {
		t.Fatalf("owner falsificado não foi recusado na fachada: %v", err)
	}
}

func TestCommandProductBridgeShutdownJoinsWorkersAndClosesAdmission(t *testing.T) {
	a := readyCommandProduct(t)
	p := a.commandProduct.Load()
	if p == nil || p.bridge == nil {
		t.Fatal("produto/ponte não montados")
	}
	owner := p.owner()
	entered := make(chan struct{})
	release := make(chan struct{})
	var enteredOnce, releaseOnce sync.Once
	hook := "test:command_product_bridge_hold_queue"
	if err := database.DB().Callback().Update().Before("gorm:update").Register(hook, func(tx *gorm.DB) {
		values, ok := tx.Statement.Dest.(map[string]any)
		status, statusOK := values["status"].(commandledger.Status)
		if !ok || !statusOK || tx.Statement.Table != "command_idempotency_keys" || status != commandledger.Queued {
			return
		}
		enteredOnce.Do(func() { close(entered) })
		<-release
	}); err != nil {
		t.Fatal(err)
	}
	defer func() {
		releaseOnce.Do(func() { close(release) })
		_ = database.DB().Callback().Update().Remove(hook)
	}()

	invocation := commandProductBridgeInvocation(p)
	if _, err := a.CommandBridgeInvoke(invocation, owner); err != nil {
		t.Fatalf("dispatch para worker real: %v", err)
	}
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("worker real não entrou no callback de persistência")
	}

	shortCtx, shortCancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	shortErr := p.Shutdown(shortCtx)
	shortCancel()
	if !errors.Is(shortErr, context.DeadlineExceeded) {
		t.Fatalf("shutdown curto não aguardou worker: %v", shortErr)
	}
	select {
	case <-p.done:
		t.Fatal("shutdown curto fechou done antes do worker liberar")
	default:
	}

	releaseOnce.Do(func() { close(release) })
	if err := p.Shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown final após liberar worker: %v", err)
	}
	select {
	case <-p.done:
	default:
		t.Fatal("shutdown final retornou sem join do worker")
	}

	if err := p.bridge.Shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown final da ponte: %v", err)
	}
	if _, err := p.bridge.Invoke(context.Background(), commandProductBridgeInvocation(p), owner); !errors.Is(err, commandbridge.ErrBridgeClosed) {
		t.Fatalf("ponte admitiu invocação após shutdown: %v", err)
	}
}
