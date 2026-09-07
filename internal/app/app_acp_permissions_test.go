package app

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"assistente/internal/acp"
	"assistente/internal/acptrust"
	"assistente/internal/core/ports"
	"assistente/internal/questionnaire"
)

// telaFalsa é o questionário do desktop com a pessoa do outro lado: guarda o
// que foi perguntado e responde o que o teste combinou.
type telaFalsa struct {
	mu        sync.Mutex
	manager   *questionnaire.Manager
	perguntas []map[string]any
	fechados  []fechamentoNaTela
	// escolhe recebe as opções oferecidas e devolve a escolhida; nulo cancela
	// o diálogo, como quem fecha a janela.
	escolhe func(opcoes []string) string
	// muda deixa o diálogo aberto sem resposta, como quem ainda está lendo.
	muda      bool
	perguntou chan struct{}
	fechou    chan struct{}
}

// fechamentoNaTela é o aviso de que um diálogo saiu da tela sem resposta.
type fechamentoNaTela struct {
	id     string
	motivo string
}

func novaTelaFalsa(escolhe func(opcoes []string) string) *telaFalsa {
	tela := &telaFalsa{
		escolhe:   escolhe,
		perguntou: make(chan struct{}, 1),
		fechou:    make(chan struct{}, 1),
	}
	tela.manager = questionnaire.NewManager(tela.aoPerguntar)
	return tela
}

// novaTelaMuda é a pessoa que abriu o diálogo e ainda não decidiu nada.
func novaTelaMuda() *telaFalsa {
	tela := novaTelaFalsa(nil)
	tela.muda = true
	return tela
}

// aoPerguntar responde aos eventos do questionário como o frontend faria.
func (t *telaFalsa) aoPerguntar(event string, data any) {
	payload, ok := data.(map[string]any)
	if !ok {
		return
	}
	if event == questionnaire.EventQuestionnaireClosed {
		id, _ := payload["id"].(string)
		motivo, _ := payload["reason"].(string)
		t.mu.Lock()
		t.fechados = append(t.fechados, fechamentoNaTela{id: id, motivo: motivo})
		t.mu.Unlock()
		avisar(t.fechou)
		return
	}

	t.mu.Lock()
	t.perguntas = append(t.perguntas, payload)
	t.mu.Unlock()
	avisar(t.perguntou)

	if t.muda {
		return
	}
	id, _ := payload["id"].(string)
	opcoes := t.opcoesDe(payload)
	go func() {
		if t.escolhe == nil {
			_ = t.manager.Respond(id, nil, true)
			return
		}
		escolha := t.escolhe(opcoes)
		_ = t.manager.Respond(id, decisionAnswers(payload, escolha), false)
	}()
}

// decisionAnswers monta Answers no contrato atual (actionId) ou legado (rótulo).
func decisionAnswers(payload map[string]any, escolha string) map[string]any {
	kind, _ := payload["kind"].(string)
	if kind == questionnaire.KindDecision {
		return map[string]any{questionnaire.AnswerActionID: actionIDDaEscolha(payload, escolha)}
	}
	return map[string]any{permissionAnswerID: escolha}
}

func actionIDDaEscolha(payload map[string]any, escolha string) string {
	for _, action := range actionsDe(payload) {
		if action.ID == escolha || action.Label.String() == escolha {
			return action.ID
		}
	}
	return escolha
}

func actionsDe(payload map[string]any) []questionnaire.DecisionAction {
	actions, _ := payload["actions"].([]questionnaire.DecisionAction)
	return actions
}

func avisar(ch chan struct{}) {
	select {
	case ch <- struct{}{}:
	default:
	}
}

func (t *telaFalsa) esperarPergunta(tb testing.TB) {
	tb.Helper()
	select {
	case <-t.perguntou:
	case <-time.After(2 * time.Second):
		tb.Fatal("o diálogo não chegou à tela")
	}
}

func (t *telaFalsa) esperarFechamento(tb testing.TB) fechamentoNaTela {
	tb.Helper()
	select {
	case <-t.fechou:
	case <-time.After(2 * time.Second):
		tb.Fatal("o diálogo ficou na tela pedindo uma decisão que já não vale")
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.fechados[len(t.fechados)-1]
}

// opcoesDe devolve o que a pessoa vê e escolhe: rótulos das ações (decision)
// ou das opções de rádio (legado).
func (t *telaFalsa) opcoesDe(payload map[string]any) []string {
	kind, _ := payload["kind"].(string)
	if kind == questionnaire.KindDecision {
		out := make([]string, 0, len(actionsDe(payload)))
		for _, action := range actionsDe(payload) {
			out = append(out, action.Label.String())
		}
		return out
	}
	for _, pergunta := range perguntasDe(payload) {
		if pergunta.ID == permissionAnswerID {
			return questionnaire.TextValues(pergunta.Options)
		}
	}
	return nil
}

func perguntasDe(payload map[string]any) []questionnaire.Question {
	perguntas, _ := payload["questions"].([]questionnaire.Question)
	return perguntas
}

// textoDoDialogo lê um texto do diálogo como a tela o exibiria: os campos
// visíveis são questionnaire.Text (chave de tradução + texto pronto).
func textoDoDialogo(payload map[string]any, campo string) string {
	texto, _ := payload[campo].(questionnaire.Text)
	return texto.String()
}

// acaoNaTela é o texto do bloco que a pessoa lê antes de decidir.
func acaoNaTela(tb testing.TB, payload map[string]any) string {
	tb.Helper()
	if body, ok := payload["body"].(string); ok && body != "" {
		return body
	}
	for _, pergunta := range perguntasDe(payload) {
		if pergunta.ID == permissionActionID {
			return pergunta.Content
		}
	}
	tb.Fatal("o diálogo não mostrou a ação pedida")
	return ""
}

func (t *telaFalsa) ultimaPergunta(tb testing.TB) map[string]any {
	tb.Helper()
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.perguntas) == 0 {
		tb.Fatal("nada foi perguntado à pessoa")
	}
	return t.perguntas[len(t.perguntas)-1]
}

