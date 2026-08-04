package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"assistente/internal/acp"
	"assistente/internal/core/ports"
	"assistente/internal/questionnaire"
)

// canalDeExtensao é a conversa de canal respondendo às extensões bloqueantes:
// guarda o diálogo que chegou e escolhe uma opção, como quem responde com o
// número dela pelo mensageiro.
type canalDeExtensao struct {
	dialogos []questionnaire.RequestPayload
	// escolhe recebe os rótulos oferecidos e devolve o escolhido.
	escolhe func(opcoes []string) string
	erro    error
}

func (c *canalDeExtensao) AskOnChannel(_ context.Context, _ questionnaire.Surface, payload questionnaire.RequestPayload) (questionnaire.Response, error) {
	c.dialogos = append(c.dialogos, payload)
	if c.erro != nil {
		return questionnaire.Response{}, c.erro
	}
	respostas := map[string]any{}
	for _, item := range payload.Questions {
		if item.Type != "single_choice" {
			continue
		}
		respostas[item.ID] = c.escolhe(questionnaire.TextValues(item.Options))
	}
	return questionnaire.Response{Answers: respostas}, nil
}

func (c *canalDeExtensao) quantosDialogos() int { return len(c.dialogos) }

func (c *canalDeExtensao) ultimoDialogo(tb testing.TB) questionnaire.RequestPayload {
	tb.Helper()
	if len(c.dialogos) == 0 {
		tb.Fatal("nada foi apresentado no canal")
	}
	return c.dialogos[len(c.dialogos)-1]
}

