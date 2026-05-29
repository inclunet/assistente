package jobs

import (
	"encoding/json"
	"time"
)

// JobStatus representa o estado runtime de um job.
type JobStatus string

const (
	JobStatusIdle    JobStatus = "idle"
	JobStatusRunning JobStatus = "running"
	JobStatusError   JobStatus = "error"
)

// TriggerType identifica o tipo de trigger que dispara um job.
type TriggerType string

const (
	TriggerCron     TriggerType = "cron"
	TriggerInterval TriggerType = "interval"
	TriggerEvent    TriggerType = "event"
	TriggerHotkey   TriggerType = "hotkey"
	TriggerManual   TriggerType = "manual"
	TriggerWebhook  TriggerType = "webhook" // v2
)

// ErrorStrategy define como lidar com falhas.
type ErrorStrategy string

const (
	ErrorRetry ErrorStrategy = "retry"
	ErrorStop  ErrorStrategy = "stop"
	ErrorSkip  ErrorStrategy = "skip"
)

// BackoffType define o tipo de backoff para retries.
type BackoffType string

const (
	BackoffLinear      BackoffType = "linear"
	BackoffExponential BackoffType = "exponential"
	BackoffFixed       BackoffType = "fixed"
)

// OnExhausted define acao quando retries se esgotam.
type OnExhausted string

const (
	OnExhaustedNotify OnExhausted = "notify"
	OnExhaustedIgnore OnExhausted = "ignore"
)

// Job representa uma unidade atomica de automacao: 1 job = 1 tool call.
type Job struct {
	ID             string         `yaml:"id" json:"id"`
	Name           string         `yaml:"name" json:"name"`
	Description    string         `yaml:"description" json:"description"`
	Enabled        bool           `yaml:"enabled" json:"enabled"`
	Pipeline       string         `yaml:"pipeline,omitempty" json:"pipeline,omitempty"`
	Tags           []string       `yaml:"tags,omitempty" json:"tags,omitempty"`
	Triggers       []Trigger      `yaml:"triggers" json:"triggers"`
	Tool           string         `yaml:"tool" json:"tool"`
	Inputs         map[string]any `yaml:"inputs,omitempty" json:"inputs,omitempty"`
	Output         OutputConfig   `yaml:"output,omitempty" json:"output,omitempty"`
	Events         EventsConfig   `yaml:"events,omitempty" json:"events,omitempty"`
	ErrorPolicy    ErrorPolicy    `yaml:"error_policy,omitempty" json:"error_policy,omitempty"`
	MaxRunsPerHour int            `yaml:"max_runs_per_hour,omitempty" json:"max_runs_per_hour,omitempty"`
	DryRun         DryRunConfig   `yaml:"dry_run,omitempty" json:"dry_run,omitempty"`
	Metadata       Metadata       `yaml:"metadata,omitempty" json:"metadata,omitempty"`

	// Campos carregados do runtime/DB e omitidos de imports YAML legados.
	LastRun         *RunLog   `yaml:"-" json:"last_run,omitempty"`
	Status          JobStatus `yaml:"-" json:"status"`
	PipelineEnabled bool      `yaml:"-" json:"pipeline_enabled"`
}

// Pipeline representa um agrupamento persistente de jobs.
type Pipeline struct {
	ID          string         `json:"id"`
	Slug        string         `json:"slug"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Enabled     bool           `json:"enabled"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	CreatedAt   time.Time      `json:"created_at,omitempty"`
	UpdatedAt   time.Time      `json:"updated_at,omitempty"`
}

