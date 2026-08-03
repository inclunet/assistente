package llm

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"assistente/internal/acp"
	"assistente/internal/logging"
)

const acpProviderComponent = "llm.acp-provider"

// ErrACPAuxiliaryRole recusa um agente de código em papel auxiliar — resumo,
// título, classificação (AEP-0084 D14). Esses papéis chamam o provedor por
// fora da conversa, e um agente cobraria um turno inteiro de agente de código
// para escrever um parágrafo. A guarda de verdade está em quem dispara o papel;
// esta aqui é a última linha, para um caminho novo não descobrir isso na conta.
var ErrACPAuxiliaryRole = errors.New("papel auxiliar não usa provedor de agente de código (AEP-0084 D14)")

// ACPChatProvider é um agente de código local no barramento como qualquer outro
// provedor (AEP-0084 D1). A diferença que atravessa tudo aqui é o histórico:
// ele vive na sessão do agente, não na request, então o turno leva só a
// mensagem nova (D4) e o processo que a recebe é emprestado pelo serviço de
// longa duração (D3) — nunca criado por esta instância, que nasce e morre a
// cada chamada de GetChatProvider.
type ACPChatProvider struct {
	provider *ProviderConfig
	agents   *acp.Manager
}

var _ ChatProvider = (*ACPChatProvider)(nil)

// NewACPChatProvider cria o provider do agente descrito em provider. agents é o
// serviço dono dos processos e das sessões; sem ele o provider recusa o turno
// em vez de subir um agente por conta própria.
func NewACPChatProvider(provider *ProviderConfig, agents *acp.Manager) *ACPChatProvider {
	return &ACPChatProvider{provider: provider, agents: agents}
}

// StreamChat conduz um turno do agente e traduz o que ele emite para o
// StreamHandler (AEP-0084 D8).
//
// As ferramentas do app não são oferecidas: quem executa ferramenta no turno de
// um agente é o agente (D7). O roteamento já planeja o turno sem elas, e uma
// que chegue aqui é descartada com registro, porque mandá-la ao agente não é
// sequer possível pelo protocolo.
func (p *ACPChatProvider) StreamChat(ctx context.Context, messages []Message, params ChatParams, handler StreamHandler, tools ...ToolDefinition) {
	if handler == nil {
		return
	}
	if len(tools) > 0 {
		logging.Warnf(ctx, acpProviderComponent,
			"[ACP] turno do agente chegou com %d ferramentas do app; ignoradas (AEP-0084 D7)", len(tools))
	}

	conv, err := p.conversation(ctx, params)
	if err != nil {
		handler.OnError(err.Error())
		return
	}
	session := conv.Session()
	if session == nil {
		handler.OnError("agente sem sessão para esta conversa")
		return
	}
	instructions := profileInstructions(messages, conv)
	content, notSent, err := p.promptContent(ctx, conv, messages)
	if err != nil {
		handler.OnError(err.Error())
		return
	}
	content = append(instructions.blocks(), content...)

	// O agente pergunta no meio do turno, e o pedido chega por outra goroutine
	// sabendo só o nome da sessão. É esta marca que diz a quem perguntar — e
	// se há alguém (AEP-0084 D9).
	endTurn := conv.BeginTurn(turnHasWatcher(params))
	defer endTurn()

	// O canal de atividade é opcional: sem ele o turno ainda entrega texto e
	// raciocínio, só não conta as ferramentas do agente nem fecha segmentos.
	activity, _ := handler.(AgentActivitySink)
	turn := &acpTurn{handler: handler, activity: activity}
	// O sink roda na goroutine de entrega do transporte, mas Prompt só volta
	// depois de desligá-lo sob trava: o que o turno acumulou pode ser lido aqui
	// sem sincronização adicional.
	stop, err := session.Prompt(ctx, content, turn.update)
	turn.finishThinking()
	if err == nil {
		// O agente ouviu o que foi mandado, mesmo que a pessoa interrompa em
		// seguida: o que já foi dito não precisa ser repetido no próximo turno.
		instructions.markSent(ctx, conv)
	}

	if ctx.Err() != nil {
		// Quem pediu para parar já é dono do desfecho: o laço de streaming
		// persiste o parcial e emite o evento terminal. Um erro daqui viraria
		// aviso de falha para uma interrupção que a própria pessoa pediu.
		turn.cancelPendingTools()
		return
	}

	accepted := err == nil || turnAccepted(err)
	if notSent > 0 && accepted {
		// O aviso só vale depois de o pedido chegar ao agente: sem aceite nada
		// foi enviado, e dizer que "o turno seguiu só com o texto" mandaria a
		// pessoa conferir uma resposta que não existe. Com aceite, ela precisa
		// saber que o agente não viu o anexo — senão espera resposta sobre ele.
		logging.Warnf(ctx, acpProviderComponent, "[ACP] %d anexo(s) ficaram de fora do turno", notSent)
		if sink, ok := handler.(TurnNoticeSink); ok {
			sink.OnTurnNotice(TurnNotice{Kind: TurnNoticeAttachmentsNotSent, Count: notSent})
		}
	}

	if err != nil {
		if accepted {
			// A auto-recuperação reinvoca StreamChat sozinha depois de um erro
			// de transporte. Para um provider HTTP isso é inofensivo; aqui
			// seria repetir para o agente um pedido que ele já aceitou — ou
			// seja, refazer arquivo editado e comando rodado (AEP-0084 D4).
			if sink, ok := handler.(NonRetryableErrorSink); ok {
				sink.MarkErrorNotRetryable()
			}
		}
		handler.OnError(turnErrorMessage(err, accepted))
		return
	}

	response := turn.response()
	if notice, empty := stopWithoutAnswer(stop, response); empty {
		// O desfecho vai como resposta, e não como erro: recusa e limite são
		// o turno terminando, não o transporte falhando. Como erro, o texto
		// não seria salvo nem falado — e a auto-recuperação ainda repetiria
		// para o agente um pedido que ele já aceitou.
		response = notice
	}
	if stop != acp.StopEndTurn {
		logging.Infof(ctx, acpProviderComponent, "[ACP] turno encerrado por %q", string(stop))
	}
	// Sem contagem de tokens: o agente cobra na conta dele e não reporta uso.
	handler.OnDone(response, Usage{}, resolveModel(p.provider, params.Model))
}

