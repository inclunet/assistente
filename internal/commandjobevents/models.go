// Package commandjobevents contém a fronteira durável entre o runtime de jobs
// e a ativação contextual de comandos. Ele não conhece jobs.Manager, claims ou
// EventBus; isso impede que um barramento best-effort vire fonte de replay.
package commandjobevents

import "time"

const (
	SchemaVersion = "command-context.job-run-state.v1"
	ProducerType  = "jobs.runtime"

	StateQueued         = "queued"
	StateStarted        = "started"
	StateRetryScheduled = "retry_scheduled"
	StateCompleted      = "completed"
	StateFailed         = "failed"
	StateSkipped        = "skipped"

	DeliveryPending    = "pending"
	DeliveryProcessing = "processing"
	DeliveryDelivered  = "delivered"
	DeliveryDeadLetter = "dead_letter"
)

// Fact é o envelope interno normalizado a partir de um job_run_event. Campos
// de identidade são derivados pelo repository autenticado, nunca aceitos como
// autoridade de um payload externo.
type Fact struct {
	SchemaVersion                string         `json:"version"`
	EventName                    string         `json:"event_name"`
	SourceEventID                string         `json:"source_event_id"`
	UserID                       string         `json:"user_id"`
	JobDatabaseID                string         `json:"job_database_id"`
	JobSlug                      string         `json:"job_slug"`
	RunID                        string         `json:"run_id"`
	RunEventID                   string         `json:"run_event_id"`
	Sequence                     int            `json:"sequence"`
	State                        string         `json:"state"`
	OccurredAt                   time.Time      `json:"occurred_at"`
	RootOriginType               string         `json:"root_origin_type"`
	RootOriginID                 string         `json:"root_origin_id"`
	Provenance                   map[string]any `json:"provenance,omitempty"`
	SourceReplayPolicyGeneration string         `json:"source_replay_policy_generation"`
	SourceReplayDeadline         time.Time      `json:"source_replay_deadline"`
	EventFingerprint             string         `json:"event_fingerprint,omitempty"`
}

// ActivationOutbox é deliberadamente independente de job_runs e
// job_run_events: não há FK nem cascade. A ocorrência precisa sobreviver ao
// count-cap/limpeza do job até o seu deadline de replay.
type ActivationOutbox struct {
	SourceEventID                string     `gorm:"type:text;primaryKey;check:ck_command_outbox_source_event_uuid7,length(source_event_id) = 36 AND substr(source_event_id,9,1) = '-' AND substr(source_event_id,14,1) = '-' AND substr(source_event_id,15,1) = '7' AND substr(source_event_id,19,1) = '-' AND substr(source_event_id,24,1) = '-' AND lower(substr(source_event_id,20,1)) IN ('8','9','a','b')"`
	SchemaVersion                string     `gorm:"type:text;not null"`
	EventName                    string     `gorm:"type:text;not null;index"`
	UserID                       string     `gorm:"type:text;not null;index;check:ck_command_outbox_user_uuid7,length(user_id) = 36 AND substr(user_id,9,1) = '-' AND substr(user_id,14,1) = '-' AND substr(user_id,15,1) = '7' AND substr(user_id,19,1) = '-' AND substr(user_id,24,1) = '-' AND lower(substr(user_id,20,1)) IN ('8','9','a','b')"`
	JobDatabaseID                string     `gorm:"type:text;not null;index;check:ck_command_outbox_job_uuid7,length(job_database_id) = 36 AND substr(job_database_id,9,1) = '-' AND substr(job_database_id,14,1) = '-' AND substr(job_database_id,15,1) = '7' AND substr(job_database_id,19,1) = '-' AND substr(job_database_id,24,1) = '-' AND lower(substr(job_database_id,20,1)) IN ('8','9','a','b')"`
	JobSlug                      string     `gorm:"type:text;not null;index"`
	RunID                        string     `gorm:"type:text;not null;index"`
	Sequence                     int        `gorm:"not null;check:ck_command_outbox_sequence,sequence > 0"`
	State                        string     `gorm:"type:text;not null;index;check:ck_command_outbox_state,state IN ('queued','started','retry_scheduled','completed','failed','skipped')"`
	OccurredAt                   time.Time  `gorm:"not null;index"`
	RootOriginType               string     `gorm:"type:text;not null;index"`
	RootOriginID                 string     `gorm:"type:text;not null"`
	Provenance                   string     `gorm:"type:text"`
	SourceReplayPolicyGeneration string     `gorm:"type:text;not null"`
	SourceReplayDeadline         time.Time  `gorm:"not null;index"`
	EventFingerprint             string     `gorm:"type:text;not null;index;check:ck_command_outbox_fingerprint,length(event_fingerprint) = 64"`
	DeliveryState                string     `gorm:"type:text;not null;index;check:ck_command_outbox_delivery_state,delivery_state IN ('pending','processing','delivered','dead_letter')"`
	LeaseOwner                   *string    `gorm:"type:text"`
	LeaseExpiresAt               *time.Time `gorm:"index"`
	Attempts                     int        `gorm:"not null;default:0;check:ck_command_outbox_attempts,attempts >= 0"`
	LastErrorCode                string     `gorm:"type:text"`
	CreatedAt                    time.Time  `gorm:"not null;index"`
	DeliveredAt                  *time.Time `gorm:"index"`
}

func (ActivationOutbox) TableName() string { return "command_job_activation_outbox" }

// ReplayPolicyEpoch é a política de replay vigente para um produtor. O
// deadline é copiado para cada ocorrência e nunca é recalculado ao alterar a
// retenção posteriormente.
type ReplayPolicyEpoch struct {
	ID                   string    `gorm:"type:text;primaryKey;check:ck_command_replay_epoch_uuid7,length(id) = 36 AND substr(id,9,1) = '-' AND substr(id,14,1) = '-' AND substr(id,15,1) = '7' AND substr(id,19,1) = '-' AND substr(id,24,1) = '-' AND lower(substr(id,20,1)) IN ('8','9','a','b')"`
	ProducerType         string    `gorm:"type:text;not null;uniqueIndex:ux_command_replay_epoch,priority:1;check:ck_command_replay_epoch_producer,producer_type = 'jobs.runtime'"`
	Generation           int64     `gorm:"not null;uniqueIndex:ux_command_replay_epoch,priority:2;check:ck_command_replay_epoch_generation,generation > 0"`
	EffectiveAt          time.Time `gorm:"not null;uniqueIndex:ux_command_replay_effective,priority:1;index"`
	ReplayHorizonSeconds int64     `gorm:"not null;check:ck_command_replay_epoch_horizon,replay_horizon_seconds > 0"`
	CreatedAt            time.Time `gorm:"not null"`
}

func (ReplayPolicyEpoch) TableName() string { return "command_event_replay_policy_epochs" }

// Models retorna os modelos que o bootstrap/migração central deve registrar.
// O pacote não chama AutoMigrate sozinho para não inventar uma migração fora
// do dono da integração.
func Models() []any { return []any{&ActivationOutbox{}, &ReplayPolicyEpoch{}} }

// OutboxRecord é a representação pública de uma entrega reivindicada.
type OutboxRecord = ActivationOutbox