// Tag representa uma tag global atribuível a jobs e outros recursos.
type Tag struct {
	ID          string    `json:"id"`
	Slug        string    `json:"slug"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Color       string    `json:"color,omitempty"`
	CreatedAt   time.Time `json:"created_at,omitempty"`
	UpdatedAt   time.Time `json:"updated_at,omitempty"`
}

// Trigger define quando um job deve ser executado.
type Trigger struct {
	Type       TriggerType `yaml:"type" json:"type"`
	Expression string      `yaml:"expression,omitempty" json:"expression,omitempty"` // cron expression
	Every      string      `yaml:"every,omitempty" json:"every,omitempty"`           // duration (2h, 30m)
	Listen     string      `yaml:"listen,omitempty" json:"listen,omitempty"`         // event name
	Keys       string      `yaml:"keys,omitempty" json:"keys,omitempty"`             // hotkey combo
	Path       string      `yaml:"path,omitempty" json:"path,omitempty"`             // webhook path (v2)
	When       string      `yaml:"when,omitempty" json:"when,omitempty"`             // Go template condition; truthy = run
}

// OutputConfig define schema e mapeamento do output da tool call.
type OutputConfig struct {
	Schema json.RawMessage   `yaml:"schema,omitempty" json:"schema,omitempty"`
	Map    map[string]string `yaml:"map,omitempty" json:"map,omitempty"`
}

// EventsConfig define os eventos emitidos pelo job.
type EventsConfig struct {
	OnSuccess       string         `yaml:"on_success,omitempty" json:"on_success,omitempty"`
	OnFailure       string         `yaml:"on_failure,omitempty" json:"on_failure,omitempty"`
	EmitWhen        string         `yaml:"emit_when,omitempty" json:"emit_when,omitempty"`
	ForEach         string         `yaml:"for_each,omitempty" json:"for_each,omitempty"`
	PayloadTemplate string         `yaml:"payload_template,omitempty" json:"payload_template,omitempty"`
	PayloadFilter   *PayloadFilter `yaml:"payload_filter,omitempty" json:"payload_filter,omitempty"`
}

// PayloadFilter filtra campos do payload do evento emitido.
type PayloadFilter struct {
	Include []string `yaml:"include,omitempty" json:"include,omitempty"`
	Exclude []string `yaml:"exclude,omitempty" json:"exclude,omitempty"`
}

// ErrorPolicy configura tratamento de erros.
type ErrorPolicy struct {
	Strategy       ErrorStrategy `yaml:"strategy,omitempty" json:"strategy,omitempty"`
	MaxRetries     int           `yaml:"max_retries,omitempty" json:"max_retries,omitempty"`
	RetryDelay     string        `yaml:"retry_delay,omitempty" json:"retry_delay,omitempty"`
	Backoff        BackoffType   `yaml:"backoff,omitempty" json:"backoff,omitempty"`
	OnExhausted    OnExhausted   `yaml:"on_exhausted,omitempty" json:"on_exhausted,omitempty"`
	NotifyChannels []string      `yaml:"notify_channels,omitempty" json:"notify_channels,omitempty"`
}

// DryRunConfig configura dry run com mock output.
type DryRunConfig struct {
	Enabled    bool           `yaml:"enabled,omitempty" json:"enabled,omitempty"`
	MockOutput map[string]any `yaml:"mock_output,omitempty" json:"mock_output,omitempty"`
}

// Metadata armazena informacoes de auditoria.
type Metadata struct {
	CreatedAt string `yaml:"created_at,omitempty" json:"created_at,omitempty"`
	CreatedBy string `yaml:"created_by,omitempty" json:"created_by,omitempty"`
	UpdatedAt string `yaml:"updated_at,omitempty" json:"updated_at,omitempty"`
}

// --- Tipos runtime (logs, eventos, info para UI) ---

// TriggerInfo descreve o trigger que disparou uma execucao.
type TriggerInfo struct {
	Type       TriggerType    `json:"type"`
	At         time.Time      `json:"at"`
	Event      string         `json:"event,omitempty"`
	Expression string         `json:"expression,omitempty"`
	Every      string         `json:"every,omitempty"`
	Keys       string         `json:"keys,omitempty"`
	When       string         `json:"when,omitempty"`
	Data       map[string]any `json:"data,omitempty"`
}

// RunLog registra uma execucao individual de um job.
type RunLog struct {
	RunID          string         `json:"run_id"`
	JobID          string         `json:"job_id"`
	ToolName       string         `json:"tool_name,omitempty"`
	Trigger        TriggerInfo    `json:"trigger"`
	Status         string         `json:"status"` // completed, failed, retrying, skipped
	StartedAt      time.Time      `json:"started_at"`
	CompletedAt    time.Time      `json:"completed_at,omitempty"`
	Duration       string         `json:"duration,omitempty"`
	ResolvedInputs map[string]any `json:"resolved_inputs,omitempty"`
	Output         map[string]any `json:"output,omitempty"`
	OutputSize     int            `json:"output_size,omitempty"`
	Error          string         `json:"error,omitempty"`
	RetryCount     int            `json:"retry_count,omitempty"`
	EventsEmitted  []string       `json:"events_emitted,omitempty"`
	IsDryRun       bool           `json:"is_dry_run,omitempty"`
	Replayable     bool           `json:"replayable"`
	RunEvents      []RunEvent     `json:"-"`
	DomainEvents   []EventEntry   `json:"-"`
}

// EventEntry representa um evento de domínio persistido (DB-backed)
// relacionado a execuções de jobs, com correlação opcional por run.
type EventEntry struct {
	ID        string         `json:"id,omitempty"`
	Timestamp time.Time      `json:"timestamp"`
	Type      string         `json:"type"` // triggered, completed, failed, event_emitted, event_received
	JobID     string         `json:"job_id"`
	RunID     string         `json:"run_id,omitempty"`
	Event     string         `json:"event,omitempty"`
	Message   string         `json:"message,omitempty"`
	Data      map[string]any `json:"data,omitempty"`
}

// RunEvent representa uma entrada ordenada na timeline de uma execução.
type RunEvent struct {
	ID        string         `json:"id,omitempty"`
	RunID     string         `json:"run_id"`
	Sequence  int            `json:"sequence"`
	Timestamp time.Time      `json:"timestamp"`
	Type      string         `json:"type"`
	Message   string         `json:"message,omitempty"`
	Data      map[string]any `json:"data,omitempty"`
}

// JobFilter define filtros de listagem do repository.
type JobFilter struct {
	Pipeline string
	Tag      string
	Enabled  *bool
}

// EventFilter define filtros de listagem para eventos de domínio de jobs.
type EventFilter struct {
	JobID   string
	RunID   string
	Type    string
	Event   string
	StartAt time.Time
	EndAt   time.Time
	Limit   int
	Offset  int
}

// RunFilter define filtros de listagem de runs.
type RunFilter struct {
	Status        []string
	StartedAfter  time.Time
	StartedBefore time.Time
	IncludeDryRun bool
	Limit         int
}

// RunDetail é o RunLog enriquecido com timeline operacional e eventos de
// domínio correlacionados. Usado por consultas dedicadas de detalhamento de
// um run específico; listagens continuam usando RunLog para permanecerem leves.
type RunDetail struct {
	RunLog
	RunEvents    []RunEvent   `json:"run_events"`
	DomainEvents []EventEntry `json:"domain_events"`
}

// --- Tipos para API/UI ---

// JobInfo e uma visao resumida de um job para listagem.
type JobInfo struct {
	ID               string    `json:"id"`
	Name             string    `json:"name"`
	Description      string    `json:"description"`
	Enabled          bool      `json:"enabled"`
	EffectiveEnabled bool      `json:"effective_enabled"`
	PipelineEnabled  bool      `json:"pipeline_enabled"`
	Pipeline         string    `json:"pipeline,omitempty"`
	Tags             []string  `json:"tags,omitempty"`
	Tool             string    `json:"tool"`
	Status           JobStatus `json:"status"`
	Triggers         []Trigger `json:"triggers"`
	LastRun          *RunLog   `json:"last_run,omitempty"`
}

// PipelineInfo agrupa jobs que compartilham o mesmo pipeline.
type PipelineInfo struct {
	Name string    `json:"name"`
	Jobs []JobInfo `json:"jobs"`
}

// CatalogEntry descreve uma tool disponivel para uso em jobs.
type CatalogEntry struct {
	ID                 string          `json:"id,omitempty" yaml:"id,omitempty"`
	MCPServerID        string          `json:"mcp_server_id,omitempty" yaml:"mcp_server_id,omitempty"`
	Name               string          `json:"name" yaml:"name"`
	Description        string          `json:"description" yaml:"description"`
	Schema             json.RawMessage `json:"schema" yaml:"schema"`
	Source             string          `json:"source" yaml:"source"` // "internal", "mcp"
	Origin             string          `json:"origin,omitempty" yaml:"origin,omitempty"`
	Risk               string          `json:"risk,omitempty" yaml:"risk,omitempty"`
	AvailabilityStatus string          `json:"availability_status,omitempty" yaml:"availability_status,omitempty"`
	AvailabilityReason string          `json:"availability_reason,omitempty" yaml:"availability_reason,omitempty"`
}

// DryRunResult contem o resultado de um dry run.
type DryRunResult struct {
	Success bool           `json:"success"`
	Output  map[string]any `json:"output,omitempty"`
	Error   string         `json:"error,omitempty"`
	RunLog  *RunLog        `json:"run_log,omitempty"`
}

// TestToolResult contem o resultado de um teste direto de tool (sem job salvo).
type TestToolResult struct {
	Success       bool           `json:"success"`
	Output        map[string]any `json:"output,omitempty"`
	Error         string         `json:"error,omitempty"`
	Duration      string         `json:"duration,omitempty"`
	Blocked       bool           `json:"blocked,omitempty"`
	Origin        string         `json:"origin,omitempty"`
	MCPServerID   string         `json:"mcp_server_id,omitempty"`
	ToolName      string         `json:"tool_name,omitempty"`
	ToolCatalogID string         `json:"tool_catalog_id,omitempty"`
}

// TestToolRequest descreve o alvo explícito de dry-run/teste de tool.
type TestToolRequest struct {
	ToolName      string         `json:"tool_name"`
	MCPServerID   string         `json:"mcp_server_id,omitempty"`
	Origin        string         `json:"origin,omitempty"`
	ToolCatalogID string         `json:"tool_catalog_id,omitempty"`
	Risk          string         `json:"risk,omitempty"`
	Inputs        map[string]any `json:"inputs,omitempty"`
	EventData     map[string]any `json:"event_data,omitempty"`
	AllowUnsafe   bool           `json:"allow_unsafe,omitempty"`
}
