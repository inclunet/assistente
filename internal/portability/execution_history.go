package portability

import (
	"context"
	"fmt"
	"strings"
	"time"

	"assistente/internal/database"
	"gorm.io/gorm"
)

func hasExecutionHistory(file *ExportFile) bool {
	for _, c := range file.Resources.Conversations {
		if len(c.ChatTurns) > 0 || len(c.ChatExecutions) > 0 {
			return true
		}
		for _, m := range c.Messages {
			if m.ChatExecutionID != nil {
				return true
			}
		}
		for _, i := range c.ToolInvocations {
			if i.ChatExecutionID != nil {
				return true
			}
		}
	}
	return false
}

// All queries share the export transaction: content and selection are one snapshot.
func loadExecutionExports(ctx context.Context, tx *gorm.DB, userID string, ids []string) (map[string][]ChatTurnExport, map[string][]ChatExecutionExport, error) {
	turns := map[string][]ChatTurnExport{}
	executions := map[string][]ChatExecutionExport{}
	var ts []struct {
		ChatTurnExport
		ConversationID string
	}
	err := tx.WithContext(ctx).Table("chat_turns t").Select("t.*").Joins("JOIN conversations c ON c.id=t.conversation_id").Where("c.user_id=? AND c.id IN ?", userID, ids).Order("t.conversation_id,t.user_message_id").Scan(&ts).Error
	if err != nil {
		return nil, nil, err
	}
	for _, t := range ts {
		turns[t.ConversationID] = append(turns[t.ConversationID], t.ChatTurnExport)
	}
	var es []struct {
		ChatExecutionExport
		ConversationID string
	}
	err = tx.WithContext(ctx).Table("chat_turn_attempts a").Select("a.*,t.conversation_id").Joins("JOIN chat_turns t ON t.user_message_id=a.turn_id JOIN conversations c ON c.id=t.conversation_id").Where("c.user_id=? AND c.id IN ?", userID, ids).Order("t.conversation_id,a.started_at,a.execution_id").Scan(&es).Error
	for _, e := range es {
		executions[e.ConversationID] = append(executions[e.ConversationID], e.ChatExecutionExport)
	}
	return turns, executions, err
}

func sameExportID(a, b *string) bool { return a == nil && b == nil || a != nil && b != nil && *a == *b }

// Validate and topologically order explicit bases. Never infer ancestry from time.
func orderImportedExecutions(conv ConversationExport, messages []*database.ChatMessage) ([]ChatExecutionExport, error) {
	byMessage := map[string]*database.ChatMessage{}
	for _, m := range messages {
		byMessage[m.ID] = m
	}
	turns := map[string]ChatTurnExport{}
	for _, t := range conv.ChatTurns {
		q := byMessage[t.TurnID]
		if _, exists := turns[t.TurnID]; exists || q == nil || q.Role != "user" {
			return nil, fmt.Errorf("turno importado inválido: %q", t.TurnID)
		}
		turns[t.TurnID] = t
	}
	byExecution := map[string]ChatExecutionExport{}
	for _, e := range conv.ChatExecutions {
		_, duplicate := byExecution[e.ExecutionID]
		_, knownTurn := turns[e.TurnID]
		if strings.TrimSpace(e.ExecutionID) == "" || duplicate || !knownTurn {
			return nil, fmt.Errorf("execução importada inválida: %q", e.ExecutionID)
		}
		byExecution[e.ExecutionID] = e
	}
	validateMessage := func(id *string, turn string, execution *string) bool {
		if id == nil {
			return true
		}
		m, q := byMessage[*id], byMessage[turn]
		return m != nil && q != nil && m.Role == "assistant" && m.TurnID != nil && *m.TurnID == turn && sameExportID(m.ParentID, q.ParentID) && sameExportID(m.ChatExecutionID, execution)
	}
	for _, t := range conv.ChatTurns {
		if !validateMessage(t.ResponseMessageID, t.TurnID, t.CurrentExecutionID) {
			return nil, fmt.Errorf("resposta inválida do turno %q", t.TurnID)
		}
		if t.CurrentExecutionID != nil {
			e, ok := byExecution[*t.CurrentExecutionID]
			if !ok || e.TurnID != t.TurnID || !sameExportID(e.ResponseMessageID, t.ResponseMessageID) || e.State != t.State {
				return nil, fmt.Errorf("seleção inválida do turno %q", t.TurnID)
			}
		}
	}
	for _, m := range messages {
		if m.ChatExecutionID == nil {
			continue
		}
		e, ok := byExecution[*m.ChatExecutionID]
		t := turns[e.TurnID]
		if !ok || t.CurrentExecutionID == nil || !validateMessage(&m.ID, e.TurnID, m.ChatExecutionID) {
			return nil, fmt.Errorf("proveniência inválida da mensagem %q", m.ID)
		}
	}
	for _, i := range conv.ToolInvocations {
		if i.ChatExecutionID == nil {
			continue
		}
		e, ok := byExecution[*i.ChatExecutionID]
		if !ok || e.TurnID != i.TurnID || turns[e.TurnID].CurrentExecutionID == nil {
			return nil, fmt.Errorf("proveniência inválida da ferramenta %q", i.ID)
		}
	}
	ordered := make([]ChatExecutionExport, 0, len(byExecution))
	marks := map[string]int{}
	var visit func(string) error
	visit = func(id string) error {
		if marks[id] == 2 {
			return nil
		}
		if marks[id] == 1 {
			return fmt.Errorf("ciclo entre execuções importadas")
		}
		e, ok := byExecution[id]
		if !ok {
			return fmt.Errorf("execução-base ausente: %q", id)
		}
		marks[id] = 1
		if !validateMessage(e.ResponseMessageID, e.TurnID, &e.ExecutionID) || !validateMessage(e.BaseResponseMessageID, e.TurnID, e.BaseExecutionID) {
			return fmt.Errorf("resposta/base inválida da execução %q", id)
		}
		if e.BaseExecutionID != nil {
			base, exists := byExecution[*e.BaseExecutionID]
			if !exists || base.TurnID != e.TurnID || base.State == database.TurnPreparing || base.State == database.TurnStreaming || (e.BaseResponseMessageID != nil && !sameExportID(e.BaseResponseMessageID, base.ResponseMessageID)) {
				return fmt.Errorf("base incompatível da execução %q", id)
			}
			if err := visit(*e.BaseExecutionID); err != nil {
				return err
			}
		}
		marks[id] = 2
		ordered = append(ordered, e)
		return nil
	}
	for _, e := range conv.ChatExecutions {
		if err := visit(e.ExecutionID); err != nil {
			return nil, err
		}
	}
	return ordered, nil
}

