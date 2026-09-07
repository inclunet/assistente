package questionnaire

import (
	"context"
	"errors"
	"testing"
	"time"
)

// canalFalso é uma superfície de canal que registra o que lhe pediram.
type canalFalso struct {
	perguntas  []RequestPayload
	superficie Surface
	resposta   Response
	erro       error
}

func (c *canalFalso) AskOnChannel(_ context.Context, surface Surface, payload RequestPayload) (Response, error) {
	c.perguntas = append(c.perguntas, payload)
	c.superficie = surface
	return c.resposta, c.erro
}

func perguntaDeSimOuNao() RequestPayload {
	return RequestPayload{
		Questions: []Question{{ID: "decision", Type: "boolean", Prompt: Plain("Pode?")}},
	}
}

func TestConversaDeCanalPerguntaNoCanal(t *testing.T) {
	canal := &canalFalso{resposta: Response{Answers: map[string]any{"decision": true}}}
	tela := NewManager(func(string, any) {})
	router := NewRouter(
		func() *Manager { return tela },
		func() ChannelAsker { return canal },
	)

	superficie := ChannelSurface("conversa-1", "telegram", "contato-1")
	resp, err := router.Ask(context.Background(), superficie, perguntaDeSimOuNao())
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if resp.Answers["decision"] != true {
		t.Errorf("resposta = %v, quer a que veio do canal", resp.Answers)
	}
	if len(canal.perguntas) != 1 {
		t.Fatalf("perguntas no canal = %d, quer 1", len(canal.perguntas))
	}
	if canal.superficie.ContactID != "contato-1" {
		t.Errorf("contato = %q, quer o dono da conversa", canal.superficie.ContactID)
	}
}

func TestConversaDeTelaContinuaPerguntandoNaTela(t *testing.T) {
	// A superfície de desktop não pode passar pelo canal: o diálogo acessível
	// da tela é o caminho de sempre, e desviá-lo seria regressão.
	canal := &canalFalso{}
	perguntou := make(chan string, 1)
	tela := NewManager(func(_ string, data any) {
		payload, ok := data.(map[string]any)
		if !ok {
			return
		}
		id, _ := payload["id"].(string)
		perguntou <- id
	})
	router := NewRouter(func() *Manager { return tela }, func() ChannelAsker { return canal })

	go func() {
		id := <-perguntou
		_ = tela.Respond(id, map[string]any{"decision": true}, false)
	}()

	resp, err := router.Ask(context.Background(), DesktopSurface("conversa-1"), perguntaDeSimOuNao())
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if resp.Answers["decision"] != true {
		t.Errorf("resposta = %v, quer a da tela", resp.Answers)
	}
	if len(canal.perguntas) != 0 {
		t.Error("a pergunta da tela vazou para o canal")
	}
}

func TestSemSuperficieNinguemEhPerguntado(t *testing.T) {
	canal := &canalFalso{}
	tela := NewManager(func(string, any) {})
	router := NewRouter(func() *Manager { return tela }, func() ChannelAsker { return canal })

	// Prazo curtíssimo: se a pergunta fosse feita a alguém, o teste esperaria
	// por ele em vez de voltar na hora.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	_, err := router.Ask(ctx, NoSurface("conversa-1"), perguntaDeSimOuNao())
	if !errors.Is(err, ErrNoInterlocutor) {
		t.Errorf("erro = %v, quer %v", err, ErrNoInterlocutor)
	}
	if len(canal.perguntas) != 0 {
		t.Error("perguntou a um canal que não era a origem da conversa")
	}
}

func TestSuperficieSemMecanismoDePerguntaNaoPendura(t *testing.T) {
	casos := map[string]struct {
		router     *Router
		superficie Surface
	}{
		"canal sem mecanismo ligado": {
			router:     NewRouter(func() *Manager { return NewManager(nil) }, nil),
			superficie: ChannelSurface("conversa-1", "telegram", "contato-1"),
		},
		"tela sem questionário": {
			router:     NewRouter(nil, nil),
			superficie: DesktopSurface("conversa-1"),
		},
		"sem roteador nenhum": {
			router:     nil,
			superficie: DesktopSurface("conversa-1"),
		},
	}
	for nome, caso := range casos {
		t.Run(nome, func(t *testing.T) {
			_, err := caso.router.Ask(context.Background(), caso.superficie, perguntaDeSimOuNao())
			if !errors.Is(err, ErrAskerUnavailable) {
				t.Errorf("erro = %v, quer %v", err, ErrAskerUnavailable)
			}
		})
	}
}

func TestCanalIncompletoNaoEhSuperficie(t *testing.T) {
	// Sem canal não há para onde mandar; sem contato não há de quem aceitar a
	// resposta. Nos dois casos é melhor negar na hora do que perguntar ao vazio.
	casos := map[string]Surface{
		"sem canal":    ChannelSurface("conversa-1", "   ", "contato-1"),
		"sem contato":  ChannelSurface("conversa-1", "telegram", ""),
		"sem conversa": ChannelSurface("", "telegram", "contato-1"),
	}
	for nome, superficie := range casos {
		t.Run(nome, func(t *testing.T) {
			if superficie.Kind != SurfaceNone {
				t.Errorf("tipo = %q, quer superfície nenhuma", superficie.Kind)
			}
			if superficie.HasInterlocutor() {
				t.Error("superfície incompleta arrumou um interlocutor")
			}
		})
	}
}

func TestSoATelaConcedeAutorizacaoPermanente(t *testing.T) {
	if !DesktopSurface("c").AllowsPersistentAuthorization() {
		t.Error("a tela deixou de poder autorizar para sempre")
	}
	if ChannelSurface("c", "telegram", "contato-1").AllowsPersistentAuthorization() {
		t.Error("canal autorizando para sempre: é o que a Fase 5 do AEP-0084 barra")
	}
	if NoSurface("c").AllowsPersistentAuthorization() {
		t.Error("conversa sem interlocutor autorizando para sempre")
	}
}

func TestOPrazoDoCanalCabeNoTetoDoTransporte(t *testing.T) {
	// O teto que o transporte do agente impõe a quem decide é de 30 minutos
	// (internal/acp). Um prazo maior que ele cortaria a pergunta antes da hora,
	// e a pessoa perderia a chance de responder.
	if ChannelTimeout <= 0 || ChannelTimeout >= 30*time.Minute {
		t.Errorf("prazo do canal = %s, quer algo curto e dentro do teto do transporte", ChannelTimeout)
	}
	if ChannelTimeout >= DefaultTimeout {
		t.Errorf("prazo do canal = %s, quer menos que o da tela (%s)", ChannelTimeout, DefaultTimeout)
	}
}
