package app

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"assistente/internal/commandexecution"
	"assistente/internal/database"
	"assistente/internal/workspace"
	"gorm.io/gorm"
)

func TestCommandJobProjectionHeartbeatPreservesCommandsAndFastPath(t *testing.T) {
	a := commandJobPublicationApp(t)
	control := startCommandMaintenanceLiveJob(t, a)
	claim, _ := waitLiveClaimAndLease(t, <-control.RunID)
	// Suspende apenas a cadência para tornar o teste determinístico; o job e
	// sua prova continuam vivos. Heartbeat abaixo usa o Consumer verdadeiro.
	if err := a.jobMgr.CloseCommandMaintenance(context.Background()); err != nil {
		t.Fatal(err)
	}
	p := a.commandProduct.Load()
	ctx := context.Background()
	if err := p.refreshCommandJobProjection(ctx); err != nil {
		t.Fatal(err)
	}
	before, err := p.host.Snapshot(ctx, p.principal)
	if err != nil {
		t.Fatal(err)
	}
	epoch, err := a.commandEpochs.Capture(ctx, p.principal.UserID, p.principal.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	watch, release, err := a.commandEpochs.WatchEpoch(ctx, epoch)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if err := a.commandMaintenance.Load().consumer.RenewRuntime(ctx, claim.ActivationID); err != nil {
		t.Fatal(err)
	}
	if _, err := p.host.Snapshot(ctx, p.principal); !errors.Is(err, commandexecution.ErrStale) {
		t.Fatalf("revisão antiga continuou válida: %v", err)
	}
	if err := p.refreshCommandJobProjection(ctx); err != nil {
		t.Fatal(err)
	}
	after, err := p.host.Snapshot(ctx, p.principal)
	if err != nil || after != before || watch.Err() != nil {
		t.Fatalf("heartbeat mudou mapa/gerações ou cancelou comando: before=%+v after=%+v err=%v watch=%v", before, after, err, watch.Err())
	}
	type queryMarker struct{}
	fastCtx := context.WithValue(ctx, queryMarker{}, true)
	var queries atomic.Int64
	db := database.DB()
	const callbackName = "test:command_projection_fast_path"
	count := func(tx *gorm.DB) {
		if tx.Statement.Context.Value(queryMarker{}) != nil {
			queries.Add(1)
		}
	}
	if err := db.Callback().Query().Before("gorm:query").Register(callbackName, count); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Callback().Query().Remove(callbackName) })
	if err := db.Callback().Raw().Before("gorm:raw").Register(callbackName, count); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Callback().Raw().Remove(callbackName) })
	for i := 0; i < 25; i++ {
		if err := p.refreshCommandJobProjection(fastCtx); err != nil {
			t.Fatal(err)
		}
		if _, _, _, err := p.host.ResolutionSnapshot(fastCtx, p.principal); err != nil {
			t.Fatal(err)
		}
	}
	if queries.Load() != 0 {
		t.Fatalf("caminho estável consultou SQLite %d vezes", queries.Load())
	}
	// A versão do Manager capturado não basta: depois da substituição, ele
	// já não é a fonte do App, mesmo que seu snapshot antigo continue válido.
	a.authMu.Lock()
	previous := a.workspaceMgr
	a.workspaceMgr = &workspace.Manager{}
	a.authMu.Unlock()
	_, staleErr := p.host.Snapshot(ctx, p.principal)
	a.authMu.Lock()
	a.workspaceMgr = previous
	a.authMu.Unlock()
	if !errors.Is(staleErr, commandexecution.ErrStale) {
		t.Fatalf("projeção aceitou Manager substituído: %v", staleErr)
	}
}

func TestCommandJobProjectionExpiresWithoutMaintenanceNotification(t *testing.T) {
	a := commandJobPublicationApp(t)
	control := startCommandMaintenanceLiveJob(t, a)
	claim, _ := waitLiveClaimAndLease(t, <-control.RunID)
	if err := a.jobMgr.CloseCommandMaintenance(context.Background()); err != nil {
		t.Fatal(err)
	}
	p := a.commandProduct.Load()
	ctx := context.Background()
	if err := p.refreshCommandJobProjection(ctx); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		_, err := p.host.Snapshot(ctx, p.principal)
		if errors.Is(err, commandexecution.ErrStale) {
			break
		}
		if err != nil || time.Now().After(deadline) {
			t.Fatalf("lease vencida não invalidou o mapa: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := p.refreshCommandJobProjection(ctx); err != nil {
		t.Fatal(err)
	}
	_, layers, _, err := p.host.ResolutionSnapshot(ctx, p.principal)
	if err != nil {
		t.Fatal(err)
	}
	for _, layer := range layers {
		if layer == claim.LayerRef {
			t.Fatal("claim auditável sem lease válida conservou camada efetiva")
		}
	}
}