// conversation empresta do serviço a conversa que o agente mantém, com a sessão
// dela e a memória do que o app já contou a essa sessão.
func (p *ACPChatProvider) conversation(ctx context.Context, params ChatParams) (*acp.Conversation, error) {
	if p.provider == nil {
		return nil, errors.New("provedor de agente sem configuração")
	}
	if p.agents == nil {
		return nil, errors.New("serviço de agentes de código indisponível: reinicie o app")
	}
	conversationID := strings.TrimSpace(params.ConversationID)
	if conversationID == "" {
		// Sem conversa não há sessão a que pertencer, e o agente guarda o
		// histórico por sessão: enviar mesmo assim seria falar com uma memória
		// que não é a desta conversa.
		return nil, errors.New("turno sem conversa: provedor de agente de código só atende conversas")
	}
	conv, err := p.agents.Conversation(ctx, acp.ProviderSpec{
		ID:      p.provider.ID,
		Name:    p.provider.Name,
		Command: p.provider.ACPCommand,
		Args:    p.provider.ACPArgs,
		Env:     p.provider.ACPEnv,
	}, conversationID)
	if err != nil {
		return nil, fmt.Errorf("agente indisponível: %w", err)
	}
	return conv, nil
}

// turnHasWatcher diz se este turno saiu de uma superfície de tela identificada
// — a mesma identidade que ports.NewChatSurfaceOrigin exige para dizer que o
// turno tem origem visual (AEP-0042/0080). Turno de canal, de job agendado, de
// subagente ou da CLI chega sem ela, e é justamente aí que o pedido de
// permissão não pode esperar por ninguém (AEP-0084 D9).
//
// Errar para o lado de "não tem ninguém" nega uma ação; errar para o outro
// pendura o agente até o prazo estourar.
func turnHasWatcher(params ChatParams) bool {
	return strings.TrimSpace(params.SurfaceSessionKey) != "" &&
		strings.TrimSpace(params.SurfaceID) != "" &&
		strings.TrimSpace(params.SurfaceType) != ""
}

// promptContent monta o que vai ao agente neste turno: só a última mensagem do
// usuário (AEP-0084 D4). O histórico está na sessão dele, e reenviá-lo
// duplicaria contexto e custo. Devolve também quantos anexos não puderam ir.
func (p *ACPChatProvider) promptContent(ctx context.Context, conv *acp.Conversation, messages []Message) ([]acp.Content, int, error) {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role != "user" {
			continue
		}
		content, notSent := turnContent(messages[i], func() bool { return p.acceptsImage(ctx, conv) })
		if len(content) == 0 {
			if notSent > 0 {
				return nil, 0, errors.New("o anexo não pôde ser enviado ao agente, e a mensagem não tem texto para enviar no lugar")
			}
			return nil, 0, errors.New("mensagem sem texto para enviar ao agente")
		}
		return content, notSent, nil
	}
	return nil, 0, errors.New("turno sem mensagem do usuário para enviar ao agente")
}

