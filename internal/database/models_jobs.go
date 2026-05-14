package database

import "time"

// ==================== Tags ====================

// Tag é uma marca normalizada e reutilizável por recursos do app.
type Tag struct {
	UUIDModel
	UserID      string `json:"userId" gorm:"not null;index;uniqueIndex:ux_tags_user_slug"`
	Slug        string `json:"slug" gorm:"not null;index;uniqueIndex:ux_tags_user_slug"`
	Name        string `json:"name" gorm:"not null"`
	Description string `json:"description,omitempty" gorm:"type:text"`
	Color       string `json:"color,omitempty"`

	User *User `json:"-" gorm:"foreignKey:UserID"`
}

// TagAssignment vincula uma tag a qualquer recurso persistente do app.
type TagAssignment struct {
	UUIDModel
	UserID       string `json:"userId" gorm:"not null;index;index:idx_tag_assignments_resource_lookup;uniqueIndex:ux_tag_assignments_resource_tag"`
	TagID        string `json:"tagId" gorm:"not null;index;uniqueIndex:ux_tag_assignments_resource_tag"`
	ResourceType string `json:"resourceType" gorm:"not null;index;index:idx_tag_assignments_resource_lookup;uniqueIndex:ux_tag_assignments_resource_tag"`
	ResourceID   string `json:"resourceId" gorm:"not null;index;index:idx_tag_assignments_resource_lookup;uniqueIndex:ux_tag_assignments_resource_tag"`

	User *User `json:"-" gorm:"foreignKey:UserID"`
	Tag  *Tag  `json:"-" gorm:"foreignKey:TagID"`
}

// ==================== Jobs ====================

// JobPipeline agrupa jobs relacionados para filtro, contagem e UI.
type JobPipeline struct {
	UUIDModel
	UserID      string `json:"userId" gorm:"not null;index;uniqueIndex:ux_job_pipelines_user_slug"`
	Slug        string `json:"slug" gorm:"not null;index;uniqueIndex:ux_job_pipelines_user_slug"`
	Name        string `json:"name" gorm:"not null"`
	Description string `json:"description,omitempty" gorm:"type:text"`
	Enabled     bool   `json:"enabled" gorm:"not null;index"`
	Metadata    string `json:"metadata,omitempty" gorm:"type:text"`

	User *User `json:"-" gorm:"foreignKey:UserID"`
	Jobs []Job `json:"-" gorm:"foreignKey:PipelineID"`
}

// Job armazena a definição persistente de uma automação.
type Job struct {
	UUIDModel
	UserID     string  `json:"userId" gorm:"not null;index;uniqueIndex:ux_jobs_user_slug"`
	PipelineID *string `json:"pipelineId,omitempty" gorm:"index"`
	Slug       string  `json:"slug" gorm:"not null;index;uniqueIndex:ux_jobs_user_slug"`

	Name           string `json:"name" gorm:"not null"`
	Description    string `json:"description,omitempty" gorm:"type:text"`
	Enabled        bool   `json:"enabled" gorm:"not null;index"`
	ToolCatalogID  string `json:"toolCatalogId" gorm:"not null;index"`
	ToolName       string `json:"toolName" gorm:"not null;index"`
	Inputs         string `json:"inputs,omitempty" gorm:"type:text"` // JSON object
	OutputConfig   string `json:"outputConfig,omitempty" gorm:"type:text"`
	EventsConfig   string `json:"eventsConfig,omitempty" gorm:"type:text"`
	ErrorPolicy    string `json:"errorPolicy,omitempty" gorm:"type:text"`
	MaxRunsPerHour int    `json:"maxRunsPerHour,omitempty"`
	DryRunConfig   string `json:"dryRunConfig,omitempty" gorm:"type:text"`
	CreatedBy      string `json:"createdBy,omitempty"`

	User        *User        `json:"-" gorm:"foreignKey:UserID"`
	Pipeline    *JobPipeline `json:"pipeline,omitempty" gorm:"foreignKey:PipelineID"`
	ToolCatalog *ToolCatalog `json:"-" gorm:"foreignKey:ToolCatalogID"`
	Triggers    []JobTrigger `json:"-" gorm:"foreignKey:JobID"`
	Runs        []JobRun     `json:"-" gorm:"foreignKey:JobID"`
}

