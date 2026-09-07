package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"assistente/internal/acp"
	"assistente/internal/acptrust"
	"assistente/internal/core/ports"
	"assistente/internal/database"
	"assistente/internal/questionnaire"
)

// canalFalso é a conversa de canal do outro lado: guarda o diálogo que chegou e
// responde o que o teste combinou, como a pessoa faria pelo mensageiro.
type canalFalso struct {
	pedidos    []questionnaire.RequestPayload
	superficie questionnaire.Surface
	// escolhe recebe os rótulos oferecidos e devolve o escolhido.
	escolhe func(opcoes []string) string
	erro    error
}

func (c *canalFalso) AskOnChannel(_ context.Context, surface questionnaire.Surface, payload questionnaire.RequestPayload) (questionnaire.Response, error) {
	c.pedidos = append(c.pedidos, payload)
	c.superficie = surface
	if c.erro != nil {
		return questionnaire.Response{}, c.erro
	}
	if c.escolhe == nil {
		return questionnaire.Response{}, errors.New("ninguém decidiu")
	}
	escolha := c.escolhe(c.opcoesOferecidas())
	if payload.Kind == questionnaire.KindDecision {
		return questionnaire.Response{
			Answers: map[string]any{questionnaire.AnswerActionID: actionIDDaAcao(payload, escolha)},
		}, nil
	}
	return questionnaire.Response{
		Answers: map[string]any{permissionAnswerID: escolha},
	}, nil
}

// opcoesOferecidas é o que a pessoa vê na mensagem: ids das ações (decision)
// ou valores estáveis das opções de rádio (legado).
func (c *canalFalso) opcoesOferecidas() []string {
	if len(c.pedidos) == 0 {
		return nil
	}
	ultimo := c.pedidos[len(c.pedidos)-1]
	if ultimo.Kind == questionnaire.KindDecision {
		out := make([]string, 0, len(ultimo.Actions))
		for _, action := range ultimo.Actions {
			out = append(out, action.ID)
		}
		return out
	}
	for _, pergunta := range ultimo.Questions {
		if pergunta.ID == permissionAnswerID {
			return questionnaire.TextValues(pergunta.Options)
		}
	}
	return nil
}

// actionIDDaAcao aceita id estável ou rótulo (resposta malformada nos testes).
func actionIDDaAcao(payload questionnaire.RequestPayload, escolha string) string {
	for _, action := range payload.Actions {
		if action.ID == escolha || action.Label.String() == escolha {
			return action.ID
		}
	}
	return escolha
}

func (c *canalFalso) quantosPedidos() int { return len(c.pedidos) }

// handlerDeCanal monta o handler de um turno que veio de um canal: sem tela, com
// a conversa do mensageiro como superfície de origem.
func handlerDeCanal(tela *telaFalsa, canal *canalFalso) *acpRequestHandler {
	h := handlerCom(tela, acp.TurnOwner{
		ConversationID: "conversa-1",
		ProfileSlug:    "perfil-do-canal",
		UserID:         "dono-1",
	}, true)
	h.origin = func(owner acp.TurnOwner) questionnaire.Surface {
		return questionnaire.ChannelSurface(owner.ConversationID, "telegram", "contato-1")
	}
	h.surfaces = questionnaire.NewRouter(
		h.questionnaireManager,
		func() questionnaire.ChannelAsker { return canal },
	)
	return h
}

func TestConversaDeCanalAutorizaPelaPropriaConversa(t *testing.T) {
	tela := novaTelaFalsa(func(opcoes []string) string { return opcoes[0] })
	canal := &canalFalso{escolhe: func(opcoes []string) string { return opcoes[0] }}
	h := handlerDeCanal(tela, canal)

	out := h.RequestPermission(context.Background(), pedidoDeExecucao())

	if out.OptionID != "allow-once" {
		t.Errorf("decisão = %q, quer a opção autorizada pelo canal", out.OptionID)
	}
	if canal.quantosPedidos() != 1 {
		t.Fatalf("pedidos no canal = %d, quer 1", canal.quantosPedidos())
	}
	if canal.superficie.ContactID != "contato-1" {
		t.Errorf("contato = %q, quer o dono da conversa do canal", canal.superficie.ContactID)
	}
	if tela.quantasPerguntas() != 0 {
		t.Error("a pergunta do canal abriu diálogo na tela de quem não pediu nada")
	}
}

