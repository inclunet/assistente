package chat

import (
	"context"
	"errors"
	"strings"

	"assistente/internal/database"
	"assistente/internal/toolinvocations"
	"gorm.io/gorm"
)

// Snapshot persistido: a versão identifica o commit lido, não epoch/sequence
// de streaming. Items contém a projeção canônica; Window descreve a paginação.
type TimelineWindowSnapshot struct {
	// Provas anteriores à troca da janela, calculadas sob a mesma barreira.
	priorTurnValidity     map[string]bool
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
	ReadRecoveryTurn(context.Context, string, string) (*TimelineTurnSnapshot, error)
}

type TimelineSnapshotReader interface {
	TurnSnapshotReader
	PublicationSnapshotReader
}

// A publicação automática exige leitura limitada antes de materializar payloads.
type PublicationSnapshotReader interface {
	ReadVersion(context.Context, string) (database.ChatTimelineVersion, error)
	TimelineLifecycleReader
	ReadPublicationTurn(context.Context, string, string) (*TimelineTurnSnapshot, error)
	TimelineSelectionReader
}

type DBTimelineReader struct{ db *gorm.DB }

func NewDBTimelineReader(db *gorm.DB) *DBTimelineReader { return &DBTimelineReader{db: db} }

type timelineFacts struct {
	turns map[string]*database.ChatTurn
	calls map[string][]TurnSegmentToolCall
}

func readTimelineFacts(ctx context.Context, tx *gorm.DB, conversationID string, turnIDs []string) (timelineFacts, error) {
	return readTimelineFactsForSelection(ctx, tx, conversationID, turnIDs, turnIDs)
}

// Lifecycle acompanha perguntas visíveis; payloads de ferramentas só respostas selecionadas.
func readTimelineFactsForSelection(ctx context.Context, tx *gorm.DB, conversationID string, turnIDs, responseIDs []string) (timelineFacts, error) {
	turns, err := database.NewChatTurnRepository(tx).GetManyWithPresentation(ctx, conversationID, turnIDs)
	if err != nil {
		return timelineFacts{}, err
	}
	summaries, err := toolinvocations.NewDBRepository(tx).LoadSummariesForTurnIDs(ctx, responseIDs)
	if err != nil {
		return timelineFacts{}, err
	}
	return timelineFacts{turns: turns, calls: ToolInvocationSummariesToTurnSegments(summaries)}, nil
}

// ReadCanonicalWindow lê uma única projeção leve para toda apresentação.
func (r *DBTimelineReader) ReadCanonicalWindow(ctx context.Context, query database.MessageWindowQuery) (*TimelineWindowSnapshot, error) {
	result := &TimelineWindowSnapshot{}
	var rows *database.MessageWindowResult
	var facts timelineFacts
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Esta primeira leitura autoriza e fixa o snapshot SQLite DEFERRED. Nenhuma
		// consulta abaixo pode voltar ao handle global ou a outra conexão.
		version, err := database.ReadChatTimelineVersion(ctx, tx, query.ConversationID)
		if err != nil {
			return err
		}
		result.Version = version
		repo := database.NewMessageRepository(tx)

		repo = repo.WithTimelineTurns()

		if err := validateTimelineParent(ctx, repo, query); err != nil {
			return err
		}

		rows, err = repo.ReadMessageWindowWithContext(ctx, query, func(items []database.MessageWindowItem) error {
			var messageIDs []string
			for _, item := range items {
				if item.Kind == database.MessageWindowItemKindTurn {
					result.TurnIDs = append(result.TurnIDs, item.TurnID)
				} else {
					messageIDs = append(messageIDs, item.MessageID)
				}
			}
			_, err := publicationSelectionReadSize(tx, query.ConversationID, messageIDs, result.TurnIDs)
			return err
		})

		if err != nil {
			return err
		}

		responseIDs := result.TurnIDs

		seen := make(map[string]bool, len(result.TurnIDs))
		for _, id := range result.TurnIDs {
			seen[id] = true
		}

		for _, message := range rows.Messages {
			if message.Role == "user" && !seen[message.ID] {
				result.TurnIDs = append(result.TurnIDs, message.ID)
				seen[message.ID] = true
			}
		}

		for _, row := range rows.Items {
			if row.Kind == database.MessageWindowItemKindTurn && !seen[row.TurnID] {
				result.TurnIDs = append(result.TurnIDs, row.TurnID)
				seen[row.TurnID] = true
			}
		}

		facts, err = readTimelineFactsForSelection(ctx, tx, query.ConversationID, result.TurnIDs, responseIDs)
		if err != nil {
			return err
		}
		// Lotes limitam parâmetros mesmo quando um único item agrega muitos segmentos.
		// Metadados e contagens de filhos pertencem à mesma leitura canônica.
		if err := database.ReadMessagePresentation(ctx, tx, rows.Messages); err != nil {
			return err
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
	if len(rows.Items) > 0 {
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
		TotalCount:     rows.TotalCount, StartIndex: rows.StartIndex, EndIndex: rows.EndIndex, HasBefore: rows.HasBefore, HasAfter: rows.HasAfter,
	}
	return result, nil
}

func (r *DBTimelineReader) ReadPublicationTurn(ctx context.Context, conversationID, turnID string) (*TimelineTurnSnapshot, error) {
	return r.readTurn(ctx, conversationID, turnID, turnReadPublication)
}

// ReadRecoveryTurn autoriza a conversa e lê o turno opcional na mesma transação.
// Remover a pergunta não revoga acesso aos buffers desvinculados dessa conversa.
func (r *DBTimelineReader) ReadRecoveryTurn(ctx context.Context, conversationID, turnID string) (*TimelineTurnSnapshot, error) {
	return r.readTurn(ctx, conversationID, turnID, turnReadRecovery)
}

type turnReadMode uint8

const (
	turnReadPublication turnReadMode = iota
	turnReadRecovery
)

func (r *DBTimelineReader) readTurn(ctx context.Context, conversationID, turnID string, mode turnReadMode) (*TimelineTurnSnapshot, error) {
	publication := mode == turnReadPublication
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
		if publication {
			if err := checkPublicationReadBudget(tx, conversationID, turnID); err != nil {
				return err
			}
		}
		question := &database.ChatMessage{}
		err = tx.Select("id", "conversation_id", "role", "parent_id").Where("id = ? AND conversation_id = ?", turnID, conversationID).First(question).Error
		if err != nil {
			if mode == turnReadRecovery && errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		if mode == turnReadRecovery && question.Role != "user" {
			return nil
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
		// Recuperação e publicação projetam texto/metadados, nunca blobs.
		// Contexto do modelo e playback resolvem mídia em seus próprios leitores.
		responseDB := tx.Omit("media", "audio", "audio_mime_type").Where("chat_messages.role <> ?", "user").Session(&gorm.Session{})
		messages, err = database.NewMessageRepository(responseDB).GetMessagesByTurnIDWithContext(ctx, conversationID, question.ParentID, turnID, 0)
		if err != nil {
			return err
		}
		facts, err = readTimelineFacts(ctx, tx, conversationID, []string{turnID})
		if err == nil {
			err = database.ReadMessagePresentation(ctx, tx, messages)
		}
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
