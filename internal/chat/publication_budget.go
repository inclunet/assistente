package chat

import (
	"reflect"
	"strings"

	"assistente/internal/database"
	"gorm.io/gorm"
)

// Executado depois da autorização, na mesma transação da materialização.
// LENGTH(BLOB) mede bytes sem transportar payloads para o processo Go. O custo
// fixo por linha também limita coleções enormes de registros quase vazios.
func checkPublicationReadBudget(tx *gorm.DB, conversationID, turnID string) error {
	messageSize := publicationSQLBytes("chat_messages", "content", "reasoning", "model", "source")
	var size int64
	err := tx.Model(&database.ChatMessage{}).Select("COALESCE(SUM(1024 + "+messageSize+"), 0)").
		Where("conversation_id = ? AND turn_id = ? AND role <> ?", conversationID, turnID, "user").
		Where(database.CurrentChatMessageSQL("chat_messages")).Scan(&size).Error
	if err != nil {
		return err
	}
	if size > canonicalSourceBytes {
		return ErrTimelineSourceLimit
	}
	callSize := publicationSQLBytes("tool_invocations", "metadata", "input_preview", "output_preview", "display_name", "tool_call_id", "result_availability", "status")
	catalogSize := publicationSQLBytes("tool_catalog", "name", "display_name", "origin")
	var callsSize int64
	userID, err := database.RequireUserID(tx.Statement.Context)
	if err != nil {
		return err
	}
	selection := database.ChatInvocationSelection{}
	err = tx.Model(&database.ToolInvocation{}).Select("COALESCE(SUM(1024 + "+callSize+" + "+catalogSize+"), 0)").
		Joins("LEFT JOIN tool_catalog ON tool_catalog.id = tool_invocations.tool_catalog_id").
		Joins(selection.Join("tool_invocations")).Where(selection.Predicate("tool_invocations")).
		// O leitor aceita vínculos legados sem conversation_id. Contamos todos
		// os registros do usuário/turno, incluindo os que o leitor filtrará depois.
		Where("tool_invocations.user_id = ? AND tool_invocations.turn_id = ?", userID, turnID).Scan(&callsSize).Error
	if err != nil {
		return err
	}
	if callsSize > canonicalSourceBytes-size {
		return ErrTimelineSourceLimit
	}
	return nil
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
	remaining := canonicalSourceBytes
	var visit func(reflect.Value) bool
	visit = func(v reflect.Value) bool {
		remaining -= 32
		if remaining < 0 {
			return false
		}
		switch v.Kind() {
		case reflect.String:
			if v.Len() > remaining/6 {
				return false
			}
			remaining -= v.Len() * 6
		case reflect.Pointer:
			if !v.IsNil() {
				return visit(v.Elem())
			}
		case reflect.Struct:
			for i := 0; i < v.NumField(); i++ {
				remaining -= len(v.Type().Field(i).Name)
				if !visit(v.Field(i)) {
					return false
				}
			}
		case reflect.Slice:
			for i := 0; i < v.Len(); i++ {
				if !visit(v.Index(i)) {
					return false
				}
			}
		}
		return remaining >= 0
	}
	base := *source
	base.messages, base.legacyRepresentative = nil, nil
	if source.Representative != nil {
		message := publicationSourceMessage(*source.Representative)
		base.Representative = &message
	}
	if !visit(reflect.ValueOf(base)) {
		return false
	}
	for _, message := range source.messages {
		if !visit(reflect.ValueOf(publicationSourceMessage(message))) {
			return false
		}
	}
	return true
}
