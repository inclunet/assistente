package chat

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"sync"
	"unicode/utf8"

	"assistente/internal/database"
	"gorm.io/gorm"
)

// Expressões constantes compartilhadas: não reconstruir nomes/concatenações
// em cada leitura. O destino de PresentationMessage é contado por invocação.
var (
	publicationLifecycleSizeSQL = publicationSQLBytes("chat_turns", "user_message_id", "active_execution_id", "current_execution_id")
	publicationMessageSizeSQL   = publicationSQLBytes("chat_messages", "id", "conversation_id", "parent_id", "turn_id", "chat_execution_id", "role", "content", "reasoning", "model", "source")
	publicationTurnSizeSQL      = publicationSQLBytes("chat_turns", "user_message_id", "conversation_id", "response_message_id", "current_execution_id", "active_execution_id", "state", "origin")
	publicationCallSizeSQL      = publicationSQLBytes("tool_invocations", "id", "user_id", "conversation_id", "turn_id", "chat_execution_id", "tool_catalog_id", "origin_id", "parent_invocation_id", "metadata", "input_preview", "output_preview", "display_name", "tool_call_id", "result_availability", "status") + " + " + publicationSQLBytes("selected_chat_turn", "response_message_id")
	publicationCatalogSizeSQL   = publicationSQLBytes("tool_catalog", "name", "display_name", "origin")
)

// Parte de turn_id para usar a busca indexada da seleção. Partir de cada
// pergunta permite ao SQLite escolher a conversa inteira como fonte interna.
// A validação de conversa/escopo continua depois da seleção por identidade.
const publicationMessageSourceSQL = `(SELECT id FROM chat_messages WHERE id IN (SELECT CAST(unhex(value) AS TEXT) FROM json_each(?))
 UNION SELECT m.id FROM chat_messages m CROSS JOIN chat_messages q ON q.id=m.turn_id
 WHERE m.turn_id IN (SELECT CAST(unhex(value) AS TEXT) FROM json_each(?))
 AND m.conversation_id=q.conversation_id AND m.parent_id IS q.parent_id
 AND m.role <> 'user') selected CROSS JOIN chat_messages ON chat_messages.id=selected.id`

var publicationReadBudgetSQL = "SELECT (SELECT COALESCE(SUM(1024 + " + publicationSQLBytes("chat_messages", "id", "conversation_id", "role", "parent_id", "created_at") + "),0) FROM (SELECT CAST(unhex(value) AS TEXT) AS scope_id FROM json_each(?)) selected_scope CROSS JOIN chat_messages ON chat_messages.id=selected_scope.scope_id WHERE conversation_id=?) + COALESCE(SUM(1024 + " + publicationMessageSizeSQL + "), 0) + (SELECT COALESCE(SUM(1024 + " + publicationTurnSizeSQL + "), 0) FROM chat_turns WHERE conversation_id=? AND user_message_id IN (SELECT CAST(unhex(value) AS TEXT) FROM json_each(?))) FROM " + publicationMessageSourceSQL + " WHERE chat_messages.conversation_id=? AND " + database.CurrentChatMessageSQL("chat_messages")
var publicationCallBudgetSQL = "COALESCE(SUM(1024 + " + publicationCallSizeSQL + " + " + publicationCatalogSizeSQL + "), 0)"

// Executado depois da autorização, na mesma transação da materialização.
// LENGTH(BLOB) mede bytes sem transportar payloads para o processo Go. O custo
// fixo por linha também limita coleções enormes de registros quase vazios.
func checkPublicationReadBudget(tx *gorm.DB, conversationID, turnID string) error {
	_, err := publicationSelectionReadSize(tx, conversationID, nil, []string{turnID})
	return err
}

