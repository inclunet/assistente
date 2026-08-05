package acp

import (
	"fmt"
	"testing"
)

// connDeTeste é a conexão sem processo nenhum: aqui interessa só a ordem entre
// a notificação que chega e o registro da sessão.
func connDeTeste(onCommands func(string, []Command)) *conn {
	return &conn{
		cfg:      Config{OnCommands: onCommands},
		sessions: make(map[string]*session),
		pending:  make(map[string][]Update),
	}
}

// O agente responde ao session/new e já conta, pela mesma conexão, o que a
// sessão oferece. Essa notificação pode ser lida antes de a resposta terminar de
// virar sessão registrada aqui, e descartá-la apagaria a lista que o menu da
// barra mostra — num defeito que aparece só em algumas máquinas, porque depende
// de qual goroutine chegou primeiro.
func TestOQueOAgenteContaAntesDoRegistroDaSessaoNaoSePerde(t *testing.T) {
	var anunciados []Command
	cn := connDeTeste(func(_ string, commands []Command) { anunciados = commands })

	guardou := cn.holdEarlyUpdate("sess-1", Update{
		Kind:     UpdateCommands,
		Commands: []Command{{Name: "plan", Description: "Monta um plano"}},
	})
	if !guardou {
		t.Fatal("a notificação que chegou antes do registro foi descartada")
	}

	sess := cn.registerSession("sess-1", "/projeto", nil)

	if got := nomesDeComandos(sess.Commands()); !iguais(got, []string{"plan"}) {
		t.Fatalf("comandos da sessão = %v; o que o agente contou antes do registro se perdeu", got)
	}
	if got := nomesDeComandos(anunciados); !iguais(got, []string{"plan"}) {
		t.Fatalf("comandos anunciados = %v", got)
	}
}

// Sessão já registrada não guarda nada: a entrega é direta, e um caminho a mais
// só atrasaria o que já tem dono.
func TestSessaoRegistradaNaoGuardaNotificacao(t *testing.T) {
	cn := connDeTeste(nil)
	cn.registerSession("sess-1", "/projeto", nil)

	if cn.holdEarlyUpdate("sess-1", Update{Kind: UpdateText, Text: "oi"}) {
		t.Fatal("a notificação de uma sessão registrada foi parar na fila de espera")
	}
}

// O agente que despeja atualizações de uma sessão que nunca vai ser registrada
// — a de uma conversa da qual o app já se despediu — não pode crescer memória
// sem fim.
func TestAFilaDeEsperaTemTeto(t *testing.T) {
	cn := connDeTeste(nil)

	for i := 0; i < maxPendingUpdates; i++ {
		if !cn.holdEarlyUpdate("sess-fantasma", Update{Kind: UpdateText, Text: "eco"}) {
			t.Fatalf("a fila recusou a notificação %d, antes do teto", i)
		}
	}
	if cn.holdEarlyUpdate("sess-fantasma", Update{Kind: UpdateText, Text: "eco"}) {
		t.Fatal("a fila de espera passou do teto")
	}
}

// O teto por sessão sozinho não protege nada: bastaria o agente trocar de
// identificador a cada notificação para abrir uma entrada nova toda vez, e a
// fila cresceria sem fim mesmo sem nenhuma sessão passando do teto dela.
func TestAFilaDeEsperaTemTetoDeSessoes(t *testing.T) {
	cn := connDeTeste(nil)

	for i := 0; i < maxPendingSessions; i++ {
		id := fmt.Sprintf("sess-fantasma-%d", i)
		if !cn.holdEarlyUpdate(id, Update{Kind: UpdateText, Text: "eco"}) {
			t.Fatalf("a fila recusou a sessão %d, antes do teto", i)
		}
	}
	if cn.holdEarlyUpdate("sess-fantasma-a-mais", Update{Kind: UpdateText, Text: "eco"}) {
		t.Fatal("a fila de espera aceitou uma sessão além do teto")
	}

	// A sessão que já está na fila continua sendo aceita: o teto é de quantas
	// sessões se acompanha, e recusar quem já entrou perderia justamente a
	// abertura que a fila existe para segurar.
	if !cn.holdEarlyUpdate("sess-fantasma-0", Update{Kind: UpdateText, Text: "eco"}) {
		t.Fatal("o teto de sessões recusou notificação de uma sessão que já estava na fila")
	}
}

// A conexão montada sem a fila pronta — o que acontece em teste, e aconteceria
// em qualquer construção futura que esqueça o mapa — não pode derrubar o app
// quando o agente fala cedo demais.
func TestFilaDeEsperaNaoExigeMapaJaCriado(t *testing.T) {
	cn := &conn{}

	if !cn.holdEarlyUpdate("sess-1", Update{Kind: UpdateText, Text: "oi"}) {
		t.Fatal("a notificação foi descartada numa conexão sem fila criada")
	}
	if len(cn.pending["sess-1"]) != 1 {
		t.Fatalf("a fila ficou com %d notificações", len(cn.pending["sess-1"]))
	}
}

// Despedir-se da sessão esvazia o que sobrou dela: guardar notificação de
// conversa encerrada é segurar memória por uma sessão que não volta.
func TestDespedidaLimpaAFilaDeEspera(t *testing.T) {
	cn := connDeTeste(nil)
	cn.holdEarlyUpdate("sess-1", Update{Kind: UpdateText, Text: "oi"})

	cn.removeSession("sess-1")

	cn.mu.Lock()
	defer cn.mu.Unlock()
	if len(cn.pending) != 0 {
		t.Fatalf("sobrou notificação guardada depois da despedida: %+v", cn.pending)
	}
}
