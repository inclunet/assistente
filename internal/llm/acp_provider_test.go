package llm

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"assistente/internal/acp"
)

// ==================== Duplos de teste ====================

// agenteFalso é uma sessão ACP controlada pelo teste: entrega as atualizações
// combinadas e devolve o desfecho pedido, sem processo nenhum.
type agenteFalso struct {
	updates []acp.Update
	stop    acp.StopReason
	err     error
	// esperaCancelamento faz o turno só terminar quando o ctx for cancelado,
	// como um agente que está trabalhando quando a pessoa manda parar.
	esperaCancelamento bool

	mu       sync.Mutex
	recebido [][]acp.Content
}

func (a *agenteFalso) ID() string { return "sessao-de-teste" }

func (a *agenteFalso) Prompt(ctx context.Context, content []acp.Content, sink acp.UpdateSink) (acp.StopReason, error) {
	a.mu.Lock()
	a.recebido = append(a.recebido, content)
	a.mu.Unlock()

	for _, update := range a.updates {
		sink(update)
	}
	if a.esperaCancelamento {
		<-ctx.Done()
		return acp.StopCancelled, nil
	}
	if a.err != nil {
		return "", a.err
	}
	stop := a.stop
	if stop == "" {
		stop = acp.StopEndTurn
	}
	return stop, nil
}

func (a *agenteFalso) Close(context.Context) error  { return nil }
func (a *agenteFalso) Cancel(context.Context) error { return nil }
func (a *agenteFalso) ConfigOptions() []acp.ConfigOption {
	return nil
}
func (a *agenteFalso) SetConfigOption(context.Context, string, string) ([]acp.ConfigOption, error) {
	return nil, nil
}

func (a *agenteFalso) turnos() [][]acp.Content {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.recebido
}

type clienteFalso struct {
	sessao *agenteFalso
	caps   acp.Capabilities
}

func (c *clienteFalso) NewSession(context.Context, string) (acp.Session, error) {
	return c.sessao, nil
}
func (c *clienteFalso) LoadSession(context.Context, string, string) (acp.Session, error) {
	return c.sessao, nil
}
func (c *clienteFalso) Capabilities(context.Context) (acp.Capabilities, error) {
	return c.caps, nil
}
func (c *clienteFalso) CloseSession(context.Context, string) error { return nil }
func (c *clienteFalso) Call(context.Context, string, any) (json.RawMessage, error) {
	return nil, nil
}
func (c *clienteFalso) Close() error { return nil }

// servicoDeAgentes monta o serviço de longa duração com o transporte trocado
// pelo agente falso: o provider passa pelo caminho real de sessão por conversa
// (AEP-0084 D3) sem subir processo.
func servicoDeAgentes(t *testing.T, sessao *agenteFalso, caps acp.Capabilities) *acp.Manager {
	t.Helper()
	dir := t.TempDir()
	mgr := acp.NewManager(acp.ManagerConfig{
		WorkDir: func() (string, error) { return dir, nil },
		Dial: func(acp.Config, acp.RequestHandler) (acp.Client, error) {
			return &clienteFalso{sessao: sessao, caps: caps}, nil
		},
	})
	t.Cleanup(mgr.Shutdown)
	return mgr
}

// providerDeAgente monta o provider sobre um agente que só recebe texto, que é
// o mínimo do protocolo.
func providerDeAgente(t *testing.T, sessao *agenteFalso) *ACPChatProvider {
	t.Helper()
	return providerComCapacidades(t, sessao, acp.Capabilities{})
}

func providerComCapacidades(t *testing.T, sessao *agenteFalso, caps acp.Capabilities) *ACPChatProvider {
	t.Helper()
	return NewACPChatProvider(&ProviderConfig{
		ID:         "cursor",
		Name:       "Cursor",
		APIFormat:  APIFormatACP,
		ACPCommand: "cursor-agent",
		Model:      "auto",
	}, servicoDeAgentes(t, sessao, caps))
}

// espiao registra o que o provider entregou ao barramento.
type espiao struct {
	chunks        []string
	pensamento    []string
	raciocinio    string
	fimPensou     bool
	ordem         []string
	erro          string
	naoRetentavel bool
	ferramentas   []AgentToolEvent
	avisos        []TurnNotice
	segmentos     int
	pronto        bool
	respostaFim   string
	modeloFim     string
	chamouOnDone  int
}

