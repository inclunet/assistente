// Package subagent implementa o núcleo de execução de sub-agentes (AEP-0068):
// sub-conversas próprias, persistidas e visíveis, disparadas pelo pipeline
// oficial de envio de mensagens (SendMessageUseCase) e com detecção de
// conclusão por callback in-process (análogo ao ResponseNotifier dos canais).
//
// Esta camada NÃO cria fluxo alternativo de envio nem mensagens locais: ela
// cria a sub-conversa e delega o envio ao pipeline compartilhado via SendFunc.
package subagent

import (
	"context"
	"time"

	"assistente/internal/database"
)

// Source identifica a origem dos envios de sub-agente no pipeline oficial.
const Source = "subagent"

// Status de run reexportados de database para consumidores (tool, UI) que não
// devem depender diretamente do pacote database.
const (
	StatusQueued    = database.SubAgentRunStatusQueued
	StatusRunning   = database.SubAgentRunStatusRunning
	StatusSucceeded = database.SubAgentRunStatusSucceeded
	StatusFailed    = database.SubAgentRunStatusFailed
	StatusCancelled = database.SubAgentRunStatusCancelled
	StatusTimedOut  = database.SubAgentRunStatusTimedOut
)

// DefaultSyncTimeout limita a espera de um run SÍNCRONO (background:false) quando
// o caller não informa Timeout. Curto o bastante para caber dentro do timeout do
// executor de tools (DefaultToolTimeout = 6m) e devolver erro legível ao pai em
// vez de estourar o executor. NÃO se aplica a background — ver
// DefaultBackgroundTimeout.
const DefaultSyncTimeout = 5 * time.Minute

// DefaultBackgroundTimeout é o backstop anti-runaway de um run em BACKGROUND
// (background:true) quando o caller não informa Timeout. NÃO é um limite de UX
// como o síncrono: o run de background não deve expirar silenciosamente nos
// mesmos 5min do modo síncrono (contrariaria o objetivo de "segundo plano" —
// AEP-0068). Mesmo assim, a AEP-0068 (Riscos) prescreve "timeout por run" como
// backstop contra runs presos/runaway; usamos um valor bem maior, suficiente
// para trabalho de fundo legítimo, mas que ainda impede uma goroutine rodando
// para sempre na sessão. Runs órfãos por app fechado são tratados à parte
// (reconciliação no startup, Fase 4). Caller pode sobrescrever via Timeout.
const DefaultBackgroundTimeout = 1 * time.Hour

// callbackTTLMargin é a folga somada ao timeout efetivo do run ao registrar o
// callback de conclusão no ResponseNotifier. O notifier descarta callbacks
// pendentes após um TTL; alinhamos esse TTL ao timeout do run (ver Run) para que
// um run de background longo não perca a conclusão por expiração precoce do
// callback. A margem garante que o TIMEOUT DO PRÓPRIO RUN dispare primeiro (o
// caminho normal já remove o callback via finalize), deixando o TTL do notifier
// como mero backstop anti-órfão — nunca o mecanismo que encerra um run legítimo.
const callbackTTLMargin = 2 * time.Minute

// SendParams descreve um envio ao pipeline oficial de mensagens. O adapter de
// wiring traduz para usecases.SendMessageRequest (sem duplicar o pipeline).
type SendParams struct {
	ConversationID string
	Prompt         string
	Media          string
	ProfileSlug    string
	Model          string
	Source         string
}

// SendFunc dispara um envio pelo pipeline oficial e retorna o conversationID.
// É um ponto de extensão para desacoplar o pacote subagent de internal/core/usecases
// (evita ciclo de imports). O erro retornado é o erro síncrono do Execute.
type SendFunc func(ctx context.Context, p SendParams) (string, error)

