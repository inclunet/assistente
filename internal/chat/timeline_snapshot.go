package chat

import (
	"context"
	"fmt"
	"strings"

	"assistente/internal/database"
	"assistente/internal/toolinvocations"
	"gorm.io/gorm"
)

// Snapshot persistido: a versão identifica o commit lido, não epoch/sequence
// de streaming. Metadados nullable aguardam o DTO canônico; Nodes/Window ainda
// são os adaptadores visuais existentes.
type TimelineWindowSnapshot struct {
	BeforeCursor          string
	AfterCursor           string
	Items                 []*TimelineItem
	InvalidTurnReferences int
	TurnIDs               []string
	Version               database.ChatTimelineVersion
	Turns                 map[string]*database.ChatTurn
	Responses             map[string]*TimelineItem
	Window                MessageWindow
}

type TimelineTurnSnapshot struct {
	Version  database.ChatTimelineVersion
	Turn     *database.ChatTurn
	Response *TimelineItem
}

// TurnSnapshotReader é a dependência de leitura do terminal, separada dos writers.
type TurnSnapshotReader interface {
	ReadVersion(context.Context, string) (database.ChatTimelineVersion, error)
	ReadTurn(context.Context, string, string) (*TimelineTurnSnapshot, error)
}

// WindowSnapshotReader mantém consulta e autorização na mesma transação.
type WindowSnapshotReader interface {
	ReadWindow(context.Context, database.MessageWindowQuery) (*TimelineWindowSnapshot, error)
}
type TimelineSnapshotReader interface {
	TurnSnapshotReader
	WindowSnapshotReader
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
	return r.readWindow(ctx, query, false)
}

// ReadCanonicalWindow inclui itens com identidade própria ainda sem mensagem.
// O adaptador legado continua usando ReadWindow até o cutover do consumidor.
func (r *DBTimelineReader) ReadCanonicalWindow(ctx context.Context, query database.MessageWindowQuery) (*TimelineWindowSnapshot, error) {
	return r.readWindow(ctx, query, true)
}

func (r *DBTimelineReader) readWindow(ctx context.Context, query database.MessageWindowQuery, canonical bool) (*TimelineWindowSnapshot, error) {
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
		if canonical {
			repo = repo.WithTimelineTurns()
		}
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
		if err := database.ResolveContinuationMedia(ctx, tx, rows.Messages); err != nil {
			return err
		}
		result.TurnIDs = snapshotTurnIDs(rows.Messages)
		seen := make(map[string]bool, len(result.TurnIDs))
		for _, id := range result.TurnIDs {
			seen[id] = true
		}
		for _, row := range rows.Items {
			if row.Kind == database.MessageWindowItemKindTurn && !seen[row.TurnID] {
				result.TurnIDs = append(result.TurnIDs, row.TurnID)
				seen[row.TurnID] = true
			}
		}
		facts, err = readTimelineFacts(ctx, tx, query.ConversationID, result.TurnIDs)
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
	result.Responses, err = projectSnapshotResponses(query.ConversationID, query.ParentID, rows.Messages, facts)
	if err != nil {
		return nil, err
	}
	for _, row := range rows.Items {
		if row.Kind == database.MessageWindowItemKindTurn && result.Responses[row.TurnID] == nil {
			result.Responses[row.TurnID], err = ProjectPersistedResponse(query.ConversationID, row.TurnID, query.ParentID, nil, facts.calls[row.TurnID], facts.turns[row.TurnID])
			if err != nil {
				return nil, err
			}
		}
	}
	if canonical && len(rows.Items) > 0 {
		result.BeforeCursor = database.EncodeTimelineCursor(query.ConversationID, query.ParentID, rows.Items[0])
		result.AfterCursor = database.EncodeTimelineCursor(query.ConversationID, query.ParentID, rows.Items[len(rows.Items)-1])
	}
	result.InvalidTurnReferences = rows.InvalidTurnReferences
	result.Turns = facts.turns
	result.Items, err = projectWindowItems(query.ConversationID, query.ParentID, rows.Items, rows.Messages, result.Responses)
	if err != nil {
		return nil, err
	}
	result.Window = MessageWindow{
		ConversationID: query.ConversationID,
		Nodes:          legacyItemNodes(result.Items, rows.Items, childCounts),
		TotalCount:     rows.TotalCount, StartIndex: rows.StartIndex, EndIndex: rows.EndIndex, HasBefore: rows.HasBefore, HasAfter: rows.HasAfter,
	}
	return result, nil
}

func (r *DBTimelineReader) ReadTurn(ctx context.Context, conversationID, turnID string) (*TimelineTurnSnapshot, error) {
	result := &TimelineTurnSnapshot{}
	var messages []Message
	var parentID *string
	var anchor database.MessageWindowItem
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
		anchor = database.MessageWindowItem{Kind: database.MessageWindowItemKindTurn, ID: turnID, TurnID: turnID, AnchorID: turnID}
		if err := tx.Raw("SELECT CAST(created_at AS TEXT) FROM chat_messages WHERE id = ?", question.ID).Scan(&anchor.AnchorCreatedAt).Error; err != nil {
			return err
		}
		// Mesma completude do item da janela: nunca cortar silenciosamente em 1.000.
		// A consulta é restrita ao turno/escopo, não recarrega a conversa inteira.
		messages, err = repo.GetMessagesByTurnIDWithContext(ctx, conversationID, question.ParentID, turnID, 0)
		if err != nil {
			return err
		}
		if err := database.ResolveContinuationMedia(ctx, tx, messages); err != nil {
			return err
		}
		facts, err = readTimelineFacts(ctx, tx, conversationID, []string{turnID})
		return err
	})
	if err != nil {
		return nil, err
	}
	responses, err := projectSnapshotResponses(conversationID, parentID, messages, facts)
	if err != nil {
		return nil, err
	}
	result.Turn = facts.turns[turnID]
	result.Response = responses[turnID]
	if result.Response == nil && result.Turn != nil {
		result.Response, err = ProjectPersistedResponse(conversationID, turnID, parentID, nil, facts.calls[turnID], result.Turn)
		if err != nil {
			return nil, err
		}
	}
	if result.Response != nil {
		result.Response.Position = itemPosition(anchor)
	}
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

func projectSnapshotResponses(conversationID string, parentID *string, messages []Message, facts timelineFacts) (map[string]*TimelineItem, error) {
	grouped := make(map[string][]Message)
	for _, message := range messages {
		if message.Role != "user" && message.TurnID != nil && strings.TrimSpace(*message.TurnID) != "" {
			grouped[*message.TurnID] = append(grouped[*message.TurnID], message)
		}
	}
	responses := make(map[string]*TimelineItem, len(grouped))
	for turnID, rows := range grouped {
		response, err := ProjectPersistedResponse(conversationID, turnID, parentID, rows, facts.calls[turnID], facts.turns[turnID])
		if err != nil {
			return nil, err
		}
		responses[turnID] = response
	}
	return responses, nil
}

// ReadVersion autoriza e identifica a encarnação antes de admitir uma execução.
// Leituras de conteúdo continuam obtendo versão e dados na mesma transação.
func (r *DBTimelineReader) ReadVersion(ctx context.Context, conversationID string) (database.ChatTimelineVersion, error) {
	return database.ReadChatTimelineVersion(ctx, r.db, conversationID)
}