func (e *espiao) OnChunk(content string) {
	e.chunks = append(e.chunks, content)
	e.ordem = append(e.ordem, "chunk")
}
func (e *espiao) OnThinking(content string) {
	e.pensamento = append(e.pensamento, content)
	e.ordem = append(e.ordem, "thinking")
}
func (e *espiao) OnThinkingDone(fullReasoning string) {
	e.fimPensou = true
	e.raciocinio = fullReasoning
	e.ordem = append(e.ordem, "thinking_done")
}
func (e *espiao) OnToolCalls([]ToolCall, string, Usage, string) {
	e.ordem = append(e.ordem, "tool_calls")
}
func (e *espiao) OnError(err string) {
	e.erro = err
	e.ordem = append(e.ordem, "error")
}
func (e *espiao) MarkErrorNotRetryable() {
	e.naoRetentavel = true
	e.ordem = append(e.ordem, "nao_retentavel")
}
func (e *espiao) OnDone(fullResponse string, _ Usage, model string) {
	e.pronto = true
	e.respostaFim = fullResponse
	e.modeloFim = model
	e.chamouOnDone++
	e.ordem = append(e.ordem, "done")
}
func (e *espiao) OnMCPToolEvent(MCPToolEvent) {}

// O espião também recebe a atividade do agente: ferramentas e fim de segmento.
func (e *espiao) OnAgentToolEvent(event AgentToolEvent) {
	e.ferramentas = append(e.ferramentas, event)
	e.ordem = append(e.ordem, "tool_"+event.Status)
}
func (e *espiao) OnSegmentDone() {
	e.segmentos++
	e.ordem = append(e.ordem, "segment_done")
}

// E também os avisos sobre o turno, como o do anexo que não pôde ir.
func (e *espiao) OnTurnNotice(notice TurnNotice) {
	e.avisos = append(e.avisos, notice)
	e.ordem = append(e.ordem, "notice_"+string(notice.Kind))
}

func (e *espiao) texto() string { return strings.Join(e.chunks, "") }

// espiaoSurdo é um handler que não sabe receber atividade de agente: o turno
// precisa seguir entregando texto para ele.
type espiaoSurdo struct {
	chunks []string
	pronto bool
}

func (e *espiaoSurdo) OnChunk(content string)                        { e.chunks = append(e.chunks, content) }
func (e *espiaoSurdo) OnThinking(string)                             {}
func (e *espiaoSurdo) OnThinkingDone(string)                         {}
func (e *espiaoSurdo) OnToolCalls([]ToolCall, string, Usage, string) {}
func (e *espiaoSurdo) OnError(string)                                {}
func (e *espiaoSurdo) OnDone(string, Usage, string)                  { e.pronto = true }
func (e *espiaoSurdo) OnMCPToolEvent(MCPToolEvent)                   {}

// ==================== Turno ====================

func TestTurnoDoAgenteEntregaRaciocinioEResposta(t *testing.T) {
	sessao := &agenteFalso{updates: []acp.Update{
		{Kind: acp.UpdateThought, Text: "vou ler o arquivo"},
		{Kind: acp.UpdateText, Text: "Encontrei o problema"},
		// O agente volta a pensar no meio do turno, e é comum: quem fecha o
		// raciocínio a cada troca faz a UI piscar "pensando" durante o texto.
		{Kind: acp.UpdateThought, Text: " e conferir o teste"},
		{Kind: acp.UpdateText, Text: " na linha 12."},
	}}
	provider := providerDeAgente(t, sessao)
	handler := &espiao{}

	provider.StreamChat(t.Context(),
		[]Message{{Role: "user", Content: "o que está errado?"}},
		ChatParams{ConversationID: "conversa-1"}, handler)

	if handler.erro != "" {
		t.Fatalf("turno falhou: %s", handler.erro)
	}
	if got, want := handler.texto(), "Encontrei o problema na linha 12."; got != want {
		t.Errorf("texto entregue = %q, quer %q", got, want)
	}
	if got, want := handler.respostaFim, "Encontrei o problema na linha 12."; got != want {
		t.Errorf("resposta final = %q, quer %q", got, want)
	}
	if got, want := handler.raciocinio, "vou ler o arquivo e conferir o teste"; got != want {
		t.Errorf("raciocínio = %q, quer %q", got, want)
	}
	if got, want := handler.modeloFim, "auto"; got != want {
		t.Errorf("modelo do turno = %q, quer %q", got, want)
	}
	// O raciocínio fecha uma vez, no fim do turno, como nos demais provedores
	// do barramento — e nunca reabre.
	want := []string{"thinking", "chunk", "thinking", "chunk", "thinking_done", "done"}
	if strings.Join(handler.ordem, ",") != strings.Join(want, ",") {
		t.Errorf("ordem dos eventos = %v, quer %v", handler.ordem, want)
	}
}

