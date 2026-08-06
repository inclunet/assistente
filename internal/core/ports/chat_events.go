package ports

// Chat event structs — typed payloads for all chat:* events.
// Every chat event MUST carry ConversationID (AEP-0040).

const (
	// ChatErrorInternal is a stable UI-facing error code for unexpected backend failures.
	ChatErrorInternal = "internal_error"
	// ChatErrorAssistantPlaceholder is emitted when the assistant placeholder cannot be created.
	ChatErrorAssistantPlaceholder = "assistant_placeholder_error"
)

// ChatSurfaceOrigin identifies the frontend surface that initiated a chat turn.
type ChatSurfaceOrigin struct {
	SessionKey     string `json:"sessionKey"`
	ConversationID string `json:"conversationId"`
	TabID          string `json:"tabId,omitempty"`
	SurfaceID      string `json:"surfaceId"`
	SurfaceType    string `json:"surfaceType"`
}

func NewChatSurfaceOrigin(conversationID, sessionKey, surfaceID, surfaceType, tabID string) *ChatSurfaceOrigin {
	if sessionKey == "" || surfaceID == "" || surfaceType == "" {
		return nil
	}
	return &ChatSurfaceOrigin{
		SessionKey:     sessionKey,
		ConversationID: conversationID,
		TabID:          tabID,
		SurfaceID:      surfaceID,
		SurfaceType:    surfaceType,
	}
}

// ThinkingEvent is the payload for chat:thinking.
type ThinkingEvent struct {
	ConversationID     string             `json:"conversationId"`
	TurnID             string             `json:"turnId,omitempty"`
	AssistantMessageID string             `json:"assistantMessageId,omitempty"`
	Content            string             `json:"content,omitempty"`
	Done               bool               `json:"done"`
	Started            bool               `json:"started,omitempty"`
	SurfaceOrigin      *ChatSurfaceOrigin `json:"surfaceOrigin,omitempty"`
}

// DoneEvent is the payload for chat:done.
type DoneEvent struct {
	ConversationID     string `json:"conversationId"`
	TurnID             string `json:"turnId,omitempty"`
	AssistantMessageID string `json:"assistantMessageId,omitempty"`
	HadToolCalls       bool   `json:"hadToolCalls,omitempty"`
	// AEP-0039 Fase 2: enriched done event
	Reason           string             `json:"reason,omitempty"` // "completed" | "limit_reached" | "error"
	IterationCount   int                `json:"iterationCount,omitempty"`
	ToolCallCount    int                `json:"toolCallCount,omitempty"`
	ToolsUsed        []string           `json:"toolsUsed,omitempty"`
	PromptTokens     int                `json:"promptTokens,omitempty"`
	CompletionTokens int                `json:"completionTokens,omitempty"`
	CacheReadTokens  int                `json:"cacheReadTokens,omitempty"`
	CacheWriteTokens int                `json:"cacheWriteTokens,omitempty"`
	CacheMissTokens  int                `json:"cacheMissTokens,omitempty"`
	ErrorMessage     string             `json:"errorMessage,omitempty"`
	SurfaceOrigin    *ChatSurfaceOrigin `json:"surfaceOrigin,omitempty"`
}

// ErrorEvent is the payload for chat:error.
type ErrorEvent struct {
	ConversationID string             `json:"conversationId"`
	Error          string             `json:"error"`
	SurfaceOrigin  *ChatSurfaceOrigin `json:"surfaceOrigin,omitempty"`
}

// MessagesReadyEvent is the payload for chat:messages_ready.
type MessagesReadyEvent struct {
	ConversationID string             `json:"conversationId"`
	UserMessageID  string             `json:"userMessageId"`
	TurnID         string             `json:"turnId,omitempty"`
	UserContent    string             `json:"userContent"`
	SurfaceOrigin  *ChatSurfaceOrigin `json:"surfaceOrigin,omitempty"`
}

// SkillLoadedEvent is the payload for chat:skill_loaded.
type SkillLoadedEvent struct {
	ConversationID string             `json:"conversationId"`
	TurnID         string             `json:"turnId,omitempty"`
	Slug           string             `json:"slug"`
	DisplayName    string             `json:"displayName,omitempty"`
	Mode           string             `json:"mode,omitempty"`
	SurfaceOrigin  *ChatSurfaceOrigin `json:"surfaceOrigin,omitempty"`
}