// RunParams são os parâmetros de um run de sub-agente.
type RunParams struct {
	// ParentConversationID é a conversa que originou o sub-agente (pode ser
	// vazio quando não há pai — job/system, fases futuras).
	ParentConversationID string
	// ParentTurnID é o turno do pai que invocou o sub-agente.
	ParentTurnID string
	// ParentInvocationID é a invocação da tool `subagent` na conversa-pai; é
	// herdada pelas sub-invocações da sub-conversa (encadeamento AEP-0063/0068).
	ParentInvocationID string

	// ConversationID, quando informado, reusa uma sub-conversa existente
	// (resume), preservando o histórico/contexto. Vazio → cria uma nova
	// sub-conversa. A sub-conversa precisa ser do usuário e Kind=subagent.
	ConversationID string
	// Clear, quando true (com ConversationID e Prompt), reseta o histórico e o
	// resumo da sub-conversa antes de enviar o novo prompt (AEP-0068 F3).
	Clear bool

	// Prompt é a tarefa/mensagem enviada ao sub-agente.
	Prompt string
	// Media é mídia opcional (JSON base64) anexada ao envio.
	Media string
	// ProfileSlug é o profile resolvido do sub-agente (já considerando herança
	// do pai pela camada chamadora). Vazio → o pipeline resolve o profile ativo.
	ProfileSlug string
	// Model sobrescreve o modelo derivado do profile para este run (opcional).
	Model string
	// Title é o título da sub-conversa (opcional; derivado do prompt se vazio).
	Title string

	// Background, quando true, retorna o handle imediatamente e executa o
	// sub-agente em goroutine; o aviso de conclusão é entregue ao pai
	// (auto-wake). Quando false, o run é síncrono (Fase 1).
	Background bool

	// Timeout limita a espera do run. <=0 usa o default POR MODO:
	// DefaultSyncTimeout (síncrono) ou DefaultBackgroundTimeout (background).
	// Um valor explícito (>0) é respeitado em ambos os modos.
	Timeout time.Duration
}

// ParentNotice descreve o aviso de conclusão de um run em background a ser
// entregue na conversa-pai (pelo lado do assistente) + auto-wake (AEP-0068).
type ParentNotice struct {
	ParentConversationID string
	ParentTurnID         string
	RunID                string
	ChildConversationID  string
	Status               string
	Summary              string
	AssistantMessageID   string
	Error                string
}

// ParentDelivery entrega o aviso de conclusão na conversa-pai e re-dispara o
// loop do pai (auto-wake), propagando proveniência (eventctx). É implementada
// pelo wiring do app reusando o pipeline oficial — o pacote subagent não
// conhece os detalhes para evitar ciclo de imports.
type ParentDelivery interface {
	Deliver(ctx context.Context, n ParentNotice) error
}

// StatusResult é o retorno de uma consulta de status (prompt omitido).
type StatusResult struct {
	ConversationID     string `json:"conversation_id"`
	RunID              string `json:"run_id"`
	Status             string `json:"status"`
	ResultSummary      string `json:"result_summary,omitempty"`
	AssistantMessageID string `json:"assistant_message_id,omitempty"`
	Error              string `json:"error,omitempty"`
}

// RunEvent é o payload dos eventos EventRunStarted/EventRunFinished emitidos ao
// frontend (AEP-0068 F5). Carrega sempre o conversationId da sub-conversa, como
// exige o contrato de eventos de chat do projeto.
//
// Este struct NÃO chega aos bindings gerados (payload de evento não aparece em
// assinatura de método exportado do App): o frontend o espelha à mão numa
// interface TypeScript junto de quem escuta o evento.
type RunEvent struct {
	RunID                string `json:"runId"`
	ConversationID       string `json:"conversationId"`
	ParentConversationID string `json:"parentConversationId,omitempty"`
	Title                string `json:"title,omitempty"`
	Status               string `json:"status"`
	Background           bool   `json:"background"`
	Error                string `json:"error,omitempty"`
}

