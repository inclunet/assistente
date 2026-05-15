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
	OriginJob         = "job"
	OriginJobRun      = "job_run"
	OriginToolCatalog = "tool_catalog"
)

type Invocation struct {
	ID                 string          `json:"id"`
	UserID             string          `json:"user_id,omitempty"`
	ToolCatalogID      string          `json:"tool_catalog_id"`
	OriginType         string          `json:"origin_type"`
	OriginID           string          `json:"origin_id,omitempty"`
	ParentInvocationID string          `json:"parent_invocation_id,omitempty"`
	ToolCallID         string          `json:"tool_call_id,omitempty"`
	Status             string          `json:"status"`
	DryRun             bool            `json:"dry_run,omitempty"`
	Input              json.RawMessage `json:"input,omitempty"`
	Output             json.RawMessage `json:"output,omitempty"`
	Metadata           json.RawMessage `json:"metadata,omitempty"`
	ErrorKind          string          `json:"error_kind,omitempty"`
	ErrorMessage       string          `json:"error_message,omitempty"`
	Retryable          bool            `json:"retryable,omitempty"`
	QueuedAt           time.Time       `json:"queued_at"`
	StartedAt          *time.Time      `json:"started_at,omitempty"`
	CompletedAt        *time.Time      `json:"completed_at,omitempty"`
	DurationMs         int64           `json:"duration_ms,omitempty"`
	CreatedAt          time.Time       `json:"created_at,omitempty"`
	UpdatedAt          time.Time       `json:"updated_at,omitempty"`
}

type Origin struct {
	Type string
	ID   string
}

type ExecuteRequest struct {
	Call               tools.ToolCall
	Origin             Origin
	ParentInvocationID string
	ToolCatalogID      string
	DryRun             bool

	// ExecutionMaxResultSize permite que um chamador (ex.: jobs) execute com
	// um limite maior de resultado para processamento interno, sem precisar
	// desativar truncamento global do executor.
	//
	// Observação: a persistência em tool_invocations pode aplicar um limite
	// separado para evitar crescimento excessivo da tabela.
	ExecutionMaxResultSize int
}

type ExecuteResult struct {
	Invocation Invocation
	Execution  tools.ToolExecutionResult
}

// RecordRequest registra uma invocação já executada fora do executor comum
// (ex.: MCP nativo), persistindo input/output/status no mesmo formato.
type RecordRequest struct {
	Call          tools.ToolCall
	Origin        Origin
	ToolCatalogID string
	DryRun        bool

	Result       tools.ToolResult
	ErrorKind    tools.ErrorKind
	ErrorMessage string
	Retryable    bool
	DurationMs   int64
}

type Filter struct {
	OriginType string
	OriginID   string
	Status     string
	DryRun     *bool
	Limit      int
}