// ToolStartEvent is the payload for chat:tool_start.
type ToolStartEvent struct {
	ConversationID     string `json:"conversationId"`
	TurnID             string `json:"turnId,omitempty"`
	AssistantMessageID string `json:"assistantMessageId,omitempty"`
	Name               string `json:"name"`
	CallID             string `json:"callId"`
	Args               string `json:"args,omitempty"`
	// Summary descreve a chamada em texto legível quando a ferramenta não é do
	// app e não há argumentos estruturados para mostrar (AEP-0084 D7).
	Summary       string             `json:"summary,omitempty"`
	ServerLabel   string             `json:"serverLabel,omitempty"`
	Origin        string             `json:"origin,omitempty"` // "builtin" | "mcp_bridge" | "mcp_native" | "acp_agent"
	Attempt       int                `json:"attempt"`          // Tentativa (0=primeira, 1=retry)
	SurfaceOrigin *ChatSurfaceOrigin `json:"surfaceOrigin,omitempty"`
}

// ToolEndEvent is the payload for chat:tool_end.
type ToolEndEvent struct {
	ConversationID     string             `json:"conversationId"`
	TurnID             string             `json:"turnId,omitempty"`
	AssistantMessageID string             `json:"assistantMessageId,omitempty"`
	Name               string             `json:"name,omitempty"`
	CallID             string             `json:"callId"`
	Status             string             `json:"status"`
	Summary            string             `json:"summary,omitempty"`
	Error              string             `json:"error,omitempty"`
	ServerLabel        string             `json:"serverLabel,omitempty"`
	Origin             string             `json:"origin,omitempty"`     // "builtin" | "mcp_bridge" | "mcp_native" | "acp_agent"
	DurationMs         int64              `json:"durationMs,omitempty"` // AEP-0039 Fase 3
	Attempt            int                `json:"attempt"`              // Tentativa (0=primeira, 1=retry)
	SurfaceOrigin      *ChatSurfaceOrigin `json:"surfaceOrigin,omitempty"`
}

// ToolFailureEvent is the payload for chat:tool_failure (AEP-0039 Fase 3).
// Emitted when a tool execution fails with structured error classification.
// Distinct from tool_end with status="error" — this carries retry context.
type ToolFailureEvent struct {
	ConversationID     string             `json:"conversationId"`
	TurnID             string             `json:"turnId,omitempty"`
	AssistantMessageID string             `json:"assistantMessageId,omitempty"`
	Name               string             `json:"name"`
	CallID             string             `json:"callId"`
	ErrorKind          string             `json:"errorKind"` // "timeout" | "invalid_args" | "not_found" | "panic" | "cancelled" | "unknown"
	Retryable          bool               `json:"retryable"`
	Message            string             `json:"message,omitempty"`
	DurationMs         int64              `json:"durationMs,omitempty"`
	Origin             string             `json:"origin,omitempty"`    // "builtin" | "mcp_bridge" | "mcp_native" | "acp_agent"
	WillRetry          bool               `json:"willRetry,omitempty"` // true se retry automático será tentado
	Attempt            int                `json:"attempt"`             // Tentativa (0=primeira, 1=retry)
	SurfaceOrigin      *ChatSurfaceOrigin `json:"surfaceOrigin,omitempty"`
}

// ToolSummary describes a tool invocation within an iteration (AEP-0039 Fase 2+3).
type ToolSummary struct {
	Name        string `json:"name"`
	Status      string `json:"status"`               // "ok" | "error"
	ErrorKind   string `json:"errorKind,omitempty"`  // AEP-0039 Fase 3
	DurationMs  int64  `json:"durationMs,omitempty"` // AEP-0039 Fase 3
	Origin      string `json:"origin,omitempty"`     // "builtin" | "mcp_bridge" | "mcp_native" | "acp_agent"
	ServerLabel string `json:"serverLabel,omitempty"`
}

// SegmentDoneEvent is the payload for chat:segment_done.
type SegmentDoneEvent struct {
	ConversationID     string `json:"conversationId"`
	TurnID             string `json:"turnId,omitempty"`
	AssistantMessageID string `json:"assistantMessageId,omitempty"`
	Content            string `json:"content,omitempty"`
	Iteration          int    `json:"iteration"`
	HasMore            bool   `json:"hasMore"`
	// AEP-0039 Fase 2: tools executed in this iteration
	ToolsInIteration []ToolSummary      `json:"toolsInIteration,omitempty"`
	SurfaceOrigin    *ChatSurfaceOrigin `json:"surfaceOrigin,omitempty"`
}

