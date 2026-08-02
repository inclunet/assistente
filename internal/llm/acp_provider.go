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

	session, err := p.session(ctx, params)
	if err != nil {
		handler.OnError(err.Error())
		return
	}
	content, err := p.promptContent(ctx, messages)
	if err != nil {
		handler.OnError(err.Error())
		return
	}

	turn := &acpTurn{handler: handler}
	// O sink roda na goroutine de entrega do transporte, mas Prompt só volta
	// depois de desligá-lo sob trava: o que o turno acumulou pode ser lido aqui
	// sem sincronização adicional.
	stop, err := session.Prompt(ctx, content, turn.update)
	turn.finishThinking()

	if ctx.Err() != nil {
		// Quem pediu para parar já é dono do desfecho: o laço de streaming
		// persiste o parcial e emite o evento terminal. Um erro daqui viraria
		// aviso de falha para uma interrupção que a própria pessoa pediu.
		return
	}
	if err != nil {
		// TODO(AEP-0084 D4): marcar como não retentável o erro de turno já
		// aceito, para a auto-recuperação não repetir edições e comandos.
		handler.OnError(p.turnError(err))
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

// session empresta do serviço a sessão que o agente mantém para esta conversa.
func (p *ACPChatProvider) session(ctx context.Context, params ChatParams) (acp.Session, error) {
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
	session := conv.Session()
	if session == nil {
		return nil, errors.New("agente sem sessão para esta conversa")
	}
	return session, nil
}

// promptContent monta o que vai ao agente neste turno: só a última mensagem do
// usuário (AEP-0084 D4). O histórico está na sessão dele, e reenviá-lo
// duplicaria contexto e custo.
func (p *ACPChatProvider) promptContent(ctx context.Context, messages []Message) ([]acp.Content, error) {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role != "user" {
			continue
		}
		if n := imagePartCount(messages[i]); n > 0 {
			logging.Warnf(ctx, acpProviderComponent,
				"[ACP] %d anexo(s) de imagem ficaram de fora do turno: o envio multimodal ao agente ainda não está implementado", n)
		}
		text := strings.TrimSpace(messages[i].GetContentAsString())
		if text == "" {
			return nil, errors.New("mensagem sem texto para enviar ao agente")
		}
		return []acp.Content{acp.TextContent(text)}, nil
	}
	return nil, errors.New("turno sem mensagem do usuário para enviar ao agente")
}

// turnError traduz a falha do turno para uma frase que diz o que aconteceu com
// o agente — quem lê precisa saber se o pedido chegou a sair da máquina.
func (p *ACPChatProvider) turnError(err error) string {
	switch {
	case errors.Is(err, acp.ErrSessionLost):
		return "O processo do agente caiu durante o turno. Envie novamente para reconectar."
	case errors.Is(err, acp.ErrSessionClosed):
		return "A sessão do agente para esta conversa foi encerrada."
	case errors.Is(err, acp.ErrCancelNotConfirmed):
		return "O agente não confirmou a interrupção do turno e pode ainda estar trabalhando nos arquivos. Confira o estado antes de pedir de novo."
	case errors.Is(err, acp.ErrConversationGone):
		return "A conversa foi encerrada antes de o agente responder."
	default:
		return fmt.Sprintf("Falha no turno do agente: %v", err)
	}
}

// stopWithoutAnswer devolve o que dizer quando o turno terminou sem texto
// nenhum. Com resposta escrita, o motivo é informação; sem ela, é o desfecho —
// e uma mensagem vazia não conta à pessoa que o agente recusou ou esbarrou num
// limite.
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
		return "", false
	}
}

// imagePartCount conta os anexos de imagem da mensagem. A lista de partes
// chega nos dois formatos que o pipeline produz: tipada, quando o builder a
// montou, e destipada, quando ela veio de JSON.
func imagePartCount(msg Message) int {
	count := 0
	switch parts := msg.Content.(type) {
	case []ContentPart:
		for _, part := range parts {
			if part.Type == "image_url" {
				count++
			}
		}
	case []interface{}:
		for _, part := range parts {
			if partMap, ok := part.(map[string]interface{}); ok && partMap["type"] == "image_url" {
				count++
			}
		}
	}
	return count
}

// acpTurn acumula o turno e o entrega ao StreamHandler (AEP-0084 D8). Texto vai
// como chunk e pensamento como raciocínio; ferramentas do agente e segmentação
// da fala entram na sequência desta fase.
type acpTurn struct {
	handler   StreamHandler
	text      strings.Builder
	reasoning strings.Builder
	thinking  bool
}

func (t *acpTurn) update(update acp.Update) {
	switch update.Kind {
	case acp.UpdateText:
		t.text.WriteString(update.Text)
		t.handler.OnChunk(update.Text)
	case acp.UpdateThought:
		t.thinking = true
		t.reasoning.WriteString(update.Text)
		t.handler.OnThinking(update.Text)
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