func TestTurnoLevaSoAUltimaMensagemDoUsuario(t *testing.T) {
	sessao := &agenteFalso{updates: []acp.Update{{Kind: acp.UpdateText, Text: "ok"}}}
	provider := providerDeAgente(t, sessao)

	provider.StreamChat(t.Context(), []Message{
		{Role: "system", Content: "persona do perfil"},
		{Role: "user", Content: "primeira pergunta"},
		{Role: "assistant", Content: "primeira resposta"},
		{Role: "user", Content: "segunda pergunta"},
	}, ChatParams{ConversationID: "conversa-1"}, &espiao{})

	turnos := sessao.turnos()
	if len(turnos) != 1 {
		t.Fatalf("o agente recebeu %d turnos, quer 1", len(turnos))
	}
	// A sessão do agente tem o histórico dela (D4): reenviá-lo duplicaria
	// contexto e custo. O que acompanha a mensagem nova são as instruções do
	// perfil, que não estão na sessão dele.
	texto := textoDoTurno(t, turnos[0])
	if strings.Contains(texto, "primeira pergunta") || strings.Contains(texto, "primeira resposta") {
		t.Errorf("o histórico foi reenviado ao agente: %q", texto)
	}
	ultimo := turnos[0][len(turnos[0])-1]
	if ultimo.Text != "segunda pergunta" {
		t.Errorf("último bloco = %q, quer a última mensagem do usuário", ultimo.Text)
	}
}

func TestTurnoEnviaOTextoDeMensagemMultimodal(t *testing.T) {
	sessao := &agenteFalso{updates: []acp.Update{{Kind: acp.UpdateText, Text: "ok"}}}
	provider := providerDeAgente(t, sessao)
	handler := &espiao{}

	// O builder monta a mensagem do turno em partes tipadas quando ela é
	// multimodal. Sem tratar esse formato, o agente receberia o despejo da
	// estrutura no lugar do pedido da pessoa.
	provider.StreamChat(t.Context(), []Message{{Role: "user", Content: []ContentPart{
		{Type: "text", Text: "<turn_context>arquivo aberto</turn_context>"},
		{Type: "text", Text: "o que mudou aqui?"},
	}}}, ChatParams{ConversationID: "conversa-1"}, handler)

	if handler.erro != "" {
		t.Fatalf("turno falhou: %s", handler.erro)
	}
	turnos := sessao.turnos()
	if len(turnos) != 1 || len(turnos[0]) != 2 {
		t.Fatalf("o agente recebeu %+v, quer as duas partes de texto", turnos)
	}
	if got, want := turnos[0][0].Text, "<turn_context>arquivo aberto</turn_context>"; got != want {
		t.Errorf("primeiro bloco = %q, quer %q", got, want)
	}
	if got, want := turnos[0][1].Text, "o que mudou aqui?"; got != want {
		t.Errorf("segundo bloco = %q, quer %q", got, want)
	}
}

func TestTurnoDoAgenteIgnoraFerramentasDoApp(t *testing.T) {
	sessao := &agenteFalso{updates: []acp.Update{{Kind: acp.UpdateText, Text: "feito"}}}
	provider := providerDeAgente(t, sessao)
	handler := &espiao{}

	// Quem executa ferramenta no turno do agente é o agente (D7). Uma tool que
	// escape do planejamento não pode derrubar o turno nem virar pedido ao
	// agente — o protocolo nem tem onde colocá-la.
	provider.StreamChat(t.Context(),
		[]Message{{Role: "user", Content: "vai"}},
		ChatParams{ConversationID: "conversa-1"}, handler,
		ToolDefinition{Type: "function", Function: FunctionDefinition{Name: "read_file"}})

	if handler.erro != "" {
		t.Fatalf("turno falhou: %s", handler.erro)
	}
	if !handler.pronto || handler.respostaFim != "feito" {
		t.Errorf("turno não concluiu com a resposta do agente: %+v", handler)
	}
}

func TestTurnoInterrompidoNaoAnunciaFalha(t *testing.T) {
	sessao := &agenteFalso{
		updates:            []acp.Update{{Kind: acp.UpdateText, Text: "comecei"}},
		esperaCancelamento: true,
	}
	provider := providerDeAgente(t, sessao)
	handler := &espiao{}

	ctx, cancel := context.WithCancel(t.Context())
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()
	provider.StreamChat(ctx,
		[]Message{{Role: "user", Content: "faz aí"}},
		ChatParams{ConversationID: "conversa-1"}, handler)

	// Parar foi decisão de quem estava lendo: o laço de streaming persiste o
	// parcial e emite o evento terminal. Um erro daqui viraria aviso de falha
	// para uma interrupção pedida.
	if handler.erro != "" {
		t.Errorf("interrupção virou erro: %s", handler.erro)
	}
	if handler.pronto {
		t.Error("interrupção concluiu o turno como resposta completa")
	}
}

// ==================== Ferramentas do agente e segmentos ====================