// TokenStatsEvent is the payload for chat:token_stats.
type TokenStatsEvent struct {
	ConversationID      string  `json:"conversationId"`
	TotalTokens         int     `json:"totalTokens"`
	ContextTokens       int     `json:"contextTokens"`
	ContextLimit        int     `json:"contextLimit"`
	ContextUsage        float64 `json:"contextUsage"`
	IsNearLimit         bool    `json:"isNearLimit"`
	IsCritical          bool    `json:"isCritical"`
	PromptTokens        int     `json:"promptTokens"`
	CompletionTokens    int     `json:"completionTokens"`
	CacheReadTokens     int     `json:"cacheReadTokens,omitempty"`
	CacheWriteTokens    int     `json:"cacheWriteTokens,omitempty"`
	CacheMissTokens     int     `json:"cacheMissTokens,omitempty"`
	CacheHitRate        float64 `json:"cacheHitRate"`
	CacheTokensReported bool    `json:"cacheTokensReported"`
	PromptCacheEnabled  *bool   `json:"promptCacheEnabled,omitempty"`
	MessageCount        int     `json:"messageCount"`
	ModelCallCount      int     `json:"modelCallCount"`
}

// TokenStatsUpdateEvent is the payload for chat:token_stats_update (realtime during agentic loop).
type TokenStatsUpdateEvent struct {
	ConversationID              string  `json:"conversationId"`
	PromptTokens                int     `json:"promptTokens"`
	CompletionTokens            int     `json:"completionTokens"`
	TotalTokens                 int     `json:"totalTokens"`
	CacheReadTokens             int     `json:"cacheReadTokens,omitempty"`
	CacheWriteTokens            int     `json:"cacheWriteTokens,omitempty"`
	CacheMissTokens             int     `json:"cacheMissTokens,omitempty"`
	CacheHitRate                float64 `json:"cacheHitRate"`
	CacheTokensReported         bool    `json:"cacheTokensReported"`
	PromptCacheEnabled          *bool   `json:"promptCacheEnabled,omitempty"`
	ContextTokens               int     `json:"contextTokens"`
	ContextUsage                float64 `json:"contextUsage"`
	ContextLimit                int     `json:"contextLimit"`
	IsNearLimit                 bool    `json:"isNearLimit"`
	IsCritical                  bool    `json:"isCritical"`
	MessageCount                int     `json:"messageCount"`
	ModelCallCount              int     `json:"modelCallCount"`
	SystemPromptEstimatedTokens int     `json:"systemPromptEstimatedTokens"`
	SummaryTokens               int     `json:"summaryTokens"`
	MessagesInContextCount      int     `json:"messagesInContextCount"`
	MessagesInContextTokens     int     `json:"messagesInContextTokens"`
	MessagesOutOfContextCount   int     `json:"messagesOutOfContextCount"`
	MessagesOutOfContextTokens  int     `json:"messagesOutOfContextTokens"`
	ToolsUsedCount              int     `json:"toolsUsedCount"`
	ToolBreakdown               any     `json:"toolBreakdown,omitempty"`
}

// ContextWarningEvent is the payload for chat:context_warning.
// ContextTokens reflete a ocupação ATUAL da janela de contexto (usage do
// último turno), não o acumulado de billing (issue #197 / AEP-0012).
type ContextWarningEvent struct {
	ConversationID string  `json:"conversationId"`
	Level          string  `json:"level"` // "warning" | "critical"
	Message        string  `json:"message"`
	Percentage     float64 `json:"percentage"`
	ContextTokens  int     `json:"contextTokens"`
	ContextLimit   int     `json:"contextLimit"`
}

// ChatNoticeKindAttachmentsNotSent identifica o turno que seguiu sem parte dos
// anexos, porque o provedor não os recebe (AEP-0084). Não é falha: o turno
// segue com o texto, e a pessoa é avisada do que ficou de fora em vez de
// esperar uma resposta sobre uma imagem que o modelo nunca viu.
const ChatNoticeKindAttachmentsNotSent = "attachments_not_sent"

