package mcp

import (
	"sync"
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

// TestGoTrackedNaoPanicaComCloseAllConcorrente reproduz o cenário do review do
// Copilot na PR #276: várias goroutines disparam trabalho rastreado (como uma
// reconexão de health check) ao mesmo tempo em que CloseAll faz o join. Sem a
// proteção da flag bgClosed, o Add(1) concorrente ao Wait dispararia o pânico
// "sync: WaitGroup misuse: Add called concurrently with Wait". Rode com -race.
func TestGoTrackedNaoPanicaComCloseAllConcorrente(t *testing.T) {
	m := newLifecycleManager()

	var spawners sync.WaitGroup
	stop := make(chan struct{})
	for i := 0; i < 32; i++ {
		spawners.Add(1)
		go func() {
			defer spawners.Done()
			for {
				select {
				case <-stop:
					return
				default:
					// Simula reconexões disparadas durante o shutdown.
					m.goTracked(func() { <-m.ctx.Done() })
				}
			}
		}()
	}

	// Deixa o tráfego concorrente começar antes de encerrar.
	time.Sleep(20 * time.Millisecond)

	done := make(chan struct{})
	go func() {
		m.CloseAll() // não deve panicar com WaitGroup misuse
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("CloseAll não retornou sob concorrência")
	}

	close(stop)
	spawners.Wait()

	// Após CloseAll, goTracked deve ser no-op (não inicia nem rastreia).
	ran := make(chan struct{})
	m.goTracked(func() { close(ran) })
	select {
	case <-ran:
		t.Fatal("goTracked iniciou goroutine após CloseAll (deveria ser no-op)")
	case <-time.After(100 * time.Millisecond):
	}
}
