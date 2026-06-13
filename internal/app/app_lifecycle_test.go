package app

import (
	"context"
	"testing"
	"time"
)

// TestShutdownCancelaContextoEFazJoin garante que o Shutdown cancela o contexto
// raiz e aguarda o join das goroutines de background (bgWG), sem deixar
// goroutine órfã nem entrar em pânico (Issue #253).
func TestShutdownCancelaContextoEFazJoin(t *testing.T) {
	a := NewApp()
	a.ctx, a.cancel = context.WithCancel(context.Background())

	started := make(chan struct{})
	finished := make(chan struct{})
	a.bgWG.Add(1)
	go func() {
		defer a.bgWG.Done()
		close(started)
		<-a.ctx.Done() // respeita o cancelamento do contexto raiz
		close(finished)
	}()

	<-started

	done := make(chan struct{})
	go func() {
		a.Shutdown()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Shutdown não retornou após o cancelamento do contexto")
	}

	select {
	case <-finished:
	default:
		t.Fatal("goroutine de background não encerrou antes de Shutdown retornar")
	}

	if a.ctx.Err() == nil {
		t.Fatal("contexto raiz deveria estar cancelado após Shutdown")
	}
}

// TestShutdownSemCancelNaoEntraEmPanico garante que Shutdown é seguro mesmo
// quando o app não passou pelo startup (cancel nil).
func TestShutdownSemCancelNaoEntraEmPanico(t *testing.T) {
	a := NewApp()
	a.Shutdown() // não deve entrar em pânico com cancel/managers nil
}
