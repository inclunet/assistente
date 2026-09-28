package app

import (
	"context"
	"sync"
	"testing"
	"time"

	"assistente/internal/commandruntime"
)

// Pausa o primeiro uso do contexto de reload: na versão corrigida, antes de
// adquirir bootstrap; na versão antiga, só após já retirar a publicação.
type workspaceReloadProbeContext struct {
	context.Context
	once    sync.Once
	entered chan struct{}
	resume  chan struct{}
}

func (c *workspaceReloadProbeContext) observe() {
	c.once.Do(func() { close(c.entered); <-c.resume })
}
func (c *workspaceReloadProbeContext) Done() <-chan struct{} { c.observe(); return c.Context.Done() }
func (c *workspaceReloadProbeContext) Err() error            { c.observe(); return c.Context.Err() }

func TestCommandWorkspaceReloadPreservesReadinessWhileBootstrapOwned(t *testing.T) {
	a := readyCommandProduct(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	probe := &workspaceReloadProbeContext{Context: ctx, entered: make(chan struct{}), resume: make(chan struct{})}
	a.ctx = probe
	// A troca não pode depender da barreira exterior, que pode estar drenando
	// a própria execução responsável por esta troca.
	if err := a.lockCommandStartup(ctx); err != nil {
		t.Fatal(err)
	}
	defer a.unlockCommandStartup()
	if err := a.lockCommandBootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	held := true
	var once sync.Once
	unblock := func() { once.Do(func() { close(probe.resume) }) }
	done := make(chan struct{})
	go func() { defer close(done); a.reloadCommandsAfterWorkspaceSwitch() }()
	t.Cleanup(func() {
		cancel()
		unblock()
		if held {
			a.unlockCommandBootstrap()
		}
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("reload não terminou após liberação/cancelamento")
		}
	})
	select {
	case <-probe.entered:
	case <-ctx.Done():
		t.Fatal("reload não chegou à barreira")
	}
	snapshot, err := CommandLifecycleSnapshot(a)
	if err != nil || snapshot.State != commandruntime.StateReady || !snapshot.Published {
		t.Fatalf("reload retirou readiness durante região crítica de Start: %+v err=%v", snapshot, err)
	}
	unblock()
	a.unlockCommandBootstrap()
	held = false
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("reload não concluiu após liberar bootstrap")
	}
	if _, err := a.GetLocalCommandKeyboardMap(); err != nil {
		t.Fatalf("mapa após reload: %v", err)
	}
}
