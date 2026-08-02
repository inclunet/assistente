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
	chunks        []string
	pensamento    []string
	raciocinio    string
	fimPensou     bool
	ordem         []string
	erro          string
	naoRetentavel bool
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

func (e *espiao) texto() string { return strings.Join(e.chunks, "") }

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