func TestFerramentaDoAgenteViraAtividadeENaoExecucao(t *testing.T) {
	sessao := &agenteFalso{updates: []acp.Update{
		{Kind: acp.UpdateText, Text: "vou procurar o TODO"},
		{Kind: acp.UpdateToolStart, Tool: &acp.ToolCall{
			ID: "call-1", Kind: "search", Title: "grep -rn \"TODO\"", Status: "pending",
		}},
		{Kind: acp.UpdateToolProgress, Tool: &acp.ToolCall{ID: "call-1", Status: "completed"}},
		{Kind: acp.UpdateText, Text: "achei na linha 12."},
	}}
	provider := providerDeAgente(t, sessao)
	handler := &espiao{}

	provider.StreamChat(t.Context(),
		[]Message{{Role: "user", Content: "cadê o TODO?"}},
		ChatParams{ConversationID: "conversa-1"}, handler)

	// O bloco de texto anterior à ferramenta fecha na hora: quem ouve acompanha
	// o trabalho do agente em vez de esperar o turno inteiro (D13).
	esperada := []string{"chunk", "segment_done", "tool_running", "tool_completed", "chunk", "done"}
	if strings.Join(handler.ordem, ",") != strings.Join(esperada, ",") {
		t.Errorf("ordem = %v, quer %v", handler.ordem, esperada)
	}
	// Executar ferramenta de agente é coisa do agente: OnToolCalls mandaria o
	// turno para o loop agêntico do app (D7).
	for _, evento := range handler.ordem {
		if evento == "tool_calls" {
			t.Fatal("provider ACP não pode pedir execução de ferramenta ao app")
		}
	}
	if len(handler.ferramentas) != 2 {
		t.Fatalf("esperava início e fim da ferramenta, obtive %+v", handler.ferramentas)
	}
	inicio := handler.ferramentas[0]
	if inicio.Kind != "search" || inicio.Title != "grep -rn \"TODO\"" || inicio.ID != "call-1" {
		t.Errorf("início da ferramenta = %+v", inicio)
	}
	// A resposta salva é o turno inteiro, mesmo com o texto cortado em blocos.
	if handler.respostaFim != "vou procurar o TODOachei na linha 12." {
		t.Errorf("resposta final = %q, quer o turno inteiro", handler.respostaFim)
	}
}

func TestAtualizacaoDeFerramentaHerdaOQueOComecoAnunciou(t *testing.T) {
	// A atualização traz só o que mudou; sem herdar, o fim da ferramenta
	// chegaria sem nome e sem resumo à tela e ao anúncio.
	sessao := &agenteFalso{updates: []acp.Update{
		{Kind: acp.UpdateToolStart, Tool: &acp.ToolCall{
			ID: "call-1", Kind: "execute", Title: "npm test", Status: "in_progress",
		}},
		{Kind: acp.UpdateToolProgress, Tool: &acp.ToolCall{ID: "call-1", Status: "failed"}},
	}}
	provider := providerDeAgente(t, sessao)
	handler := &espiao{}

	provider.StreamChat(t.Context(),
		[]Message{{Role: "user", Content: "roda o teste"}},
		ChatParams{ConversationID: "conversa-1"}, handler)

	if len(handler.ferramentas) != 2 {
		t.Fatalf("esperava início e fim, obtive %+v", handler.ferramentas)
	}
	fim := handler.ferramentas[1]
	if fim.Kind != "execute" || fim.Title != "npm test" {
		t.Errorf("fim da ferramenta = %+v, quer herdar classe e título do começo", fim)
	}
	if fim.Status != AgentToolFailed {
		t.Errorf("status = %q, quer falha", fim.Status)
	}
}

func TestTextoEntreAtualizacoesDaMesmaFerramentaNaoFicaMudo(t *testing.T) {
	// A ferramenta demorada atualiza várias vezes, e o agente escreve no meio.
	// Sem corte a cada atividade, esse texto só seria falado no fim do turno —
	// que é o silêncio que a segmentação existe para evitar.
	sessao := &agenteFalso{updates: []acp.Update{
		{Kind: acp.UpdateToolStart, Tool: &acp.ToolCall{ID: "call-1", Kind: "execute", Title: "npm test", Status: "in_progress"}},
		{Kind: acp.UpdateText, Text: "os testes estão rodando."},
		{Kind: acp.UpdateToolProgress, Tool: &acp.ToolCall{ID: "call-1", Title: "npm test (2/40)", Status: "in_progress"}},
		{Kind: acp.UpdateText, Text: "quase lá."},
		{Kind: acp.UpdateToolProgress, Tool: &acp.ToolCall{ID: "call-1", Status: "completed"}},
		{Kind: acp.UpdateText, Text: "passou tudo."},
	}}
	provider := providerDeAgente(t, sessao)
	handler := &espiao{}

	provider.StreamChat(t.Context(),
		[]Message{{Role: "user", Content: "roda os testes"}},
		ChatParams{ConversationID: "conversa-1"}, handler)

	if handler.segmentos != 2 {
		t.Errorf("segmentos = %d, quer um para cada bloco escrito antes de nova atividade", handler.segmentos)
	}
	// O último bloco não vira segmento: ele é a mensagem final do assistente.
	if handler.respostaFim != "os testes estão rodando.quase lá.passou tudo." {
		t.Errorf("resposta final = %q, quer o turno inteiro", handler.respostaFim)
	}
}

