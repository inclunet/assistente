package database

// SelectLegacyTurnRepresentative preserva a seleção histórica do consolidador.
// Mensagens devem vir ordenadas por created_at e id; não usar em turnos com
// representante explícito. Migração e projeção legada compartilham esta regra.
func SelectLegacyTurnRepresentative(messages []ChatMessage, intermediateIDs map[string]bool) ChatMessage {
	if len(messages) == 0 {
		return ChatMessage{}
	}
	hasFinalCandidate := false
	for _, message := range messages {
		if message.Role == "assistant" && !intermediateIDs[message.ID] {
			hasFinalCandidate = true
			break
		}
	}
	representative := messages[0]
	hasAssistant := false
	for _, message := range messages {
		if message.Role == "assistant" && (!hasFinalCandidate || !intermediateIDs[message.ID]) {
			// O registro final pode ser criado antes das rodadas e atualizado
			// no encerramento. Usar a escrita, não só criação ou usage.
			if !hasAssistant || legacyTurnMessageWrittenAfter(message, representative) {
				representative = message
			}
			hasAssistant = true
		}
	}

	return representative
}

func legacyTurnMessageWrittenAfter(left, right ChatMessage) bool {
	leftTime, rightTime := left.CreatedAt, right.CreatedAt
	if left.UpdatedAt.After(leftTime) {
		leftTime = left.UpdatedAt
	}
	if right.UpdatedAt.After(rightTime) {
		rightTime = right.UpdatedAt
	}
	if !leftTime.Equal(rightTime) {
		return leftTime.After(rightTime)
	}
	if (left.TotalTokens > 0) != (right.TotalTokens > 0) {
		return left.TotalTokens > 0
	}
	if !left.CreatedAt.Equal(right.CreatedAt) {
		return left.CreatedAt.After(right.CreatedAt)
	}
	return left.ID > right.ID
}
