package app

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"assistente/internal/acp"
	"assistente/internal/core/ports"
	"assistente/internal/questionnaire"
)

// telaDeExtensao é o questionário do desktop com a pessoa do outro lado,
// respondendo aos diálogos das extensões. É prima da telaFalsa das permissões
// e existe à parte porque aqui a resposta é por item: uma pergunta do agente
// pode trazer várias, e o plano tem a sua.
type telaDeExtensao struct {
	mu       sync.Mutex
	manager  *questionnaire.Manager
	dialogos []map[string]any
	// responde recebe os itens do diálogo e devolve a resposta por item; nulo
	// cancela, como quem fecha a janela.
	responde func(itens []questionnaire.Question) map[string]any
	// muda deixa o diálogo aberto sem resposta, como quem ainda está lendo.
	muda      bool
	perguntou chan struct{}
}

func novaTelaDeExtensao(responde func(itens []questionnaire.Question) map[string]any) *telaDeExtensao {
	tela := &telaDeExtensao{responde: responde, perguntou: make(chan struct{}, 1)}
	tela.manager = questionnaire.NewManager(tela.aoPerguntar)
	return tela
}

func novaTelaDeExtensaoMuda() *telaDeExtensao {
	tela := novaTelaDeExtensao(nil)
	tela.muda = true
	return tela
}

func (t *telaDeExtensao) aoPerguntar(event string, data any) {
	payload, ok := data.(map[string]any)
	if !ok || event != questionnaire.EventQuestionnaire {
		return
	}
	t.mu.Lock()
	t.dialogos = append(t.dialogos, payload)
	t.mu.Unlock()
	avisar(t.perguntou)

	if t.muda {
		return
	}
	id, _ := payload["id"].(string)
	itens := perguntasDe(payload)
	go func() {
		if t.responde == nil {
			_ = t.manager.Respond(id, nil, true)
			return
		}
		_ = t.manager.Respond(id, t.responde(itens), false)
	}()
}

func (t *telaDeExtensao) esperarDialogo(tb testing.TB) {
	tb.Helper()
	select {
	case <-t.perguntou:
	case <-time.After(2 * time.Second):
		tb.Fatal("o diálogo não chegou à tela")
	}
}

func (t *telaDeExtensao) ultimoDialogo(tb testing.TB) map[string]any {
	tb.Helper()
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.dialogos) == 0 {
		tb.Fatal("nada foi apresentado à pessoa")
	}
	return t.dialogos[len(t.dialogos)-1]
}

func (t *telaDeExtensao) quantosDialogos() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.dialogos)
}

// respondendo escolhe, em cada item de escolha do diálogo, o que a função
// devolver a partir das opções oferecidas.
func respondendo(escolhe func(opcoes []string) any) func([]questionnaire.Question) map[string]any {
	return func(itens []questionnaire.Question) map[string]any {
		respostas := make(map[string]any, len(itens))
		for _, item := range itens {
			if item.Type != "single_choice" && item.Type != "multiple_choice" {
				continue
			}
			// O valor que volta em Answers é o estável, não o rótulo
			// traduzido — é assim que a tela responde (AEP-0085).
			respostas[item.ID] = escolhe(questionnaire.TextValues(item.Options))
		}
		return respostas
	}
}

// escolhendoAPrimeira é a pessoa que marca a primeira opção de cada pergunta.
func escolhendoAPrimeira() func([]questionnaire.Question) map[string]any {
	return respondendo(func(opcoes []string) any { return opcoes[0] })
}

func handlerDeExtensao(tela *telaDeExtensao, owner acp.TurnOwner, temTurno bool) *acpRequestHandler {
	h := &acpRequestHandler{
		owner: func(string) (acp.TurnOwner, bool) { return owner, temTurno },
	}
	if tela != nil {
		h.questions = func() *questionnaire.Manager { return tela.manager }
	}
	return h
}

func pedidoCom(tb testing.TB, method string, params map[string]any) acp.CustomRequest {
	tb.Helper()
	corpo, err := json.Marshal(params)
	if err != nil {
		tb.Fatalf("corpo do pedido inválido: %v", err)
	}
	return acp.CustomRequest{Method: method, SessionID: "sessao-1", Params: corpo}
}

func pedidoDePergunta(tb testing.TB) acp.CustomRequest {
	tb.Helper()
	return pedidoCom(tb, methodAskQuestion, map[string]any{
		"toolCallId": "call-1\nfc-2",
		"title":      "Escolher a abordagem",
		"questions": []any{
			map[string]any{
				"id":     "q1",
				"prompt": "Qual banco de dados usar?",
				"options": []any{
					map[string]any{"id": "sqlite", "label": "SQLite"},
					map[string]any{"id": "postgres", "label": "Postgres"},
				},
			},
		},
	})
}

