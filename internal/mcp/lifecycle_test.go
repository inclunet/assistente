package mcp

import (
	"testing"
	"time"

	"assistente/internal/credentials"
	"assistente/internal/tools"
)

func newLifecycleManager() *Manager {
	registry := tools.NewRegistry()
	credMgr := credentials.NewManager(nil)
	return NewManager(registry, credMgr, func(string, any) {})
}

// TestCloseAllFazJoinDasGoroutines garante que CloseAll cancela o ctx base e
// aguarda o término das goroutines rastreadas em bgWG, sem deixar órfãs
// (Issue #253).
func TestCloseAllFazJoinDasGoroutines(t *testing.T) {
	m := newLifecycleManager()

	started := make(chan struct{})
	finished := make(chan struct{})
	m.goTracked(func() {
		close(started)
		<-m.ctx.Done() // respeita o cancelamento do Manager
		close(finished)
	})

	<-started

	done := make(chan struct{})
	go func() {
		m.CloseAll()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("CloseAll não retornou após o cancelamento")
	}

	select {
	case <-finished:
	default:
		t.Fatal("goroutine rastreada não encerrou antes de CloseAll retornar")
	}
}

// TestReconnectWithRetrySaiCedoComCtxCancelado garante que reconnectWithRetry
// não inicia o loop de reconexão quando o Manager já foi encerrado.
func TestReconnectWithRetrySaiCedoComCtxCancelado(t *testing.T) {
	m := newLifecycleManager()
	m.cancel() // simula Manager já encerrado

	done := make(chan struct{})
	go func() {
		m.reconnectWithRetry("inexistente")
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("reconnectWithRetry não saiu cedo com o contexto cancelado")
	}
}
