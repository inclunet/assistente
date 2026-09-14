package commandledger

import "time"

// ledgerRow é a chave de idempotência durável. InvocationID é uma referência
// de auditoria deliberadamente sem FK: a auditoria pode ser compactada.
type ledgerRow struct {
	ID                           string `gorm:"type:text;primaryKey"`
	Key                          string `gorm:"type:text;not null;uniqueIndex:ux_command_idempotency_keys_key"`
	InvocationID                 string `gorm:"type:text;not null;uniqueIndex:ux_command_idempotency_keys_invocation_id"`
	UserID                       *string
	AuthContextType              string `gorm:"type:text;not null"`
	AuthContextID                string `gorm:"type:text;not null"`
	SourceType                   *string
	SourceInstanceID             *string
	SourceEventID                *string
	SourceOccurredAt             *time.Time
	SourceReplayPolicyGeneration *string
	SourceReplayDeadline         *time.Time
	RequestFingerprintVersion    string `gorm:"type:text;not null"`
	RequestFingerprint           string `gorm:"type:text;not null"`
	Status                       Status `gorm:"type:text;not null"`
	ResultSummary                *string
	ResultRef                    *string
	ReceivedAt                   time.Time `gorm:"not null"`
	ExpiresAt                    time.Time `gorm:"not null"`
}

func (ledgerRow) TableName() string { return "command_idempotency_keys" }

// invocationRow é a auditoria técnica da invocação. Snapshots e resumos são
// texto JSON/redigido; a validação semântica dos grupos pertence às camadas
// superiores quando a API de execução for habilitada.
type invocationRow struct {
	InvocationID                 string `gorm:"type:text;primaryKey"`
	SchemaVersion                int    `gorm:"not null"`
	UserID                       *string
	AuthContextType              string `gorm:"type:text;not null"`
	AuthContextID                string `gorm:"type:text;not null"`
	AuthGeneration               string `gorm:"type:text;not null"`
	SessionID                    *string
	SecurityGeneration           string `gorm:"type:text;not null"`
	WorkspaceID                  *string
	RegistryVersion              string `gorm:"type:text;not null"`
	GlobalConfigGeneration       *string
	WorkspaceConfigGeneration    *string
	ActiveLayersGeneration       *string
	CommandID                    *string
	BindingIDs                   string `gorm:"type:text;not null;default:'[]'"`
	ObservedTriggerType          *string
	TriggerType                  *string
	TriggerSpecSnapshot          *string
	TriggerFingerprint           *string
	ActorType                    string `gorm:"type:text;not null"`
	ActorID                      string `gorm:"type:text;not null"`
	SourceType                   *string
	ObserverType                 *string
	SourceInstanceID             *string
	SourceEventID                *string
	SourceOccurredAt             *time.Time
	SourceReplayPolicyGeneration *string
	SourceReplayDeadline         *time.Time
	ArgumentsSummary             string `gorm:"type:text;not null"`
	ArgumentsFingerprint         string `gorm:"type:text;not null"`
	ConversationID               *string
	TurnID                       *string
	SurfaceType                  *string
	SurfaceID                    *string
	SurfaceSnapshotVersion       *string
	ContextVersion               *string
	ContextCapturedAtByProvider  *string
	ContextSummary               *string
	ForegroundSummary            *string
	SourceProfileSlug            *string
	TargetProfileSlug            *string
	AuthorizationDecisionID      *string
	DelegationFingerprint        *string
	GrantGeneration              *string
	JobID                        *string
	JobSlug                      *string
	JobDefinitionFingerprint     *string
	RunID                        *string
	Provenance                   *string
	CorrelationID                string `gorm:"type:text;not null"`
	RequestFingerprintVersion    string `gorm:"type:text;not null"`
	RequestFingerprint           string `gorm:"type:text;not null"`
	Risk                         string `gorm:"type:text;not null"`
	PolicyDecision               string `gorm:"type:text;not null"`
	ResultSummary                *string
	ResultRef                    *string
	Status                       Status `gorm:"type:text;not null"`
	ErrorCode                    *string
	ClientRequestedAt            *time.Time
	ReceivedAt                   time.Time `gorm:"not null"`
	CompletedAt                  *time.Time
}

func (invocationRow) TableName() string { return "command_invocations" }
