package acp

import (
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
		leitura.Close()
		escrita.Close()
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
