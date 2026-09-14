package database

import "time"

// ToolInvocation registra uma execução técnica de tool, independente da origem
// (chat, job, dry-run manual do catálogo, etc.).
type ToolInvocation struct {
	UUIDModel
	UserID        string `json:"userId" gorm:"not null;index:idx_tool_invocations_user_origin,priority:1;index:idx_tool_invocations_user_origin_queued,priority:1;index:idx_tool_invocations_user_tool_started,priority:1;index:idx_tool_invocations_user_status_queued,priority:1;index:idx_tool_invocations_user_dryrun_queued,priority:1;index:idx_tool_invocations_user_conversation_turn,priority:1"`
	ToolCatalogID string `json:"toolCatalogId" gorm:"not null;index:idx_tool_invocations_user_tool_started,priority:2"`

	OriginType         string  `json:"originType" gorm:"not null;index:idx_tool_invocations_user_origin,priority:2;index:idx_tool_invocations_user_origin_queued,priority:2"`
	OriginID           string  `json:"originId,omitempty" gorm:"index:idx_tool_invocations_user_origin,priority:3"`
	ConversationID     *string `json:"conversationId,omitempty" gorm:"index:idx_tool_invocations_user_conversation_turn,priority:2"`
	TurnID             *string `json:"turnId,omitempty" gorm:"index:idx_tool_invocations_user_conversation_turn,priority:3"`
	ParentInvocationID *string `json:"parentInvocationId,omitempty" gorm:"index"`
	ToolCallID         string  `json:"toolCallId,omitempty" gorm:"index"`
	Attempt            int     `json:"attempt" gorm:"not null;default:1"`

	Status   string `json:"status" gorm:"not null;index:idx_tool_invocations_user_status_queued,priority:2"`
	DryRun   bool   `json:"dryRun,omitempty" gorm:"not null;default:false;index:idx_tool_invocations_user_dryrun_queued,priority:2"`
	Input    string `json:"input,omitempty" gorm:"type:text"`
	Output   string `json:"output,omitempty" gorm:"type:text"`
	Metadata string `json:"metadata,omitempty" gorm:"type:text"`

	DisplayName         string `json:"displayName,omitempty"`
	InputPreview        string `json:"inputPreview,omitempty" gorm:"type:text"`
	OutputPreview       string `json:"outputPreview,omitempty" gorm:"type:text"`
	InputBytes          int64  `json:"inputBytes,omitempty"`
	OutputBytes         int64  `json:"outputBytes,omitempty"`
	InputHash           string `json:"inputHash,omitempty" gorm:"index"`
	OutputHash          string `json:"outputHash,omitempty" gorm:"index"`
	ResultAvailability  string `json:"resultAvailability,omitempty" gorm:"not null;default:'available';index"`
	MigrationSourceKey  string `json:"-" gorm:"not null;default:'';index"`
	MigrationProvenance string `json:"-" gorm:"not null;default:'';index"`

	ErrorKind         string     `json:"errorKind,omitempty" gorm:"index"`
	ErrorCode         string     `json:"errorCode,omitempty" gorm:"index"`
	ErrorMessage      string     `json:"errorMessage,omitempty" gorm:"type:text"`
	Retryable         bool       `json:"retryable,omitempty" gorm:"not null;default:false"`
	RetryabilityKnown bool       `json:"retryabilityKnown,omitempty" gorm:"not null;default:false"`
	QueuedAt          time.Time  `json:"queuedAt" gorm:"not null;index:idx_tool_invocations_user_origin_queued,priority:3;index:idx_tool_invocations_user_status_queued,priority:3;index:idx_tool_invocations_user_dryrun_queued,priority:3"`
	StartedAt         *time.Time `json:"startedAt,omitempty" gorm:"index:idx_tool_invocations_user_tool_started,priority:3"`
	CompletedAt       *time.Time `json:"completedAt,omitempty" gorm:"index"`
	DurationMs        int64      `json:"durationMs,omitempty"`

	User             *User            `json:"-" gorm:"foreignKey:UserID"`
	ToolCatalog      *ToolCatalog     `json:"-" gorm:"foreignKey:ToolCatalogID"`
	Conversation     *Conversation    `json:"-" gorm:"foreignKey:ConversationID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	ParentInvocation *ToolInvocation  `json:"-" gorm:"foreignKey:ParentInvocationID"`
	ChildInvocations []ToolInvocation `json:"-" gorm:"foreignKey:ParentInvocationID"`
}

// ToolLedgerMigrationState torna o backfill retomável por recurso. O estado
// canonical só é escrito pela migração final da AEP-0104.
type ToolLedgerMigrationState struct {
	UUIDModel
	UserID       string `json:"-" gorm:"not null;index;uniqueIndex:ux_tool_ledger_state_resource"`
	ResourceType string `json:"resourceType" gorm:"not null;check:ck_tool_ledger_resource_type,resource_type IN ('conversation','job_run');index;uniqueIndex:ux_tool_ledger_state_resource"`
	ResourceID   string `json:"resourceId" gorm:"not null;index;uniqueIndex:ux_tool_ledger_state_resource"`
	State        string `json:"state" gorm:"not null;default:'pending';check:ck_tool_ledger_state,state IN ('pending','backfilled','canonical');index"`

	LegacyRows          int        `json:"legacyRows,omitempty"`
	LedgerRows          int        `json:"ledgerRows,omitempty"`
	LegacyInputDigest   string     `json:"legacyInputDigest,omitempty"`
	LegacyOutputDigest  string     `json:"legacyOutputDigest,omitempty"`
	LedgerInputDigest   string     `json:"ledgerInputDigest,omitempty"`
	LedgerOutputDigest  string     `json:"ledgerOutputDigest,omitempty"`
	AmbiguousCount      int        `json:"ambiguousCount,omitempty"`
	LastErrorCode       string     `json:"lastErrorCode,omitempty"`
	LastProcessedKey    string     `json:"lastProcessedKey,omitempty"`
	LegacyHighWatermark *time.Time `json:"legacyHighWatermark,omitempty"`

	User *User `json:"-" gorm:"foreignKey:UserID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
}

func (ToolLedgerMigrationState) TableName() string {
	return "tool_ledger_migration_states"
}