func pedidoDePlano(tb testing.TB) acp.CustomRequest {
	tb.Helper()
	return pedidoCom(tb, methodCreatePlan, map[string]any{
		"toolCallId": "call-2",
		"name":       "Autenticação de dois fatores",
		"overview":   "Adicionar TOTP ao login existente.",
		"plan":       "Trocar o fluxo de login e guardar o segredo por usuário.",
		"todos": []any{
			map[string]any{"id": "t1", "content": "Criar a tabela de segredos", "status": "pending"},
			map[string]any{"id": "t2", "content": "Validar o código no login", "status": "pending"},
		},
	})
}

// respostaDaPergunta lê o desfecho que o handler devolveu ao agente.
func respostaDaPergunta(tb testing.TB, resultado any) askResponse {
	tb.Helper()
	resposta, ok := resultado.(askResponse)
	if !ok {
		tb.Fatalf("desfecho veio como %T, quer askResponse", resultado)
	}
	return resposta
}

func respostaDoPlano(tb testing.TB, resultado any) planResponse {
	tb.Helper()
	resposta, ok := resultado.(planResponse)
	if !ok {
		tb.Fatalf("desfecho veio como %T, quer planResponse", resultado)
	}
	return resposta
}

// blocoNaTela devolve o texto do item de leitura cujo identificador começa com
// o prefixo pedido — é o que a pessoa lê antes de decidir.
func blocoNaTela(tb testing.TB, payload map[string]any, prefixo string) string {
	tb.Helper()
	for _, item := range perguntasDe(payload) {
		if item.Type == "readonly_code" && strings.HasPrefix(item.ID, prefixo) {
			return item.Content
		}
	}
	tb.Fatalf("o diálogo não mostrou o bloco %q", prefixo)
	return ""
}

// rotulosDosBlocos lista, na ordem, o nome de cada bloco de leitura do
// diálogo — é o que quem usa leitor de telas ouve ao chegar em cada um.
func rotulosDosBlocos(payload map[string]any, prefixo string) []string {
	var out []string
	for _, item := range perguntasDe(payload) {
		if item.Type == "readonly_code" && strings.HasPrefix(item.ID, prefixo) {
			out = append(out, item.Prompt.String())
		}
	}
	return out
}

func opcoesNaTela(payload map[string]any) []string {
	for _, item := range perguntasDe(payload) {
		if item.Type == "single_choice" || item.Type == "multiple_choice" {
			return questionnaire.TextValues(item.Options)
		}
	}
	return nil
}

func TestPerguntaDoAgenteChegaNaTelaEVoltaComAEscolhaDaPessoa(t *testing.T) {
	tela := novaTelaDeExtensao(respondendo(func(opcoes []string) any { return opcoes[1] }))
	h := handlerDeExtensao(tela, acp.TurnOwner{ConversationID: "conversa-1", Interactive: true}, true)

	resultado, tratou := h.HandleCustom(context.Background(), pedidoDePergunta(t))

	if !tratou {
		t.Fatal("a extensão bloqueante não foi tratada: o agente ouviria que o método não existe")
	}
	resposta := respostaDaPergunta(t, resultado)
	if resposta.Outcome.Outcome != askOutcomeAnswered {
		t.Fatalf("desfecho = %q, quer %q", resposta.Outcome.Outcome, askOutcomeAnswered)
	}
	if len(resposta.Outcome.Answers) != 1 {
		t.Fatalf("respostas = %d, quer 1", len(resposta.Outcome.Answers))
	}
	answer := resposta.Outcome.Answers[0]
	if answer.QuestionID != "q1" {
		t.Errorf("pergunta respondida = %q, quer q1", answer.QuestionID)
	}
	if len(answer.SelectedOptionIDs) != 1 || answer.SelectedOptionIDs[0] != "postgres" {
		t.Errorf("escolha = %v, quer a opção que a pessoa marcou", answer.SelectedOptionIDs)
	}
	if pergunta := blocoNaTela(t, tela.ultimoDialogo(t), askPromptPrefix); pergunta != "Qual banco de dados usar?" {
		t.Errorf("pergunta na tela = %q, quer o texto do agente", pergunta)
	}
}

func TestPerguntaDeMultiplaEscolhaVoltaComTudoQueFoiMarcado(t *testing.T) {
	// A tela devolve uma lista nesse caso, e ela atravessa a ponte como []any.
	tela := novaTelaDeExtensao(respondendo(func(opcoes []string) any {
		return []any{opcoes[0], opcoes[2]}
	}))
	h := handlerDeExtensao(tela, acp.TurnOwner{ConversationID: "c", Interactive: true}, true)

	pedido := pedidoCom(t, methodAskQuestion, map[string]any{
		"toolCallId": "call-1",
		"questions": []any{
			map[string]any{
				"id":            "q1",
				"prompt":        "Quais provedores habilitar?",
				"allowMultiple": true,
				"options": []any{
					map[string]any{"id": "google", "label": "Google"},
					map[string]any{"id": "github", "label": "GitHub"},
					map[string]any{"id": "gitlab", "label": "GitLab"},
				},
			},
		},
	})

	resposta := respostaDaPergunta(t, mustHandle(t, h, pedido))

	if resposta.Outcome.Outcome != askOutcomeAnswered {
		t.Fatalf("desfecho = %q, quer %q", resposta.Outcome.Outcome, askOutcomeAnswered)
	}
	escolhidas := resposta.Outcome.Answers[0].SelectedOptionIDs
	if len(escolhidas) != 2 || escolhidas[0] != "google" || escolhidas[1] != "gitlab" {
		t.Errorf("escolhas = %v, quer as duas marcadas", escolhidas)
	}
}

