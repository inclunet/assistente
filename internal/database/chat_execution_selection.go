package database

import "fmt"

// Os argumentos destas funções são aliases/expressões definidos no código,
// nunca entrada do usuário. A seleção não substitui o escopo autorizado da
// consulta. A recursão é restrita a um turno, não materializa toda a instalação.
func executionChainFromSeedSQL(seed string) string {
	return `WITH RECURSIVE execution_chain(execution_id,omitted_response_id,depth) AS (` + seed + `
 UNION ALL
 SELECT a.base_execution_id,a.base_response_message_id,c.depth+1
 FROM execution_chain c JOIN chat_turn_attempts a ON a.execution_id=c.execution_id
 WHERE a.mode='continue' AND a.base_execution_id IS NOT NULL
) `
}

func selectedMessageExecutionSQL(alias string) string {
	chain := executionChainFromSeedSQL("SELECT selected_turn.current_execution_id,NULL,0")
	omit := fmt.Sprintf(" AND %s.id IS NOT c.omitted_response_id", alias)
	legacyOmit := fmt.Sprintf(" AND %s.id<>a.base_response_message_id", alias)
	return fmt.Sprintf(`EXISTS (%s
 SELECT 1 FROM execution_chain c WHERE c.execution_id=%s.chat_execution_id%s
 UNION ALL
 SELECT 1 FROM execution_chain c JOIN chat_turn_attempts a ON a.execution_id=c.execution_id
 WHERE %s.chat_execution_id IS NULL AND a.mode='continue'
 AND a.base_execution_id IS NULL AND a.base_response_message_id IS NOT NULL%s
)`, chain, alias, omit, alias, legacyOmit)
}

// CurrentChatMessageSQL é aplicada ANTES de LIMIT, contagem e paginação.
// Perguntas e vínculos legados inválidos continuam independentes. Sem seleção
// explícita, preserva o histórico legado; seleção nova nunca cai para o legado.
func currentChatMessagePredicate(alias string) string {
	return fmt.Sprintf(`(%s.role='user' OR NOT EXISTS (
 SELECT 1 FROM chat_turns selected_turn JOIN chat_messages q ON q.id=selected_turn.user_message_id
 WHERE selected_turn.user_message_id=%s.turn_id AND selected_turn.conversation_id=%s.conversation_id
 AND q.parent_id IS %s.parent_id AND selected_turn.current_execution_id IS NOT NULL
 AND NOT %s))`, alias, alias, alias, alias, selectedMessageExecutionSQL(alias))
}

var currentMessagePredicates = map[string]string{
	"chat_messages": currentChatMessagePredicate("chat_messages"),
	"m":             currentChatMessagePredicate("m"),
}

func CurrentChatMessageSQL(alias string) string {
	if predicate, exists := currentMessagePredicates[alias]; exists {
		return predicate
	}
	return currentChatMessagePredicate(alias)
}

// ChatInvocationSelection compartilha o percurso da cadeia entre histórico atual
// e acesso explícito a uma execução preservada. O último usa o alias fixo
// requested_chat_execution, ligado por parâmetro pelo leitor autorizado.
type ChatInvocationSelection struct{ Historical bool }

// Join relaciona a seleção uma vez por consulta; não repete lookup por coluna.
func (s ChatInvocationSelection) Join(alias string) string {
	return "LEFT JOIN chat_turns selected_chat_turn ON selected_chat_turn.user_message_id=" + alias + ".turn_id AND selected_chat_turn.conversation_id=" + alias + ".conversation_id"
}

func (s ChatInvocationSelection) chain(alias string) string {
	if !s.Historical {
		return executionChainFromSeedSQL("SELECT selected_chat_turn.current_execution_id,NULL,0")
	}
	return executionChainFromSeedSQL("SELECT requested_chat_execution.execution_id,NULL,0 FROM chat_turn_attempts root_attempt WHERE root_attempt.execution_id=requested_chat_execution.execution_id AND root_attempt.turn_id=" + alias + ".turn_id")
}

func (s ChatInvocationSelection) Predicate(alias string) string {
	selected := fmt.Sprintf(`EXISTS (%s
 SELECT 1 FROM execution_chain c WHERE c.execution_id=%s.chat_execution_id
 UNION ALL
 SELECT 1 FROM execution_chain c JOIN chat_turn_attempts a ON a.execution_id=c.execution_id
 WHERE %s.chat_execution_id IS NULL AND a.mode='continue'
 AND a.base_execution_id IS NULL AND a.base_response_message_id IS NOT NULL
 )`, s.chain(alias), alias, alias)
	if s.Historical {
		return fmt.Sprintf("((requested_chat_execution.execution_id IS NULL AND %s.chat_execution_id IS NULL) OR %s)", alias, selected)
	}
	return "(selected_chat_turn.current_execution_id IS NULL OR " + selected + ")"
}

// Depth ordena bases antes da execução selecionada, sem comparar timestamps.
func (s ChatInvocationSelection) Depth(alias string) string {
	query := fmt.Sprintf(`COALESCE((%s
 SELECT c.depth FROM execution_chain c WHERE c.execution_id=%s.chat_execution_id
 UNION ALL
 SELECT c.depth+1 FROM execution_chain c JOIN chat_turn_attempts a ON a.execution_id=c.execution_id
 WHERE %s.chat_execution_id IS NULL AND a.mode='continue'
 AND a.base_execution_id IS NULL AND a.base_response_message_id IS NOT NULL LIMIT 1),0)`, s.chain(alias), alias, alias)
	if !s.Historical {
		return "CASE WHEN selected_chat_turn.current_execution_id IS NULL THEN 0 ELSE " + query + " END"
	}
	return query
}

// PresentationMessage reancora somente o prefixo explicitamente herdado. O
// ledger mantém a referência original; a projeção não compara conteúdo textual.
func (s ChatInvocationSelection) PresentationMessage(alias, original string) string {
	response := "selected_chat_turn.response_message_id"
	if s.Historical {
		response = "(SELECT response_message_id FROM chat_turn_attempts WHERE execution_id=requested_chat_execution.execution_id AND turn_id=" + alias + ".turn_id)"
	}
	query := fmt.Sprintf(`CASE WHEN EXISTS (%s
 SELECT 1 FROM execution_chain c JOIN chat_turn_attempts a ON a.execution_id=c.execution_id
 WHERE a.base_response_message_id=(%s)) THEN %s ELSE (%s) END`, s.chain(alias), original, response, original)
	if !s.Historical {
		return "CASE WHEN selected_chat_turn.current_execution_id IS NULL THEN (" + original + ") ELSE " + query + " END"
	}
	return query
}