func (t *telaFalsa) quantasPerguntas() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.perguntas)
}

// handlerCom monta o handler sobre uma tela e um turno de origem conhecida. Sem
// canal ligado: o turno sem tela continua sem ninguém a quem perguntar, que é o
// comportamento de sempre do desktop.
func handlerCom(tela *telaFalsa, owner acp.TurnOwner, temTurno bool) *acpRequestHandler {
	h := &acpRequestHandler{
		owner: func(string) (acp.TurnOwner, bool) { return owner, temTurno },
	}
	if tela != nil {
		h.questions = func() *questionnaire.Manager { return tela.manager }
	}
	// O questionário é lido na hora do uso, como em produção: teste que troca a
	// tela depois de montar o handler continua valendo.
	h.surfaces = questionnaire.NewRouter(h.questionnaireManager, nil)
	return h
}

// lembrandoAutorizacoes dá ao handler onde guardar o "permitir sempre", num
// diretório que só existe durante o teste.
func lembrandoAutorizacoes(tb testing.TB, h *acpRequestHandler, perfilAtivo string) *acptrust.Store {
	tb.Helper()
	store := acptrust.NewStoreWithDir(tb.TempDir())
	h.trust = func() *acptrust.Store { return store }
	h.activeProfile = func() string { return perfilAtivo }
	return store
}

// escutandoAvisos liga o handler à conversa para ler o que ela recebe.
func escutandoAvisos(h *acpRequestHandler) *testEmitter {
	emitter := &testEmitter{}
	h.notices = func() ports.Emitter { return emitter }
	return emitter
}

// avisoNaConversa devolve o único aviso recebido, falhando se houver outro
// número deles.
func avisoNaConversa(tb testing.TB, emitter *testEmitter) ports.ChatNoticeEvent {
	tb.Helper()
	eventos := emitter.find("chat:notice")
	if len(eventos) != 1 {
		tb.Fatalf("avisos na conversa = %d, quer 1", len(eventos))
	}
	aviso, ok := eventos[0].data.(ports.ChatNoticeEvent)
	if !ok {
		tb.Fatalf("aviso veio como %T, quer ports.ChatNoticeEvent", eventos[0].data)
	}
	return aviso
}

func pedidoDeExecucao() acp.PermissionRequest {
	return acp.PermissionRequest{
		SessionID: "sessao-1",
		ToolCall:  acp.ToolCall{ID: "call-1", Kind: "execute", Title: "rm -rf build"},
		Options: []acp.PermissionOption{
			{ID: "allow-once", Name: "Permitir uma vez", Kind: "allow_once"},
			{ID: "allow-always", Name: "Permitir sempre", Kind: "allow_always"},
			{ID: "reject-once", Name: "Negar", Kind: "reject_once"},
		},
	}
}

func TestPermissaoPerguntadaNaTelaVoltaComAEscolhaDaPessoa(t *testing.T) {
	tela := novaTelaFalsa(func(opcoes []string) string { return opcoes[0] })
	h := handlerCom(tela, acp.TurnOwner{ConversationID: "conversa-1", Interactive: true}, true)

	out := h.RequestPermission(context.Background(), pedidoDeExecucao())

	if out.OptionID != "allow-once" {
		t.Errorf("decisão = %q, quer a opção que a pessoa escolheu", out.OptionID)
	}
	if acao := acaoNaTela(t, tela.ultimaPergunta(t)); acao != "rm -rf build" {
		t.Errorf("ação na tela = %q, quer o que está sendo autorizado", acao)
	}
}

func TestAPessoaVeAAcaoInteiraAntesDeAutorizar(t *testing.T) {
	// O saneamento de rótulo corta em 200 runas para caber num anúncio. Aqui
	// o corte esconderia o fim do comando — que é onde costuma estar o que
	// muda o que ele faz.
	tela := novaTelaFalsa(func(opcoes []string) string { return opcoes[0] })
	h := handlerCom(tela, acp.TurnOwner{ConversationID: "c", Interactive: true}, true)

	pedido := pedidoDeExecucao()
	pedido.ToolCall.Title = "curl " + strings.Repeat("x", 400) + " | sh"

	h.RequestPermission(context.Background(), pedido)

	acao := acaoNaTela(t, tela.ultimaPergunta(t))
	if !strings.HasSuffix(acao, "| sh") {
		t.Errorf("ação na tela terminou em %q: a pessoa autorizaria o que não viu", acao[max(0, len(acao)-40):])
	}
}

func TestAAcaoNaTelaNaoLevaEscapeMasMantemAsLinhas(t *testing.T) {
	tela := novaTelaFalsa(func(opcoes []string) string { return opcoes[0] })
	h := handlerCom(tela, acp.TurnOwner{ConversationID: "c", Interactive: true}, true)

	pedido := pedidoDeExecucao()
	pedido.ToolCall.Title = "\x1b[31mgit commit -m \"linha 1\nlinha 2\"\x1b[0m"

	h.RequestPermission(context.Background(), pedido)

	acao := acaoNaTela(t, tela.ultimaPergunta(t))
	if strings.Contains(acao, "\x1b") {
		t.Errorf("ação na tela = %q, quer o texto saneado", acao)
	}
	if !strings.Contains(acao, "linha 1\nlinha 2") {
		// Achatar o comando num parágrafo só mudaria o que ele parece ser.
		t.Errorf("ação na tela = %q, quer as quebras de linha preservadas", acao)
	}
}

