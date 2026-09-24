package toolinvocations

import (
	"encoding/json"
	"time"

	"assistente/internal/tools"
)

const (
	StatusQueued    = "queued"
	StatusRunning   = "running"
	StatusSucceeded = "succeeded"
	StatusFailed    = "failed"
	StatusCancelled = "cancelled"
	StatusTimedOut  = "timed_out"

	OriginChat        = "chat"
	OriginJobRun      = "job_run"
	OriginToolCatalog = "tool_catalog"

	ToolOriginArchival = "archival"
)

type Invocation struct {
	ID                 string          `json:"id"`
	UserID             string          `json:"user_id,omitempty"`
	ToolCatalogID      string          `json:"tool_catalog_id"`
	OriginType         string          `json:"origin_type"`
	OriginID           string          `json:"origin_id,omitempty"`
	ConversationID     string          `json:"conversation_id,omitempty"`
	TurnID             string          `json:"turn_id,omitempty"`
	ParentInvocationID string          `json:"parent_invocation_id,omitempty"`
	ToolCallID         string          `json:"tool_call_id,omitempty"`
	Attempt            int             `json:"attempt"`
	Status             string          `json:"status"`
	DryRun             bool            `json:"dry_run,omitempty"`
	Input              json.RawMessage `json:"input,omitempty"`
	Output             json.RawMessage `json:"output,omitempty"`
	Metadata           json.RawMessage `json:"metadata,omitempty"`
	ModelIteration     int             `json:"model_iteration,omitempty"`
	External           bool            `json:"external,omitempty"`
	DisplayName        string          `json:"display_name,omitempty"`
	InputPreview       string          `json:"input_preview,omitempty"`
	OutputPreview      string          `json:"output_preview,omitempty"`
	InputBytes         int64           `json:"input_bytes,omitempty"`
	OutputBytes        int64           `json:"output_bytes,omitempty"`
	InputHash          string          `json:"input_hash,omitempty"`
	OutputHash         string          `json:"output_hash,omitempty"`
	ResultAvailability string          `json:"result_availability,omitempty"`
	ErrorKind          string          `json:"error_kind,omitempty"`
	ErrorCode          string          `json:"error_code,omitempty"`
	ErrorMessage       string          `json:"error_message,omitempty"`
	Retryable          bool            `json:"retryable,omitempty"`
	RetryabilityKnown  bool            `json:"retryability_known,omitempty"`
	QueuedAt           time.Time       `json:"queued_at"`
	StartedAt          *time.Time      `json:"started_at,omitempty"`
	CompletedAt        *time.Time      `json:"completed_at,omitempty"`
	DurationMs         int64           `json:"duration_ms,omitempty"`
	CreatedAt          time.Time       `json:"created_at,omitempty"`
	UpdatedAt          time.Time       `json:"updated_at,omitempty"`
}

type Origin struct {
	Type           string
	ID             string
	ConversationID string
	TurnID         string
}

type ExecuteRequest struct {
	Call tools.ToolCall
	// PersistedArguments substitui apenas os argumentos gravados no ledger e
	// no snapshot de exibição. A execução continua usando Call sem alteração.
	// Chamadores que resolvem templates secretos devem fornecer a versão
	// explicitamente redigida.
	PersistedArguments *string
	Origin             Origin
	ParentInvocationID string
	ToolCatalogID      string
	DryRun             bool
	Iteration          int

	// ExecutionMaxResultSize permite que um chamador (ex.: jobs) execute com
	// um limite maior de resultado para processamento interno, sem precisar
	// desativar truncamento global do executor.
	//
	// Observação: a persistência em tool_invocations pode aplicar um limite
	// separado para evitar crescimento excessivo da tabela.
	ExecutionMaxResultSize int

	// RequireCompleteResult impede prévias retomáveis para consumidores
	// machine-facing que processam Result.Content diretamente (ex.: jobs).
	RequireCompleteResult bool
}

type ExecuteResult struct {
	Invocation Invocation
	Execution  tools.ToolExecutionResult

	// Persisted indica se esta execução foi registrada com sucesso em tool_invocations.
	// Falha nunca autoriza cópia alternativa em chat_messages.
	Persisted bool
}

// RecordRequest registra uma invocação já executada fora do executor comum
// (ex.: MCP nativo), persistindo input/output/status no mesmo formato.
type RecordRequest struct {
	// ACPActivity distingue observação externa de MCP nativo, sem executar tools.
	ACPActivity bool
	ACPTitle    string // resumo saneado, não resultado técnico
	ObservedAt  time.Time
	Call        tools.ToolCall
	// PersistedArguments tem a mesma semântica de ExecuteRequest: substitui
	// somente o snapshot persistido, nunca o payload já executado.
	PersistedArguments *string
	Origin             Origin
	ToolCatalogID      string
	DryRun             bool
	Iteration          int

	Result            tools.ToolResult
	ErrorKind         tools.ErrorKind
	ErrorCode         string
	ErrorMessage      string
	Retryable         bool
	RetryabilityKnown bool
	DurationMs        int64
}

type Filter struct {
	OriginType string
	OriginID   string
	Status     string
	DryRun     *bool
	Limit      int
}
