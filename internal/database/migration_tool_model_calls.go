package database

import (
	"fmt"

	"gorm.io/gorm"
)

const (
	toolModelCallsConversationIndex = "idx_tool_invocations_model_calls_conversation"
	toolModelCallsTurnIndex         = "idx_tool_invocations_model_calls_turn"
)

// migrateToolModelCallProjection materializa somente atributos técnicos já
// presentes no ledger canônico. A migração não consulta chat_messages: a v19
// já garantiu e validou conversation_id/turn_id para todo histórico de chat.
func migrateToolModelCallProjection(database *gorm.DB) error {
	if database == nil || !database.Migrator().HasTable(&ToolInvocation{}) {
		return nil
	}
	if !database.Migrator().HasColumn(&ToolInvocation{}, "model_iteration") ||
		!database.Migrator().HasColumn(&ToolInvocation{}, "external") {
		return fmt.Errorf("colunas materializadas de model calls ausentes")
	}

	if err := database.Exec(`
		UPDATE tool_invocations
		   SET model_iteration = CASE
		         WHEN json_valid(metadata)
		          AND json_type(metadata, '$.display.iteration') IN ('integer', 'real')
		         THEN CAST(json_extract(metadata, '$.display.iteration') AS INTEGER)
		         ELSE 0
		       END,
		       external = CASE
		         WHEN json_valid(metadata)
		          AND json_extract(metadata, '$.external') = 1
		         THEN 1
		         ELSE 0
		       END`).Error; err != nil {
		return fmt.Errorf("materializar iteração e origem externa: %w", err)
	}

	statements := []string{
		`CREATE INDEX IF NOT EXISTS ` + toolModelCallsConversationIndex + `
		 ON tool_invocations (user_id, conversation_id, origin_id, model_iteration)
		 WHERE origin_type = 'chat' AND external = 0 AND trim(tool_call_id) <> ''`,
		`CREATE INDEX IF NOT EXISTS ` + toolModelCallsTurnIndex + `
		 ON tool_invocations (user_id, conversation_id, turn_id, origin_id, model_iteration)
		 WHERE origin_type = 'chat' AND external = 0 AND trim(tool_call_id) <> ''`,
	}
	for _, statement := range statements {
		if err := database.Exec(statement).Error; err != nil {
			return fmt.Errorf("criar índice de contagem de model calls: %w", err)
		}
	}
	return nil
}
