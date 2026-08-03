package app

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"assistente/internal/acp"
	"assistente/internal/questionnaire"
)

// telaFalsa é o questionário do desktop com a pessoa do outro lado: guarda o
// que foi perguntado e responde o que o teste combinou.
type telaFalsa struct {
	mu        sync.Mutex
	manager   *questionnaire.Manager
	perguntas []map[string]any
	// escolhe recebe as opções oferecidas e devolve a escolhida; nulo cancela
	// o diálogo, como quem fecha a janela.
	escolhe func(opcoes []string) string
}

func novaTelaFalsa(escolhe func(opcoes []string) string) *telaFalsa {
	tela := &telaFalsa{escolhe: escolhe}
	tela.manager = questionnaire.NewManager(tela.aoPerguntar)
	return tela
}

// aoPerguntar responde ao evento do questionário como o frontend faria.
func (t *telaFalsa) aoPerguntar(_ string, data any) {
	payload, ok := data.(map[string]any)
	if !ok {
		return
	}
	t.mu.Lock()
	t.perguntas = append(t.perguntas, payload)
	t.mu.Unlock()

	id, _ := payload["id"].(string)
	opcoes := t.opcoesDe(payload)
	go func() {
		if t.escolhe == nil {
			_ = t.manager.Respond(id, nil, true)
			return
		}
		_ = t.manager.Respond(id, map[string]any{permissionAnswerID: t.escolhe(opcoes)}, false)
	}()
}

func (t *telaFalsa) opcoesDe(payload map[string]any) []string {
	perguntas, _ := payload["questions"].([]questionnaire.Question)
	if len(perguntas) == 0 {
		return nil
	}
	return perguntas[0].Options
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

// handlerCom monta o handler sobre uma tela e um turno de origem conhecida.
func handlerCom(tela *telaFalsa, owner acp.TurnOwner, temTurno bool) *acpRequestHandler {
	h := &acpRequestHandler{
		owner: func(string) (acp.TurnOwner, bool) { return owner, temTurno },
	}
	if tela != nil {
		h.questions = func() *questionnaire.Manager { return tela.manager }
	}
	return h
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
	pergunta := tela.ultimaPergunta(t)
	if desc, _ := pergunta["description"].(string); !strings.Contains(desc, "rm -rf build") {
		t.Errorf("descrição = %q, quer mostrar o que está sendo autorizado", desc)
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

func TestPrazoEstouradoNaoPenduraOAgente(t *testing.T) {
	// Questionário que nunca é respondido: o handler precisa voltar mesmo
	// assim, senão o agente fica esperando até o teto do transporte.
	mudo := questionnaire.NewManager(func(string, any) {})
	h := &acpRequestHandler{
		owner:     func(string) (acp.TurnOwner, bool) { return acp.TurnOwner{ConversationID: "c", Interactive: true}, true },
		questions: func() *questionnaire.Manager { return mudo },
	}
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

	pergunta := tela.ultimaPergunta(t)
	desc, _ := pergunta["description"].(string)
	if strings.Contains(desc, "\x1b") || strings.Contains(desc, "rm -rf /\ncontinua") {
		t.Errorf("descrição = %q, quer o texto do agente saneado", desc)
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
	if rotulo, _ := pergunta["submitLabel"].(string); rotulo == "" {
		t.Error("o diálogo foi para a tela com o botão genérico de enviar")
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

func TestExtensaoDoAgenteAindaNaoEhTratadaAqui(t *testing.T) {
	h := handlerCom(nil, acp.TurnOwner{}, false)

	if _, tratou := h.HandleCustom(context.Background(), "cursor/ask_question", nil); tratou {
		t.Error("o handler disse tratar uma extensão que não trata")
	}
	if _, ok := h.CustomFallback("cursor/ask_question"); ok {
		t.Error("ofereceu desfecho para um método que não implementa")
	}
}
