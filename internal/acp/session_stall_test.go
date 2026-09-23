package acp

import (
	"errors"
	"io"
	"testing"
	"time"

	sdk "github.com/coder/acp-go-sdk"
)

// novaSessaoVigiada monta sessão com watchdog curto, sem agente real: o cano
// nunca responde, então só o relógio de atividade decide.
func novaSessaoVigiada(t *testing.T) *session {
	t.Helper()
	leitura, escrita := io.Pipe()
	t.Cleanup(func() {
		_ = leitura.Close()
		_ = escrita.Close()
	})
	cn := &conn{
		handler:  denyAll{},
		kill:     func() {},
		sessions: map[string]*session{},
		dead:     make(chan struct{}),
	}
	cn.rpc = sdk.NewConnection(cn.handleInbound, escrita, leitura)
	sess := cn.registerSession("sess-stall", t.TempDir(), nil)
	sess.stallTimeout = 120 * time.Millisecond
	sess.stallPoll = 10 * time.Millisecond
	return sess
}

func TestWatchStallDisparaQuandoAgenteCala(t *testing.T) {
	sess := novaSessaoVigiada(t)
	seq := sess.startTurn()
	pare := make(chan struct{})
	t.Cleanup(func() { close(pare) })

	travou := sess.watchStall(seq, pare)
	select {
	case <-travou:
	case <-time.After(3 * time.Second):
		t.Fatal("watchdog não disparou para turno sem atividade")
	}
}

func TestWatchStallNaoDisparaComAtividade(t *testing.T) {
	sess := novaSessaoVigiada(t)
	sess.stallTimeout = 300 * time.Millisecond
	seq := sess.startTurn()
	pare := make(chan struct{})
	t.Cleanup(func() { close(pare) })

	// Agente vivo: carimba a cada 20ms, bem abaixo do prazo de 300ms.
	vivo := make(chan struct{})
	defer close(vivo)
	go func() {
		ticker := time.NewTicker(20 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-vivo:
				return
			case <-pare:
				return
			case <-ticker.C:
				sess.noteActivity()
			}
		}
	}()

	travou := sess.watchStall(seq, pare)
	select {
	case <-travou:
		t.Fatal("watchdog disparou com agente ativo")
	case <-time.After(350 * time.Millisecond):
	}
}

func TestWatchStallIgnoraTurnoAntigo(t *testing.T) {
	sess := novaSessaoVigiada(t)
	velho := sess.startTurn()
	sess.startTurn()
	pare := make(chan struct{})
	t.Cleanup(func() { close(pare) })

	// O prazo do turno velho não pode derrubar o turno novo e saudável.
	travou := sess.watchStall(velho, pare)
	select {
	case <-travou:
		t.Fatal("watchdog do turno velho disparou no turno novo")
	case <-time.After(200 * time.Millisecond):
	}
}

func TestPrazoEstouradoCarregaOrigemDoAbandono(t *testing.T) {
	// A origem (watchdog x pedido de quem chamou) viaja no PromptError para a
	// mensagem dizer a verdade sobre quem interrompeu (AEP-0108 D2).
	for _, travado := range []bool{false, true} {
		s := &session{
			id:             "sess-teste",
			turnSlot:       make(chan struct{}, 1),
			unconfirmedSig: make(chan struct{}),
			closedSig:      make(chan struct{}),
		}
		expirado := make(chan time.Time, 1)
		expirado <- time.Now()
		_, err := s.awaitCancelled(7, make(chan promptOutcome), expirado, travado)
		var falha *PromptError
		if !errors.As(err, &falha) {
			t.Fatalf("travado=%v: erro não é PromptError: %T", travado, err)
		}
		if !errors.Is(err, ErrCancelNotConfirmed) {
			t.Fatalf("travado=%v: esperava ErrCancelNotConfirmed, veio %v", travado, err)
		}
		if falha.Stalled != travado {
			t.Errorf("travado=%v: Stalled=%v no PromptError", travado, falha.Stalled)
		}
	}
}

func TestDeliverCarimbaAtividade(t *testing.T) {
	sess := novaSessaoVigiada(t)
	sess.mu.Lock()
	sess.lastActivity = time.Now().Add(-time.Hour)
	sess.mu.Unlock()

	sess.deliver(Update{})

	if ocioso := sess.idleSince(); ocioso >= time.Minute {
		t.Fatalf("deliver não carimbou: ocioso há %s", ocioso)
	}
}