func TestAvisoRepetidoDeConclusaoNaoCriaFerramentaFantasma(t *testing.T) {
	// Para o handler, um fim sem começo é ferramenta nova: o aviso repetido
	// abriria um segundo item na tela e um segundo anúncio.
	sessao := &agenteFalso{updates: []acp.Update{
		{Kind: acp.UpdateToolStart, Tool: &acp.ToolCall{ID: "call-1", Kind: "read", Status: "in_progress"}},
		{Kind: acp.UpdateToolProgress, Tool: &acp.ToolCall{ID: "call-1", Status: "completed"}},
		{Kind: acp.UpdateToolProgress, Tool: &acp.ToolCall{ID: "call-1", Status: "completed"}},
	}}
	provider := providerDeAgente(t, sessao)
	handler := &espiao{}

	provider.StreamChat(t.Context(),
		[]Message{{Role: "user", Content: "lê o arquivo"}},
		ChatParams{ConversationID: "conversa-1"}, handler)

	if len(handler.ferramentas) != 2 {
		t.Errorf("esperava início e um único fim, obtive %+v", handler.ferramentas)
	}
}

func TestChamadasSemIdentificadorNaoEngolemUmaAOutra(t *testing.T) {
	// O protocolo exige identificador, mas o Cursor já mandou sem. Guardadas sob
	// a mesma chave, a primeira chamada concluída faria o app calar a seguinte.
	sessao := &agenteFalso{updates: []acp.Update{
		{Kind: acp.UpdateToolStart, Tool: &acp.ToolCall{Kind: "read", Title: "lendo a.go", Status: "in_progress"}},
		{Kind: acp.UpdateToolProgress, Tool: &acp.ToolCall{Kind: "read", Status: "completed"}},
		{Kind: acp.UpdateToolStart, Tool: &acp.ToolCall{Kind: "read", Title: "lendo b.go", Status: "in_progress"}},
		{Kind: acp.UpdateToolProgress, Tool: &acp.ToolCall{Kind: "read", Status: "completed"}},
		// O aviso repetido continua sendo repetição, e não uma terceira leitura.
		{Kind: acp.UpdateToolProgress, Tool: &acp.ToolCall{Kind: "read", Status: "completed"}},
	}}
	provider := providerDeAgente(t, sessao)
	handler := &espiao{}

	provider.StreamChat(t.Context(),
		[]Message{{Role: "user", Content: "lê os dois"}},
		ChatParams{ConversationID: "conversa-1"}, handler)

	if len(handler.ferramentas) != 4 {
		t.Fatalf("esperava começo e fim das duas chamadas, obtive %+v", handler.ferramentas)
	}
	// A segunda chamada não herda o título da primeira: são leituras diferentes.
	if handler.ferramentas[2].Title != "lendo b.go" || handler.ferramentas[3].Title != "lendo b.go" {
		t.Errorf("a segunda chamada anônima veio com o rótulo da primeira: %+v", handler.ferramentas[2:])
	}
	for _, ferramenta := range handler.ferramentas {
		if ferramenta.ID != "" {
			t.Errorf("identificador inventado vazou para o barramento: %+v", ferramenta)
		}
	}
}

func TestFimSemIdentificadorNemClasseAindaEncontraOComeco(t *testing.T) {
	// Sem identificador e com a atualização trazendo só o que mudou, o fim pode
	// chegar sem classe nenhuma. Tratá-lo como chamada nova abriria uma segunda
	// ferramenta na tela para algo que está terminando.
	sessao := &agenteFalso{updates: []acp.Update{
		{Kind: acp.UpdateToolStart, Tool: &acp.ToolCall{Kind: "search", Title: "grep TODO", Status: "in_progress"}},
		{Kind: acp.UpdateToolProgress, Tool: &acp.ToolCall{Status: "completed"}},
	}}
	provider := providerDeAgente(t, sessao)
	handler := &espiao{}

	provider.StreamChat(t.Context(),
		[]Message{{Role: "user", Content: "procura"}},
		ChatParams{ConversationID: "conversa-1"}, handler)

	if len(handler.ferramentas) != 2 {
		t.Fatalf("esperava começo e fim da mesma chamada, obtive %+v", handler.ferramentas)
	}
	fim := handler.ferramentas[1]
	if fim.Kind != "search" || fim.Title != "grep TODO" {
		t.Errorf("fim = %+v, quer a classe e o título herdados do começo", fim)
	}
	if fim.Status != AgentToolCompleted {
		t.Errorf("status = %q, quer conclusão", fim.Status)
	}
}