// acceptsImage diz se o agente recebe imagem. Não saber conta como não aceitar:
// mandar assim mesmo faria o turno inteiro falhar por causa do anexo, quando
// seguir só com o texto entrega a resposta e ainda conta o que ficou de fora.
func (p *ACPChatProvider) acceptsImage(ctx context.Context, conv *acp.Conversation) bool {
	caps, err := conv.Capabilities(ctx)
	if err != nil {
		logging.Warnf(ctx, acpProviderComponent,
			"[ACP] capacidades do agente indisponíveis; anexos ficam de fora deste turno: %v", err)
		return false
	}
	return caps.PromptImage
}

// turnAccepted diz se o pedido chegou a sair para o agente. A definição é
// conservadora de propósito (AEP-0084 D4): o turno conta como aceito assim que
// o envio não falha, e um erro que não sabe dizer conta como aceito. Silêncio
// depois do envio não prova que nada aconteceu — a linha já está com o agente,
// e ele pode estar editando.
func turnAccepted(err error) bool {
	var promptErr *acp.PromptError
	if errors.As(err, &promptErr) {
		return promptErr.Accepted
	}
	return true
}

// turnErrorMessage traduz a falha do turno para uma frase que diz o que
// aconteceu com o agente. Ter aceitado o pedido ou não muda a orientação: sem
// aceite, reenviar é seguro; com aceite, pode haver trabalho feito pela metade
// no disco, e mandar de novo sem conferir repetiria edição e comando.
func turnErrorMessage(err error, accepted bool) string {
	switch {
	case errors.Is(err, acp.ErrCancelNotConfirmed):
		return "O agente não confirmou a interrupção do turno e pode ainda estar trabalhando nos arquivos. Confira o estado antes de pedir de novo."
	case errors.Is(err, acp.ErrSessionLost) && !accepted:
		return "O processo do agente caiu antes de receber o pedido. Envie novamente para reconectar."
	case errors.Is(err, acp.ErrSessionLost):
		return "O processo do agente caiu no meio do turno. Ele pode ter feito parte do pedido; confira o estado antes de enviar de novo."
	case errors.Is(err, acp.ErrSessionClosed):
		return "A sessão do agente para esta conversa foi encerrada."
	case errors.Is(err, acp.ErrConversationGone):
		return "A conversa foi encerrada antes de o agente responder."
	case accepted:
		return fmt.Sprintf("Falha no turno depois de o agente aceitá-lo; ele pode ter feito parte do pedido. Confira o estado antes de enviar de novo: %v", err)
	default:
		return fmt.Sprintf("Falha no turno do agente: %v", err)
	}
}

// stopWithoutAnswer devolve o que dizer quando o turno terminou sem texto
// nenhum. Com resposta escrita, o motivo é informação e o texto basta; sem ela,
// o motivo é o desfecho — e uma mensagem vazia não conta à pessoa que o agente
// recusou, esbarrou num limite ou simplesmente não escreveu nada.
func stopWithoutAnswer(stop acp.StopReason, response string) (string, bool) {
	if strings.TrimSpace(response) != "" {
		return "", false
	}
	switch stop {
	case acp.StopRefusal:
		return "O agente recusou o pedido.", true
	case acp.StopCancelled:
		// Interrupção partida do agente: quem parou o turno não foi a pessoa,
		// que sem isso receberia uma mensagem vazia sem saber por quê. A
		// interrupção pedida por ela nem chega aqui — sai antes, pelo ctx.
		return "O agente interrompeu o turno antes de responder.", true
	case acp.StopMaxTokens:
		return "O agente atingiu o limite de tokens antes de escrever a resposta.", true
	case acp.StopMaxTurnRequests:
		return "O agente atingiu o limite de requisições do turno antes de escrever a resposta.", true
	default:
		// Um agente pode terminar sem escrever nada — só rodou ferramentas, por
		// exemplo. Dizer isso é melhor que uma bolha vazia, que para quem ouve
		// é o mesmo que resposta nenhuma sem explicação.
		return "O agente terminou o turno sem escrever resposta.", true
	}
}