func TestConversaDeCanalNegaPelaPropriaConversa(t *testing.T) {
	canal := &canalFalso{escolhe: func(opcoes []string) string { return opcoes[len(opcoes)-1] }}
	h := handlerDeCanal(nil, canal)
	avisos := escutandoAvisos(h)

	out := h.RequestPermission(context.Background(), pedidoDeExecucao())

	if out.OptionID != "reject-once" {
		t.Errorf("decisão = %q, quer a negativa que o canal escolheu", out.OptionID)
	}
	// Negar é decisão de alguém: não é surpresa que precise de aviso.
	if eventos := avisos.find("chat:notice"); len(eventos) != 0 {
		t.Errorf("avisos = %d, quer 0: quem negou já sabe o que fez", len(eventos))
	}
}

func TestForaDoDesktopNaoSeOfereceAutorizarParaSempre(t *testing.T) {
	canal := &canalFalso{escolhe: func(opcoes []string) string { return opcoes[0] }}
	h := handlerDeCanal(nil, canal)

	h.RequestPermission(context.Background(), pedidoDeExecucao())

	oferecidas := canal.opcoesOferecidas()
	if len(oferecidas) != 2 {
		t.Fatalf("opções oferecidas = %q, quer só permitir uma vez e negar", oferecidas)
	}
	for _, rotulo := range oferecidas {
		if strings.Contains(strings.ToLower(rotulo), "sempre") {
			t.Errorf("o canal ofereceu %q: autorizar para sempre é barrado fora do desktop", rotulo)
		}
	}
}

func TestPedidoQueSoOfereceSempreEhNegadoForaDoDesktop(t *testing.T) {
	// Tirado o "sempre", não sobra como dizer sim: mandar a mensagem custaria
	// uma ida e volta por uma decisão que já está tomada.
	canal := &canalFalso{escolhe: func(opcoes []string) string { return opcoes[0] }}
	h := handlerDeCanal(nil, canal)
	avisos := escutandoAvisos(h)

	pedido := pedidoDeExecucao()
	pedido.Options = []acp.PermissionOption{
		{ID: "allow-always", Name: "Permitir sempre", Kind: "allow_always"},
		{ID: "reject-once", Name: "Negar", Kind: "reject_once"},
	}

	out := h.RequestPermission(context.Background(), pedido)

	if out.OptionID != "" {
		t.Errorf("decisão = %q, quer nenhuma", out.OptionID)
	}
	if canal.quantosPedidos() != 0 {
		t.Error("mandou ao canal uma pergunta em que só cabia negar")
	}
	if aviso := avisoNaConversa(t, avisos); aviso.Kind != ports.ChatNoticeKindPermissionUnavailable {
		t.Errorf("motivo = %q, quer %q", aviso.Kind, ports.ChatNoticeKindPermissionUnavailable)
	}
}

func TestRespostaDoCanalComOpcaoNaoOferecidaNaoAutoriza(t *testing.T) {
	// A trava do "sempre" não depende de a lista de opções estar certa: uma
	// resposta que nomeie o que não foi oferecido não encontra escolha nenhuma.
	canal := &canalFalso{escolhe: func([]string) string { return "Permitir sempre" }}
	h := handlerDeCanal(nil, canal)
	store := lembrandoAutorizacoes(t, h, "perfil-ativo")

	out := h.RequestPermission(context.Background(), pedidoDeExecucao())

	if out.OptionID != "" {
		t.Errorf("decisão = %q, quer nenhuma", out.OptionID)
	}
	if store.Allows("perfil-do-canal", "execute") {
		t.Error("gravou autorização permanente a partir de uma resposta de canal")
	}
}

func TestAutorizacaoParaSempreNaoEhGravadaForaDoDesktop(t *testing.T) {
	// A opção não é oferecida no canal, mas a recusa de gravar é uma trava
	// própria: outra superfície, outro montador de diálogo ou um erro futuro não
	// podem transformar uma mensagem em autorização permanente.
	h := handlerDeCanal(nil, &canalFalso{})
	store := lembrandoAutorizacoes(t, h, "perfil-ativo")
	avisos := escutandoAvisos(h)

	superficie := questionnaire.ChannelSurface("conversa-1", "telegram", "contato-1")
	h.rememberAlways(context.Background(), acp.TurnOwner{ConversationID: "conversa-1"},
		superficie, "perfil-do-canal", "execute", "execute, chamada \"call-1\"")

	if store.Allows("perfil-do-canal", "execute") {
		t.Error("a autorização permanente foi gravada a partir de um canal")
	}
	if aviso := avisoNaConversa(t, avisos); aviso.Kind != ports.ChatNoticeKindPermissionAlwaysNotSaved {
		t.Errorf("motivo = %q, quer %q", aviso.Kind, ports.ChatNoticeKindPermissionAlwaysNotSaved)
	}
}

