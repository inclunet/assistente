package database

import "time"

// Status possíveis de um run de sub-agente (AEP-0068). Alinhado ao enum de
// tool_invocations (AEP-0063) para consistência de telemetria.
const (
	SubAgentRunStatusQueued    = "queued"
	SubAgentRunStatusRunning   = "running"
	SubAgentRunStatusSucceeded = "succeeded"
	SubAgentRunStatusFailed    = "failed"
	SubAgentRunStatusCancelled = "cancelled"
	SubAgentRunStatusTimedOut  = "timed_out"
)

// SubAgentRun registra um run (turno) de um sub-agente (AEP-0068).
//
// A "sessão" do sub-agente é a própria sub-conversa (ChildConversationID);
// cada chamada com prompt gera uma linha aqui (um run por turno). O vínculo
// pai↔filho do ponto de vista técnico de tools vem de
// tool_invocations.ParentInvocationID (AEP-0063); esta tabela guarda o estado
// de negócio do run e o handle durável (ChildConversationID).
type SubAgentRun struct {
	UUIDModel
	UserID string `json:"userId" gorm:"not null;index:idx_subagent_runs_user_parent,priority:1;index:idx_subagent_runs_user_child,priority:1;index:idx_subagent_runs_user_status,priority:1"`

	// ParentConversationID é a conversa que disparou o sub-agente. Pode ser
	// vazio quando a origem não é uma conversa (ex.: job/system — fases futuras).
	ParentConversationID string `json:"parentConversationId,omitempty" gorm:"index:idx_subagent_runs_user_parent,priority:2"`
	// ParentTurnID é o ID da mensagem (turno) do pai que originou o run.
	ParentTurnID string `json:"parentTurnId,omitempty" gorm:"index"`
	// ChildConversationID é a sub-conversa onde o sub-agente executa. É o handle durável.
	ChildConversationID string `json:"childConversationId" gorm:"not null;index:idx_subagent_runs_user_child,priority:2"`
	// TurnIndex é o índice incremental do run dentro da sub-conversa (0-based).
	TurnIndex int `json:"turnIndex"`

	Status     string `json:"status" gorm:"not null;index:idx_subagent_runs_user_status,priority:2"`
	Background bool   `json:"background,omitempty" gorm:"not null;default:false"`

	// ResultSummary guarda o conteúdo final da resposta do sub-agente (truncado).
	ResultSummary string `json:"resultSummary,omitempty" gorm:"type:text"`
	// AssistantMessageID referencia a mensagem do assistente que concluiu o run.
	AssistantMessageID string `json:"assistantMessageId,omitempty"`
	// Error guarda a mensagem de erro legível. Preenchido quando o run termina
	// sem sucesso: Status=failed, timed_out ou cancelled (o Manager também
	// registra o motivo em cancelamentos, ex.: ctx.Done()/send cancelado).
	Error string `json:"error,omitempty" gorm:"type:text"`

	// Proveniência / anti-runaway compartilhada com jobs (AEP-0067/0001).
	ChainID      string `json:"chainId,omitempty" gorm:"index"`
	ChainHistory string `json:"chainHistory,omitempty" gorm:"type:text"` // JSON: []string

	StartedAt   *time.Time `json:"startedAt,omitempty"`
	CompletedAt *time.Time `json:"completedAt,omitempty"`

	// DeliveredAt marca quando o aviso de conclusão (background) já foi entregue
	// ao pai. Garante idempotência por run_id: sem aviso duplicado em
	// retry/recovery (AEP-0068 Fase 2).
	DeliveredAt *time.Time `json:"deliveredAt,omitempty"`

	User *User `json:"-" gorm:"foreignKey:UserID"`
}