// acpTurn acumula o turno e o entrega ao StreamHandler (AEP-0084 D8). Texto vai
// como chunk, pensamento como raciocínio e a atividade de ferramenta pelo canal
// próprio do agente, que também fecha os segmentos de fala (D7 e D13).
type acpTurn struct {
	handler   StreamHandler
	activity  AgentActivitySink
	text      strings.Builder
	reasoning strings.Builder
	thinking  bool
	// segmentPending diz que há texto escrito depois do último corte, e é o que
	// impede um segmento vazio quando o agente emenda uma ferramenta na outra.
	segmentPending bool
	// tools guarda o que cada chamada em andamento anunciou, porque a
	// atualização só repete o que mudou.
	tools     map[string]agentToolState
	toolOrder []string
	// lastAnonymous é a chave da última chamada sem identificador ainda aberta,
	// e é por ela que uma atualização sem identificador nem classe encontra o
	// começo dela.
	lastAnonymous string
}

// agentToolState é o que o app precisa lembrar de uma chamada entre o começo e
// o fim dela, já saneado.
type agentToolState struct {
	// id é o identificador que veio do agente, que pode ser vazio: a chave de
	// acompanhamento é outra coisa, e quem recebe o evento precisa do original.
	id    string
	kind  string
	title string
	// done marca a chamada que já teve desfecho. Um agente que repete o aviso
	// de conclusão criaria uma segunda ferramenta na tela, porque para o
	// handler ela seria um fim sem começo.
	done bool
}

func (t *acpTurn) update(update acp.Update) {
	switch update.Kind {
	case acp.UpdateText:
		t.text.WriteString(update.Text)
		t.segmentPending = true
		t.handler.OnChunk(update.Text)
	case acp.UpdateThought:
		t.thinking = true
		t.reasoning.WriteString(update.Text)
		t.handler.OnThinking(update.Text)
	case acp.UpdateToolStart, acp.UpdateToolProgress:
		t.toolActivity(update.Tool)
	}
}

// toolActivity conta o que o agente está fazendo com as ferramentas dele. O app
// não executa nada aqui e nunca chama OnToolCalls: esse callback significa "o
// modelo pediu que o app execute uma tool" e mandaria o turno para o loop
// agêntico tentar rodar uma ferramenta que não é dele (AEP-0084 D7).
func (t *acpTurn) toolActivity(call *acp.ToolCall) {
	if t.activity == nil || call == nil {
		return
	}
	key := t.trackingKey(call)
	state, seen := t.tools[key]
	status := agentToolStatus(call.Status)
	if state.done {
		// Depois do desfecho, só um novo começo reabre o acompanhamento. O aviso
		// terminal repetido não pode passar: para o handler, um fim sem começo é
		// chamada nova, e ele abriria uma ferramenta fantasma na tela.
		if status != AgentToolRunning {
			return
		}
		state = agentToolState{}
	}
	state.id = call.ID
	// A atualização de uma chamada traz só o que mudou; o campo que vier vazio
	// continua valendo o que foi anunciado no começo.
	if strings.TrimSpace(call.Kind) != "" {
		state.kind = agentToolKind(call.Kind)
	}
	if state.kind == "" {
		state.kind = AgentToolKindOther
	}
	if strings.TrimSpace(call.Title) != "" {
		state.title = acp.SanitizeLabel(call.Title)
	}

	// O bloco de texto que veio antes desta atividade está encerrado: vira
	// segmento e é lido em voz alta agora, em vez de esperar um turno que pode
	// passar minutos alternando texto e ferramenta (AEP-0084 D13). Vale para
	// qualquer atividade, e não só para o início de uma chamada: o agente
	// também escreve entre duas atualizações da mesma ferramenta.
	t.cutSegment()
	if !seen {
		if t.tools == nil {
			t.tools = map[string]agentToolState{}
		}
		t.toolOrder = append(t.toolOrder, key)
	}

	state.done = status != AgentToolRunning
	t.tools[key] = state
	if call.ID == "" {
		if state.done && t.lastAnonymous == key {
			t.lastAnonymous = ""
		} else if !state.done {
			t.lastAnonymous = key
		}
	}

	t.activity.OnAgentToolEvent(AgentToolEvent{
		ID:     call.ID,
		Kind:   state.kind,
		Title:  state.title,
		Status: status,
	})
}

// trackingKey escolhe como acompanhar a chamada entre o começo e o fim dela. O
// protocolo exige identificador, mas o Cursor já mandou tool call sem — e, sem
// ele, a classe é o que resta para ligar as pontas, que é como o handler
// também correlaciona. Uma chave só para todas as anônimas faria uma engolir os
// eventos da outra.
func (t *acpTurn) trackingKey(call *acp.ToolCall) string {
	if call.ID != "" {
		return call.ID
	}
	// A atualização traz só o que mudou, então ela pode vir sem classe também.
	// Aí a última chamada anônima ainda aberta é a correlação possível: cair na
	// classe "other" abriria uma ferramenta fantasma para o que é o fim de uma
	// que já está na tela.
	if strings.TrimSpace(call.Kind) == "" && t.lastAnonymous != "" {
		return t.lastAnonymous
	}
	return "\x00anônima:" + agentToolKind(call.Kind)
}