func TestRotuloDeFerramentaChegaSaneadoAoAnuncio(t *testing.T) {
	// Título é dado não confiável: pode ser a linha de comando literal, com
	// escape de terminal e quebra de linha (AEP-0084 D11).
	sessao := &agenteFalso{updates: []acp.Update{
		{Kind: acp.UpdateToolStart, Tool: &acp.ToolCall{
			ID:     "call-1",
			Kind:   "rm -rf /",
			Title:  "\x1b[31mgit commit\n-m \"pronto\"\x1b[0m",
			Status: "in_progress",
		}},
	}}
	provider := providerDeAgente(t, sessao)
	handler := &espiao{}

	provider.StreamChat(t.Context(),
		[]Message{{Role: "user", Content: "commita"}},
		ChatParams{ConversationID: "conversa-1"}, handler)

	if len(handler.ferramentas) == 0 {
		t.Fatal("a ferramenta não chegou ao barramento")
	}
	inicio := handler.ferramentas[0]
	if inicio.Title != "git commit -m \"pronto\"" {
		t.Errorf("título = %q, quer saneado e em linha única", inicio.Title)
	}
	// A classe vira o nome anunciado da ferramenta; fora do conjunto do
	// protocolo, o agente estaria escrevendo direto no leitor de telas.
	if inicio.Kind != AgentToolKindOther {
		t.Errorf("classe = %q, quer %q para valor fora do protocolo", inicio.Kind, AgentToolKindOther)
	}
}

func TestFerramentaSemTextoAntesNaoAbreSegmentoVazio(t *testing.T) {
	sessao := &agenteFalso{updates: []acp.Update{
		{Kind: acp.UpdateToolStart, Tool: &acp.ToolCall{ID: "call-1", Kind: "read", Status: "in_progress"}},
		{Kind: acp.UpdateToolProgress, Tool: &acp.ToolCall{ID: "call-1", Status: "completed"}},
		{Kind: acp.UpdateToolStart, Tool: &acp.ToolCall{ID: "call-2", Kind: "read", Status: "in_progress"}},
		{Kind: acp.UpdateText, Text: "li os dois arquivos."},
	}}
	provider := providerDeAgente(t, sessao)
	handler := &espiao{}

	provider.StreamChat(t.Context(),
		[]Message{{Role: "user", Content: "lê os arquivos"}},
		ChatParams{ConversationID: "conversa-1"}, handler)

	if handler.segmentos != 0 {
		t.Errorf("segmentos = %d, quer nenhum: não houve bloco de texto para fechar", handler.segmentos)
	}
	// O texto final não vira segmento: ele é a mensagem do assistente, e falá-lo
	// duas vezes seria repetir a resposta para quem ouve.
	if handler.respostaFim != "li os dois arquivos." {
		t.Errorf("resposta final = %q", handler.respostaFim)
	}
}

func TestInterrupcaoFechaFerramentaQueFicouGirando(t *testing.T) {
	sessao := &agenteFalso{
		updates: []acp.Update{
			{Kind: acp.UpdateToolStart, Tool: &acp.ToolCall{
				ID: "call-1", Kind: "execute", Title: "npm run build", Status: "in_progress",
			}},
		},
		esperaCancelamento: true,
	}
	provider := providerDeAgente(t, sessao)
	handler := &espiao{}

	ctx, cancel := context.WithCancel(t.Context())
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()
	provider.StreamChat(ctx,
		[]Message{{Role: "user", Content: "builda"}},
		ChatParams{ConversationID: "conversa-1"}, handler)

	// Este caminho não passa por OnDone nem por OnError, que são onde o handler
	// faz a limpeza: sem desfecho, a ferramenta gira na tela para sempre.
	if len(handler.ferramentas) != 2 {
		t.Fatalf("esperava início e desfecho da ferramenta, obtive %+v", handler.ferramentas)
	}
	fim := handler.ferramentas[1]
	if fim.Status != AgentToolCancelled {
		t.Errorf("status = %q, quer cancelamento", fim.Status)
	}
	if fim.Kind != "execute" || fim.Title != "npm run build" {
		t.Errorf("desfecho perdeu o que a ferramenta anunciou: %+v", fim)
	}
}

func TestHandlerSemCanalDeAtividadeAindaRecebeOTurno(t *testing.T) {
	sessao := &agenteFalso{updates: []acp.Update{
		{Kind: acp.UpdateText, Text: "oi"},
		{Kind: acp.UpdateToolStart, Tool: &acp.ToolCall{ID: "call-1", Kind: "read", Status: "in_progress"}},
		{Kind: acp.UpdateText, Text: " tudo bem"},
	}}
	provider := providerDeAgente(t, sessao)
	handler := &espiaoSurdo{}

	provider.StreamChat(t.Context(),
		[]Message{{Role: "user", Content: "oi"}},
		ChatParams{ConversationID: "conversa-1"}, handler)

	if strings.Join(handler.chunks, "") != "oi tudo bem" || !handler.pronto {
		t.Errorf("turno incompleto para handler sem canal de atividade: %+v", handler)
	}
}

// ==================== Falhas ====================

func TestTurnoSemConversaNaoChegaAoAgente(t *testing.T) {
	sessao := &agenteFalso{}
	provider := providerDeAgente(t, sessao)
	handler := &espiao{}

	provider.StreamChat(t.Context(),
		[]Message{{Role: "user", Content: "oi"}}, ChatParams{}, handler)

	if handler.erro == "" {
		t.Fatal("turno sem conversa deveria falhar: o histórico do agente vive na sessão da conversa")
	}
	if len(sessao.turnos()) != 0 {
		t.Error("turno sem conversa chegou ao agente")
	}
}

