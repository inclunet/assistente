package database

import "gorm.io/gorm"

// RestartToolResult contém apenas os campos destinados ao modelo. O conteúdo
// integral de auditoria (argumentos, metadados e diagnósticos) fica no ledger.
type RestartToolResult struct {
	ID     string
	Name   string
	Status string
	Output string
}

// O chamador fornece expressões SQL internas. A autorização pertence à
// consulta externa e todos os ancestrais pertencem ao turno selecionado.
func hasRestartInChainSQL(seed, turn string) string {
	return "EXISTS (" + executionChainFromSeedSQL(seed) + `SELECT 1 FROM execution_chain c
 JOIN chat_turn_attempts a ON a.execution_id=c.execution_id
 WHERE a.turn_id=` + turn + ` AND a.state='interrupted' AND a.terminal_reason='restart')`
}

// A admissão já confirmou a posse da conversa e a base da continuação nesta
// transação. Um retry quebra a cadeia, inclusive quando repete o mesmo turno.
func readRestartToolResults(tx *gorm.DB, userID, conversationID, turnID, baseExecution string) ([]RestartToolResult, error) {
	chain := executionChainFromSeedSQL("SELECT ?,NULL,0")
	var restart bool
	if err := tx.Raw("SELECT "+hasRestartInChainSQL("SELECT ?,NULL,0", "?"), baseExecution, turnID).Scan(&restart).Error; err != nil || !restart {
		return nil, err
	}

	// Quando há muitos resultados ou uma saída é extensa, o modelo ainda
	// pode consultar a base pelo get_messages. Nunca corte JSON ou texto exato.
	// Uma lista vazia confirma o reinício mesmo sem recibo local. nil significa continuação comum.
	rows := make([]RestartToolResult, 0)
	err := tx.Raw(chain+`SELECT i.id, substr(coalesce(nullif(i.display_name,''),catalog.name,i.tool_catalog_id),1,128) AS name, i.status,
 CASE WHEN i.result_availability='available' AND json_valid(i.output)
 THEN CASE WHEN json_type(i.output)='object' THEN json_remove(i.output,'$.metadata') ELSE i.output END
 ELSE '' END AS output
 FROM execution_chain c JOIN tool_invocations i ON i.chat_execution_id=c.execution_id
 LEFT JOIN tool_catalog catalog ON catalog.id=i.tool_catalog_id
 WHERE i.origin_type='chat' AND i.user_id=? AND i.conversation_id=? AND i.turn_id=?
 ORDER BY c.depth, i.model_iteration DESC, i.queued_at DESC, i.id DESC LIMIT 9`,
		baseExecution, userID, conversationID, turnID).Scan(&rows).Error
	return rows, err
}