// cutSegment fecha o bloco corrente de resposta. Sem texto novo desde o último
// corte não há bloco: cortar aí só produziria segmento vazio.
func (t *acpTurn) cutSegment() {
	if t.activity == nil || !t.segmentPending {
		return
	}
	t.segmentPending = false
	t.activity.OnSegmentDone()
}

// cancelPendingTools dá desfecho às ferramentas que ficaram em andamento quando
// a pessoa manda parar. Este caminho não passa por OnDone nem por OnError, que
// são onde o handler faz essa limpeza — sem isso, a ferramenta giraria na tela
// até o fim dos tempos.
func (t *acpTurn) cancelPendingTools() {
	if t.activity == nil {
		return
	}
	for _, key := range t.toolOrder {
		state, running := t.tools[key]
		if !running || state.done {
			continue
		}
		state.done = true
		t.tools[key] = state
		t.activity.OnAgentToolEvent(AgentToolEvent{
			ID:     state.id,
			Kind:   state.kind,
			Title:  state.title,
			Status: AgentToolCancelled,
		})
	}
}

// agentToolKinds é o conjunto de classes do protocolo. O kind vira o nome
// exibido e anunciado da ferramenta, então aceitar qualquer string deixaria o
// agente escrever direto no anúncio do leitor de telas; o que não for do
// conjunto conhecido cai em "other" (AEP-0084 D7 e D11).
var agentToolKinds = map[string]struct{}{
	"read":        {},
	"edit":        {},
	"delete":      {},
	"move":        {},
	"search":      {},
	"execute":     {},
	"think":       {},
	"fetch":       {},
	"switch_mode": {},
	"other":       {},
}

func agentToolKind(kind string) string {
	normalized := strings.ToLower(strings.TrimSpace(kind))
	if _, known := agentToolKinds[normalized]; known {
		return normalized
	}
	return AgentToolKindOther
}

// agentToolStatus traduz o ciclo de vida do protocolo. Pendente e em andamento
// são a mesma coisa para quem olha a tela: a ferramenta ainda não terminou.
func agentToolStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "completed":
		return AgentToolCompleted
	case "failed":
		return AgentToolFailed
	default:
		return AgentToolRunning
	}
}

// finishThinking fecha o raciocínio do turno, uma vez só e no fim, como fazem
// os demais provedores do barramento. Um agente alterna pensamento e resposta
// várias vezes no mesmo turno, e fechar a cada troca faria a UI abrir e fechar
// o estado de "pensando" no meio do texto — barulho que nenhum outro provedor
// produz.
func (t *acpTurn) finishThinking() {
	if !t.thinking {
		return
	}
	t.thinking = false
	t.handler.OnThinkingDone(t.reasoning.String())
}

func (t *acpTurn) response() string {
	return t.text.String()
}

// SendChat não existe para um agente: o turno dele é conduzido por streaming e
// pertence a uma conversa com sessão. Quem precisa de resposta única está num
// papel auxiliar, que o D14 mantém fora do agente.
func (p *ACPChatProvider) SendChat(ctx context.Context, messages []Message, params ChatParams) (string, error) {
	return "", ErrACPAuxiliaryRole
}

// SimpleChat é o atalho dos papéis auxiliares e, por isso mesmo, recusado aqui
// (AEP-0084 D14).
func (p *ACPChatProvider) SimpleChat(ctx context.Context, model, systemPrompt, userMessage string) (string, error) {
	return "", ErrACPAuxiliaryRole
}

// GetModels depende de uma sessão de descoberta no processo do agente e chega
// com a troca de modelo e modo (AEP-0084 D6, Fase 4). Devolver uma lista vazia
// em silêncio faria a tela de modelos parecer quebrada.
func (p *ACPChatProvider) GetModels(ctx context.Context) ([]string, error) {
	return nil, errors.New("listar modelos de um agente de código ainda não está disponível: quem escolhe o modelo é o agente")
}

// NativeMCPCapable é falso: o MCP de um agente é dele, configurado no projeto
// (AEP-0084 D1). O app não injeta servidor no turno do agente.
func (p *ACPChatProvider) NativeMCPCapable() bool { return false }

// WithMCPServers é no-op pelo mesmo motivo.
func (p *ACPChatProvider) WithMCPServers(servers []MCPServerConfig) ChatProvider { return p }
