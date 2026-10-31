package database

import "time"

// ChatTurn contém apenas lifecycle e referências. Conteúdo continua em chat_messages.
// Autoridade de lifecycle do pipeline; a projeção visual migra separadamente (AEP-0116).
type ChatTurn struct {
	CurrentExecutionID *string `gorm:"->;-:migration"`
	UserMessageID      string  `gorm:"primaryKey"`
	ConversationID     string
	ResponseMessageID  *string
	State              string
	ActiveExecutionID  *string
	Origin             string
}

type ChatTurnAttempt struct {
	ResponseMessageID     *string `gorm:"->;-:migration"`
	BaseExecutionID       *string `gorm:"->;-:migration"`
	BaseResponseMessageID *string `gorm:"->;-:migration"`
	ExecutionID           string  `gorm:"primaryKey"`
	TurnID                string
	Mode                  string
	State                 string
	StartedAt             time.Time
	FinishedAt            *time.Time
	TerminalReason        string
}

const (
	TurnPreparing         = "preparing"
	TurnStreaming         = "streaming"
	TurnCompleted         = "completed"
	TurnInterrupted       = "interrupted"
	TurnFailed            = "failed"
	TurnHistoricalUnknown = "historical_unknown"
	TurnModeNew           = "new"
	TurnModeRetry         = "retry"
	TurnModeContinue      = "continue"
)
