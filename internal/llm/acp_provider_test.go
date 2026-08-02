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

type clienteFalso struct{ sessao *agenteFalso }

func (c *clienteFalso) NewSession(context.Context, string) (acp.Session, error) {
	return c.sessao, nil
}
func (c *clienteFalso) LoadSession(context.Context, string, string) (acp.Session, error) {
	return c.sessao, nil
}
func (c *clienteFalso) Capabilities(context.Context) (acp.Capabilities, error) {
	return acp.Capabilities{}, nil
}
func (c *clienteFalso) CloseSession(context.Context, string) error { return nil }
func (c *clienteFalso) Call(context.Context, string, any) (json.RawMessage, error) {
	return nil, nil
}
func (c *clienteFalso) Close() error { return nil }

// servicoDeAgentes monta o serviço de longa duração com o transporte trocado
// pelo agente falso: o provider passa pelo caminho real de sessão por conversa
// (AEP-0084 D3) sem subir processo.
func servicoDeAgentes(t *testing.T, sessao *agenteFalso) *acp.Manager {
	t.Helper()
	dir := t.TempDir()
	mgr := acp.NewManager(acp.ManagerConfig{
		WorkDir: func() (string, error) { return dir, nil },
		Dial: func(acp.Config, acp.RequestHandler) (acp.Client, error) {
			return &clienteFalso{sessao: sessao}, nil
		},
	})
	t.Cleanup(mgr.Shutdown)
	return mgr
}

func providerDeAgente(t *testing.T, sessao *agenteFalso) *ACPChatProvider {
	t.Helper()
	return NewACPChatProvider(&ProviderConfig{
		ID:         "cursor",
		Name:       "Cursor",
		APIFormat:  APIFormatACP,
		ACPCommand: "cursor-agent",
		Model:      "auto",
	}, servicoDeAgentes(t, sessao))
}

// espiao registra o que o provider entregou ao barramento.
type espiao struct {
	chunks       []string
	pensamento   []string
	raciocinio   string
	fimPensou    bool
	ordem        []string
	erro         string
	pronto       bool
	respostaFim  string
	modeloFim    string
	chamouOnDone int
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
func (e *espiao) OnDone(fullResponse string, _ Usage, model string) {
	e.pronto = true
	e.respostaFim = fullResponse
	e.modeloFim = model
	e.chamouOnDone++
	e.ordem = append(e.ordem, "done")
}
func (e *espiao) OnMCPToolEvent(MCPToolEvent) {}

func (e *espiao) texto() string { return strings.Join(e.chunks, "") }

// ==================== Turno ====================

func TestTurnoDoAgenteEntregaRaciocinioEResposta(t *testing.T) {
	sessao := &agenteFalso{updates: []acp.Update{
		{Kind: acp.UpdateThought, Text: "vou ler o arquivo"},
		{Kind: acp.UpdateThought, Text: " e conferir o teste"},
		{Kind: acp.UpdateText, Text: "Encontrei o problema"},
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
	// O raciocínio precisa fechar antes do primeiro texto, senão a UI segue
	// anunciando "pensando" enquanto a resposta já está sendo escrita.
	want := []string{"thinking", "thinking", "thinking_done", "chunk", "chunk", "done"}
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
	// contexto e custo.
	if len(turnos[0]) != 1 || turnos[0][0].Text != "segunda pergunta" {
		t.Errorf("conteúdo enviado ao agente = %+v, quer só a última mensagem do usuário", turnos[0])
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
		{Type: "image_url", ImageURL: &ImageURL{URL: "data:image/png;base64,abc"}},
		{Type: "text", Text: "o que tem nesta imagem?"},
	}}}, ChatParams{ConversationID: "conversa-1"}, handler)

	if handler.erro != "" {
		t.Fatalf("turno falhou: %s", handler.erro)
	}
	turnos := sessao.turnos()
	if len(turnos) != 1 || len(turnos[0]) != 1 {
		t.Fatalf("o agente recebeu %+v, quer um bloco de texto", turnos)
	}
	if got, want := turnos[0][0].Text, "<turn_context>arquivo aberto</turn_context>\no que tem nesta imagem?"; got != want {
		t.Errorf("texto enviado ao agente = %q, quer %q", got, want)
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

func TestRecusaSemTextoViraRespostaDoTurno(t *testing.T) {
	sessao := &agenteFalso{stop: acp.StopRefusal}
	provider := providerDeAgente(t, sessao)
	handler := &espiao{}

	provider.StreamChat(t.Context(),
		[]Message{{Role: "user", Content: "faz algo proibido"}},
		ChatParams{ConversationID: "conversa-1"}, handler)

	// Recusa é o turno terminando, não o transporte falhando: como erro, o
	// texto não seria salvo nem falado, e a auto-recuperação repetiria para o
	// agente um pedido que ele já aceitou.
	if handler.erro != "" {
		t.Fatalf("recusa virou erro: %s", handler.erro)
	}
	if !handler.pronto || !strings.Contains(handler.respostaFim, "recusou") {
		t.Errorf("resposta final = %q, quer contar que o agente recusou", handler.respostaFim)
	}
}

func TestInterrupcaoVindaDoAgenteNaoViraMensagemVazia(t *testing.T) {
	sessao := &agenteFalso{stop: acp.StopCancelled}
	provider := providerDeAgente(t, sessao)
	handler := &espiao{}

	// Aqui quem parou o turno foi o agente: o ctx segue vivo, e sem contar o
	// desfecho a pessoa receberia uma mensagem vazia sem saber por quê.
	provider.StreamChat(t.Context(),
		[]Message{{Role: "user", Content: "faz aí"}},
		ChatParams{ConversationID: "conversa-1"}, handler)

	if handler.erro != "" {
		t.Fatalf("interrupção do agente virou erro: %s", handler.erro)
	}
	if !strings.Contains(handler.respostaFim, "interrompeu") {
		t.Errorf("resposta final = %q, quer contar que o agente interrompeu o turno", handler.respostaFim)
	}
}

func TestLimiteDeTokensSemTextoViraRespostaDoTurno(t *testing.T) {
	sessao := &agenteFalso{stop: acp.StopMaxTokens}
	provider := providerDeAgente(t, sessao)
	handler := &espiao{}

	provider.StreamChat(t.Context(),
		[]Message{{Role: "user", Content: "escreve um livro"}},
		ChatParams{ConversationID: "conversa-1"}, handler)

	if handler.erro != "" {
		t.Fatalf("limite de tokens virou erro: %s", handler.erro)
	}
	if !strings.Contains(handler.respostaFim, "limite de tokens") {
		t.Errorf("resposta final = %q, quer explicar o limite atingido", handler.respostaFim)
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