// RunListItem descreve um run de sub-agente para a superfície de visibilidade
// da UI (AEP-0068 F5): lista de runs ativos/recentes com ação de cancelar.
type RunListItem struct {
	RunID                string     `json:"runId"`
	ConversationID       string     `json:"conversationId"`
	ParentConversationID string     `json:"parentConversationId,omitempty"`
	Title                string     `json:"title,omitempty"`
	Status               string     `json:"status"`
	Background           bool       `json:"background"`
	// Active informa se o run ainda pode ser cancelado (queued/running).
	Active      bool       `json:"active"`
	Error       string     `json:"error,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
	StartedAt   *time.Time `json:"startedAt,omitempty"`
	CompletedAt *time.Time `json:"completedAt,omitempty"`
}

// RunListResult agrega a lista de runs e a ocupação dos tetos de concorrência,
// para a UI mostrar quanto do limite já está em uso (AEP-0068 F5: visibilidade
// de custo). Os contadores vêm do estado em memória do Manager, que é a fonte
// de verdade dos tetos — a lista vem do banco.
type RunListResult struct {
	Runs                 []RunListItem `json:"runs"`
	ActiveForUser        int           `json:"activeForUser"`
	ActiveGlobal         int           `json:"activeGlobal"`
	MaxConcurrentPerUser int           `json:"maxConcurrentPerUser"`
	MaxConcurrentGlobal  int           `json:"maxConcurrentGlobal"`
}

// CancelResult é o retorno de um cancel. Cancelled distingue cancelamento real
// (havia run ativo) de no-op (run já terminal/inexistente), conforme AEP-0068.
type CancelResult struct {
	ConversationID string `json:"conversation_id"`
	RunID          string `json:"run_id"`
	Status         string `json:"status"`
	Cancelled      bool   `json:"cancelled"`
	Message        string `json:"message,omitempty"`
}

// isTerminal informa se um status de run é terminal (não há mais transições).
func isTerminal(status string) bool {
	switch status {
	case StatusSucceeded, StatusFailed, StatusCancelled, StatusTimedOut:
		return true
	default:
		return false
	}
}

// IsActiveStatus informa se um run ainda está em andamento (cancelável).
func IsActiveStatus(status string) bool {
	return status == StatusQueued || status == StatusRunning
}

// RunResult é o retorno de um run de sub-agente.
type RunResult struct {
	ConversationID     string `json:"conversation_id"`
	RunID              string `json:"run_id"`
	Status             string `json:"status"`
	ResultSummary      string `json:"result_summary,omitempty"`
	AssistantMessageID string `json:"assistant_message_id,omitempty"`
	Error              string `json:"error,omitempty"`
	// Response preserva em memória a resposta integral recebida do callback.
	// Não é serializada no envelope nem persistida; a tool `subagent` a usa
	// somente no retorno raw de runs síncronos. ResultSummary continua limitado
	// para persistência e para o contrato compatível do envelope/status.
	Response string `json:"-"`
}

// Repository persiste runs de sub-agente (tabela sub_agent_runs, AEP-0068).
type Repository interface {
	Create(ctx context.Context, run *database.SubAgentRun) error
	Get(ctx context.Context, id string) (*database.SubAgentRun, error)
	GetLatestByChildConversation(ctx context.Context, childConversationID string) (*database.SubAgentRun, error)
	Update(ctx context.Context, run *database.SubAgentRun) error
	// ListRecent devolve os runs do usuário para a UI: primeiro os ativos
	// (queued/running), depois os mais recentes, limitados a limit itens.
	ListRecent(ctx context.Context, limit int) ([]RunListItem, error)
	// ReconcileOrphans marca como failed runs em queued/running (órfãos após
	// restart). Operação instance-wide de startup (não é pedido de usuário).
	// cutoff limita aos runs criados antes do início do app; now carimba o
	// desfecho.
	ReconcileOrphans(ctx context.Context, cutoff, now time.Time) (int64, error)
}