func publicationSelectionReadSize(tx *gorm.DB, conversationID string, messageIDs, turnIDs []string) (int64, error) {
	var size int64
	if err := database.CheckTimelineIdentityBudget(messageIDs, turnIDs); err != nil {
		return 0, err
	}
	// Perguntas e respostas da mesma página frequentemente repetem o conjunto.
	// Serializar uma vez quando os IDs coincidem, preservando a seleção exata.
	turnsJSON := database.EncodeTimelineIDSet(turnIDs)
	messagesJSON, metadataJSON := turnsJSON, turnsJSON
	if !slices.Equal(messageIDs, turnIDs) {
		messagesJSON = database.EncodeTimelineIDSet(messageIDs)
		metadataJSON = database.EncodeTimelineIDSet(uniquePublicationIDs(append(append([]string(nil), messageIDs...), turnIDs...)))
	}
	err := tx.Raw(publicationReadBudgetSQL, turnsJSON, conversationID, conversationID, metadataJSON, messagesJSON, turnsJSON, conversationID).Row().Scan(&size)
	if err != nil {
		return 0, err
	}
	if size > canonicalSourceBytes {
		return 0, ErrTimelineSourceLimit
	}
	if len(turnIDs) == 0 {
		return size, nil
	}
	var callsSize int64
	userID, err := database.RequireUserID(tx.Statement.Context)
	if err != nil {
		return 0, err
	}
	selection := database.ChatInvocationSelection{}
	err = tx.Model(&database.ToolInvocation{}).Select(publicationCallBudgetSQL).
		Joins("LEFT JOIN tool_catalog ON tool_catalog.id = tool_invocations.tool_catalog_id").
		Joins(selection.Join("tool_invocations")).Where(selection.Predicate("tool_invocations")).
		// O leitor aceita vínculos legados sem conversation_id. Contamos todos
		// os registros do usuário/turno, incluindo os que o leitor filtrará depois.
		Where("tool_invocations.user_id = ? AND tool_invocations.origin_type = 'chat' AND tool_invocations.turn_id IN (SELECT CAST(unhex(value) AS TEXT) FROM json_each(?))", userID, turnsJSON).Row().Scan(&callsSize)
	if err != nil {
		return 0, err
	}
	if callsSize > canonicalSourceBytes-size {
		return 0, ErrTimelineSourceLimit
	}
	return size + callsSize, nil
}

// Identificadores são constantes locais, nunca parâmetros vindos da UI.
func publicationSQLBytes(table string, columns ...string) string {
	parts := make([]string, len(columns))
	for i, column := range columns {
		parts[i] = "COALESCE(LENGTH(CAST(" + table + "." + column + " AS BLOB)), 0)"
	}
	return strings.Join(parts, " + ")
}

// Limite conservador antes de copiar/serializar: contempla o pior escape JSON
// (seis bytes por byte de string) e encerra a travessia assim que esgota a cota.
// Só recebe os structs acíclicos da projeção; não percorre graphs arbitrários.
func publicationSourceWithinBudget(source *TimelineItem) bool {
	budget := projectionBudget{remaining: canonicalSourceBytes}
	return budget.item(source)
}

type projectionBudget struct {
	remaining    int
	exactStrings bool
}

// Só descreve os tipos internos visitados pelo orçamento; não guarda dados
// de conversa. Evita reconstruir StructField/Index a cada campo de cada item.
var projectionFieldSizes sync.Map // reflect.Type -> []int imutável

func projectionFieldNameSizes(kind reflect.Type) []int {
	if cached, ok := projectionFieldSizes.Load(kind); ok {
		return cached.([]int)
	}
	names := make([]int, kind.NumField())
	for i := range names {
		names[i] = len(kind.Field(i).Name)
	}
	actual, _ := projectionFieldSizes.LoadOrStore(kind, names)
	return actual.([]int)
}

func (b *projectionBudget) visit(v reflect.Value) bool {
	b.remaining -= 32
	if b.remaining < 0 {
		return false
	}
	switch v.Kind() {
	case reflect.String:
		if !b.stringValue(v.String()) {
			return false
		}
	case reflect.Pointer:
		if !v.IsNil() {
			return b.visit(v.Elem())
		}
	case reflect.Struct:
		names := projectionFieldNameSizes(v.Type())
		for i := 0; i < v.NumField(); i++ {
			b.remaining -= names[i]
			if !b.visit(v.Field(i)) {
				return false
			}
		}
	case reflect.Slice:
		for i := 0; i < v.Len(); i++ {
			if !b.visit(v.Index(i)) {
				return false
			}
		}
	}
	return b.remaining >= 0
}

