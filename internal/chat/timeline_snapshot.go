package chat

import (
	"context"
	"fmt"

	"assistente/internal/database"
	"assistente/internal/toolinvocations"
	"gorm.io/gorm"
)

// Snapshot persistido: a versão identifica o commit lido, não epoch/sequence
// de streaming. Metadados nullable aguardam o DTO canônico; Nodes/Window ainda
// são os adaptadores visuais existentes.
type TimelineWindowSnapshot struct {
	Version database.ChatTimelineVersion
	Turns   map[string]*database.ChatTurn
	Window  MessageWindow
}

type TimelineTurnSnapshot struct {
	Version database.ChatTimelineVersion
	Turn    *database.ChatTurn
	Nodes   []MessageNode
}

// TurnSnapshotReader é a dependência de leitura do terminal, separada dos writers.
type TurnSnapshotReader interface {
	ReadTurn(context.Context, string, string) (*TimelineTurnSnapshot, error)
}

type DBTimelineReader struct{ db *gorm.DB }

func NewDBTimelineReader(db *gorm.DB) *DBTimelineReader { return &DBTimelineReader{db: db} }

type timelineFacts struct {
	turns map[string]*database.ChatTurn
	calls map[string][]TurnSegmentToolCall
}

func readTimelineFacts(ctx context.Context, tx *gorm.DB, conversationID string, turnIDs []string) (timelineFacts, error) {
	turns, err := database.NewChatTurnRepository(tx).GetMany(ctx, conversationID, turnIDs)
	if err != nil {
		return timelineFacts{}, err
	}
	summaries, err := toolinvocations.NewDBRepository(tx).LoadSummariesForTurnIDs(ctx, turnIDs)
	if err != nil {
		return timelineFacts{}, err
	}
	return timelineFacts{turns: turns, calls: ToolInvocationSummariesToTurnSegments(summaries)}, nil
}

func (r *DBTimelineReader) ReadWindow(ctx context.Context, query database.MessageWindowQuery) (*TimelineWindowSnapshot, error) {
	result := &TimelineWindowSnapshot{}
	var rows *database.MessageWindowResult
	var facts timelineFacts
	childCounts := make(map[string]int)
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Esta primeira leitura autoriza e fixa o snapshot SQLite DEFERRED. Nenhuma
		// consulta abaixo pode voltar ao handle global ou a outra conexão.
		version, err := database.ReadChatTimelineVersion(ctx, tx, query.ConversationID)
		if err != nil {
			return err
		}
		result.Version = version
		repo := database.NewMessageRepository(tx)
		if query.ParentID != nil {
			parent, err := repo.GetMessageWithContext(ctx, *query.ParentID)
			if err != nil {
				return fmt.Errorf("threadParentId inválido: %w", err)
			}
			if parent.ConversationID != query.ConversationID {
				return fmt.Errorf("threadParentId não pertence à conversa solicitada")
			}
			if parent.ParentID != nil {
				return fmt.Errorf("threadParentId deve apontar para uma mensagem raiz")
			}
		}
		rows, err = repo.GetMessageWindowWithContext(ctx, query)
		if err != nil {
			return err
		}
		facts, err = readTimelineFacts(ctx, tx, query.ConversationID, snapshotTurnIDs(rows.Messages))
		if err != nil {
			return err
		}
		// Lotes limitam parâmetros mesmo quando um único item agrega muitos segmentos.
		const batchSize = 400
		for start := 0; start < len(rows.Messages); start += batchSize {
			end := min(start+batchSize, len(rows.Messages))
			ids := make([]string, 0, end-start)
			for _, message := range rows.Messages[start:end] {
				ids = append(ids, message.ID)
			}
			counts, err := repo.CountChildrenWithContext(ctx, ids)
			if err != nil {
				return err
			}
			for id, count := range counts {
				childCounts[id] = count
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	// Consolidação fora da transação: nenhuma consulta nem escrita é necessária.
	result.Turns = facts.turns
	result.Window = MessageWindow{
		ConversationID: query.ConversationID,
		Nodes:          BuildTimelineMessageNodes(rows.Items, rows.Messages, query.ParentID, childCounts, facts.calls),
		TotalCount:     rows.TotalCount, StartIndex: rows.StartIndex, EndIndex: rows.EndIndex, HasBefore: rows.HasBefore, HasAfter: rows.HasAfter,
	}
	return result, nil
}

func (r *DBTimelineReader) ReadTurn(ctx context.Context, conversationID, turnID string) (*TimelineTurnSnapshot, error) {
	result := &TimelineTurnSnapshot{}
	var messages []Message
	var parentID *string
	var facts timelineFacts
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		version, err := database.ReadChatTimelineVersion(ctx, tx, conversationID)
		if err != nil {
			return err
		}
		result.Version = version
		repo := database.NewMessageRepository(tx)
		question, err := repo.GetMessageWithContext(ctx, turnID)
		if err != nil {
			return err
		}
		if question.ConversationID != conversationID || question.Role != "user" {
			return database.ErrChatTurnInvalid
		}
		parentID = question.ParentID
		// Mesma completude do item da janela: nunca cortar silenciosamente em 1.000.
		// A consulta é restrita ao turno/escopo, não recarrega a conversa inteira.
		messages, err = repo.GetMessagesByTurnIDWithContext(ctx, conversationID, question.ParentID, turnID, 0)
		if err != nil {
			return err
		}
		facts, err = readTimelineFacts(ctx, tx, conversationID, []string{turnID})
		return err
	})
	if err != nil {
		return nil, err
	}
	result.Turn = facts.turns[turnID]
	result.Nodes = BuildNodesWithTimelineConsolidation(messages, parentID, nil, facts.calls)
	return result, nil
}

func snapshotTurnIDs(messages []Message) []string {
	ids := CollectTurnIDsWithToolCalls(messages)
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		seen[id] = true
	}
	for _, message := range messages {
		if message.Role == "user" && !seen[message.ID] {
			ids = append(ids, message.ID)
			seen[message.ID] = true
		}
	}
	return ids
}
