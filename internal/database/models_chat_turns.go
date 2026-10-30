package database

import "time"

// ChatTurn contém apenas lifecycle e referências. Conteúdo continua em chat_messages.
// Ainda não é autoridade do pipeline: a ativação exige reconciliação (AEP-0116).
type ChatTurn struct {
	UserMessageID     string `gorm:"primaryKey"`
	ConversationID    string
	ResponseMessageID *string
	State             string
	ActiveExecutionID *string
	Origin            string
}

type ChatTurnAttempt struct {
	ExecutionID    string `gorm:"primaryKey"`
	TurnID         string
	Mode           string
	State          string
	StartedAt      time.Time
	FinishedAt     *time.Time
	TerminalReason string
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