func TestPrazoEstouradoNaPerguntaVoltaComoPulada(t *testing.T) {
	// "rejected" aqui seria erro de protocolo: o método diz não com
	// "skipped", e o agente leria o outro como decisão de verdade.
	mudo := questionnaire.NewManager(func(string, any) {})
	h := handlerDeExtensao(nil, acp.TurnOwner{ConversationID: "conversa-1", Interactive: true}, true)
	h.questions = func() *questionnaire.Manager { return mudo }
	avisos := escutandoAvisos(h)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	resposta := respostaDaPergunta(t, mustHandleCtx(t, ctx, h, pedidoDePergunta(t)))

	if resposta.Outcome.Outcome != askOutcomeSkipped {
		t.Errorf("desfecho = %q, quer %q", resposta.Outcome.Outcome, askOutcomeSkipped)
	}
	if aviso := avisoNaConversa(t, avisos); aviso.Kind != ports.ChatNoticeKindQuestionTimeout {
		t.Errorf("motivo do aviso = %q, quer %q", aviso.Kind, ports.ChatNoticeKindQuestionTimeout)
	}
}

func TestPrazoEstouradoNoPlanoVoltaComoRecusado(t *testing.T) {
	mudo := questionnaire.NewManager(func(string, any) {})
	h := handlerDeExtensao(nil, acp.TurnOwner{ConversationID: "conversa-1", Interactive: true}, true)
	h.questions = func() *questionnaire.Manager { return mudo }
	avisos := escutandoAvisos(h)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	resposta := respostaDoPlano(t, mustHandleCtx(t, ctx, h, pedidoDePlano(t)))

	if resposta.Outcome.Outcome != planOutcomeRejected {
		t.Errorf("desfecho = %q, quer %q", resposta.Outcome.Outcome, planOutcomeRejected)
	}
	if aviso := avisoNaConversa(t, avisos); aviso.Kind != ports.ChatNoticeKindPlanTimeout {
		t.Errorf("motivo do aviso = %q, quer %q", aviso.Kind, ports.ChatNoticeKindPlanTimeout)
	}
}

