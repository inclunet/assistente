package database

import "time"

// ToolInvocation registra uma execução técnica de tool, independente da origem
// (chat, job, dry-run manual do catálogo, etc.).
type ToolInvocation struct {
	UUIDModel
	UserID        string `json:"userId" gorm:"not null;index"`
	ToolCatalogID string `json:"toolCatalogId" gorm:"not null;index"`

	OriginType         string  `json:"originType" gorm:"not null;index"`
	OriginID           string  `json:"originId,omitempty" gorm:"index"`
	ParentInvocationID *string `json:"parentInvocationId,omitempty" gorm:"index"`
	ToolCallID         string  `json:"toolCallId,omitempty" gorm:"index"`

	Status   string `json:"status" gorm:"not null;index"`
	DryRun   bool   `json:"dryRun,omitempty" gorm:"not null;default:false;index"`
	Input    string `json:"input,omitempty" gorm:"type:text"`
	Output   string `json:"output,omitempty" gorm:"type:text"`
	Metadata string `json:"metadata,omitempty" gorm:"type:text"`

	ErrorKind    string     `json:"errorKind,omitempty" gorm:"index"`
	ErrorMessage string     `json:"errorMessage,omitempty" gorm:"type:text"`
	Retryable    bool       `json:"retryable,omitempty"`
	QueuedAt     time.Time  `json:"queuedAt" gorm:"not null;index"`
	StartedAt    *time.Time `json:"startedAt,omitempty" gorm:"index"`
	CompletedAt  *time.Time `json:"completedAt,omitempty" gorm:"index"`
	DurationMs   int64      `json:"durationMs,omitempty"`

	User             *User            `json:"-" gorm:"foreignKey:UserID"`
	ToolCatalog      *ToolCatalog     `json:"-" gorm:"foreignKey:ToolCatalogID"`
	ParentInvocation *ToolInvocation  `json:"-" gorm:"foreignKey:ParentInvocationID"`
	ChildInvocations []ToolInvocation `json:"-" gorm:"foreignKey:ParentInvocationID"`
}