func TestTurnoSemServicoDeAgentesFalhaComExplicacao(t *testing.T) {
	provider := NewACPChatProvider(&ProviderConfig{ID: "cursor", ACPCommand: "cursor-agent"}, nil)
	handler := &espiao{}

	provider.StreamChat(t.Context(),
		[]Message{{Role: "user", Content: "oi"}},
		ChatParams{ConversationID: "conversa-1"}, handler)

	if !strings.Contains(handler.erro, "serviço de agentes") {
		t.Errorf("erro = %q, quer explicar que o serviço de agentes não está de pé", handler.erro)
	}
}

func TestProcessoDoAgenteCaidoViraMensagemAcionavel(t *testing.T) {
	sessao := &agenteFalso{err: &acp.PromptError{Accepted: true, Err: acp.ErrSessionLost}}
	provider := providerDeAgente(t, sessao)
	handler := &espiao{}

	provider.StreamChat(t.Context(),
		[]Message{{Role: "user", Content: "oi"}},
		ChatParams{ConversationID: "conversa-1"}, handler)

	if !strings.Contains(handler.erro, "processo do agente caiu") {
		t.Errorf("erro = %q, quer dizer que o processo do agente caiu", handler.erro)
	}
	if handler.pronto {
		t.Error("turno falho não pode concluir como resposta")
	}
}

func TestRecusaComTextoEntregaOQueOAgenteEscreveu(t *testing.T) {
	sessao := &agenteFalso{
		stop:    acp.StopRefusal,
		updates: []acp.Update{{Kind: acp.UpdateText, Text: "não posso fazer isso porque..."}},
	}
	provider := providerDeAgente(t, sessao)
	handler := &espiao{}

	provider.StreamChat(t.Context(),
		[]Message{{Role: "user", Content: "faz algo proibido"}},
		ChatParams{ConversationID: "conversa-1"}, handler)

	if handler.erro != "" {
		t.Fatalf("recusa explicada pelo agente virou erro: %s", handler.erro)
	}
	if handler.respostaFim != "não posso fazer isso porque..." {
		t.Errorf("resposta final = %q, quer o texto que o agente escreveu", handler.respostaFim)
	}
}

func TestTurnoSemTextoNenhumSempreDizOQueAconteceu(t *testing.T) {
	// Cada desfecho sem texto precisa virar frase: uma bolha vazia, para quem
	// ouve, é igual a resposta nenhuma sem explicação.
	casos := []struct {
		nome  string
		stop  acp.StopReason
		trata string
	}{
		{"recusa", acp.StopRefusal, "recusou"},
		{"limite de tokens", acp.StopMaxTokens, "limite de tokens"},
		{"limite de requisições", acp.StopMaxTurnRequests, "limite de requisições"},
		{"interrupção do agente", acp.StopCancelled, "interrompeu"},
		{"fim de turno calado", acp.StopEndTurn, "sem escrever resposta"},
		{"motivo desconhecido", acp.StopReason("outra_coisa"), "sem escrever resposta"},
	}
	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			provider := providerDeAgente(t, &agenteFalso{stop: caso.stop})
			handler := &espiao{}

			provider.StreamChat(t.Context(),
				[]Message{{Role: "user", Content: "vai"}},
				ChatParams{ConversationID: "conversa-1"}, handler)

			if handler.erro != "" {
				t.Fatalf("desfecho virou erro: %s", handler.erro)
			}
			if !strings.Contains(handler.respostaFim, caso.trata) {
				t.Errorf("resposta final = %q, quer conter %q", handler.respostaFim, caso.trata)
			}
		})
	}
}

func TestFalhaDoTurnoVirouFraseQueDizOEstadoDoAgente(t *testing.T) {
	casos := []struct {
		nome    string
		err     error
		aceito  bool
		trata   string
		naoQuer string
	}{
		{nome: "processo caiu antes de receber", err: acp.ErrSessionLost, trata: "caiu antes de receber o pedido"},
		// Depois do aceite, "envie novamente" seria conselho ruim: o agente pode
		// ter mexido no disco antes de cair.
		{nome: "processo caiu no meio", err: acp.ErrSessionLost, aceito: true, trata: "confira o estado", naoQuer: "Envie novamente"},
		{nome: "sessão encerrada", err: acp.ErrSessionClosed, aceito: true, trata: "foi encerrada"},
		// Sem confirmação do "pare", o agente pode continuar mexendo no disco:
		// pedir de novo sem conferir repetiria edição e comando.
		{nome: "cancelamento sem confirmação", err: acp.ErrCancelNotConfirmed, aceito: true, trata: "pode ainda estar trabalhando"},
		{nome: "conversa excluída", err: acp.ErrConversationGone, aceito: true, trata: "conversa foi encerrada"},
		{nome: "falha qualquer antes do envio", err: errors.New("cano quebrado"), trata: "cano quebrado"},
		{nome: "falha qualquer depois do aceite", err: errors.New("cano quebrado"), aceito: true, trata: "pode ter feito parte do pedido"},
	}
	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			got := turnErrorMessage(&acp.PromptError{Accepted: caso.aceito, Err: caso.err}, caso.aceito)
			if !strings.Contains(got, caso.trata) {
				t.Errorf("mensagem = %q, quer conter %q", got, caso.trata)
			}
			if caso.naoQuer != "" && strings.Contains(got, caso.naoQuer) {
				t.Errorf("mensagem = %q, não pode conter %q", got, caso.naoQuer)
			}
		})
	}
}

