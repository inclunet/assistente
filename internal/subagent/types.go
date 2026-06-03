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

// DefaultSyncTimeout limita a espera de um run síncrono (background:false).
// Curto o bastante para caber dentro do timeout do executor de tools
// (DefaultToolTimeout = 6m) e devolver erro legível ao pai em vez de estourar
// o executor.
const DefaultSyncTimeout = 5 * time.Minute

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

	// Timeout limita a espera de um run síncrono. <=0 usa DefaultSyncTimeout.
	Timeout time.Duration
}

// RunResult é o retorno de um run de sub-agente.
type RunResult struct {
	ConversationID     string `json:"conversation_id"`
	RunID              string `json:"run_id"`
	Status             string `json:"status"`
	ResultSummary      string `json:"result_summary,omitempty"`
	AssistantMessageID string `json:"assistant_message_id,omitempty"`
	Error              string `json:"error,omitempty"`
}

// Repository persiste runs de sub-agente (tabela sub_agent_runs, AEP-0068).
type Repository interface {
	Create(ctx context.Context, run *database.SubAgentRun) error
	Get(ctx context.Context, id string) (*database.SubAgentRun, error)
	GetLatestByChildConversation(ctx context.Context, childConversationID string) (*database.SubAgentRun, error)
	Update(ctx context.Context, run *database.SubAgentRun) error
}