func TestAutorizacaoParaSempreContinuaValendoNaTela(t *testing.T) {
	// A trava é da superfície, e não do mecanismo: na tela o "sempre" segue
	// sendo guardado como antes.
	tela := novaTelaFalsa(func(opcoes []string) string { return opcoes[1] })
	h := handlerCom(tela, acp.TurnOwner{ConversationID: "conversa-1", ProfileSlug: "perfil-1", Interactive: true}, true)
	store := lembrandoAutorizacoes(t, h, "perfil-1")

	out := h.RequestPermission(context.Background(), pedidoDeExecucao())

	if out.OptionID != "allow-always" {
		t.Fatalf("decisão = %q, quer a autorização permanente", out.OptionID)
	}
	if !store.Allows("perfil-1", "execute") {
		t.Error("a tela deixou de guardar o que a pessoa autorizou para sempre")
	}
}

func TestAutorizacaoPermanenteNaoEhConsultadaForaDoDesktop(t *testing.T) {
	// Senão um canal remoto colheria o sim que alguém deu na tela (AEP-0084 D9).
	canal := &canalFalso{escolhe: func(opcoes []string) string { return opcoes[len(opcoes)-1] }}
	h := handlerDeCanal(nil, canal)
	store := acptrust.NewStoreWithDir(t.TempDir())
	if err := store.Allow("perfil-do-canal", "execute"); err != nil {
		t.Fatalf("erro ao preparar a autorização: %v", err)
	}
	h.trust = func() *acptrust.Store { return store }
	h.activeProfile = func() string { return "perfil-do-canal" }

	out := h.RequestPermission(context.Background(), pedidoDeExecucao())

	if canal.quantosPedidos() != 1 {
		t.Error("o canal não foi perguntado: a permissão saiu da allowlist da tela")
	}
	if out.OptionID != "reject-once" {
		t.Errorf("decisão = %q, quer a que o canal respondeu", out.OptionID)
	}
}

func TestSemRespostaNoCanalAConversaFicaSabendo(t *testing.T) {
	canal := &canalFalso{erro: errors.New("prazo da pergunta no canal esgotado")}
	h := handlerDeCanal(nil, canal)
	avisos := escutandoAvisos(h)

	out := h.RequestPermission(context.Background(), pedidoDeExecucao())

	if out.OptionID != "" {
		t.Errorf("decisão = %q, quer nenhuma", out.OptionID)
	}
	if aviso := avisoNaConversa(t, avisos); aviso.Kind != ports.ChatNoticeKindPermissionTimeout {
		t.Errorf("motivo = %q, quer %q", aviso.Kind, ports.ChatNoticeKindPermissionTimeout)
	}
}

func TestPerguntaQueNaoChegouAoCanalNaoCulpaQuemNaoAViu(t *testing.T) {
	// "Ninguém respondeu a tempo" numa pergunta que nunca apareceu contaria uma
	// coisa pela outra a quem for ler a conversa depois.
	canal := &canalFalso{erro: fmt.Errorf("mensageiro fora do ar: %w", questionnaire.ErrAskerUnavailable)}
	h := handlerDeCanal(nil, canal)
	avisos := escutandoAvisos(h)

	h.RequestPermission(context.Background(), pedidoDeExecucao())

	if aviso := avisoNaConversa(t, avisos); aviso.Kind != ports.ChatNoticeKindPermissionUnavailable {
		t.Errorf("motivo = %q, quer %q", aviso.Kind, ports.ChatNoticeKindPermissionUnavailable)
	}
}

func TestConversaQueNaoVeioDeCanalNemDeTelaNegaNaHora(t *testing.T) {
	canal := &canalFalso{escolhe: func(opcoes []string) string { return opcoes[0] }}
	h := handlerDeCanal(nil, canal)
	h.origin = func(owner acp.TurnOwner) questionnaire.Surface {
		return questionnaire.NoSurface(owner.ConversationID)
	}
	avisos := escutandoAvisos(h)

	out := h.RequestPermission(context.Background(), pedidoDeExecucao())

	if out.OptionID != "" {
		t.Errorf("decisão = %q, quer nenhuma", out.OptionID)
	}
	if canal.quantosPedidos() != 0 {
		t.Error("perguntou a um canal que não era a origem da conversa")
	}
	if aviso := avisoNaConversa(t, avisos); aviso.Kind != ports.ChatNoticeKindPermissionNoWatcher {
		t.Errorf("motivo = %q, quer %q", aviso.Kind, ports.ChatNoticeKindPermissionNoWatcher)
	}
}