func TestPerguntaSemRespostaNaoPenduraOAgente(t *testing.T) {
	// Questionário que nunca é respondido: o handler precisa voltar mesmo
	// assim, senão o agente fica esperando até o teto do transporte.
	mudo := questionnaire.NewManager(func(string, any) {})
	h := handlerDeExtensao(nil, acp.TurnOwner{ConversationID: "c", Interactive: true}, true)
	h.questions = func() *questionnaire.Manager { return mudo }

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	pronto := make(chan any, 1)
	go func() {
		resultado, _ := h.HandleCustom(ctx, pedidoDePergunta(t))
		pronto <- resultado
	}()

	select {
	case resultado := <-pronto:
		if resposta := respostaDaPergunta(t, resultado); resposta.Outcome.Outcome != askOutcomeSkipped {
			t.Errorf("desfecho = %q, quer %q", resposta.Outcome.Outcome, askOutcomeSkipped)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("o handler não voltou: o agente ficaria pendurado")
	}
}

func TestTurnoCanceladoTiraAPerguntaDaTelaSemCobrarExplicacao(t *testing.T) {
	// Quem cancelou o turno já sabe o que houve; um aviso aqui cobraria
	// explicação de quem acabou de dar uma. Quem devolve ao agente o erro de
	// pedido cancelado é o transporte, que observa o mesmo cancelamento.
	tela := novaTelaDeExtensaoMuda()
	h := handlerDeExtensao(tela, acp.TurnOwner{ConversationID: "conversa-1", Interactive: true}, true)
	avisos := escutandoAvisos(h)

	ctx, cancelarTurno := context.WithCancel(context.Background())
	pronto := make(chan any, 1)
	go func() {
		resultado, _ := h.HandleCustom(ctx, pedidoDePergunta(t))
		pronto <- resultado
	}()

	tela.esperarDialogo(t)
	cancelarTurno()

	select {
	case <-pronto:
	case <-time.After(5 * time.Second):
		t.Fatal("o handler não voltou depois do cancelamento")
	}
	if eventos := avisos.find("chat:notice"); len(eventos) != 0 {
		t.Errorf("avisos = %d, quer 0: quem cancelou já sabe o que houve", len(eventos))
	}
}

func TestTurnoSemNinguemNaTelaPulaAPerguntaNaHora(t *testing.T) {
	// Canal, job agendado, subagente, CLI: esperar aqui penduraria o agente
	// até o teto do transporte.
	tela := novaTelaDeExtensao(escolhendoAPrimeira())
	h := handlerDeExtensao(tela, acp.TurnOwner{ConversationID: "conversa-1"}, true)
	avisos := escutandoAvisos(h)

	resposta := respostaDaPergunta(t, mustHandle(t, h, pedidoDePergunta(t)))

	if resposta.Outcome.Outcome != askOutcomeSkipped {
		t.Errorf("desfecho = %q, quer %q", resposta.Outcome.Outcome, askOutcomeSkipped)
	}
	if tela.quantosDialogos() != 0 {
		t.Error("abriu diálogo para um turno que ninguém está vendo")
	}
	aviso := avisoNaConversa(t, avisos)
	if aviso.Kind != ports.ChatNoticeKindQuestionNoWatcher {
		t.Errorf("motivo do aviso = %q, quer %q", aviso.Kind, ports.ChatNoticeKindQuestionNoWatcher)
	}
	if aviso.ConversationID != "conversa-1" {
		t.Errorf("aviso foi para %q, quer a conversa dona do turno", aviso.ConversationID)
	}
}

func TestTurnoSemNinguemNaTelaRecusaOPlanoNaHora(t *testing.T) {
	tela := novaTelaDeExtensao(escolhendoAPrimeira())
	h := handlerDeExtensao(tela, acp.TurnOwner{ConversationID: "conversa-1"}, true)
	avisos := escutandoAvisos(h)

	resposta := respostaDoPlano(t, mustHandle(t, h, pedidoDePlano(t)))

	if resposta.Outcome.Outcome != planOutcomeRejected {
		t.Errorf("desfecho = %q, quer %q", resposta.Outcome.Outcome, planOutcomeRejected)
	}
	if tela.quantosDialogos() != 0 {
		t.Error("apresentou o plano a um turno que ninguém está vendo")
	}
	if aviso := avisoNaConversa(t, avisos); aviso.Kind != ports.ChatNoticeKindPlanNoWatcher {
		t.Errorf("motivo do aviso = %q, quer %q", aviso.Kind, ports.ChatNoticeKindPlanNoWatcher)
	}
}

func TestPedidoSemTurnoNaoAvisaConversaNenhuma(t *testing.T) {
	// Sem turno não há conversa a quem contar; avisar "alguma conversa" faria
	// a pessoa procurar o pedido onde ele não aconteceu. É o que acontece
	// quando o transporte não consegue dizer de quem é a extensão.
	h := handlerDeExtensao(nil, acp.TurnOwner{}, false)
	avisos := escutandoAvisos(h)

	resposta := respostaDaPergunta(t, mustHandle(t, h, pedidoDePergunta(t)))

	if resposta.Outcome.Outcome != askOutcomeSkipped {
		t.Errorf("desfecho = %q, quer %q", resposta.Outcome.Outcome, askOutcomeSkipped)
	}
	if eventos := avisos.find("chat:notice"); len(eventos) != 0 {
		t.Errorf("avisos = %d, quer 0", len(eventos))
	}
}

func TestPedidoSemDonoNaoDizQueNinguemEstavaNaTela(t *testing.T) {
	// Não saber de quem é o pedido é diferente de saber e não haver ninguém
	// vendo. O agente costuma repetir o motivo à pessoa: dizer "não havia
	// ninguém para responder" a quem está olhando a tela seria mentira.
	semDono := handlerDeExtensao(nil, acp.TurnOwner{}, false)
	semTela := handlerDeExtensao(nil, acp.TurnOwner{ConversationID: "c"}, true)

	pergunta := respostaDaPergunta(t, mustHandle(t, semDono, pedidoDePergunta(t)))
	if pergunta.Outcome.Reason == reasonNoWatcher {
		t.Errorf("motivo = %q, quer um que não afirme que ninguém estava na tela", pergunta.Outcome.Reason)
	}
	if outro := respostaDaPergunta(t, mustHandle(t, semTela, pedidoDePergunta(t))); outro.Outcome.Reason != reasonNoWatcher {
		t.Errorf("motivo do turno sem tela = %q, quer %q", outro.Outcome.Reason, reasonNoWatcher)
	}

	plano := respostaDoPlano(t, mustHandle(t, semDono, pedidoDePlano(t)))
	if plano.Outcome.Reason == reasonNoWatcher {
		t.Errorf("motivo = %q, quer um que não afirme que ninguém estava na tela", plano.Outcome.Reason)
	}
}

func TestPerguntaSemOpcaoNaoInventaResposta(t *testing.T) {
	tela := novaTelaDeExtensao(escolhendoAPrimeira())
	h := handlerDeExtensao(tela, acp.TurnOwner{ConversationID: "c", Interactive: true}, true)
	avisos := escutandoAvisos(h)

	pedido := pedidoCom(t, methodAskQuestion, map[string]any{
		"toolCallId": "call-1",
		"questions": []any{
			map[string]any{"id": "q1", "prompt": "Qual caminho seguir?", "options": []any{}},
		},
	})

	resposta := respostaDaPergunta(t, mustHandle(t, h, pedido))

	if resposta.Outcome.Outcome != askOutcomeSkipped {
		t.Errorf("desfecho = %q, quer %q", resposta.Outcome.Outcome, askOutcomeSkipped)
	}
	if tela.quantosDialogos() != 0 {
		t.Error("abriu diálogo sem ter o que oferecer")
	}
	if aviso := avisoNaConversa(t, avisos); aviso.Kind != ports.ChatNoticeKindQuestionUnavailable {
		t.Errorf("motivo do aviso = %q, quer %q", aviso.Kind, ports.ChatNoticeKindQuestionUnavailable)
	}
}

func TestANumeracaoContaSoAsPerguntasQueAparecem(t *testing.T) {
	// A do meio não tem opção e fica de fora. Numerar pela lista do agente
	// diria "Pergunta 3 de 3" na segunda tela de duas, e quem ouve iria
	// procurar a que não está lá.
	tela := novaTelaDeExtensao(escolhendoAPrimeira())
	h := handlerDeExtensao(tela, acp.TurnOwner{ConversationID: "c", Interactive: true}, true)

	comOpcao := func(id, prompt string) map[string]any {
		return map[string]any{
			"id": id, "prompt": prompt,
			"options": []any{map[string]any{"id": "a", "label": "Sim"}},
		}
	}
	pedido := pedidoCom(t, methodAskQuestion, map[string]any{
		"toolCallId": "call-1",
		"questions": []any{
			comOpcao("q1", "Primeira?"),
			map[string]any{"id": "q2", "prompt": "Sem opção?", "options": []any{}},
			comOpcao("q3", "Segunda?"),
		},
	})

	mustHandle(t, h, pedido)

	rotulos := rotulosDosBlocos(tela.ultimoDialogo(t), askPromptPrefix)
	quer := []string{"Pergunta 1 de 2", "Pergunta 2 de 2"}
	if len(rotulos) != len(quer) {
		t.Fatalf("blocos na tela = %q, quer %q", rotulos, quer)
	}
	for i, rotulo := range rotulos {
		if rotulo != quer[i] {
			t.Errorf("bloco %d = %q, quer %q", i, rotulo, quer[i])
		}
	}
}

func TestSemQuestionarioOPlanoEhRecusadoComAviso(t *testing.T) {
	h := handlerDeExtensao(nil, acp.TurnOwner{ConversationID: "c", Interactive: true}, true)
	avisos := escutandoAvisos(h)

	resposta := respostaDoPlano(t, mustHandle(t, h, pedidoDePlano(t)))

	if resposta.Outcome.Outcome != planOutcomeRejected {
		t.Errorf("desfecho = %q, quer %q", resposta.Outcome.Outcome, planOutcomeRejected)
	}
	if aviso := avisoNaConversa(t, avisos); aviso.Kind != ports.ChatNoticeKindPlanUnavailable {
		t.Errorf("motivo do aviso = %q, quer %q", aviso.Kind, ports.ChatNoticeKindPlanUnavailable)
	}
}

func TestPedidoIlegivelNaoPenduraOAgente(t *testing.T) {
	h := handlerDeExtensao(nil, acp.TurnOwner{ConversationID: "c", Interactive: true}, true)

	pergunta, tratou := h.HandleCustom(context.Background(),
		acp.CustomRequest{Method: methodAskQuestion, Params: json.RawMessage("isto não é json")})
	if !tratou {
		t.Fatal("o método é tratado aqui mesmo quando o corpo não se entende")
	}
	if resposta := respostaDaPergunta(t, pergunta); resposta.Outcome.Outcome != askOutcomeSkipped {
		t.Errorf("desfecho = %q, quer %q", resposta.Outcome.Outcome, askOutcomeSkipped)
	}

	plano, _ := h.HandleCustom(context.Background(),
		acp.CustomRequest{Method: methodCreatePlan, Params: json.RawMessage("{")})
	if resposta := respostaDoPlano(t, plano); resposta.Outcome.Outcome != planOutcomeRejected {
		t.Errorf("desfecho = %q, quer %q", resposta.Outcome.Outcome, planOutcomeRejected)
	}
}

func TestPlanoAprovadoNaTelaSegueParaOAgente(t *testing.T) {
	tela := novaTelaDeExtensao(respondendo(func([]string) any { return planApproveLabel }))
	h := handlerDeExtensao(tela, acp.TurnOwner{ConversationID: "c", Interactive: true}, true)

	resposta := respostaDoPlano(t, mustHandle(t, h, pedidoDePlano(t)))

	if resposta.Outcome.Outcome != planOutcomeAccepted {
		t.Fatalf("desfecho = %q, quer %q", resposta.Outcome.Outcome, planOutcomeAccepted)
	}
	plano := blocoNaTela(t, tela.ultimoDialogo(t), planContentID)
	if !strings.Contains(plano, "Adicionar TOTP") || !strings.Contains(plano, "Validar o código no login") {
		t.Errorf("plano na tela = %q, quer o que está sendo aprovado", plano)
	}
}

func TestPlanoRecusadoNaTelaVoltaRecusadoAoAgente(t *testing.T) {
	tela := novaTelaDeExtensao(respondendo(func([]string) any { return planRejectLabel }))
	h := handlerDeExtensao(tela, acp.TurnOwner{ConversationID: "c", Interactive: true}, true)
	avisos := escutandoAvisos(h)

	resposta := respostaDoPlano(t, mustHandle(t, h, pedidoDePlano(t)))

	if resposta.Outcome.Outcome != planOutcomeRejected {
		t.Errorf("desfecho = %q, quer %q", resposta.Outcome.Outcome, planOutcomeRejected)
	}
	if eventos := avisos.find("chat:notice"); len(eventos) != 0 {
		t.Errorf("avisos = %d, quer 0: recusar por escolha é decisão, não surpresa", len(eventos))
	}
}

func TestSairPeloBotaoDeRecusarERecusarOPlano(t *testing.T) {
	// O botão de cancelar deste diálogo se chama "Recusar". O agente costuma
	// repetir à pessoa o motivo que recebe: dizer que ela "dispensou o pedido"
	// contaria de volta uma coisa diferente da que ela fez.
	tela := novaTelaDeExtensao(nil) // saiu pelo cancelar
	h := handlerDeExtensao(tela, acp.TurnOwner{ConversationID: "c", Interactive: true}, true)

	resposta := respostaDoPlano(t, mustHandle(t, h, pedidoDePlano(t)))

	if resposta.Outcome.Outcome != planOutcomeRejected {
		t.Fatalf("desfecho = %q, quer %q", resposta.Outcome.Outcome, planOutcomeRejected)
	}
	escolhido := respostaDoPlano(t, mustHandle(t,
		handlerDeExtensao(
			novaTelaDeExtensao(respondendo(func([]string) any { return planRejectLabel })),
			acp.TurnOwner{ConversationID: "c", Interactive: true}, true),
		pedidoDePlano(t)))
	if resposta.Outcome.Reason != escolhido.Outcome.Reason {
		t.Errorf("motivo do cancelar = %q, quer o mesmo de escolher recusar (%q)",
			resposta.Outcome.Reason, escolhido.Outcome.Reason)
	}
	if rotulo := textoDoDialogo(tela.ultimoDialogo(t), "cancelLabel"); rotulo != "Recusar" {
		t.Errorf("rótulo do cancelar = %q, quer o que o motivo diz", rotulo)
	}
}

func TestDialogoDispensadoPulaAPerguntaSemAvisar(t *testing.T) {
	tela := novaTelaDeExtensao(nil) // ninguém escolheu: a janela foi fechada
	h := handlerDeExtensao(tela, acp.TurnOwner{ConversationID: "c", Interactive: true}, true)
	avisos := escutandoAvisos(h)

	resposta := respostaDaPergunta(t, mustHandle(t, h, pedidoDePergunta(t)))

	if resposta.Outcome.Outcome != askOutcomeSkipped {
		t.Errorf("desfecho = %q, quer %q", resposta.Outcome.Outcome, askOutcomeSkipped)
	}
	if eventos := avisos.find("chat:notice"); len(eventos) != 0 {
		t.Errorf("avisos = %d, quer 0: dispensar é decisão de quem estava lendo", len(eventos))
	}
}

func TestOTextoDoAgenteEhSaneadoAntesDeIrParaATela(t *testing.T) {
	tela := novaTelaDeExtensao(escolhendoAPrimeira())
	h := handlerDeExtensao(tela, acp.TurnOwner{ConversationID: "c", Interactive: true}, true)

	pedido := pedidoCom(t, methodAskQuestion, map[string]any{
		"toolCallId": "call-1",
		"title":      "\x1b[31mEscolher\x1b[0m",
		"questions": []any{
			map[string]any{
				"id":     "q1",
				"prompt": "\x1b[31mQual banco?\x1b[0m\nsegunda linha",
				"options": []any{
					map[string]any{"id": "a", "label": "SQLite\x1b[1m"},
					map[string]any{"id": "b", "label": "Postgres"},
				},
			},
		},
	})

	mustHandle(t, h, pedido)

	dialogo := tela.ultimoDialogo(t)
	pergunta := blocoNaTela(t, dialogo, askPromptPrefix)
	if strings.Contains(pergunta, "\x1b") {
		t.Errorf("pergunta na tela = %q, quer o texto do agente saneado", pergunta)
	}
	if !strings.Contains(pergunta, "Qual banco?\nsegunda linha") {
		// Achatar num parágrafo só mudaria o que a pergunta parece ser.
		t.Errorf("pergunta na tela = %q, quer as quebras de linha preservadas", pergunta)
	}
	if descricao := textoDoDialogo(dialogo, "description"); strings.Contains(descricao, "\x1b") {
		t.Errorf("descrição = %q, quer o assunto saneado", descricao)
	}
	for _, opcao := range opcoesNaTela(dialogo) {
		if strings.Contains(opcao, "\x1b") {
			t.Errorf("opção na tela = %q, quer o rótulo saneado", opcao)
		}
	}
}

func TestAPessoaLeAPerguntaInteiraAntesDeResponder(t *testing.T) {
	// O saneamento de rótulo corta em 200 runas para caber num anúncio. Aqui
	// o corte esconderia o fim da pergunta — que é onde costuma estar a
	// condição que muda a resposta.
	tela := novaTelaDeExtensao(escolhendoAPrimeira())
	h := handlerDeExtensao(tela, acp.TurnOwner{ConversationID: "c", Interactive: true}, true)

	pedido := pedidoCom(t, methodAskQuestion, map[string]any{
		"toolCallId": "call-1",
		"questions": []any{
			map[string]any{
				"id":      "q1",
				"prompt":  strings.Repeat("contexto ", 60) + "apago o banco de produção?",
				"options": []any{map[string]any{"id": "sim", "label": "Sim"}},
			},
		},
	})

	mustHandle(t, h, pedido)

	pergunta := blocoNaTela(t, tela.ultimoDialogo(t), askPromptPrefix)
	if !strings.HasSuffix(pergunta, "apago o banco de produção?") {
		t.Errorf("pergunta na tela terminou em %q: a pessoa responderia o que não leu",
			pergunta[max(0, len(pergunta)-40):])
	}
}

func TestOPlanoInteiroVaiParaATela(t *testing.T) {
	tela := novaTelaDeExtensao(respondendo(func([]string) any { return planRejectLabel }))
	h := handlerDeExtensao(tela, acp.TurnOwner{ConversationID: "c", Interactive: true}, true)

	pedido := pedidoCom(t, methodCreatePlan, map[string]any{
		"toolCallId": "call-2",
		"name":       "Migração",
		"plan":       strings.Repeat("passo detalhado. ", 40) + "e por fim apagar a tabela antiga.",
		"phases": []any{
			map[string]any{
				"name":  "Preparação",
				"todos": []any{map[string]any{"id": "t1", "content": "Fazer backup", "status": "pending"}},
			},
		},
	})

	mustHandle(t, h, pedido)

	plano := blocoNaTela(t, tela.ultimoDialogo(t), planContentID)
	if !strings.Contains(plano, "apagar a tabela antiga") {
		t.Error("o fim do plano não chegou à tela: a pessoa aprovaria o que não viu")
	}
	if !strings.Contains(plano, "Preparação") || !strings.Contains(plano, "Fazer backup") {
		t.Errorf("plano na tela = %q, quer as fases e os passos", plano)
	}
}

func TestOpcoesComOMesmoRotuloNaoViramAMesmaEscolha(t *testing.T) {
	// A tela devolve o rótulo escolhido. Com dois iguais, a resposta apontaria
	// sempre para o primeiro, e a pessoa mandaria ao agente uma escolha que
	// não fez.
	var oferecidas []string
	tela := novaTelaDeExtensao(func(itens []questionnaire.Question) map[string]any {
		respostas := map[string]any{}
		for _, item := range itens {
			if item.Type != "single_choice" {
				continue
			}
			oferecidas = questionnaire.TextValues(item.Options)
			respostas[item.ID] = oferecidas[1]
		}
		return respostas
	})
	h := handlerDeExtensao(tela, acp.TurnOwner{ConversationID: "c", Interactive: true}, true)

	pedido := pedidoCom(t, methodAskQuestion, map[string]any{
		"toolCallId": "call-1",
		"questions": []any{
			map[string]any{
				"id":     "q1",
				"prompt": "Qual?",
				"options": []any{
					map[string]any{"id": "a", "label": "Mesma coisa"},
					map[string]any{"id": "b", "label": "Mesma coisa"},
				},
			},
		},
	})

	resposta := respostaDaPergunta(t, mustHandle(t, h, pedido))

	if len(oferecidas) != 2 || oferecidas[0] == oferecidas[1] {
		t.Fatalf("opções na tela = %q, quer rótulos distinguíveis", oferecidas)
	}
	if escolhidas := resposta.Outcome.Answers[0].SelectedOptionIDs; len(escolhidas) != 1 || escolhidas[0] != "b" {
		t.Errorf("escolha = %v, quer a segunda opção", escolhidas)
	}
}

func TestOIdentificadorDaOpcaoVoltaComoOAgenteOMandou(t *testing.T) {
	// O agente só reconhece a opção escrita como ele a ofereceu: um
	// identificador aparado aqui não bateria com nada do lado dele.
	tela := novaTelaDeExtensao(escolhendoAPrimeira())
	h := handlerDeExtensao(tela, acp.TurnOwner{ConversationID: "c", Interactive: true}, true)

	pedido := pedidoCom(t, methodAskQuestion, map[string]any{
		"toolCallId": "call-1",
		"questions": []any{
			map[string]any{
				"id":      "q1",
				"prompt":  "Qual?",
				"options": []any{map[string]any{"id": " opcao-1 ", "label": "Primeira"}},
			},
		},
	})

	resposta := respostaDaPergunta(t, mustHandle(t, h, pedido))

	if escolhidas := resposta.Outcome.Answers[0].SelectedOptionIDs; len(escolhidas) != 1 || escolhidas[0] != " opcao-1 " {
		t.Errorf("escolha = %v, quer o identificador como o agente o mandou", escolhidas)
	}
}

func TestDesfechoDeUltimaHoraEhODeCadaMetodo(t *testing.T) {
	// É o que o transporte pede quando o teto de tempo dele estoura ou o
	// handler quebra. Sem esse desfecho ele responde erro interno, e o agente
	// conclui que o app falhou em vez de entender que a resposta foi não.
	h := handlerDeExtensao(nil, acp.TurnOwner{}, false)

	pergunta, ok := h.CustomFallback(methodAskQuestion)
	if !ok {
		t.Fatal("a pergunta ficou sem desfecho de última hora")
	}
	if resposta := respostaDaPergunta(t, pergunta); resposta.Outcome.Outcome != askOutcomeSkipped {
		t.Errorf("desfecho da pergunta = %q, quer %q", resposta.Outcome.Outcome, askOutcomeSkipped)
	}

	plano, ok := h.CustomFallback(methodCreatePlan)
	if !ok {
		t.Fatal("o plano ficou sem desfecho de última hora")
	}
	if resposta := respostaDoPlano(t, plano); resposta.Outcome.Outcome != planOutcomeRejected {
		t.Errorf("desfecho do plano = %q, quer %q", resposta.Outcome.Outcome, planOutcomeRejected)
	}

	if _, ok := h.CustomFallback("cursor/algo_que_nao_tratamos"); ok {
		t.Error("ofereceu desfecho para um método que não implementa")
	}
}

func TestMetodoQueNaoTratamosContinuaSemSuporte(t *testing.T) {
	// Dizer que tratou faria o agente esperar uma resposta que o app não sabe
	// dar; "método não encontrado" o desbloqueia sem fingir suporte.
	h := handlerDeExtensao(nil, acp.TurnOwner{ConversationID: "c", Interactive: true}, true)

	if _, tratou := h.HandleCustom(context.Background(),
		acp.CustomRequest{Method: "cursor/generate_image"}); tratou {
		t.Error("o handler disse tratar uma extensão que não trata")
	}
}

func TestOLogNaoGuardaOQueOAgentePerguntouNemOPlano(t *testing.T) {
	// A pergunta e o plano falam do trabalho da pessoa e podem carregar
	// caminho, segredo e nome de cliente. Na tela eles aparecem inteiros; no
	// log fica só o que identifica o pedido.
	pergunta := askLogSummary("sessao-1", askQuestionRequest{
		ToolCallID: "call-1",
		Title:      "segredo-do-cliente",
		Questions: []askQuestion{
			{ID: "q1", Prompt: "Posso usar a chave segredo-do-cliente?"},
		},
	})
	if strings.Contains(pergunta, "segredo") {
		t.Errorf("registro = %q, quer o pedido sem o que o agente escreveu", pergunta)
	}
	if !strings.Contains(pergunta, "call-1") || !strings.Contains(pergunta, "1 pergunta") {
		t.Errorf("registro = %q, quer identificar o pedido", pergunta)
	}

	plano := planLogSummary("sessao-1", createPlanRequest{
		ToolCallID: "call-2",
		Name:       "segredo-do-cliente",
		Plan:       "apagar /home/segredo-do-cliente",
		Todos:      []planTodo{{Content: "passo"}},
	})
	if strings.Contains(plano, "segredo") {
		t.Errorf("registro = %q, quer o pedido sem o que o agente escreveu", plano)
	}
	if !strings.Contains(plano, "call-2") || !strings.Contains(plano, "1 passo") {
		t.Errorf("registro = %q, quer identificar o pedido", plano)
	}
}

// mustHandle trata o pedido e exige que ele tenha sido tratado aqui.
func mustHandle(tb testing.TB, h *acpRequestHandler, req acp.CustomRequest) any {
	tb.Helper()
	return mustHandleCtx(tb, context.Background(), h, req)
}

func mustHandleCtx(tb testing.TB, ctx context.Context, h *acpRequestHandler, req acp.CustomRequest) any {
	tb.Helper()
	resultado, tratou := h.HandleCustom(ctx, req)
	if !tratou {
		tb.Fatalf("o método %q não foi tratado", req.Method)
	}
	return resultado
}