// JobTrigger registra um gatilho individual de um job.
type JobTrigger struct {
	UUIDModel
	UserID string `json:"userId" gorm:"not null;index;uniqueIndex:ux_job_triggers_identity,priority:1"`
	JobID  string `json:"jobId" gorm:"not null;index;uniqueIndex:ux_job_triggers_identity,priority:2"`

	Type            string     `json:"type" gorm:"not null;index;uniqueIndex:ux_job_triggers_identity,priority:3"`
	Enabled         bool       `json:"enabled" gorm:"not null;index"`
	Expression      string     `json:"expression,omitempty" gorm:"type:text;uniqueIndex:ux_job_triggers_identity,priority:4"`
	Config          string     `json:"config,omitempty" gorm:"type:text;uniqueIndex:ux_job_triggers_identity,priority:5"`
	LastTriggeredAt *time.Time `json:"lastTriggeredAt,omitempty"`

	User *User `json:"-" gorm:"foreignKey:UserID"`
	Job  *Job  `json:"-" gorm:"foreignKey:JobID"`
}

// JobRun registra a execução operacional de um job.
type JobRun struct {
	UUIDModel
	UserID    string `json:"userId" gorm:"not null;index;index:idx_job_runs_user_job_started_at,priority:1;index:idx_job_runs_user_started_at,priority:1"`
	JobID     string `json:"jobId" gorm:"not null;index;index:idx_job_runs_user_job_started_at,priority:2"`
	TriggerID string `json:"triggerId" gorm:"not null;index"`

	Status        string     `json:"status" gorm:"not null;index"`
	StartedAt     time.Time  `json:"startedAt" gorm:"not null;index;index:idx_job_runs_user_job_started_at,priority:3;index:idx_job_runs_user_started_at,priority:2"`
	CompletedAt   *time.Time `json:"completedAt,omitempty"`
	DurationMs    int64      `json:"durationMs,omitempty"`
	Error         string     `json:"error,omitempty" gorm:"type:text"`
	RetryCount    int        `json:"retryCount,omitempty"`
	IsDryRun      bool       `json:"isDryRun,omitempty" gorm:"index"`
	ToolName      string     `json:"toolName,omitempty" gorm:"index"`
	TriggerData   string     `json:"triggerData,omitempty" gorm:"type:text"`
	Inputs        string     `json:"inputs,omitempty" gorm:"type:text"`
	Output        string     `json:"output,omitempty" gorm:"type:text"`
	EventsEmitted string     `json:"eventsEmitted,omitempty" gorm:"type:text"`

	User    *User         `json:"-" gorm:"foreignKey:UserID"`
	Job     *Job          `json:"-" gorm:"foreignKey:JobID"`
	Trigger *JobTrigger   `json:"-" gorm:"foreignKey:TriggerID"`
	Events  []JobRunEvent `json:"-" gorm:"foreignKey:JobRunID"`
}

// JobEvent registra eventos globais do sistema de jobs.
type JobEvent struct {
	UUIDModel
	UserID     string    `json:"userId" gorm:"not null;index;index:idx_job_events_user_occurred_at,priority:1"`
	JobID      *string   `json:"jobId,omitempty" gorm:"index"`
	JobRunID   *string   `json:"jobRunId,omitempty" gorm:"index"`
	OccurredAt time.Time `json:"occurredAt" gorm:"not null;index;index:idx_job_events_user_occurred_at,priority:2"`
	Type       string    `json:"type" gorm:"not null;index"`
	Event      string    `json:"event,omitempty" gorm:"index"`
	Message    string    `json:"message,omitempty" gorm:"type:text"`
	Data       string    `json:"data,omitempty" gorm:"type:text"`

	User   *User   `json:"-" gorm:"foreignKey:UserID"`
	Job    *Job    `json:"-" gorm:"foreignKey:JobID"`
	JobRun *JobRun `json:"-" gorm:"foreignKey:JobRunID"`
}

// JobRunEvent registra a timeline técnica de uma execução.
type JobRunEvent struct {
	UUIDModel
	UserID     string    `json:"userId" gorm:"not null;index;index:idx_job_run_events_user_occurred_at,priority:1"`
	JobRunID   string    `json:"jobRunId" gorm:"not null;index;uniqueIndex:ux_job_run_events_run_sequence"`
	Sequence   int       `json:"sequence" gorm:"not null;uniqueIndex:ux_job_run_events_run_sequence"`
	OccurredAt time.Time `json:"occurredAt" gorm:"not null;index;index:idx_job_run_events_user_occurred_at,priority:2"`
	Type       string    `json:"type" gorm:"not null;index"`
	Message    string    `json:"message,omitempty" gorm:"type:text"`
	Data       string    `json:"data,omitempty" gorm:"type:text"`

	User   *User   `json:"-" gorm:"foreignKey:UserID"`
	JobRun *JobRun `json:"-" gorm:"foreignKey:JobRunID"`
}