// O modelo escolhido não pôde valer neste turno (AEP-0084 D6). O turno seguiu no
// modelo em que o agente está, porque resposta do modelo errado é melhor do que
// resposta nenhuma — mas quem escolheu precisa saber, senão lê a resposta
// atribuindo-a a um modelo que não a escreveu.
const (
	// ChatNoticeKindModelNotOffered é o modelo que este agente não tem.
	ChatNoticeKindModelNotOffered = "model_not_offered"
	// ChatNoticeKindModelNotApplied é a troca que o agente recusou.
	ChatNoticeKindModelNotApplied = "model_not_applied"
)

// Permissão que o agente pediu e o app negou sem que ninguém decidisse
// (AEP-0084 D9). Negar é melhor do que pendurar o turno, mas negar em silêncio
// deixa a pessoa diante de um agente que desiste sem explicar por quê.
const (
	// ChatNoticeKindPermissionNoWatcher é o turno sem tela onde perguntar —
	// canal, job agendado, subagente, CLI não interativa.
	ChatNoticeKindPermissionNoWatcher = "permission_denied_no_watcher"
	// ChatNoticeKindPermissionTimeout é a pergunta que foi feita e ficou sem
	// resposta dentro do prazo.
	ChatNoticeKindPermissionTimeout = "permission_denied_timeout"
	// ChatNoticeKindPermissionUnavailable é o pedido que o app não conseguiu
	// apresentar: sem o questionário no ar, ou sem opção nenhuma para oferecer.
	ChatNoticeKindPermissionUnavailable = "permission_denied_unavailable"
)

// Pergunta bloqueante do agente (cursor/ask_question) que o app pulou sem que
// ninguém decidisse (AEP-0084 D9). Diferente da permissão, aqui nada foi
// negado: o agente perguntou e seguiu o turno sem a resposta, o que costuma
// mudar o que ele entrega — e sem aviso a pessoa lê a resposta sem saber que
// ela partiu de um palpite.
const (
	// ChatNoticeKindQuestionNoWatcher é a pergunta feita num turno sem tela
	// onde perguntar — canal, job agendado, subagente, CLI não interativa.
	ChatNoticeKindQuestionNoWatcher = "question_skipped_no_watcher"
	// ChatNoticeKindQuestionTimeout é a pergunta que ficou sem resposta dentro
	// do prazo.
	ChatNoticeKindQuestionTimeout = "question_skipped_timeout"
	// ChatNoticeKindQuestionUnavailable é a pergunta que o app não conseguiu
	// apresentar: sem o questionário no ar, ou sem opção nenhuma para oferecer.
	ChatNoticeKindQuestionUnavailable = "question_skipped_unavailable"
)

// Plano que o agente montou (cursor/create_plan) e que o app recusou sem que
// ninguém decidisse (AEP-0084 D9). Recusar é o desfecho seguro — executar um
// plano que ninguém leu seria o oposto —, mas o agente costuma seguir dizendo
// apenas que o plano foi rejeitado.
const (
	// ChatNoticeKindPlanNoWatcher é o plano apresentado num turno sem tela
	// onde aprová-lo.
	ChatNoticeKindPlanNoWatcher = "plan_rejected_no_watcher"
	// ChatNoticeKindPlanTimeout é o plano que ficou sem decisão dentro do
	// prazo.
	ChatNoticeKindPlanTimeout = "plan_rejected_timeout"
	// ChatNoticeKindPlanUnavailable é o plano que o app não conseguiu
	// apresentar.
	ChatNoticeKindPlanUnavailable = "plan_rejected_unavailable"
)

// O que aconteceu com um "permitir sempre" (AEP-0084 D9). A escolha muda o
// comportamento do app daí em diante, e uma mudança dessas não pode acontecer
// dentro de um diálogo que já sumiu da tela: a conversa é onde a pessoa fica
// sabendo que existe algo a revogar, e onde.
const (
	// ChatNoticeKindPermissionAlwaysAllowed é a autorização permanente que
	// passou a valer no perfil.
	ChatNoticeKindPermissionAlwaysAllowed = "permission_always_allowed"
	// ChatNoticeKindPermissionAlwaysNotSaved é o "sempre" que o app não
	// conseguiu guardar. A ação desta vez foi autorizada, mas a próxima volta a
	// perguntar — e quem escolheu "sempre" precisa saber disso antes de estranhar
	// a pergunta repetida.
	ChatNoticeKindPermissionAlwaysNotSaved = "permission_always_not_saved"
)

