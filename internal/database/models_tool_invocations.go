package database

import "time"

// ToolInvocation registra uma execução técnica de tool, independente da origem
// (chat, job, dry-run manual do catálogo, etc.).
type ToolInvocation struct {
	UUIDModel
	UserID        string `json:"userId" gorm:"not null;index:idx_tool_invocations_user_origin,priority:1;index:idx_tool_invocations_user_origin_queued,priority:1;index:idx_tool_invocations_user_tool_started,priority:1;index:idx_tool_invocations_user_status_queued,priority:1;index:idx_tool_invocations_user_dryrun_queued,priority:1"`
	ToolCatalogID string `json:"toolCatalogId" gorm:"not null;index:idx_tool_invocations_user_tool_started,priority:2"`

	OriginType         string  `json:"originType" gorm:"not null;index:idx_tool_invocations_user_origin,priority:2;index:idx_tool_invocations_user_origin_queued,priority:2"`
	OriginID           string  `json:"originId,omitempty" gorm:"index:idx_tool_invocations_user_origin,priority:3"`
	ParentInvocationID *string `json:"parentInvocationId,omitempty" gorm:"index"`
	ToolCallID         string  `json:"toolCallId,omitempty" gorm:"index"`

	Status   string `json:"status" gorm:"not null;index:idx_tool_invocations_user_status_queued,priority:2"`
	DryRun   bool   `json:"dryRun,omitempty" gorm:"not null;default:false;index:idx_tool_invocations_user_dryrun_queued,priority:2"`
	Input    string `json:"input,omitempty" gorm:"type:text"`
	Output   string `json:"output,omitempty" gorm:"type:text"`
	Metadata string `json:"metadata,omitempty" gorm:"type:text"`

	ErrorKind    string     `json:"errorKind,omitempty" gorm:"index"`
	ErrorMessage string     `json:"errorMessage,omitempty" gorm:"type:text"`
	Retryable    bool       `json:"retryable,omitempty"`
	QueuedAt     time.Time  `json:"queuedAt" gorm:"not null;index:idx_tool_invocations_user_origin_queued,priority:3;index:idx_tool_invocations_user_status_queued,priority:3;index:idx_tool_invocations_user_dryrun_queued,priority:3"`
	StartedAt    *time.Time `json:"startedAt,omitempty" gorm:"index:idx_tool_invocations_user_tool_started,priority:3"`
	CompletedAt  *time.Time `json:"completedAt,omitempty" gorm:"index"`
	DurationMs   int64      `json:"durationMs,omitempty"`

	User             *User            `json:"-" gorm:"foreignKey:UserID"`
	ToolCatalog      *ToolCatalog     `json:"-" gorm:"foreignKey:ToolCatalogID"`
	ParentInvocation *ToolInvocation  `json:"-" gorm:"foreignKey:ParentInvocationID"`
	ChildInvocations []ToolInvocation `json:"-" gorm:"foreignKey:ParentInvocationID"`
}