// ==================== Turno já aceito não se repete (D4) ====================

func TestTurnoJaAceitoNaoPodeSerRepetidoPelaAutoRecuperacao(t *testing.T) {
	casos := []struct {
		nome   string
		err    error
		marcar bool
	}{
		{
			nome:   "falha depois do aceite",
			err:    &acp.PromptError{Accepted: true, Err: acp.ErrSessionLost},
			marcar: true,
		},
		{
			// Sem aceite nada saiu para o agente: reenviar não refaz trabalho.
			nome: "falha antes do envio",
			err:  &acp.PromptError{Err: acp.ErrSessionClosed},
		},
		{
			// No escuro, a escolha é a cara: parar e devolver o controle.
			nome:   "falha que não sabe dizer se saiu",
			err:    errors.New("cano quebrado"),
			marcar: true,
		},
	}
	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			provider := providerDeAgente(t, &agenteFalso{err: caso.err})
			handler := &espiao{}

			provider.StreamChat(t.Context(),
				[]Message{{Role: "user", Content: "refatora o módulo"}},
				ChatParams{ConversationID: "conversa-1"}, handler)

			if handler.erro == "" {
				t.Fatal("falha do turno precisa chegar ao barramento como erro")
			}
			if handler.naoRetentavel != caso.marcar {
				t.Errorf("marca de não retentável = %v, quer %v", handler.naoRetentavel, caso.marcar)
			}
		})
	}
}

func TestErroAntesDaSessaoNaoBloqueiaNovaTentativa(t *testing.T) {
	// A conversa sem sessão falha dentro do app, sem nada ter saído para o
	// agente: barrar a repetição aqui só tiraria uma chance de acertar.
	provider := providerDeAgente(t, &agenteFalso{})
	handler := &espiao{}

	provider.StreamChat(t.Context(),
		[]Message{{Role: "user", Content: "oi"}},
		ChatParams{}, handler)

	if handler.erro == "" {
		t.Fatal("turno sem conversa precisa falhar")
	}
	if handler.naoRetentavel {
		t.Error("falha antes de falar com o agente não pode barrar a auto-recuperação")
	}
}

// ==================== Papéis auxiliares e capacidades ====================

func TestPapeisAuxiliaresNaoUsamAgente(t *testing.T) {
	provider := providerDeAgente(t, &agenteFalso{})

	if _, err := provider.SimpleChat(t.Context(), "auto", "resuma", "texto"); !errors.Is(err, ErrACPAuxiliaryRole) {
		t.Errorf("SimpleChat devolveu %v, quer recusa de papel auxiliar (D14)", err)
	}
	if _, err := provider.SendChat(t.Context(), []Message{{Role: "user", Content: "oi"}}, ChatParams{}); !errors.Is(err, ErrACPAuxiliaryRole) {
		t.Errorf("SendChat devolveu %v, quer recusa de papel auxiliar (D14)", err)
	}
}

func TestAgenteNaoRecebeMCPDoApp(t *testing.T) {
	provider := providerDeAgente(t, &agenteFalso{})

	if provider.NativeMCPCapable() {
		t.Error("o MCP de um agente é dele, configurado no projeto (D1)")
	}
	if got := provider.WithMCPServers([]MCPServerConfig{{Name: "x"}}); got != ChatProvider(provider) {
		t.Error("WithMCPServers deveria ser no-op no provedor de agente")
	}
}

func TestListarModelosDoAgenteExplicaAAusencia(t *testing.T) {
	provider := providerDeAgente(t, &agenteFalso{})

	models, err := provider.GetModels(t.Context())
	if err == nil {
		t.Fatal("listar modelos deveria explicar que a descoberta ainda não existe, e não devolver lista vazia em silêncio")
	}
	if len(models) != 0 {
		t.Errorf("modelos = %v, quer nenhum", models)
	}
}

func TestFabricaDevolveProviderDeAgenteParaFormatoACP(t *testing.T) {
	provider := NewChatProvider(&ProviderConfig{
		ID: "cursor", Name: "Cursor", APIFormat: APIFormatACP, ACPCommand: "cursor-agent",
	}, nil, nil)

	if _, ok := provider.(*ACPChatProvider); !ok {
		t.Fatalf("fábrica devolveu %T para api_format acp, quer *ACPChatProvider", provider)
	}
}