// O que o modo do agente passou a valer para o pedido de permissão (AEP-0084
// D9, Fase 7). Há modos que dispensam o `session/request_permission`, e ele é a
// única barreira que o app tem para autorizar o que o agente faz na máquina.
// É o mesmo caso do "permitir sempre": a escolha muda o comportamento daí em
// diante, e o seletor que a recebeu não fica na tela contando isso.
//
// O aviso é da transição, e não do estado: quem já estava sem barreira e trocou
// para outro modo que também não pergunta não é avisado de novo, pelo mesmo
// motivo que a autorização permanente não se repete a cada pedido.
const (
	// ChatNoticeKindModeSkipsPermission é a barreira que caiu: daqui em diante
	// o agente age sem perguntar.
	ChatNoticeKindModeSkipsPermission = "agent_mode_skips_permission"
	// ChatNoticeKindModeAsksPermission é a barreira que voltou. Fecha o aviso
	// anterior: quem leu que o agente ia agir sozinho precisa saber quando
	// isso deixou de valer, e nada mais na tela conta essa volta.
	ChatNoticeKindModeAsksPermission = "agent_mode_asks_permission"
)

// ChatNoticeEvent is the payload for chat:notice — um aviso sobre o turno que
// não é a resposta, não é falha e não encerra nada.
//
// O motivo vai como código, e não como frase: quem exibe traduz para o idioma
// de quem lê.
type ChatNoticeEvent struct {
	ConversationID string `json:"conversationId"`
	Kind           string `json:"kind"`
	Count          int    `json:"count,omitempty"`
	// Action é a classe da ação envolvida (`read`, `edit`, `execute`…), quando
	// o aviso é sobre uma. Vai como código pelo mesmo motivo do Kind, e nunca
	// leva o texto que o agente escreveu.
	Action string `json:"action,omitempty"`
	// Model é o modelo de que o aviso fala — o que atendeu ao turno, quando o
	// escolhido não pôde valer. É identificador do provedor, e não frase: quem
	// exibe o mostra como ele aparece no seletor.
	Model string `json:"model,omitempty"`
	// Mode é o modo de que o aviso fala, escrito como o seletor o escreve: o
	// rótulo que o agente deu, e o valor cru quando ele não deu nenhum. Vem
	// resolvido daqui, ao contrário do Model, porque quem exibe o aviso é uma
	// superfície global que não conhece as opções desta sessão.
	Mode string `json:"mode,omitempty"`
}

// SummaryStartedEvent is the payload for chat:summary_started.
type SummaryStartedEvent struct {
	ConversationID string `json:"conversationId"`
	MessageCount   int    `json:"messageCount"`
}

// SummaryErrorCodeAgentProvider identifica a recusa de resumir uma conversa
// cujo provider é um agente externo (AEP-0084 D14). Não é uma falha: é uma
// condição prevista, e a interface a traduz no idioma de quem lê.
const SummaryErrorCodeAgentProvider = "agent_provider"

// SummaryErrorEvent is the payload for chat:summary_error.
//
// Code nomeia os motivos que a interface sabe traduzir; quando vazio, resta
// Error, que é a mensagem crua (mensagem de erro do provedor, por exemplo).
type SummaryErrorEvent struct {
	ConversationID string `json:"conversationId"`
	Error          string `json:"error"`
	Code           string `json:"code,omitempty"`
}

// SummaryCompletedEvent is the payload for chat:summary_completed.
type SummaryCompletedEvent struct {
	ConversationID       string `json:"conversationId"`
	SummaryUpToMessageID string `json:"summaryUpToMessageId"`
	SummaryLength        int    `json:"summaryLength"`
	MessageCount         int    `json:"messageCount"`
}

// MessageDeletedEvent is the payload for message:deleted.
type MessageDeletedEvent struct {
	ConversationID string `json:"conversationId"`
	MessageID      string `json:"messageId"`
}

// MessageUpdatedEvent is the payload for message:updated.
type MessageUpdatedEvent struct {
	ConversationID string `json:"conversationId"`
	MessageID      string `json:"messageId"`
	Content        string `json:"content"`
}

// ConversationRenamedEvent is the payload for conversation:renamed.
type ConversationRenamedEvent struct {
	ConversationID string `json:"conversationId"`
	NewTitle       string `json:"newTitle"`
}