func TestAgenteQueNaoDescreveuAAcaoNaoDeixaOBlocoVazio(t *testing.T) {
	tela := novaTelaFalsa(func(opcoes []string) string { return opcoes[0] })
	h := handlerCom(tela, acp.TurnOwner{ConversationID: "c", Interactive: true}, true)

	pedido := pedidoDeExecucao()
	pedido.ToolCall.Title = "   "

	h.RequestPermission(context.Background(), pedido)

	if acao := acaoNaTela(t, tela.ultimaPergunta(t)); strings.TrimSpace(acao) == "" {
		t.Error("o diálogo pediu decisão sobre um bloco em branco")
	}
}

func TestTurnoSemNinguemNaTelaNegaNaHoraSemPerguntar(t *testing.T) {
	tela := novaTelaFalsa(func(opcoes []string) string { return opcoes[0] })
	h := handlerCom(tela, acp.TurnOwner{ConversationID: "conversa-1"}, true)

	out := h.RequestPermission(context.Background(), pedidoDeExecucao())

	// Sem decisão o transporte responde a recusa pontual do próprio pedido.
	if out.OptionID != "" {
		t.Errorf("decisão = %q, quer nenhuma: não havia quem respondesse", out.OptionID)
	}
	if tela.quantasPerguntas() != 0 {
		t.Error("abriu diálogo para um turno que ninguém está vendo")
	}
}

func TestConversaSemTelaFicaSabendoDoQueFoiNegado(t *testing.T) {
	// Job agendado, canal, subagente: o agente segue o turno dizendo apenas
	// que não conseguiu. Sem este aviso ninguém saberia que houve um pedido,
	// muito menos por que ele não chegou a ninguém.
	tela := novaTelaFalsa(func(opcoes []string) string { return opcoes[0] })
	h := handlerCom(tela, acp.TurnOwner{ConversationID: "conversa-1"}, true)
	avisos := escutandoAvisos(h)

	h.RequestPermission(context.Background(), pedidoDeExecucao())

	aviso := avisoNaConversa(t, avisos)
	if aviso.ConversationID != "conversa-1" {
		t.Errorf("aviso foi para %q, quer a conversa dona do turno", aviso.ConversationID)
	}
	if aviso.Kind != ports.ChatNoticeKindPermissionNoWatcher {
		t.Errorf("motivo = %q, quer %q", aviso.Kind, ports.ChatNoticeKindPermissionNoWatcher)
	}
	if aviso.Action != "execute" {
		t.Errorf("ação = %q, quer a classe do que foi negado", aviso.Action)
	}
}

func TestAvisoNaoLevaOComandoDoAgenteParaAConversa(t *testing.T) {
	// O aviso pode parar numa conversa que ninguém está olhando agora, e linha
	// de comando carrega segredo em flag e em variável de ambiente. A classe
	// da ação diz o que houve sem guardar o que o agente escreveu.
	h := handlerCom(nil, acp.TurnOwner{ConversationID: "conversa-1"}, true)
	avisos := escutandoAvisos(h)

	pedido := pedidoDeExecucao()
	pedido.ToolCall.Title = "deploy --token=segredo-do-cliente"

	h.RequestPermission(context.Background(), pedido)

	aviso := avisoNaConversa(t, avisos)
	if strings.Contains(aviso.Action, "segredo") || strings.Contains(aviso.Kind, "segredo") {
		t.Errorf("o aviso levou o comando do agente: %+v", aviso)
	}
}

func TestClasseDeAcaoDesconhecidaNaoViraCodigoCruNaTela(t *testing.T) {
	h := handlerCom(nil, acp.TurnOwner{ConversationID: "conversa-1"}, true)
	avisos := escutandoAvisos(h)

	pedido := pedidoDeExecucao()
	pedido.ToolCall.Kind = "invocar_o_kraken"

	h.RequestPermission(context.Background(), pedido)

	if aviso := avisoNaConversa(t, avisos); aviso.Action != acp.ToolKindOther {
		t.Errorf("ação = %q, quer %q: o que o agente inventa não vai para a frase", aviso.Action, acp.ToolKindOther)
	}
}

func TestPrazoEstouradoAvisaAConversa(t *testing.T) {
	mudo := questionnaire.NewManager(func(string, any) {})
	h := handlerCom(nil, acp.TurnOwner{ConversationID: "conversa-1", Interactive: true}, true)
	h.questions = func() *questionnaire.Manager { return mudo }
	avisos := escutandoAvisos(h)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	h.RequestPermission(ctx, pedidoDeExecucao())

	if aviso := avisoNaConversa(t, avisos); aviso.Kind != ports.ChatNoticeKindPermissionTimeout {
		t.Errorf("motivo = %q, quer %q", aviso.Kind, ports.ChatNoticeKindPermissionTimeout)
	}
}