func (b *projectionBudget) item(source *TimelineItem) bool {
	base := *source
	base.messages = nil
	if source.Representative != nil {
		message := publicationSourceMessage(*source.Representative)
		base.Representative = &message
	}
	if !b.visit(reflect.ValueOf(base)) {
		return false
	}
	for _, message := range source.messages {
		if !b.visit(reflect.ValueOf(publicationSourceMessage(message))) {
			return false
		}
	}
	return true
}

// Janela inteira, sob a barreira, antes de snapshotLiveTurn clonar os buffers.
// A estimativa é conservadora: inclui o DTO persistido mesmo quando o live
// vai substituí-las. Não percorre retained nem turnos fora da página.
func windowCompositionWithinBudget(c *projectionConversation, items []*TimelineItem) bool {
	budget := projectionBudget{remaining: canonicalSourceBytes, exactStrings: true}
	for _, item := range items {
		if !budget.visit(reflect.ValueOf(canonicalItem(item))) {
			return false
		}
		t := c.turns[item.TurnID]
		if item.Kind != "response" || t == nil || t.detached || (t.finished && !t.volatile) {
			continue
		}
		// Cada chamada pode virar um grupo próprio e cortar mais um segmento.
		// Conta identidades derivadas mesmo quando o texto persistido ainda não
		// permite posicionar a chamada, antes de clonar ou compor o live.
		maxSourceID := len(t.executionID)*2 + 128
		for _, message := range item.messages {
			maxSourceID = max(maxSourceID, len(message.ID))
		}
		for _, stream := range t.streams {
			maxSourceID = max(maxSourceID, len(stream.messageID))
		}
		for _, segment := range item.Segments {
			maxSourceID = max(maxSourceID, len(segment.SourceMessageID), len(segment.SegmentID))
		}
		for _, segment := range item.Segments {
			for _, call := range segment.Invocations {
				budget.remaining -= 1024
				if !budget.stringBytes(len(call.InvocationID)) || !budget.stringBytes(maxSourceID) || !budget.stringBytes(maxSourceID) {
					return false
				}
			}
		}
		for _, value := range []string{t.turnID, t.executionID, t.responseID, t.state, t.reason} {
			if !budget.stringValue(value) {
				return false
			}
		}
		for _, stream := range t.streams {
			if !budget.stringValue(stream.messageID) {
				return false
			}
			budget.remaining -= 1024
			if budget.remaining < 0 {
				return false
			}
			for _, part := range stream.parts {
				// IDs draft codificam executionID em base64 por segmento. A margem
				// de quatro cópias cobre os dois segmentos e suas identidades.
				for range 4 {
					if !budget.stringBytes(len(t.executionID)) {
						return false
					}
				}
				budget.remaining -= 1024
				if !budget.stringValue(part.text.String()) || !budget.stringValue(part.reasoning.String()) {
					return false
				}
			}
		}
	}
	return true
}

func (b *projectionBudget) stringBytes(size int) bool {
	if b.remaining < 0 || size > b.remaining/6 {
		return false
	}
	b.remaining -= size * 6
	return true
}

// Conta o escape de encoding/json sem construir uma cópia do texto. O modo
// conservador anterior continua usado para bases internas de publicação.
func (b *projectionBudget) stringValue(value string) bool {
	if !b.exactStrings {
		return b.stringBytes(len(value))
	}
	if len(value) > b.remaining {
		return false
	}
	for len(value) > 0 {
		r, size := utf8.DecodeRuneInString(value)
		cost := size
		switch {
		case r == '"' || r == '\\' || r == '\b' || r == '\f' || r == '\n' || r == '\r' || r == '\t':
			cost = 2
		case r < 0x20 || r == '<' || r == '>' || r == '&' || r == 0x2028 || r == 0x2029:
			cost = 6
		case r == utf8.RuneError && size == 1:
			cost = invalidUTF8JSONBytes
		}
		b.remaining -= cost
		if b.remaining < 0 {
			return false
		}
		value = value[size:]
	}
	return true
}

// Go 1.25 escapa a substituição; versões mais novas podem emitir U+FFFD em
// UTF-8. Medir uma constante uma vez mantém a conta alinhada ao encoder ativo.
var invalidUTF8JSONBytes = func() int {
	encoded, _ := json.Marshal(string([]byte{0xff}))
	return len(encoded) - 2
}()
