package app

import (
	"context"
	"testing"
	"time"
)

func TestCommandCatalogRecoveryCoalescedWakeKeepsLatestTicket(t *testing.T) {
	latest := &commandCatalogRecoveryTicket{workspaceID: "latest"}
	a := &App{commandCatalogRecoveryWake: make(chan struct{}, 1), commandCatalogRecoveryPendingTicket: latest}
	// O worker consumiu um wake no select; antes de adquirir o mutex, outro
	// evento substituiu o ticket e enfileirou um segundo wake.
	a.commandCatalogRecoveryWake <- struct{}{}
	if got := a.takeCommandCatalogRecoveryTicket(); got != latest {
		t.Fatal("wake coalescido apagou o ticket mais recente")
	}
	if a.commandCatalogRecoveryPendingTicket != nil || len(a.commandCatalogRecoveryWake) != 0 {
		t.Fatal("evento já incorporado ficou pendente")
	}
}

func TestCommandCatalogRecoveryIdleConsumesPendingEventBeforeClosing(t *testing.T) {
	latest := &commandCatalogRecoveryTicket{workspaceID: "latest"}
	done := make(chan struct{})
	a := &App{commandCatalogRecoveryRunning: true, commandCatalogRecoveryDone: done,
		commandCatalogRecoveryWake: make(chan struct{}, 1), commandCatalogRecoveryPendingTicket: latest}
	a.commandCatalogRecoveryWake <- struct{}{}
	finished, ticket, event := a.finishCommandCatalogRecoveryIfIdle(context.Background(), a.commandCatalogRecoveryWake, done)
	if finished || !event || ticket != latest || !a.commandCatalogRecoveryRunning {
		t.Fatal("worker encerrou apesar de haver evento novo")
	}
	finished, _, event = a.finishCommandCatalogRecoveryIfIdle(context.Background(), a.commandCatalogRecoveryWake, done)
	if !finished || event || a.commandCatalogRecoveryRunning {
		t.Fatal("worker sem trabalho não encerrou")
	}
	select {
	case <-done:
	default:
		t.Fatal("worker ocioso não sinalizou conclusão")
	}
}

func TestCommandCatalogRecoveryRejectsEventsAfterShutdown(t *testing.T) {
	a := commandJobPublicationApp(t)
	a.stopCommandCatalogRecovery(context.Background())
	a.requestCommandCatalogRecovery()
	a.commandCatalogRecoveryMu.Lock()
	defer a.commandCatalogRecoveryMu.Unlock()
	if a.commandCatalogRecoveryRunning || a.commandCatalogRecoveryDone != nil {
		t.Fatal("evento tardio iniciou worker após shutdown")
	}
}

func TestCommandCatalogRecoverySerializesSessionCaptureAndEnqueue(t *testing.T) {
	a := commandJobPublicationApp(t)
	a.commandCatalogRecoveryRunning = true // fila controlada, sem worker real
	a.commandCatalogRecoveryWake = make(chan struct{}, 1)
	a.commandCatalogRecoveryPendingTicket = a.captureCommandCatalogRecoveryTicket()
	a.authMu.Lock()
	requested := make(chan struct{})
	go func() {
		a.requestCommandCatalogRecovery()
		close(requested)
	}()
	// Bloquear a leitura de auth deve manter o mutex da fila ocupado. Assim,
	// nenhuma outra requisição pode publicar uma captura e ser sobrescrita
	// depois por esta chamada pausada.
	deadline := time.Now().Add(5 * time.Second)
	serialized := false
	for time.Now().Before(deadline) {
		if !a.commandCatalogRecoveryMu.TryLock() {
			serialized = true
			break
		}
		a.commandCatalogRecoveryMu.Unlock()
		time.Sleep(time.Millisecond)
	}
	a.currentAuthUser = nil
	a.currentUserID = ""
	a.authMu.Unlock()
	select {
	case <-requested:
	case <-time.After(5 * time.Second):
		t.Fatal("requisição não terminou após liberar auth")
	}
	if !serialized {
		t.Fatal("captura de sessão ocorreu fora da serialização da fila")
	}
	if a.commandCatalogRecoveryPendingTicket != nil || len(a.commandCatalogRecoveryWake) != 1 {
		t.Fatal("logout não substituiu o ticket anterior com evento sem autoridade")
	}
}

func TestCommandCatalogRecoveryBackoffIsBounded(t *testing.T) {
	for i, want := range []int64{1, 2, 5, 10, 30, 30, 30} {
		if got := int64(commandCatalogRecoveryBackoff(i).Seconds()); got != want {
			t.Fatalf("recuo %d: got=%d want=%d", i, got, want)
		}
	}
}