func TestQuemCancelouOTurnoNaoRecebeAvisoDoQueEleMesmoFez(t *testing.T) {
	// A pessoa desistiu e o diálogo já saiu da tela dizendo isso. Um aviso
	// aqui cobraria explicação de quem acabou de dar uma.
	tela := novaTelaMuda()
	h := handlerCom(tela, acp.TurnOwner{ConversationID: "conversa-1", Interactive: true}, true)
	avisos := escutandoAvisos(h)

	ctx, cancelarTurno := context.WithCancel(context.Background())
	pronto := make(chan acp.PermissionOutcome, 1)
	go func() { pronto <- h.RequestPermission(ctx, pedidoDeExecucao()) }()

	tela.esperarPergunta(t)
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

func TestCancelamentoCalaOAvisoAindaQueOErroNaoDigaAcausa(t *testing.T) {
	// A decisão não pode depender de o erro carregar a causa do contexto: se
	// esse elo se perder, quem cancelou o turno receberia um "ninguém
	// respondeu a tempo" logo depois de ter desistido.
	ctx, cancelar := context.WithCancel(context.Background())
	cancelar()

	if !turnCancelled(ctx, errors.New("solicitação encerrada sem resposta")) {
		t.Error("o cancelamento do turno passou despercebido")
	}
	if turnCancelled(context.Background(), errors.New("timeout aguardando respostas do usuário")) {
		t.Error("prazo estourado virou cancelamento: o aviso não sairia")
	}
}

func TestDecisaoDaPessoaNaoViraAviso(t *testing.T) {
	tela := novaTelaFalsa(func(opcoes []string) string { return opcoes[len(opcoes)-1] })
	h := handlerCom(tela, acp.TurnOwner{ConversationID: "conversa-1", Interactive: true}, true)
	avisos := escutandoAvisos(h)

	h.RequestPermission(context.Background(), pedidoDeExecucao())

	if eventos := avisos.find("chat:notice"); len(eventos) != 0 {
		t.Errorf("avisos = %d, quer 0: negar por escolha é decisão, não surpresa", len(eventos))
	}
}

func TestPedidoQueOAppNaoConsegueMostrarTambemEhAvisado(t *testing.T) {
	h := handlerCom(nil, acp.TurnOwner{ConversationID: "conversa-1", Interactive: true}, true)
	avisos := escutandoAvisos(h)

	h.RequestPermission(context.Background(), pedidoDeExecucao())

	if aviso := avisoNaConversa(t, avisos); aviso.Kind != ports.ChatNoticeKindPermissionUnavailable {
		t.Errorf("motivo = %q, quer %q", aviso.Kind, ports.ChatNoticeKindPermissionUnavailable)
	}
}

func TestPedidoSemDonoNaoAvisaConversaNenhuma(t *testing.T) {
	// Sem turno não há conversa a quem contar; avisar "alguma conversa" seria
	// pior do que o silêncio, porque a pessoa procuraria o pedido onde ele não
	// aconteceu.
	h := handlerCom(nil, acp.TurnOwner{}, false)
	avisos := escutandoAvisos(h)

	h.RequestPermission(context.Background(), pedidoDeExecucao())

	if eventos := avisos.find("chat:notice"); len(eventos) != 0 {
		t.Errorf("avisos = %d, quer 0", len(eventos))
	}
}

func TestPedidoDeSessaoSemTurnoNaoAbreDialogo(t *testing.T) {
	tela := novaTelaFalsa(func(opcoes []string) string { return opcoes[0] })
	h := handlerCom(tela, acp.TurnOwner{}, false)

	if out := h.RequestPermission(context.Background(), pedidoDeExecucao()); out.OptionID != "" {
		t.Errorf("decisão = %q, quer nenhuma", out.OptionID)
	}
	if tela.quantasPerguntas() != 0 {
		t.Error("perguntou sobre um turno que não existe")
	}
}

func TestDialogoFechadoSemEscolherNega(t *testing.T) {
	tela := novaTelaFalsa(nil) // ninguém escolheu: a janela foi fechada
	h := handlerCom(tela, acp.TurnOwner{ConversationID: "conversa-1", Interactive: true}, true)

	if out := h.RequestPermission(context.Background(), pedidoDeExecucao()); out.OptionID != "" {
		t.Errorf("decisão = %q, quer nenhuma", out.OptionID)
	}
}

func TestTurnoCanceladoTiraOPedidoDaTela(t *testing.T) {
	// Quem cancela o turno não deve continuar diante de um diálogo pedindo
	// autorização para uma ação que já não vai acontecer — e nada dali levaria
	// resposta ao agente, que também já desistiu do pedido.
	tela := novaTelaMuda()
	h := handlerCom(tela, acp.TurnOwner{ConversationID: "conversa-1", Interactive: true}, true)

	ctx, cancelarTurno := context.WithCancel(context.Background())
	pronto := make(chan acp.PermissionOutcome, 1)
	go func() { pronto <- h.RequestPermission(ctx, pedidoDeExecucao()) }()

	tela.esperarPergunta(t)
	aberta, _ := tela.ultimaPergunta(t)["id"].(string)
	cancelarTurno()

	select {
	case out := <-pronto:
		if out.OptionID != "" {
			t.Errorf("decisão = %q, quer nenhuma", out.OptionID)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("o handler não voltou depois do cancelamento")
	}

	fechado := tela.esperarFechamento(t)
	if fechado.id != aberta {
		t.Errorf("fechou o diálogo %q, quer o %q que estava aberto", fechado.id, aberta)
	}
	if fechado.motivo != questionnaire.ClosedCancelled {
		t.Errorf("motivo = %q, quer %q", fechado.motivo, questionnaire.ClosedCancelled)
	}
}

func TestPrazoEstouradoNaoPenduraOAgente(t *testing.T) {
	// Questionário que nunca é respondido: o handler precisa voltar mesmo
	// assim, senão o agente fica esperando até o teto do transporte.
	mudo := questionnaire.NewManager(func(string, any) {})
	h := handlerCom(nil, acp.TurnOwner{ConversationID: "c", Interactive: true}, true)
	h.questions = func() *questionnaire.Manager { return mudo }
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	pronto := make(chan acp.PermissionOutcome, 1)
	go func() { pronto <- h.RequestPermission(ctx, pedidoDeExecucao()) }()

	select {
	case out := <-pronto:
		if out.OptionID != "" {
			t.Errorf("decisão = %q, quer nenhuma", out.OptionID)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("o handler não voltou: o agente ficaria pendurado")
	}
}

func TestSemQuestionarioAPermissaoEhNegada(t *testing.T) {
	h := handlerCom(nil, acp.TurnOwner{ConversationID: "c", Interactive: true}, true)

	if out := h.RequestPermission(context.Background(), pedidoDeExecucao()); out.OptionID != "" {
		t.Errorf("decisão = %q, quer nenhuma", out.OptionID)
	}
}

func TestPedidoSemOpcaoNaoInventaResposta(t *testing.T) {
	tela := novaTelaFalsa(func(opcoes []string) string { return "" })
	h := handlerCom(tela, acp.TurnOwner{ConversationID: "c", Interactive: true}, true)

	pedido := pedidoDeExecucao()
	pedido.Options = []acp.PermissionOption{{ID: "  ", Name: "sem identificador"}}

	if out := h.RequestPermission(context.Background(), pedido); out.OptionID != "" {
		t.Errorf("decisão = %q, quer nenhuma", out.OptionID)
	}
	if tela.quantasPerguntas() != 0 {
		t.Error("abriu diálogo sem ter o que oferecer")
	}
}

func TestORotuloDoAgenteNaoLevaEscapeDeTerminalParaATela(t *testing.T) {
	var oferecidas []string
	tela := novaTelaFalsa(func(opcoes []string) string {
		oferecidas = opcoes
		return opcoes[0]
	})
	h := handlerCom(tela, acp.TurnOwner{ConversationID: "c", Interactive: true}, true)

	pedido := pedidoDeExecucao()
	pedido.ToolCall.Title = "\x1b[31mrm -rf /\x1b[0m\ncontinua na linha de baixo"
	pedido.Options = []acp.PermissionOption{{ID: "allow-once", Name: "Permitir\x1b[1m uma vez", Kind: "allow_once"}}

	h.RequestPermission(context.Background(), pedido)

	if acao := acaoNaTela(t, tela.ultimaPergunta(t)); strings.Contains(acao, "\x1b") {
		t.Errorf("ação na tela = %q, quer o texto do agente saneado", acao)
	}
	if len(oferecidas) != 1 || oferecidas[0] != "Permitir uma vez" {
		t.Errorf("opções na tela = %q, quer o rótulo saneado", oferecidas)
	}
}

func TestOpcoesComOMesmoNomeNaoViramAMesmaEscolha(t *testing.T) {
	// O questionário devolve o rótulo escolhido. Com dois rótulos iguais, a
	// resposta apontaria sempre para o primeiro — "permitir sempre" viraria
	// "permitir uma vez" sem ninguém notar.
	var oferecidas []string
	tela := novaTelaFalsa(func(opcoes []string) string {
		oferecidas = opcoes
		return opcoes[1]
	})
	h := handlerCom(tela, acp.TurnOwner{ConversationID: "c", Interactive: true}, true)

	pedido := pedidoDeExecucao()
	pedido.Options = []acp.PermissionOption{
		{ID: "allow-once", Name: "Permitir", Kind: "allow_once"},
		{ID: "allow-always", Name: "Permitir", Kind: "allow_always"},
	}

	out := h.RequestPermission(context.Background(), pedido)

	if oferecidas[0] == oferecidas[1] {
		t.Fatalf("opções na tela = %q, quer rótulos distinguíveis", oferecidas)
	}
	if out.OptionID != "allow-always" {
		t.Errorf("decisão = %q, quer a segunda opção", out.OptionID)
	}
}

func TestOIdentificadorVoltaAoAgenteComoEleOMandou(t *testing.T) {
	// O transporte só aceita a opção escrita como foi oferecida: um
	// identificador aparado aqui não bateria com nenhuma, e a escolha da
	// pessoa cairia na recusa.
	tela := novaTelaFalsa(func(opcoes []string) string { return opcoes[0] })
	h := handlerCom(tela, acp.TurnOwner{ConversationID: "c", Interactive: true}, true)

	pedido := pedidoDeExecucao()
	pedido.Options = []acp.PermissionOption{{ID: " allow-once ", Name: "Permitir uma vez", Kind: "allow_once"}}

	if out := h.RequestPermission(context.Background(), pedido); out.OptionID != " allow-once " {
		t.Errorf("decisão = %q, quer o identificador como o agente o mandou", out.OptionID)
	}
}

func TestIdentificadorQueDesempataRotuloTambemEhSaneado(t *testing.T) {
	var oferecidas []string
	tela := novaTelaFalsa(func(opcoes []string) string {
		oferecidas = opcoes
		return opcoes[1]
	})
	h := handlerCom(tela, acp.TurnOwner{ConversationID: "c", Interactive: true}, true)

	pedido := pedidoDeExecucao()
	pedido.Options = []acp.PermissionOption{
		{ID: "allow-once", Name: "Permitir", Kind: "allow_once"},
		{ID: "\x1b[31mallow-always", Name: "Permitir", Kind: "allow_always"},
	}

	out := h.RequestPermission(context.Background(), pedido)

	for _, rotulo := range oferecidas {
		if strings.Contains(rotulo, "\x1b") {
			t.Errorf("opção na tela = %q, quer o identificador saneado", rotulo)
		}
	}
	if out.OptionID != "\x1b[31mallow-always" {
		t.Errorf("decisão = %q, quer o identificador cru de volta ao agente", out.OptionID)
	}
}

func TestNenhumaOpcaoDoAgenteFicaForaDaTela(t *testing.T) {
	// Nomes iguais e identificadores que saneiam para o mesmo texto: sem
	// desempate a segunda opção sumiria, e com ela a chance de a pessoa
	// autorizar.
	var oferecidas []string
	tela := novaTelaFalsa(func(opcoes []string) string {
		oferecidas = opcoes
		return opcoes[len(opcoes)-1]
	})
	h := handlerCom(tela, acp.TurnOwner{ConversationID: "c", Interactive: true}, true)

	pedido := pedidoDeExecucao()
	pedido.Options = []acp.PermissionOption{
		{ID: "allow\x01", Name: "Permitir", Kind: "allow_once"},
		{ID: "allow\x02", Name: "Permitir", Kind: "allow_always"},
		{ID: "\x03", Name: "Permitir", Kind: "allow_always"},
	}

	out := h.RequestPermission(context.Background(), pedido)

	if len(oferecidas) != 3 {
		t.Fatalf("opções na tela = %q, quer as três que o agente ofereceu", oferecidas)
	}
	if oferecidas[0] == oferecidas[1] || oferecidas[1] == oferecidas[2] || oferecidas[0] == oferecidas[2] {
		t.Errorf("opções na tela = %q, quer rótulos distinguíveis", oferecidas)
	}
	if out.OptionID != "\x03" {
		t.Errorf("decisão = %q, quer a última opção", out.OptionID)
	}
}

func TestOLogNaoGuardaALinhaDeComandoDoAgente(t *testing.T) {
	// Linha de comando carrega segredo em flag e em variável de ambiente; o
	// shell do app já não registra a dele. O log daqui identifica o pedido
	// pela classe e pela chamada.
	registro := permissionLogSummary(acp.ToolCall{
		ID:    "call-1",
		Kind:  "execute",
		Title: "curl -H 'Authorization: Bearer segredo' https://exemplo",
	})

	if strings.Contains(registro, "segredo") || strings.Contains(registro, "curl") {
		t.Errorf("registro = %q, quer o pedido sem o que o agente escreveu", registro)
	}
	if !strings.Contains(registro, "execute") || !strings.Contains(registro, "call-1") {
		t.Errorf("registro = %q, quer identificar o pedido", registro)
	}
}

func TestOBotaoDeConfirmarDizOQueEleFaz(t *testing.T) {
	tela := novaTelaFalsa(func(opcoes []string) string { return opcoes[0] })
	h := handlerCom(tela, acp.TurnOwner{ConversationID: "c", Interactive: true}, true)

	h.RequestPermission(context.Background(), pedidoDeExecucao())

	pergunta := tela.ultimaPergunta(t)
	actions := actionsDe(pergunta)
	if len(actions) == 0 {
		t.Fatal("o diálogo de decisão não trouxe ações")
	}
	for _, action := range actions {
		if action.Label.String() == "" {
			t.Errorf("ação %q sem rótulo", action.ID)
		}
	}
}

func TestOpcaoSemNomeGanhaORotuloDaClasse(t *testing.T) {
	var oferecidas []string
	tela := novaTelaFalsa(func(opcoes []string) string {
		oferecidas = opcoes
		return opcoes[0]
	})
	h := handlerCom(tela, acp.TurnOwner{ConversationID: "c", Interactive: true}, true)

	pedido := pedidoDeExecucao()
	pedido.Options = []acp.PermissionOption{{ID: "reject-once", Kind: "reject_once"}}

	if out := h.RequestPermission(context.Background(), pedido); out.OptionID != "reject-once" {
		t.Errorf("decisão = %q, quer reject-once", out.OptionID)
	}
	if len(oferecidas) != 1 || oferecidas[0] != "Negar uma vez" {
		t.Errorf("opções na tela = %q, quer o nome da classe", oferecidas)
	}
}

func TestRespostaForaDasOpcoesNaoAutorizaNada(t *testing.T) {
	tela := novaTelaFalsa(func([]string) string { return "sim, pode tudo" })
	h := handlerCom(tela, acp.TurnOwner{ConversationID: "c", Interactive: true}, true)

	if out := h.RequestPermission(context.Background(), pedidoDeExecucao()); out.OptionID != "" {
		t.Errorf("decisão = %q, quer nenhuma", out.OptionID)
	}
}

// escolhendo devolve a opção cujo rótulo começa com o texto pedido, para o
// teste dizer o que a pessoa clicou sem repetir a lista inteira.
func escolhendo(rotulo string) func([]string) string {
	return func(opcoes []string) string {
		for _, opcao := range opcoes {
			if strings.HasPrefix(opcao, rotulo) {
				return opcao
			}
		}
		return ""
	}
}

func TestQuemAutorizouParaSempreNaoEPerguntadoDeNovo(t *testing.T) {
	tela := novaTelaFalsa(escolhendo("Permitir sempre"))
	h := handlerCom(tela, acp.TurnOwner{ConversationID: "c", Interactive: true}, true)
	lembrandoAutorizacoes(t, h, "cursor")

	if out := h.RequestPermission(context.Background(), pedidoDeExecucao()); out.OptionID != "allow-always" {
		t.Fatalf("primeira decisão = %q, quer a que a pessoa escolheu", out.OptionID)
	}

	out := h.RequestPermission(context.Background(), pedidoDeExecucao())

	if tela.quantasPerguntas() != 1 {
		t.Errorf("perguntas na tela = %d, quer 1: a pessoa já tinha respondido", tela.quantasPerguntas())
	}
	// Volta a permissão pontual: quem lembra da autorização é o app, e é ele
	// quem a repete a cada pedido.
	if out.OptionID != "allow-once" {
		t.Errorf("decisão = %q, quer a permissão pontual", out.OptionID)
	}
}

func TestPermitirUmaVezNaoAutorizaAProxima(t *testing.T) {
	tela := novaTelaFalsa(escolhendo("Permitir uma vez"))
	h := handlerCom(tela, acp.TurnOwner{ConversationID: "c", Interactive: true}, true)
	lembrandoAutorizacoes(t, h, "cursor")

	h.RequestPermission(context.Background(), pedidoDeExecucao())
	h.RequestPermission(context.Background(), pedidoDeExecucao())

	if tela.quantasPerguntas() != 2 {
		t.Errorf("perguntas na tela = %d, quer 2: só a desta vez foi autorizada", tela.quantasPerguntas())
	}
}

func TestAutorizarUmaClasseNaoLiberaAsOutras(t *testing.T) {
	tela := novaTelaFalsa(escolhendo("Permitir sempre"))
	h := handlerCom(tela, acp.TurnOwner{ConversationID: "c", Interactive: true}, true)
	lembrandoAutorizacoes(t, h, "cursor")
	h.RequestPermission(context.Background(), pedidoDeExecucao())

	// Autorizar comandos não pode liberar edição de arquivo.
	pedido := pedidoDeExecucao()
	pedido.ToolCall = acp.ToolCall{ID: "call-2", Kind: "edit", Title: "reescrever main.go"}
	h.RequestPermission(context.Background(), pedido)

	if tela.quantasPerguntas() != 2 {
		t.Errorf("perguntas na tela = %d, quer 2: a outra classe nunca foi autorizada", tela.quantasPerguntas())
	}
}

func TestAAutorizacaoDoTurnoEhADoPerfilQuePediuOTurno(t *testing.T) {
	// Canal e job dizem em qual perfil correm. Guardar tudo no perfil ativo do
	// desktop misturaria autorizações de agentes diferentes.
	tela := novaTelaFalsa(escolhendo("Permitir sempre"))
	h := handlerCom(tela, acp.TurnOwner{ConversationID: "c", Interactive: true, ProfileSlug: "cursor"}, true)
	store := lembrandoAutorizacoes(t, h, "outro-perfil")

	h.RequestPermission(context.Background(), pedidoDeExecucao())

	if !store.Allows("cursor", "execute") {
		t.Error("a autorização não ficou no perfil que pediu o turno")
	}
	if store.Allows("outro-perfil", "execute") {
		t.Error("a autorização foi parar no perfil ativo, e não no do turno")
	}
}

func TestSemPerfilNenhumOAgenteContinuaPerguntando(t *testing.T) {
	// Sem saber de quem é a autorização, guardá-la valeria para todos os
	// perfis — o oposto do que a pessoa autorizou.
	tela := novaTelaFalsa(escolhendo("Permitir sempre"))
	h := handlerCom(tela, acp.TurnOwner{ConversationID: "c", Interactive: true}, true)
	lembrandoAutorizacoes(t, h, "")

	h.RequestPermission(context.Background(), pedidoDeExecucao())
	h.RequestPermission(context.Background(), pedidoDeExecucao())

	if tela.quantasPerguntas() != 2 {
		t.Errorf("perguntas na tela = %d, quer 2: não havia onde guardar a autorização", tela.quantasPerguntas())
	}
}

func TestSemOndeGuardarOTurnoAindaSegueComOSimDaPessoa(t *testing.T) {
	// Não conseguir lembrar não é motivo para desautorizar o que a pessoa
	// acabou de permitir.
	tela := novaTelaFalsa(escolhendo("Permitir sempre"))
	h := handlerCom(tela, acp.TurnOwner{ConversationID: "c", Interactive: true}, true)

	if out := h.RequestPermission(context.Background(), pedidoDeExecucao()); out.OptionID != "allow-always" {
		t.Errorf("decisão = %q, quer a que a pessoa escolheu", out.OptionID)
	}
}

func TestAConversaFicaSabendoDaAutorizacaoPermanente(t *testing.T) {
	// O diálogo some assim que a pessoa responde, e com ele a única pista de
	// que o app passou a autorizar sozinho. O aviso fica na conversa para que
	// isso não vire uma mudança silenciosa de comportamento.
	tela := novaTelaFalsa(escolhendo("Permitir sempre"))
	h := handlerCom(tela, acp.TurnOwner{ConversationID: "conversa-1", Interactive: true}, true)
	lembrandoAutorizacoes(t, h, "cursor")
	emitter := escutandoAvisos(h)

	h.RequestPermission(context.Background(), pedidoDeExecucao())

	aviso := avisoNaConversa(t, emitter)
	if aviso.Kind != ports.ChatNoticeKindPermissionAlwaysAllowed {
		t.Errorf("aviso = %q, quer o da autorização permanente", aviso.Kind)
	}
	if aviso.ConversationID != "conversa-1" {
		t.Errorf("conversa do aviso = %q, quer a do turno", aviso.ConversationID)
	}
	if aviso.Action != "execute" {
		t.Errorf("classe no aviso = %q, quer a que ficou autorizada", aviso.Action)
	}
}

func TestPermitirUmaVezNaoAnunciaAutorizacaoPermanente(t *testing.T) {
	tela := novaTelaFalsa(escolhendo("Permitir uma vez"))
	h := handlerCom(tela, acp.TurnOwner{ConversationID: "c", Interactive: true}, true)
	lembrandoAutorizacoes(t, h, "cursor")
	emitter := escutandoAvisos(h)

	h.RequestPermission(context.Background(), pedidoDeExecucao())

	if avisos := emitter.find("chat:notice"); len(avisos) != 0 {
		t.Errorf("avisos na conversa = %d, quer 0: nada passou a valer além deste turno", len(avisos))
	}
}

func TestOSempreQueNaoPodeSerGuardadoEhContadoAConversa(t *testing.T) {
	// Quem escolheu "sempre" espera não ser perguntado de novo. Se o app não
	// conseguiu lembrar, a pergunta volta — e a pessoa precisa saber disso
	// antes de estranhar a repetição.
	tela := novaTelaFalsa(escolhendo("Permitir sempre"))
	h := handlerCom(tela, acp.TurnOwner{ConversationID: "c", Interactive: true}, true)
	emitter := escutandoAvisos(h)

	h.RequestPermission(context.Background(), pedidoDeExecucao())

	aviso := avisoNaConversa(t, emitter)
	if aviso.Kind != ports.ChatNoticeKindPermissionAlwaysNotSaved {
		t.Errorf("aviso = %q, quer o de autorização não guardada", aviso.Kind)
	}
}

func TestOAvisoDaAutorizacaoNaoSeRepeteACadaPedido(t *testing.T) {
	// Depois de guardada, a autorização passa a valer em silêncio: repetir o
	// aviso a cada pedido encheria a conversa de uma notícia velha.
	tela := novaTelaFalsa(escolhendo("Permitir sempre"))
	h := handlerCom(tela, acp.TurnOwner{ConversationID: "c", Interactive: true}, true)
	lembrandoAutorizacoes(t, h, "cursor")
	emitter := escutandoAvisos(h)

	h.RequestPermission(context.Background(), pedidoDeExecucao())
	h.RequestPermission(context.Background(), pedidoDeExecucao())

	if aviso := avisoNaConversa(t, emitter); aviso.Kind != ports.ChatNoticeKindPermissionAlwaysAllowed {
		t.Errorf("aviso = %q, quer o da autorização permanente", aviso.Kind)
	}
}

func TestODialogoDizAteOndeVaiOSempre(t *testing.T) {
	// "Permitir sempre" vale para a classe inteira, e não só para o comando
	// que está na tela. Quem não souber disso autoriza mais do que pretende.
	tela := novaTelaFalsa(escolhendo("Permitir uma vez"))
	h := handlerCom(tela, acp.TurnOwner{ConversationID: "c", Interactive: true}, true)
	lembrandoAutorizacoes(t, h, "cursor")

	h.RequestPermission(context.Background(), pedidoDeExecucao())

	descricao := textoDoDialogo(tela.ultimaPergunta(t), "description")
	if !strings.Contains(descricao, "permitir sempre") || !strings.Contains(descricao, "execute") {
		t.Errorf("descrição = %q, quer dizer o que o sempre abrange", descricao)
	}
}

func TestPedidoSemOpcaoDeSempreNaoFalaDeAutorizacaoPermanente(t *testing.T) {
	tela := novaTelaFalsa(func(opcoes []string) string { return opcoes[0] })
	h := handlerCom(tela, acp.TurnOwner{ConversationID: "c", Interactive: true}, true)
	lembrandoAutorizacoes(t, h, "cursor")

	pedido := pedidoDeExecucao()
	pedido.Options = []acp.PermissionOption{
		{ID: "allow-once", Name: "Permitir uma vez", Kind: "allow_once"},
		{ID: "reject-once", Name: "Negar", Kind: "reject_once"},
	}
	h.RequestPermission(context.Background(), pedido)

	descricao := textoDoDialogo(tela.ultimaPergunta(t), "description")
	if strings.Contains(descricao, "permitir sempre") {
		t.Errorf("descrição = %q, fala de uma opção que o agente não ofereceu", descricao)
	}
}

func TestSemOpcaoDePermitirAPerguntaVoltaParaATela(t *testing.T) {
	// A autorização é do app, mas quem responde é o agente: se ele não
	// ofereceu como dizer sim, não há o que responder no lugar dele.
	tela := novaTelaFalsa(escolhendo("Permitir sempre"))
	h := handlerCom(tela, acp.TurnOwner{ConversationID: "c", Interactive: true}, true)
	lembrandoAutorizacoes(t, h, "cursor")
	h.RequestPermission(context.Background(), pedidoDeExecucao())

	pedido := pedidoDeExecucao()
	pedido.Options = []acp.PermissionOption{{ID: "reject-once", Name: "Negar", Kind: "reject_once"}}
	h.RequestPermission(context.Background(), pedido)

	if tela.quantasPerguntas() != 2 {
		t.Errorf("perguntas na tela = %d, quer 2: não havia como dizer sim", tela.quantasPerguntas())
	}
}

func TestClasseQueOAgenteNaoNomeouNaoViraAutorizacaoDeTudo(t *testing.T) {
	// Pedido sem classe cai em "other", e é só "other" que fica autorizado:
	// um pedido de execução depois disso continua sendo perguntado.
	tela := novaTelaFalsa(escolhendo("Permitir sempre"))
	h := handlerCom(tela, acp.TurnOwner{ConversationID: "c", Interactive: true}, true)
	store := lembrandoAutorizacoes(t, h, "cursor")

	pedido := pedidoDeExecucao()
	pedido.ToolCall = acp.ToolCall{ID: "call-1", Title: "algo"}
	h.RequestPermission(context.Background(), pedido)

	if !store.Allows("cursor", acp.ToolKindOther) {
		t.Error("a autorização da classe sem nome não foi guardada")
	}
	if store.Allows("cursor", "execute") {
		t.Error("autorizar uma ação sem classe liberou execução de comando")
	}
}