func TestTurnoComTelaNaoProcuraCanalNenhum(t *testing.T) {
	// A conversa aberta na tela é atendida ali, sem consulta ao banco para
	// descobrir uma origem que já se conhece.
	tela := novaTelaFalsa(func(opcoes []string) string { return opcoes[0] })
	canal := &canalFalso{escolhe: func(opcoes []string) string { return opcoes[0] }}
	h := handlerDeCanal(tela, canal)
	h.owner = func(string) (acp.TurnOwner, bool) {
		return acp.TurnOwner{ConversationID: "conversa-1", Interactive: true, UserID: "dono-1"}, true
	}
	procurou := false
	h.origin = func(owner acp.TurnOwner) questionnaire.Surface {
		procurou = true
		return questionnaire.NoSurface(owner.ConversationID)
	}

	out := h.RequestPermission(context.Background(), pedidoDeExecucao())

	if out.OptionID != "allow-once" {
		t.Errorf("decisão = %q, quer a da tela", out.OptionID)
	}
	if procurou {
		t.Error("foi procurar a origem de uma conversa que já estava na tela")
	}
	if canal.quantosPedidos() != 0 {
		t.Error("a pergunta da tela vazou para o canal")
	}
}

func TestOrigemDaConversaVemDoCanalDaConversa(t *testing.T) {
	conversa := &database.Conversation{Channel: "telegram", ContactID: "contato-1"}
	superficie := conversationSurface(
		acp.TurnOwner{ConversationID: "conversa-1", UserID: "dono-1"},
		func(ctx context.Context, id string) (*database.Conversation, error) {
			if userID, _ := database.UserIDFromContext(ctx); userID != "dono-1" {
				t.Errorf("consulta sem o dono do turno (userID = %q): leria a conversa de outra pessoa", userID)
			}
			if id != "conversa-1" {
				t.Errorf("conversa consultada = %q, quer a do turno", id)
			}
			return conversa, nil
		},
	)

	if superficie.Kind != questionnaire.SurfaceChannel {
		t.Fatalf("tipo = %q, quer superfície de canal", superficie.Kind)
	}
	if superficie.Channel != "telegram" || superficie.ContactID != "contato-1" {
		t.Errorf("superfície = %+v, quer o canal e o contato da conversa", superficie)
	}
}

func TestOrigemDesconhecidaNaoInventaInterlocutor(t *testing.T) {
	casos := map[string]struct {
		owner  acp.TurnOwner
		lookup func(context.Context, string) (*database.Conversation, error)
	}{
		"turno sem dono": {
			owner: acp.TurnOwner{ConversationID: "conversa-1"},
			lookup: func(context.Context, string) (*database.Conversation, error) {
				t.Error("consultou a conversa sem saber de quem é o turno")
				return nil, nil
			},
		},
		"conversa que não existe mais": {
			owner: acp.TurnOwner{ConversationID: "conversa-1", UserID: "dono-1"},
			lookup: func(context.Context, string) (*database.Conversation, error) {
				return nil, errors.New("record not found")
			},
		},
		"conversa local": {
			owner: acp.TurnOwner{ConversationID: "conversa-1", UserID: "dono-1"},
			lookup: func(context.Context, string) (*database.Conversation, error) {
				return &database.Conversation{}, nil
			},
		},
		"canal sem contato": {
			owner: acp.TurnOwner{ConversationID: "conversa-1", UserID: "dono-1"},
			lookup: func(context.Context, string) (*database.Conversation, error) {
				return &database.Conversation{Channel: "telegram"}, nil
			},
		},
	}
	for nome, caso := range casos {
		t.Run(nome, func(t *testing.T) {
			superficie := conversationSurface(caso.owner, caso.lookup)
			if superficie.HasInterlocutor() {
				t.Errorf("superfície = %+v, quer nenhuma", superficie)
			}
			if superficie.ConversationID != "conversa-1" {
				t.Errorf("conversa = %q, quer a do turno: é por ela que o aviso chega", superficie.ConversationID)
			}
		})
	}
}