func restoreConversationExecutionHistory(ctx context.Context, tx *gorm.DB, conversationID string, conv ConversationExport, messages []*database.ChatMessage) error {
	ordered, err := orderImportedExecutions(conv, messages)
	if err != nil {
		return err
	}
	messageGroups := map[string][]*database.ChatMessage{}
	invocationGroups := map[string][]ToolInvocationExport{}
	for _, m := range messages {
		if m.ChatExecutionID == nil {
			if err := tx.Create(m).Error; err != nil {
				return err
			}
		} else {
			messageGroups[*m.ChatExecutionID] = append(messageGroups[*m.ChatExecutionID], m)
		}
	}
	for _, i := range conv.ToolInvocations {
		key := ""
		if i.ChatExecutionID != nil {
			key = *i.ChatExecutionID
		}
		invocationGroups[key] = append(invocationGroups[key], i)
	}
	if err := importConversationToolInvocations(ctx, tx, conversationID, invocationGroups[""]); err != nil {
		return err
	}
	if err := database.NewChatTurnRepository(tx).ReconcileImportedConversationInTransaction(ctx, conversationID); err != nil {
		return err
	}
	// Temporary admission only inside this transaction satisfies the same constraints
	// as live writers. No event, lease or worker is created by importing history.
	for _, e := range ordered {
		if err := tx.Exec(`INSERT INTO chat_turn_attempts(execution_id,turn_id,mode,state,started_at,base_execution_id,base_response_message_id) VALUES (?,?,?,'preparing',?,?,?)`, e.ExecutionID, e.TurnID, e.Mode, e.StartedAt, e.BaseExecutionID, e.BaseResponseMessageID).Error; err != nil {
			return err
		}
		hasContent := len(messageGroups[e.ExecutionID]) > 0 || len(invocationGroups[e.ExecutionID]) > 0
		if hasContent {
			if err := tx.Exec(`UPDATE chat_turns SET current_execution_id=?,active_execution_id=?,response_message_id=NULL,state='preparing' WHERE user_message_id=?`, e.ExecutionID, e.ExecutionID, e.TurnID).Error; err != nil {
				return err
			}
			for _, m := range messageGroups[e.ExecutionID] {
				if err := tx.Table("chat_messages").Create(&struct {
					*database.ChatMessage
					ChatExecutionID *string
				}{m, m.ChatExecutionID}).Error; err != nil {
					return err
				}
			}
			if err := importConversationToolInvocations(ctx, tx, conversationID, invocationGroups[e.ExecutionID]); err != nil {
				return err
			}
		}
		state, reason, finished := e.State, e.TerminalReason, e.FinishedAt
		if state == database.TurnPreparing || state == database.TurnStreaming {
			now := time.Now().UTC()
			state, reason, finished = database.TurnInterrupted, "restart", &now
		}
		if err := tx.Exec(`UPDATE chat_turn_attempts SET response_message_id=?,state=?,terminal_reason=?,finished_at=? WHERE execution_id=?`, e.ResponseMessageID, state, reason, finished, e.ExecutionID).Error; err != nil {
			return err
		}
		if hasContent {
			if err := tx.Exec(`UPDATE chat_turns SET active_execution_id=NULL,state=? WHERE user_message_id=?`, state, e.TurnID).Error; err != nil {
				return err
			}
		}
	}
	for _, t := range conv.ChatTurns {
		state := t.State
		if state == database.TurnPreparing || state == database.TurnStreaming {
			state = database.TurnInterrupted
		}
		if err := tx.Exec(`UPDATE chat_turns SET current_execution_id=?,response_message_id=?,state=?,active_execution_id=NULL,origin='imported' WHERE user_message_id=? AND conversation_id=?`, t.CurrentExecutionID, t.ResponseMessageID, state, t.TurnID, conversationID).Error; err != nil {
			return err
		}
	}
	return nil
}