// handlerDeExtensaoNoCanal monta o handler de um turno que veio de canal.
func handlerDeExtensaoNoCanal(tela *telaDeExtensao, canal *canalDeExtensao) *acpRequestHandler {
	h := handlerDeExtensao(tela, acp.TurnOwner{
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

func TestPerguntaDoAgenteVaiParaOCanalEVoltaComAEscolha(t *testing.T) {
	tela := novaTelaDeExtensao(escolhendoAPrimeira())
	canal := &canalDeExtensao{escolhe: func(opcoes []string) string { return opcoes[1] }}
	h := handlerDeExtensaoNoCanal(tela, canal)

	resposta := respostaDaPergunta(t, mustHandle(t, h, pedidoDePergunta(t)))

	if resposta.Outcome.Outcome != askOutcomeAnswered {
		t.Fatalf("desfecho = %q, quer %q", resposta.Outcome.Outcome, askOutcomeAnswered)
	}
	escolhidas := resposta.Outcome.Answers[0].SelectedOptionIDs
	if len(escolhidas) != 1 || escolhidas[0] != "postgres" {
		t.Errorf("escolha = %v, quer a opção respondida pelo canal", escolhidas)
	}
	if canal.quantosDialogos() != 1 {
		t.Errorf("diálogos no canal = %d, quer 1", canal.quantosDialogos())
	}
	if tela.quantosDialogos() != 0 {
		t.Error("a pergunta do canal abriu diálogo na tela de quem não pediu nada")
	}
	// O texto do agente vai inteiro no bloco, do mesmo jeito que na tela: quem
	// responde precisa ler a pergunta antes de escolher.
	if pergunta := blocoDoDialogo(canal.ultimoDialogo(t), askPromptPrefix); pergunta != "Qual banco de dados usar?" {
		t.Errorf("pergunta no canal = %q, quer o texto do agente", pergunta)
	}
}

func TestPlanoDoAgenteVaiParaOCanalEPodeSerAprovado(t *testing.T) {
	canal := &canalDeExtensao{escolhe: func([]string) string { return planApproveLabel }}
	h := handlerDeExtensaoNoCanal(nil, canal)

	resposta := respostaDoPlano(t, mustHandle(t, h, pedidoDePlano(t)))

	if resposta.Outcome.Outcome != planOutcomeAccepted {
		t.Fatalf("desfecho = %q, quer %q", resposta.Outcome.Outcome, planOutcomeAccepted)
	}
	if plano := blocoDoDialogo(canal.ultimoDialogo(t), planContentID); strings.TrimSpace(plano) == "" {
		t.Error("o canal recebeu um plano em branco para aprovar")
	}
}

func TestPlanoRecusadoPeloCanalVoltaRecusadoAoAgente(t *testing.T) {
	canal := &canalDeExtensao{escolhe: func([]string) string { return planRejectLabel }}
	h := handlerDeExtensaoNoCanal(nil, canal)
	avisos := escutandoAvisos(h)

	resposta := respostaDoPlano(t, mustHandle(t, h, pedidoDePlano(t)))

	if resposta.Outcome.Outcome != planOutcomeRejected {
		t.Fatalf("desfecho = %q, quer %q", resposta.Outcome.Outcome, planOutcomeRejected)
	}
	if resposta.Outcome.Reason != reasonPlanRefused {
		t.Errorf("motivo = %q, quer o de quem leu e recusou", resposta.Outcome.Reason)
	}
	if eventos := avisos.find("chat:notice"); len(eventos) != 0 {
		t.Errorf("avisos = %d, quer 0: recusar por escolha é decisão", len(eventos))
	}
}

func TestPerguntaQueOCanalNaoSabeApresentarEhPuladaComOMotivoCerto(t *testing.T) {
	// Pergunta de múltipla escolha, ou com mais de uma decisão, não cabe numa
	// mensagem em que a resposta é um número. O agente ouve que o app não
	// conseguiu apresentar o pedido, e não que ninguém respondeu a tempo: ele
	// repete esse motivo à pessoa.
	canal := &canalDeExtensao{
		erro: fmt.Errorf("não cabe numa mensagem: %w", questionnaire.ErrAskerUnavailable),
	}
	h := handlerDeExtensaoNoCanal(nil, canal)
	avisos := escutandoAvisos(h)

	resposta := respostaDaPergunta(t, mustHandle(t, h, pedidoDePergunta(t)))

	if resposta.Outcome.Outcome != askOutcomeSkipped {
		t.Fatalf("desfecho = %q, quer %q", resposta.Outcome.Outcome, askOutcomeSkipped)
	}
	if resposta.Outcome.Reason != reasonUnavailable {
		t.Errorf("motivo = %q, quer %q", resposta.Outcome.Reason, reasonUnavailable)
	}
	if aviso := avisoNaConversa(t, avisos); aviso.Kind != ports.ChatNoticeKindQuestionUnavailable {
		t.Errorf("motivo do aviso = %q, quer %q", aviso.Kind, ports.ChatNoticeKindQuestionUnavailable)
	}
}

func TestPlanoQueOCanalNaoSabeApresentarEhRecusadoComOMotivoCerto(t *testing.T) {
	canal := &canalDeExtensao{
		erro: fmt.Errorf("mensageiro fora do ar: %w", questionnaire.ErrAskerUnavailable),
	}
	h := handlerDeExtensaoNoCanal(nil, canal)
	avisos := escutandoAvisos(h)

	resposta := respostaDoPlano(t, mustHandle(t, h, pedidoDePlano(t)))

	if resposta.Outcome.Outcome != planOutcomeRejected {
		t.Fatalf("desfecho = %q, quer %q", resposta.Outcome.Outcome, planOutcomeRejected)
	}
	if resposta.Outcome.Reason != reasonUnavailable {
		t.Errorf("motivo = %q, quer %q", resposta.Outcome.Reason, reasonUnavailable)
	}
	if aviso := avisoNaConversa(t, avisos); aviso.Kind != ports.ChatNoticeKindPlanUnavailable {
		t.Errorf("motivo do aviso = %q, quer %q", aviso.Kind, ports.ChatNoticeKindPlanUnavailable)
	}
}

func TestSemRespostaNoPrazoDoCanalAPerguntaEhPulada(t *testing.T) {
	canal := &canalDeExtensao{erro: errors.New("prazo da pergunta no canal esgotado")}
	h := handlerDeExtensaoNoCanal(nil, canal)
	avisos := escutandoAvisos(h)

	resposta := respostaDaPergunta(t, mustHandle(t, h, pedidoDePergunta(t)))

	if resposta.Outcome.Reason != reasonNoAnswer {
		t.Errorf("motivo = %q, quer %q", resposta.Outcome.Reason, reasonNoAnswer)
	}
	if aviso := avisoNaConversa(t, avisos); aviso.Kind != ports.ChatNoticeKindQuestionTimeout {
		t.Errorf("motivo do aviso = %q, quer %q", aviso.Kind, ports.ChatNoticeKindQuestionTimeout)
	}
}

func TestConversaSemOrigemContinuaPulandoAPerguntaNaHora(t *testing.T) {
	canal := &canalDeExtensao{escolhe: func(opcoes []string) string { return opcoes[0] }}
	h := handlerDeExtensaoNoCanal(nil, canal)
	h.origin = func(owner acp.TurnOwner) questionnaire.Surface {
		return questionnaire.NoSurface(owner.ConversationID)
	}
	avisos := escutandoAvisos(h)

	resposta := respostaDaPergunta(t, mustHandle(t, h, pedidoDePergunta(t)))

	if resposta.Outcome.Reason != reasonNoWatcher {
		t.Errorf("motivo = %q, quer %q", resposta.Outcome.Reason, reasonNoWatcher)
	}
	if canal.quantosDialogos() != 0 {
		t.Error("apresentou a pergunta a um canal que não era a origem da conversa")
	}
	if aviso := avisoNaConversa(t, avisos); aviso.Kind != ports.ChatNoticeKindQuestionNoWatcher {
		t.Errorf("motivo do aviso = %q, quer %q", aviso.Kind, ports.ChatNoticeKindQuestionNoWatcher)
	}
}

// blocoDoDialogo lê o conteúdo do bloco de leitura cujo id começa com o prefixo.
func blocoDoDialogo(payload questionnaire.RequestPayload, prefixo string) string {
	for _, item := range payload.Questions {
		if item.Type == "readonly_code" && strings.HasPrefix(item.ID, prefixo) {
			return item.Content
		}
	}
	return ""
}
