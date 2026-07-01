package database

import (
	"assistente/internal/logging"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
)

// MessageRepository encapsula a persistencia de mensagens de chat com um *gorm.DB injetado.
type MessageRepository struct {
	db *gorm.DB
}

// NewMessageRepository cria um MessageRepository com o *gorm.DB injetado.
func NewMessageRepository(database *gorm.DB) *MessageRepository {
	return &MessageRepository{db: database}
}

// ==================== ChatMessage ====================

// MessageOptions contém opções para criar uma mensagem
type MessageOptions struct {
	ConversationID   string
	ParentID         *string // ID da mensagem pai (define hierarquia)
	TurnID           *string // Agrupa mensagens de um turno (aponta para user message)
	Role             string  // user, assistant, tool, system
	Content          string
	Reasoning        string // Reasoning/thinking do modelo
	Media            string // JSON com mídias
	Audio            string // Áudio em base64 (recebido ou TTS)
	AudioMimeType    string // MIME do áudio
	ToolCalls        string // JSON: [{"id":"call_x","type":"function","function":{...}}]
	ToolCallID       string // Para role="tool": ID da chamada que este resultado responde
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	CacheReadTokens  int
	CacheWriteTokens int
	CacheMissTokens  int
	Model            string
	Source           string // Origem da mensagem: "wails", "telegram", "signal", etc.
}

func scopedMessageQuery(ctx context.Context, base *gorm.DB) *gorm.DB {
	return ScopeByUser(ctx,
		base.WithContext(ctx).Joins("JOIN conversations ON conversations.id = chat_messages.conversation_id"),
		"conversations.user_id",
	)
}

// CreateMessageWithContext cria uma mensagem em uma conversa do usuário do
// contexto. Falha fechado com ErrUserScopeRequired sem userID — uma mensagem
// sem dono é cross-user leak garantido (a query de busca de conversa pai
// passa fail-open via ScopeByUser e qualquer conversa por ID seria
// candidata).
func CreateMessageWithContext(ctx context.Context, opts MessageOptions) (*ChatMessage, error) {
	return NewMessageRepository(db).CreateMessageWithContext(ctx, opts)
}

func (r *MessageRepository) CreateMessageWithContext(ctx context.Context, opts MessageOptions) (*ChatMessage, error) {
	db := r.db
	if _, err := RequireUserID(ctx); err != nil {
		return nil, err
	}
	var conv Conversation
	if err := ScopeByUser(ctx, db.WithContext(ctx), "user_id").First(&conv, "id = ?", opts.ConversationID).Error; err != nil {
		return nil, fmt.Errorf("%w: conversa %s", ErrConversationDeleted, opts.ConversationID)
	}

	// Verifica se a mensagem pai existe (se parentId foi fornecido)
	if opts.ParentID != nil && *opts.ParentID != "" {
		var parentMsg ChatMessage
		if err := scopedMessageQuery(ctx, db.Model(&ChatMessage{})).First(&parentMsg, "chat_messages.id = ?", *opts.ParentID).Error; err != nil {
			return nil, fmt.Errorf("%w: mensagem %s", ErrParentMessageDeleted, *opts.ParentID)
		}
		if parentMsg.ConversationID != opts.ConversationID {
			return nil, fmt.Errorf("%w: mensagem %s", ErrParentMessageDeleted, *opts.ParentID)
		}
	}

	msg := &ChatMessage{
		ConversationID:   opts.ConversationID,
		ParentID:         opts.ParentID,
		TurnID:           opts.TurnID,
		Role:             opts.Role,
		Content:          opts.Content,
		Reasoning:        opts.Reasoning,
		Media:            opts.Media,
		Audio:            opts.Audio,
		AudioMimeType:    opts.AudioMimeType,
		ToolCalls:        opts.ToolCalls,
		ToolCallID:       opts.ToolCallID,
		PromptTokens:     opts.PromptTokens,
		CompletionTokens: opts.CompletionTokens,
		TotalTokens:      opts.TotalTokens,
		CacheReadTokens:  opts.CacheReadTokens,
		CacheWriteTokens: opts.CacheWriteTokens,
		CacheMissTokens:  opts.CacheMissTokens,
		Model:            opts.Model,
		Source:           opts.Source,
	}
	if err := db.WithContext(ctx).Create(msg).Error; err != nil {
		return nil, err
	}
	ScopeByUser(ctx, db.WithContext(ctx).Model(&Conversation{}), "user_id").
		Where("id = ?", opts.ConversationID).
		Update("updated_at", time.Now())
	return msg, nil
}

// AddMessageWithContext adiciona uma mensagem simples (sem parent - nível 0)
// para o usuário do contexto.
func AddMessageWithContext(ctx context.Context, conversationID string, role, content string) (*ChatMessage, error) {
	return CreateMessageWithContext(ctx, MessageOptions{
		ConversationID: conversationID,
		Role:           role,
		Content:        content,
	})
}

// AddMessageWithMediaWithContext adiciona uma mensagem com mídias (sem parent
// - nível 0) para o usuário do contexto.
func AddMessageWithMediaWithContext(ctx context.Context, conversationID string, role, content, media string) (*ChatMessage, error) {
	return CreateMessageWithContext(ctx, MessageOptions{
		ConversationID: conversationID,
		Role:           role,
		Content:        content,
		Media:          media,
	})
}

// AddMessageWithTokensWithContext adiciona uma mensagem com informações de
// tokens para o usuário do contexto.
func AddMessageWithTokensWithContext(ctx context.Context, conversationID string, role, content string, promptTokens, completionTokens, totalTokens int, model string) (*ChatMessage, error) {
	return CreateMessageWithContext(ctx, MessageOptions{
		ConversationID:   conversationID,
		Role:             role,
		Content:          content,
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		TotalTokens:      totalTokens,
		Model:            model,
	})
}

// AddMessageWithTokensAndMediaWithContext adiciona uma mensagem com mídias e
// informações de tokens para o usuário do contexto.
func AddMessageWithTokensAndMediaWithContext(ctx context.Context, conversationID string, role, content, media string, promptTokens, completionTokens, totalTokens int, model string) (*ChatMessage, error) {
	return CreateMessageWithContext(ctx, MessageOptions{
		ConversationID:   conversationID,
		Role:             role,
		Content:          content,
		Media:            media,
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		TotalTokens:      totalTokens,
		Model:            model,
	})
}

// GetMessageAudioWithContext retorna o áudio base64 e MIME de uma mensagem
// pertencente ao usuário do contexto. Retorna ("", "", nil) se a mensagem não
// tem áudio.
func GetMessageAudioWithContext(ctx context.Context, messageID string) (string, string, error) {
	return NewMessageRepository(db).GetMessageAudioWithContext(ctx, messageID)
}

func (r *MessageRepository) GetMessageAudioWithContext(ctx context.Context, messageID string) (string, string, error) {
	db := r.db
	if _, err := RequireUserID(ctx); err != nil {
		return "", "", err
	}
	var msg ChatMessage
	if err := scopedMessageQuery(ctx, db.Model(&ChatMessage{})).
		Select("chat_messages.audio", "chat_messages.audio_mime_type").
		First(&msg, "chat_messages.id = ?", messageID).Error; err != nil {
		return "", "", err
	}
	return msg.Audio, msg.AudioMimeType, nil
}

// SaveMessageAudioWithContext salva áudio (base64) numa mensagem existente do
// usuário do contexto.
func SaveMessageAudioWithContext(ctx context.Context, messageID string, audioBase64 string, mimeType string) error {
	return NewMessageRepository(db).SaveMessageAudioWithContext(ctx, messageID, audioBase64, mimeType)
}

func (r *MessageRepository) SaveMessageAudioWithContext(ctx context.Context, messageID string, audioBase64 string, mimeType string) error {
	db := r.db
	if _, err := RequireUserID(ctx); err != nil {
		return err
	}
	messageIDs := scopedMessageQuery(ctx, db.Model(&ChatMessage{}).Select("chat_messages.id").Where("chat_messages.id = ?", messageID))
	return db.WithContext(ctx).Model(&ChatMessage{}).Where("id = ?", messageID).Where("id IN (?)", messageIDs).Updates(map[string]interface{}{
		"audio":           audioBase64,
		"audio_mime_type": mimeType,
	}).Error
}

// HasMessageAudioWithContext verifica se uma mensagem do usuário do contexto
// tem áudio salvo.
//
// SECURITY: fail-closed (AEP-0052 / B11). Sem userID retorna false sem tocar
// o banco — antes vazava existência de áudio em mensagens cross-user via
// scopedMessageQuery (fail-open).
func HasMessageAudioWithContext(ctx context.Context, messageID string) bool {
	return NewMessageRepository(db).HasMessageAudioWithContext(ctx, messageID)
}

func (r *MessageRepository) HasMessageAudioWithContext(ctx context.Context, messageID string) bool {
	db := r.db
	if _, err := RequireUserID(ctx); err != nil {
		return false
	}
	var count int64
	scopedMessageQuery(ctx, db.Model(&ChatMessage{})).Where("chat_messages.id = ? AND chat_messages.audio != '' AND chat_messages.audio IS NOT NULL", messageID).Count(&count)
	return count > 0
}

// GetMessageContentWithContext retorna o conteúdo textual de uma mensagem
// pertencente ao usuário do contexto.
func GetMessageContentWithContext(ctx context.Context, messageID string) (string, error) {
	return NewMessageRepository(db).GetMessageContentWithContext(ctx, messageID)
}

func (r *MessageRepository) GetMessageContentWithContext(ctx context.Context, messageID string) (string, error) {
	db := r.db
	if _, err := RequireUserID(ctx); err != nil {
		return "", err
	}
	var msg ChatMessage
	if err := scopedMessageQuery(ctx, db.Model(&ChatMessage{})).
		Select("chat_messages.content").
		First(&msg, "chat_messages.id = ?", messageID).Error; err != nil {
		return "", err
	}
	return msg.Content, nil
}

// GetMessageWithContext retorna a mensagem completa pelo ID, restrita ao
// usuário do contexto. Falha fechado sem userID — leitura por ID sem escopo
// retornaria mensagens de qualquer usuário (vetor de leak silencioso).
func GetMessageWithContext(ctx context.Context, messageID string) (*ChatMessage, error) {
	return NewMessageRepository(db).GetMessageWithContext(ctx, messageID)
}

func (r *MessageRepository) GetMessageWithContext(ctx context.Context, messageID string) (*ChatMessage, error) {
	db := r.db
	if _, err := RequireUserID(ctx); err != nil {
		return nil, err
	}
	var msg ChatMessage
	if err := scopedMessageQuery(ctx, db.Model(&ChatMessage{})).First(&msg, "chat_messages.id = ?", messageID).Error; err != nil {
		return nil, err
	}
	return &msg, nil
}

// AddToolMessageWithContext adiciona uma mensagem de role="tool" (resposta de
// tool ao orquestrador) para o usuário do contexto.
func AddToolMessageWithContext(ctx context.Context, conversationID string, content string) (*ChatMessage, error) {
	return CreateMessageWithContext(ctx, MessageOptions{
		ConversationID: conversationID,
		Role:           "tool",
		Content:        content,
	})
}

// AddToolResultMessageWithContext adiciona uma mensagem de resultado de tool
// com TurnID e ToolCallID para o usuário do contexto. Usado pelo agentic loop
// para salvar o resultado de uma execução de ferramenta.
func AddToolResultMessageWithContext(ctx context.Context, conversationID string, turnID string, content, toolCallID string) (*ChatMessage, error) {
	return CreateMessageWithContext(ctx, MessageOptions{
		ConversationID: conversationID,
		TurnID:         &turnID,
		Role:           "tool",
		Content:        content,
		ToolCallID:     toolCallID,
	})
}

// AddAssistantToolMessageWithContext adiciona uma mensagem do assistente que
// contém tool_calls para o usuário do contexto. Usada quando o LLM responde
// com texto + pedidos de ferramentas.
func AddAssistantToolMessageWithContext(ctx context.Context, conversationID string, turnID string, content, toolCalls, reasoning, model string) (*ChatMessage, error) {
	return CreateMessageWithContext(ctx, MessageOptions{
		ConversationID: conversationID,
		TurnID:         &turnID,
		Role:           "assistant",
		Content:        content,
		ToolCalls:      toolCalls,
		Reasoning:      reasoning,
		Model:          model,
	})
}

// GetTurnMessagesWithContext retorna todas as mensagens de um turno (mesmo
// TurnID) pertencentes ao usuário do contexto, ordenadas por criação.
func GetTurnMessagesWithContext(ctx context.Context, turnID string) ([]ChatMessage, error) {
	return NewMessageRepository(db).GetTurnMessagesWithContext(ctx, turnID)
}

func (r *MessageRepository) GetTurnMessagesWithContext(ctx context.Context, turnID string) ([]ChatMessage, error) {
	db := r.db
	if _, err := RequireUserID(ctx); err != nil {
		return nil, err
	}
	var messages []ChatMessage
	err := scopedMessageQuery(ctx, db.Model(&ChatMessage{})).
		Where("chat_messages.turn_id = ?", turnID).
		Order("chat_messages.created_at ASC, chat_messages.id ASC").
		Find(&messages).Error
	return messages, err
}

// GetMessagesByTurnIDWithContext retorna mensagens de um turno específico
// pertencentes ao usuário do contexto. Mantém o mesmo escopo de parent da
// janela para não misturar raiz e threads.
func GetMessagesByTurnIDWithContext(ctx context.Context, conversationID string, parentID *string, turnID string, limit int) ([]ChatMessage, error) {
	return NewMessageRepository(db).GetMessagesByTurnIDWithContext(ctx, conversationID, parentID, turnID, limit)
}

func (r *MessageRepository) GetMessagesByTurnIDWithContext(ctx context.Context, conversationID string, parentID *string, turnID string, limit int) ([]ChatMessage, error) {
	db := r.db
	if _, err := RequireUserID(ctx); err != nil {
		return nil, err
	}
	if turnID == "" {
		return []ChatMessage{}, nil
	}
	query := scopedMessageQuery(ctx, db.Model(&ChatMessage{})).
		Where("chat_messages.conversation_id = ? AND chat_messages.turn_id = ?", conversationID, turnID)
	if parentID != nil {
		query = query.Where("chat_messages.parent_id = ?", *parentID)
	} else {
		query = query.Where("chat_messages.parent_id IS NULL")
	}
	if limit > 0 {
		query = query.Limit(limit)
	}
	var messages []ChatMessage
	err := query.Order("chat_messages.created_at ASC, chat_messages.id ASC").Find(&messages).Error
	return messages, err
}

// AddChildMessageWithContext adiciona uma mensagem filha (com ParentID
// definido) para o usuário do contexto.
func AddChildMessageWithContext(ctx context.Context, conversationID string, parentID string, role, content, model string) (*ChatMessage, error) {
	return CreateMessageWithContext(ctx, MessageOptions{
		ConversationID: conversationID,
		ParentID:       &parentID,
		Role:           role,
		Content:        content,
		Model:          model,
	})
}

// UpdateMessageContentWithContext atualiza o conteúdo e tokens de uma mensagem
// existente do usuário do contexto.
func UpdateMessageContentWithContext(ctx context.Context, messageID string, content string, promptTokens, completionTokens, totalTokens int, model string) error {
	return NewMessageRepository(db).UpdateMessageContentWithContext(ctx, messageID, content, promptTokens, completionTokens, totalTokens, model)
}

func (r *MessageRepository) UpdateMessageContentWithContext(ctx context.Context, messageID string, content string, promptTokens, completionTokens, totalTokens int, model string) error {
	db := r.db
	if _, err := RequireUserID(ctx); err != nil {
		return err
	}
	messageIDs := scopedMessageQuery(ctx, db.Model(&ChatMessage{}).Select("chat_messages.id").Where("chat_messages.id = ?", messageID))
	return db.WithContext(ctx).Model(&ChatMessage{}).Where("id = ?", messageID).Where("id IN (?)", messageIDs).Updates(map[string]interface{}{
		"content":           content,
		"prompt_tokens":     promptTokens,
		"completion_tokens": completionTokens,
		"total_tokens":      totalTokens,
		"model":             model,
	}).Error
}

// UpdateMessageContentAndReasoningWithContext atualiza conteúdo, reasoning e tokens de uma mensagem
// existente no contexto do usuário atual.
func UpdateMessageContentAndReasoningWithContext(ctx context.Context, messageID string, content string, reasoning string, promptTokens, completionTokens, totalTokens int, model string) error {
	return NewMessageRepository(db).UpdateMessageContentAndReasoningWithContext(ctx, messageID, content, reasoning, promptTokens, completionTokens, totalTokens, model)
}

func (r *MessageRepository) UpdateMessageContentAndReasoningWithContext(ctx context.Context, messageID string, content string, reasoning string, promptTokens, completionTokens, totalTokens int, model string) error {
	db := r.db
	if _, err := RequireUserID(ctx); err != nil {
		return err
	}
	messageIDs := scopedMessageQuery(ctx, db.Model(&ChatMessage{}).Select("chat_messages.id").Where("chat_messages.id = ?", messageID))
	return db.WithContext(ctx).Model(&ChatMessage{}).Where("id = ?", messageID).Where("id IN (?)", messageIDs).Updates(map[string]interface{}{
		"content":           content,
		"reasoning":         reasoning,
		"prompt_tokens":     promptTokens,
		"completion_tokens": completionTokens,
		"total_tokens":      totalTokens,
		"model":             model,
	}).Error
}

// UpdateMessageContentReasoningAndUsageWithContext finaliza uma resposta em uma
// única atualização para manter tokens comuns e métricas de cache consistentes.
func UpdateMessageContentReasoningAndUsageWithContext(ctx context.Context, messageID string, content string, reasoning string, promptTokens, completionTokens, totalTokens int, cacheReadTokens, cacheWriteTokens, cacheMissTokens int, model string) error {
	return NewMessageRepository(db).UpdateMessageContentReasoningAndUsageWithContext(ctx, messageID, content, reasoning, promptTokens, completionTokens, totalTokens, cacheReadTokens, cacheWriteTokens, cacheMissTokens, model)
}

func (r *MessageRepository) UpdateMessageContentReasoningAndUsageWithContext(ctx context.Context, messageID string, content string, reasoning string, promptTokens, completionTokens, totalTokens int, cacheReadTokens, cacheWriteTokens, cacheMissTokens int, model string) error {
	db := r.db
	if _, err := RequireUserID(ctx); err != nil {
		return err
	}
	messageIDs := scopedMessageQuery(ctx, db.Model(&ChatMessage{}).Select("chat_messages.id").Where("chat_messages.id = ?", messageID))
	return db.WithContext(ctx).Model(&ChatMessage{}).Where("id = ?", messageID).Where("id IN (?)", messageIDs).Updates(map[string]interface{}{
		"content":            content,
		"reasoning":          reasoning,
		"prompt_tokens":      promptTokens,
		"completion_tokens":  completionTokens,
		"total_tokens":       totalTokens,
		"cache_read_tokens":  cacheReadTokens,
		"cache_write_tokens": cacheWriteTokens,
		"cache_miss_tokens":  cacheMissTokens,
		"model":              model,
	}).Error
}

// UpdateMessageCacheTokensWithContext atualiza métricas opcionais de prompt cache
// de uma mensagem existente. Campos zero preservam compatibilidade com mensagens
// antigas e providers que não reportam cache.
func UpdateMessageCacheTokensWithContext(ctx context.Context, messageID string, cacheReadTokens, cacheWriteTokens, cacheMissTokens int) error {
	return NewMessageRepository(db).UpdateMessageCacheTokensWithContext(ctx, messageID, cacheReadTokens, cacheWriteTokens, cacheMissTokens)
}

func (r *MessageRepository) UpdateMessageCacheTokensWithContext(ctx context.Context, messageID string, cacheReadTokens, cacheWriteTokens, cacheMissTokens int) error {
	updates := map[string]interface{}{}
	if cacheReadTokens > 0 {
		updates["cache_read_tokens"] = cacheReadTokens
	}
	if cacheWriteTokens > 0 {
		updates["cache_write_tokens"] = cacheWriteTokens
	}
	if cacheMissTokens > 0 {
		updates["cache_miss_tokens"] = cacheMissTokens
	}
	if len(updates) == 0 {
		return nil
	}
	db := r.db
	if _, err := RequireUserID(ctx); err != nil {
		return err
	}
	messageIDs := scopedMessageQuery(ctx, db.Model(&ChatMessage{}).Select("chat_messages.id").Where("chat_messages.id = ?", messageID))
	return db.WithContext(ctx).Model(&ChatMessage{}).Where("id = ?", messageID).Where("id IN (?)", messageIDs).Updates(updates).Error
}

// DeleteMessageWithContext exclui uma mensagem e todas as suas filhas
// (respostas) do usuário do contexto.
func DeleteMessageWithContext(ctx context.Context, messageID string) error {
	return NewMessageRepository(db).DeleteMessageWithContext(ctx, messageID)
}

func (r *MessageRepository) DeleteMessageWithContext(ctx context.Context, messageID string) error {
	db := r.db
	if _, err := RequireUserID(ctx); err != nil {
		return err
	}

	// Carrega metadados para suportar deleção por turno quando a raiz (user) é removida.
	var root ChatMessage
	_ = scopedMessageQuery(ctx, db.Model(&ChatMessage{})).
		Select("chat_messages.id", "chat_messages.role", "chat_messages.turn_id", "chat_messages.conversation_id").
		First(&root, "chat_messages.id = ?", messageID).Error

	// Best-effort: ao apagar uma mensagem, remove também invocações técnicas
	// associadas para não deixar tool_invocations órfãs.
	// Regra importante: não apagar invocações do turno inteiro ao deletar uma
	// mensagem assistant/tool isolada. Quando aplicável, limpa apenas as
	// invocações do turno que correspondem aos tool_call_id citados na mensagem.
	cleanup := deleteChatToolInvocationCleanupForMessage(ctx, db, messageID)
	if len(cleanup.OriginIDs) > 0 {
		if err := deleteChatToolInvocationsForOriginIDs(ctx, db, cleanup.OriginIDs); err != nil {
			// Best-effort: tool_invocations são registros técnicos; não bloquear a deleção do usuário.
			logging.Warnf(ctx, "database.message-repository", "[DB] aviso: falha ao limpar tool_invocations de mensagem %s: %v", messageID, err)
		}
	}
	if cleanup.DeleteWholeTurn && strings.TrimSpace(cleanup.TurnID) != "" {
		if err := deleteChatToolInvocationsForOriginIDs(ctx, db, []string{cleanup.TurnID}); err != nil {
			logging.Warnf(ctx, "database.message-repository", "[DB] aviso: falha ao limpar tool_invocations do turno %s: %v", cleanup.TurnID, err)
		}
	} else if strings.TrimSpace(cleanup.TurnID) != "" && len(cleanup.ToolCallIDs) > 0 {
		if err := deleteChatToolInvocationsForTurnToolCallIDs(ctx, db, cleanup.TurnID, cleanup.ToolCallIDs); err != nil {
			logging.Warnf(ctx, "database.message-repository", "[DB] aviso: falha ao limpar tool_invocations por tool_call_id do turno %s: %v", cleanup.TurnID, err)
		}
	}
	var childIDs []string
	// Se estamos deletando a mensagem do usuário (raiz do turno), apaga também as
	// demais mensagens do mesmo turno (assistant/tool) mesmo que não estejam
	// conectadas via parent_id.
	if strings.TrimSpace(root.ID) != "" && root.Role == "user" {
		turnRootID := strings.TrimSpace(root.ID)
		var turnMessageIDs []string
		if err := scopedMessageQuery(ctx, db.Model(&ChatMessage{})).
			Where("chat_messages.conversation_id = ? AND chat_messages.turn_id = ?", root.ConversationID, turnRootID).
			Pluck("chat_messages.id", &turnMessageIDs).Error; err != nil {
			return err
		}
		for _, id := range turnMessageIDs {
			id = strings.TrimSpace(id)
			if id == "" || id == messageID {
				continue
			}
			childIDs = append(childIDs, id)
		}
	}
	var parentChildIDs []string
	if err := scopedMessageQuery(ctx, db.Model(&ChatMessage{})).Where("chat_messages.parent_id = ?", messageID).Pluck("chat_messages.id", &parentChildIDs).Error; err != nil {
		return err
	}
	childIDs = append(childIDs, parentChildIDs...)
	for _, childID := range childIDs {
		if err := r.DeleteMessageWithContext(ctx, childID); err != nil {
			return err
		}
	}
	messageIDs := scopedMessageQuery(ctx, db.Model(&ChatMessage{}).Select("chat_messages.id").Where("chat_messages.id = ?", messageID))
	return db.WithContext(ctx).Where("id IN (?)", messageIDs).Delete(&ChatMessage{}).Error
}

type chatToolInvocationCleanup struct {
	OriginIDs       []string
	TurnID          string
	ToolCallIDs     []string
	DeleteWholeTurn bool
}

func deleteChatToolInvocationCleanupForMessage(ctx context.Context, exec *gorm.DB, messageID string) chatToolInvocationCleanup {
	if _, err := RequireUserID(ctx); err != nil {
		return chatToolInvocationCleanup{}
	}
	if strings.TrimSpace(messageID) == "" {
		return chatToolInvocationCleanup{}
	}
	// scopedMessageQuery garante que não vazamos cross-user.
	var msg ChatMessage
	err := scopedMessageQuery(ctx, exec.Model(&ChatMessage{})).
		Select("chat_messages.id", "chat_messages.role", "chat_messages.turn_id", "chat_messages.tool_calls", "chat_messages.tool_call_id").
		First(&msg, "chat_messages.id = ?", messageID).Error
	if err != nil {
		return chatToolInvocationCleanup{}
	}
	cleanup := chatToolInvocationCleanup{
		OriginIDs: []string{strings.TrimSpace(msg.ID)},
	}

	turn := ""
	if msg.TurnID != nil {
		turn = strings.TrimSpace(*msg.TurnID)
	}
	cleanup.TurnID = turn

	// Deletar a raiz do turno (role=user, turn_id == id) pode limpar o turno inteiro.
	if msg.Role == "user" && turn != "" && turn == strings.TrimSpace(msg.ID) {
		cleanup.DeleteWholeTurn = true
		return dedupCleanup(cleanup)
	}

	// Ao deletar mensagens assistant/tool, limpar apenas as invocações do turno
	// que correspondem aos tool_call_id referenciados por esta mensagem.
	if turn == "" {
		return dedupCleanup(cleanup)
	}

	if msg.Role == "tool" {
		callID := strings.TrimSpace(msg.ToolCallID)
		if callID != "" {
			cleanup.ToolCallIDs = append(cleanup.ToolCallIDs, callID)
		}
		return dedupCleanup(cleanup)
	}

	if msg.Role == "assistant" {
		toolCallsJSON := strings.TrimSpace(msg.ToolCalls)
		if toolCallsJSON == "" {
			cleanup.ToolCallIDs = append(cleanup.ToolCallIDs, chatToolInvocationCallIDsForTurn(ctx, exec, turn)...)
			return dedupCleanup(cleanup)
		}
		// Aceita tanto `[{...}]` quanto `{...}`.
		var anyPayload any
		if err := json.Unmarshal([]byte(toolCallsJSON), &anyPayload); err == nil {
			switch v := anyPayload.(type) {
			case []any:
				for _, item := range v {
					if obj, ok := item.(map[string]any); ok {
						if id, _ := obj["id"].(string); strings.TrimSpace(id) != "" {
							cleanup.ToolCallIDs = append(cleanup.ToolCallIDs, strings.TrimSpace(id))
						}
					}
				}
			case map[string]any:
				if id, _ := v["id"].(string); strings.TrimSpace(id) != "" {
					cleanup.ToolCallIDs = append(cleanup.ToolCallIDs, strings.TrimSpace(id))
				}
			}
		}
		return dedupCleanup(cleanup)
	}

	return dedupCleanup(cleanup)
}

func chatToolInvocationCallIDsForTurn(ctx context.Context, exec *gorm.DB, turnID string) []string {
	userID, err := RequireUserID(ctx)
	if err != nil || strings.TrimSpace(turnID) == "" {
		return nil
	}
	if exec == nil || !exec.Migrator().HasTable(&ToolInvocation{}) {
		return nil
	}
	var rows []ToolInvocation
	if err := exec.WithContext(ctx).
		Select("tool_call_id").
		Where("user_id = ? AND origin_type = ? AND origin_id = ? AND tool_call_id <> ''", userID, "chat", strings.TrimSpace(turnID)).
		Find(&rows).Error; err != nil {
		logging.Warnf(ctx, "database.message-repository", "[DB] aviso: falha ao listar tool_call_id de tool_invocations do turno %s: %v", turnID, err)
		return nil
	}
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		if callID := strings.TrimSpace(row.ToolCallID); callID != "" {
			out = append(out, callID)
		}
	}
	return out
}

func dedupCleanup(in chatToolInvocationCleanup) chatToolInvocationCleanup {
	seen := map[string]struct{}{}
	outOrigin := make([]string, 0, len(in.OriginIDs))
	for _, id := range in.OriginIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		outOrigin = append(outOrigin, id)
	}
	seen = map[string]struct{}{}
	outCalls := make([]string, 0, len(in.ToolCallIDs))
	for _, id := range in.ToolCallIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		outCalls = append(outCalls, id)
	}
	in.OriginIDs = outOrigin
	in.ToolCallIDs = outCalls
	in.TurnID = strings.TrimSpace(in.TurnID)
	return in
}

func deleteChatToolInvocationsForTurnToolCallIDs(ctx context.Context, exec *gorm.DB, turnID string, toolCallIDs []string) error {
	if _, err := RequireUserID(ctx); err != nil {
		return err
	}
	turn := strings.TrimSpace(turnID)
	if turn == "" {
		return nil
	}
	if len(toolCallIDs) == 0 {
		return nil
	}
	if !exec.Migrator().HasTable(&ToolInvocation{}) {
		return nil
	}
	userID, _ := UserIDFromContext(ctx)
	ids := make([]string, 0, len(toolCallIDs))
	for _, id := range toolCallIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return nil
	}
	// Batch para evitar estourar limite de variáveis do SQLite.
	const batchSize = 400
	for start := 0; start < len(ids); start += batchSize {
		end := start + batchSize
		if end > len(ids) {
			end = len(ids)
		}
		if err := exec.WithContext(ctx).
			Where("user_id = ? AND origin_type = ? AND origin_id = ? AND tool_call_id IN ?", userID, "chat", turn, ids[start:end]).
			Delete(&ToolInvocation{}).Error; err != nil {
			return err
		}
	}
	return nil
}

func deleteChatToolInvocationsForOriginIDs(ctx context.Context, exec *gorm.DB, originIDs []string) error {
	if _, err := RequireUserID(ctx); err != nil {
		return err
	}
	if len(originIDs) == 0 {
		return nil
	}
	if !exec.Migrator().HasTable(&ToolInvocation{}) {
		return nil
	}
	userID, _ := UserIDFromContext(ctx)
	// Batch para evitar estourar limite de variáveis do SQLite.
	const batchSize = 400
	for start := 0; start < len(originIDs); start += batchSize {
		end := start + batchSize
		if end > len(originIDs) {
			end = len(originIDs)
		}
		if err := exec.WithContext(ctx).
			Where("user_id = ? AND origin_type = ? AND origin_id IN ?", userID, "chat", originIDs[start:end]).
			Delete(&ToolInvocation{}).Error; err != nil {
			return err
		}
	}
	return nil
}

// DeleteAllMessagesWithContext remove todas as mensagens de uma conversa
// pertencente ao usuário do contexto.
func DeleteAllMessagesWithContext(ctx context.Context, conversationID string) error {
	return NewMessageRepository(db).DeleteAllMessagesWithContext(ctx, conversationID)
}

func (r *MessageRepository) DeleteAllMessagesWithContext(ctx context.Context, conversationID string) error {
	db := r.db
	if _, err := RequireUserID(ctx); err != nil {
		return err
	}
	// Limpa tool invocations associadas ao histórico desta conversa.
	if err := deleteChatToolInvocationsForConversation(ctx, db, conversationID); err != nil {
		return err
	}
	messageIDs := scopedMessageQuery(ctx, db.Model(&ChatMessage{}).Select("chat_messages.id").Where("chat_messages.conversation_id = ?", conversationID))
	return db.WithContext(ctx).Where("id IN (?)", messageIDs).Delete(&ChatMessage{}).Error
}

// ClearConversationContentWithContext apaga, de forma ATÔMICA, TODO o conteúdo
// destrutivo de uma conversa do usuário do contexto: tool invocations de chat,
// mensagens (histórico) e o resumo (summary). As três operações rodam na MESMA
// transação GORM, então qualquer erro no meio faz rollback TOTAL — nunca fica um
// estado parcialmente limpo (ex.: tool invocations apagadas mas mensagens/summary
// preservados, ou summary antigo apontando para mensagens já apagadas), que
// confundiria UI/sumarização. Usado pelo clear de sub-agente (AEP-0068).
//
// Ordem dentro da transação: tool invocations PRIMEIRO (seus ids são coletados a
// partir das mensagens, que ainda precisam existir), depois mensagens, depois
// summary.
//
// SECURITY: fail-closed (AEP-0052). Sem userID no ctx retorna ErrUserScopeRequired.
func ClearConversationContentWithContext(ctx context.Context, conversationID string) error {
	return NewMessageRepository(db).ClearConversationContentWithContext(ctx, conversationID)
}

func (r *MessageRepository) ClearConversationContentWithContext(ctx context.Context, conversationID string) error {
	db := r.db
	if _, err := RequireUserID(ctx); err != nil {
		return err
	}
	conversationID = strings.TrimSpace(conversationID)
	if conversationID == "" {
		return nil
	}
	// Valida posse/escopo antes de mutar (mesma checagem dos helpers originais).
	if _, err := NewConversationRepository(db).GetConversationInfoWithContext(ctx, conversationID); err != nil {
		return err
	}
	return db.Transaction(func(tx *gorm.DB) error {
		// 1) Tool invocations de chat (coletadas a partir das mensagens que ainda
		//    existem) — dentro do mesmo tx para que um rollback as preserve.
		if err := deleteChatToolInvocationsForConversationTx(ctx, tx, conversationID); err != nil {
			return fmt.Errorf("erro ao limpar tool invocations da sub-conversa: %w", err)
		}
		// 2) Mensagens (histórico).
		messageIDs := scopedMessageQuery(ctx, tx.Model(&ChatMessage{}).
			Select("chat_messages.id").
			Where("chat_messages.conversation_id = ?", conversationID))
		if err := tx.WithContext(ctx).Where("id IN (?)", messageIDs).Delete(&ChatMessage{}).Error; err != nil {
			return fmt.Errorf("erro ao limpar histórico da sub-conversa: %w", err)
		}
		// 3) Resumo (summary).
		if err := ScopeByUser(ctx, tx.WithContext(ctx).Model(&Conversation{}), "user_id").
			Where("id = ?", conversationID).
			Updates(map[string]interface{}{
				"summary":                  "",
				"summary_up_to_message_id": "",
				"summarizing_in_progress":  false,
			}).Error; err != nil {
			return fmt.Errorf("erro ao limpar resumo da sub-conversa: %w", err)
		}
		return nil
	})
}

// ClearAllConversationsWithContext apaga mensagens e conversas pertencentes ao
// usuário do contexto. Falha fechado com ErrUserScopeRequired sem userID —
// não há caso legítimo de "limpar global"; AdoptLegacyData/RebuildFTSIndex
// têm assinaturas próprias para operações instance-wide. Antes do AEP-0052
// esta função apagava tudo de todos quando chamada sem ctx; o comportamento
// foi removido para não ser uma bomba-relógio assinada.
func ClearAllConversationsWithContext(ctx context.Context) error {
	return NewMessageRepository(db).ClearAllConversationsWithContext(ctx)
}

func (r *MessageRepository) ClearAllConversationsWithContext(ctx context.Context) error {
	db := r.db
	if _, err := RequireUserID(ctx); err != nil {
		return err
	}
	userID, _ := UserIDFromContext(ctx)
	// As três deleções (tool invocations + mensagens + conversas) destroem TODO
	// o conteúdo do usuário. Rodam na MESMA transação para garantir atomicidade:
	// qualquer erro no meio faz rollback TOTAL, sem estados parciais (ex.: tool
	// invocations apagadas mas mensagens/conversas preservadas, ou mensagens
	// apagadas com conversas órfãs). Espelha ClearConversationContentWithContext.
	return db.Transaction(func(tx *gorm.DB) error {
		// 1) Tool invocations de chat do usuário (histórico será apagado).
		//    HasTable tolera cenários de teste com migrações parciais.
		if tx.Migrator().HasTable(&ToolInvocation{}) {
			if err := tx.WithContext(ctx).
				Where("user_id = ? AND origin_type = ?", userID, "chat").
				Delete(&ToolInvocation{}).Error; err != nil {
				return fmt.Errorf("erro ao limpar tool invocations: %w", err)
			}
		}
		// 2) Mensagens (histórico).
		messageIDs := scopedMessageQuery(ctx, tx.WithContext(ctx).Model(&ChatMessage{}).Select("chat_messages.id"))
		if err := tx.WithContext(ctx).Where("id IN (?)", messageIDs).Delete(&ChatMessage{}).Error; err != nil {
			return fmt.Errorf("erro ao limpar mensagens: %w", err)
		}
		// 3) Conversas.
		if err := ScopeByUser(ctx, tx.WithContext(ctx), "user_id").Delete(&Conversation{}).Error; err != nil {
			return fmt.Errorf("erro ao limpar conversas: %w", err)
		}
		return nil
	})
}

// GetMessagesWithContext retorna mensagens de uma conversa do usuário do
// contexto, com filtro opcional por parent.
func GetMessagesWithContext(ctx context.Context, conversationID string, parentID *string) ([]ChatMessage, error) {
	return NewMessageRepository(db).GetMessagesWithContext(ctx, conversationID, parentID)
}

func (r *MessageRepository) GetMessagesWithContext(ctx context.Context, conversationID string, parentID *string) ([]ChatMessage, error) {
	db := r.db
	if _, err := RequireUserID(ctx); err != nil {
		return nil, err
	}
	var messages []ChatMessage
	query := scopedMessageQuery(ctx, db.Model(&ChatMessage{})).Order("chat_messages.created_at ASC, chat_messages.id ASC")

	if parentID != nil {
		query = query.Where("chat_messages.parent_id = ?", *parentID)
		if conversationID != "" {
			query = query.Where("chat_messages.conversation_id = ?", conversationID)
		}
	} else {
		if conversationID == "" {
			return nil, fmt.Errorf("conversationID é obrigatório para buscar mensagens raiz")
		}
		query = query.Where("chat_messages.conversation_id = ? AND chat_messages.parent_id IS NULL", conversationID)
	}

	err := query.Find(&messages).Error
	return messages, err
}

// GetRecentRootMessagesWithContext retorna as mensagens raiz mais recentes de
// uma conversa do usuário do contexto, preservando ordem cronológica no retorno.
func GetRecentRootMessagesWithContext(ctx context.Context, conversationID string, limit int) ([]ChatMessage, error) {
	return NewMessageRepository(db).GetRecentRootMessagesWithContext(ctx, conversationID, limit)
}

func (r *MessageRepository) GetRecentRootMessagesWithContext(ctx context.Context, conversationID string, limit int) ([]ChatMessage, error) {
	db := r.db
	if _, err := RequireUserID(ctx); err != nil {
		return nil, err
	}
	if conversationID == "" {
		return nil, fmt.Errorf("conversationID é obrigatório para buscar mensagens recentes")
	}
	if limit <= 0 {
		return []ChatMessage{}, nil
	}

	var messages []ChatMessage
	err := scopedMessageQuery(ctx, db.Model(&ChatMessage{})).Where("chat_messages.conversation_id = ? AND chat_messages.parent_id IS NULL", conversationID).
		Order("chat_messages.created_at DESC, chat_messages.id DESC").
		Limit(limit).
		Find(&messages).Error
	if err != nil {
		return nil, err
	}

	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}
	return messages, nil
}

// GetRootMessagesBeforeWithContext retorna mensagens raiz anteriores a
// beforeID, do usuário do contexto, preservando ordem cronológica no retorno.
func GetRootMessagesBeforeWithContext(ctx context.Context, conversationID string, beforeID string, limit int) ([]ChatMessage, error) {
	return NewMessageRepository(db).GetRootMessagesBeforeWithContext(ctx, conversationID, beforeID, limit)
}

func (r *MessageRepository) GetRootMessagesBeforeWithContext(ctx context.Context, conversationID string, beforeID string, limit int) ([]ChatMessage, error) {
	db := r.db
	if _, err := RequireUserID(ctx); err != nil {
		return nil, err
	}
	if conversationID == "" {
		return nil, fmt.Errorf("conversationID é obrigatório para buscar mensagens anteriores")
	}
	if beforeID == "" {
		return nil, fmt.Errorf("beforeID é obrigatório para buscar mensagens anteriores")
	}
	if limit <= 0 {
		return []ChatMessage{}, nil
	}

	var before ChatMessage
	if err := scopedMessageQuery(ctx, db.Model(&ChatMessage{})).
		Select("chat_messages.id", "chat_messages.created_at").
		First(&before, "chat_messages.id = ? AND chat_messages.conversation_id = ?", beforeID, conversationID).Error; err != nil {
		return nil, err
	}

	var messages []ChatMessage
	err := scopedMessageQuery(ctx, db.Model(&ChatMessage{})).Where(
		"chat_messages.conversation_id = ? AND chat_messages.parent_id IS NULL AND (chat_messages.created_at < ? OR (chat_messages.created_at = ? AND chat_messages.id < ?))",
		conversationID,
		before.CreatedAt,
		before.CreatedAt,
		before.ID,
	).
		Order("chat_messages.created_at DESC, chat_messages.id DESC").
		Limit(limit).
		Find(&messages).Error
	if err != nil {
		return nil, err
	}

	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}
	return messages, nil
}

type MessageWindowQuery struct {
	ConversationID  string
	ParentID        *string
	Anchor          string
	AnchorMessageID string
	Direction       string
	Limit           int
}

type MessageWindowResult struct {
	Items      []MessageWindowItem
	Messages   []ChatMessage
	TotalCount int
	StartIndex int
	EndIndex   int
	HasBefore  bool
	HasAfter   bool
}

type MessageWindowItem struct {
	Kind          string
	ID            string
	MessageID     string
	TurnID        string
	OriginalIndex int
	CreatedAt     time.Time
	FirstID       string
}

const (
	// MaxMessageWindowRows limita cada janela canônica de timeline para manter
	// consultas e renderização previsíveis em conversas longas. Ver AEP-0059.
	MaxMessageWindowRows = 240

	// MessageWindowItemKindMessage identifica uma mensagem raiz navegável sem consolidação.
	MessageWindowItemKindMessage = "message"
	// MessageWindowItemKindTurn identifica um turno consolidado por turn_id.
	MessageWindowItemKindTurn = "turn"

	messageWindowAnchorStart = "start"
	messageWindowAnchorEnd   = "end"

	messageWindowDirectionBefore = "before"
	messageWindowDirectionAfter  = "after"
	messageWindowDirectionAround = "around"
)

func normalizeMessageWindowLimit(limit int) int {
	if limit <= 0 {
		return 0
	}
	if limit > MaxMessageWindowRows {
		return MaxMessageWindowRows
	}
	return limit
}

func computeMessageWindowHasAfter(total int, startIndex int, endIndex int, itemCount int) bool {
	if total <= 0 {
		return false
	}
	if itemCount == 0 {
		return startIndex < total
	}
	return endIndex < total-1
}

func normalizeMessageWindowCursor(query MessageWindowQuery) (anchor string, direction string, err error) {
	anchor = strings.TrimSpace(query.Anchor)
	direction = strings.TrimSpace(query.Direction)
	if direction == "" {
		direction = messageWindowDirectionBefore
	}
	if anchor != "" && anchor != messageWindowAnchorStart && anchor != messageWindowAnchorEnd {
		return "", "", fmt.Errorf("anchor de janela de mensagens inválido: %s", query.Anchor)
	}
	if direction != messageWindowDirectionBefore &&
		direction != messageWindowDirectionAfter &&
		direction != messageWindowDirectionAround {
		return "", "", fmt.Errorf("direction de janela de mensagens inválido: %s", query.Direction)
	}
	if anchor != "" && query.AnchorMessageID != "" {
		return "", "", fmt.Errorf("anchor e anchorMessageId são mutuamente exclusivos")
	}
	if anchor == messageWindowAnchorStart && direction == messageWindowDirectionBefore {
		return "", "", fmt.Errorf("anchor=start não aceita direction=before")
	}
	if anchor == messageWindowAnchorEnd && direction == messageWindowDirectionAfter {
		return "", "", fmt.Errorf("anchor=end não aceita direction=after")
	}
	if direction == messageWindowDirectionAround && query.AnchorMessageID == "" {
		return "", "", fmt.Errorf("direction=around exige anchorMessageId")
	}
	return anchor, direction, nil
}

func (r *MessageRepository) messageScopeQuery(conversationID string, parentID *string) *gorm.DB {
	query := r.db.Model(&ChatMessage{}).Where("conversation_id = ?", conversationID)
	if parentID != nil {
		return query.Where("parent_id = ?", *parentID)
	}
	return query.Where("parent_id IS NULL")
}

func timelineItemCTE(parentID *string) string {
	parentPredicate := "parent_id IS NULL"
	if parentID != nil {
		parentPredicate = "parent_id = ?"
	}
	return fmt.Sprintf(`
WITH scoped AS (
	SELECT
		id,
		turn_id,
		created_at,
		CASE WHEN COALESCE(turn_id, '') <> '' THEN 'turn' ELSE 'message' END AS item_kind,
		CASE WHEN COALESCE(turn_id, '') <> '' THEN turn_id ELSE id END AS item_id,
		ROW_NUMBER() OVER (
			PARTITION BY
				CASE WHEN COALESCE(turn_id, '') <> '' THEN 'turn' ELSE 'message' END,
				CASE WHEN COALESCE(turn_id, '') <> '' THEN turn_id ELSE id END
			ORDER BY created_at ASC, id ASC
		) AS rn
	FROM chat_messages
	WHERE conversation_id = ? AND %s
),
timeline_items AS (
	SELECT
		item_kind AS kind,
		item_id AS id,
		id AS message_id,
		CASE WHEN item_kind = 'turn' THEN item_id ELSE '' END AS turn_id,
		created_at,
		id AS first_id
	FROM scoped
	WHERE rn = 1
)`, parentPredicate)
}

func timelineItemArgs(conversationID string, parentID *string) []interface{} {
	args := []interface{}{conversationID}
	if parentID != nil {
		args = append(args, *parentID)
	}
	return args
}

func (r *MessageRepository) countTimelineItems(conversationID string, parentID *string) (int, error) {
	var count int64
	sql := timelineItemCTE(parentID) + ` SELECT COUNT(*) FROM timeline_items`
	err := r.db.Raw(sql, timelineItemArgs(conversationID, parentID)...).Scan(&count).Error
	return int(count), err
}

func (r *MessageRepository) getAnchorTimelineItem(query MessageWindowQuery) (*MessageWindowItem, error) {
	sql := timelineItemCTE(query.ParentID) + `
SELECT ti.kind, ti.id, ti.message_id, ti.turn_id, ti.created_at, ti.first_id
FROM timeline_items ti
JOIN scoped s ON s.item_kind = ti.kind AND s.item_id = ti.id
WHERE s.id = ?
LIMIT 1`
	args := append(timelineItemArgs(query.ConversationID, query.ParentID), query.AnchorMessageID)
	var item MessageWindowItem
	if err := r.db.Raw(sql, args...).Scan(&item).Error; err != nil {
		return nil, err
	}
	if item.ID == "" {
		return nil, fmt.Errorf("anchorMessageId inválido: %s", query.AnchorMessageID)
	}
	return &item, nil
}

func (r *MessageRepository) countTimelineItemsBefore(conversationID string, parentID *string, anchor MessageWindowItem) (int, error) {
	var count int64
	sql := timelineItemCTE(parentID) + `
SELECT COUNT(*)
FROM timeline_items
WHERE created_at < ? OR (created_at = ? AND first_id < ?)`
	args := append(timelineItemArgs(conversationID, parentID), anchor.CreatedAt, anchor.CreatedAt, anchor.FirstID)
	err := r.db.Raw(sql, args...).Scan(&count).Error
	return int(count), err
}

func (r *MessageRepository) queryTimelineItems(conversationID string, parentID *string, where string, order string, limit int, extraArgs ...interface{}) ([]MessageWindowItem, error) {
	if limit <= 0 {
		return []MessageWindowItem{}, nil
	}
	sql := timelineItemCTE(parentID) + `
SELECT kind, id, message_id, turn_id, created_at, first_id
FROM timeline_items`
	args := timelineItemArgs(conversationID, parentID)
	if where != "" {
		sql += " WHERE " + where
		args = append(args, extraArgs...)
	}
	sql += " ORDER BY " + order + " LIMIT ?"
	args = append(args, limit)
	var items []MessageWindowItem
	if err := r.db.Raw(sql, args...).Scan(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func (r *MessageRepository) queryTimelineItemsAround(conversationID string, parentID *string, offset int, limit int) ([]MessageWindowItem, error) {
	if limit <= 0 {
		return []MessageWindowItem{}, nil
	}
	sql := timelineItemCTE(parentID) + `
SELECT kind, id, message_id, turn_id, created_at, first_id
FROM timeline_items
ORDER BY created_at ASC, first_id ASC
LIMIT ? OFFSET ?`
	args := append(timelineItemArgs(conversationID, parentID), limit, offset)
	var items []MessageWindowItem
	if err := r.db.Raw(sql, args...).Scan(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func reverseTimelineItems(items []MessageWindowItem) {
	for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
		items[i], items[j] = items[j], items[i]
	}
}

func (r *MessageRepository) fetchMessagesForTimelineItems(conversationID string, parentID *string, items []MessageWindowItem) ([]ChatMessage, error) {
	if len(items) == 0 {
		return []ChatMessage{}, nil
	}
	turnIDs := make([]string, 0)
	messageIDs := make([]string, 0)
	for _, item := range items {
		if item.Kind == MessageWindowItemKindTurn {
			turnIDs = append(turnIDs, item.TurnID)
		} else {
			messageIDs = append(messageIDs, item.MessageID)
		}
	}
	query := r.messageScopeQuery(conversationID, parentID)
	switch {
	case len(turnIDs) > 0 && len(messageIDs) > 0:
		query = query.Where("turn_id IN ? OR id IN ?", turnIDs, messageIDs)
	case len(turnIDs) > 0:
		query = query.Where("turn_id IN ?", turnIDs)
	default:
		query = query.Where("id IN ?", messageIDs)
	}
	var messages []ChatMessage
	if err := query.Order("created_at ASC, id ASC").Find(&messages).Error; err != nil {
		return nil, err
	}
	return messages, nil
}

// GetMessageWindowWithContext retorna uma fatia ordenada de itens de timeline
// para o usuário do contexto, acompanhada de metadados absolutos para
// renderização acessível e navegação incremental.
func GetMessageWindowWithContext(ctx context.Context, query MessageWindowQuery) (*MessageWindowResult, error) {
	return NewMessageRepository(db).GetMessageWindowWithContext(ctx, query)
}

func (r *MessageRepository) GetMessageWindowWithContext(ctx context.Context, query MessageWindowQuery) (*MessageWindowResult, error) {
	if _, err := RequireUserID(ctx); err != nil {
		return nil, err
	}
	if query.ConversationID == "" {
		return nil, fmt.Errorf("conversationID é obrigatório para buscar janela de mensagens")
	}
	if _, err := NewConversationRepository(r.db).GetConversationInfoWithContext(ctx, query.ConversationID); err != nil {
		return nil, err
	}
	if query.ParentID != nil {
		parent, err := r.GetMessageWithContext(ctx, *query.ParentID)
		if err != nil {
			return nil, err
		}
		if parent.ConversationID != query.ConversationID {
			return nil, fmt.Errorf("parentID não pertence à conversa solicitada")
		}
	}
	anchor, direction, err := normalizeMessageWindowCursor(query)
	if err != nil {
		return nil, err
	}

	limit := normalizeMessageWindowLimit(query.Limit)
	total, err := r.countTimelineItems(query.ConversationID, query.ParentID)
	if err != nil {
		return nil, err
	}
	if limit == 0 {
		return &MessageWindowResult{
			Items:      []MessageWindowItem{},
			Messages:   []ChatMessage{},
			TotalCount: total,
			StartIndex: 0,
			EndIndex:   -1,
			HasBefore:  false,
			HasAfter:   total > 0,
		}, nil
	}
	if total == 0 {
		return &MessageWindowResult{
			Items:      []MessageWindowItem{},
			Messages:   []ChatMessage{},
			TotalCount: 0,
			StartIndex: 0,
			EndIndex:   -1,
		}, nil
	}

	startIndex := 0
	windowLimit := min(limit, total)
	var items []MessageWindowItem

	if query.AnchorMessageID != "" {
		anchorItem, err := r.getAnchorTimelineItem(query)
		if err != nil {
			return nil, err
		}
		anchorIndex, err := r.countTimelineItemsBefore(query.ConversationID, query.ParentID, *anchorItem)
		if err != nil {
			return nil, err
		}
		switch direction {
		case messageWindowDirectionAfter:
			startIndex = anchorIndex + 1
			items, err = r.queryTimelineItems(
				query.ConversationID,
				query.ParentID,
				"created_at > ? OR (created_at = ? AND first_id > ?)",
				"created_at ASC, first_id ASC",
				windowLimit,
				anchorItem.CreatedAt,
				anchorItem.CreatedAt,
				anchorItem.FirstID,
			)
		case messageWindowDirectionAround:
			startIndex = anchorIndex - (limit / 2)
			if startIndex < 0 {
				startIndex = 0
			}
			if startIndex+windowLimit > total {
				startIndex = total - windowLimit
				if startIndex < 0 {
					startIndex = 0
				}
			}
			items, err = r.queryTimelineItemsAround(query.ConversationID, query.ParentID, startIndex, windowLimit)
		default:
			items, err = r.queryTimelineItems(
				query.ConversationID,
				query.ParentID,
				"created_at < ? OR (created_at = ? AND first_id < ?)",
				"created_at DESC, first_id DESC",
				windowLimit,
				anchorItem.CreatedAt,
				anchorItem.CreatedAt,
				anchorItem.FirstID,
			)
			reverseTimelineItems(items)
			startIndex = anchorIndex - len(items)
			if startIndex < 0 {
				startIndex = 0
			}
		}
		if err != nil {
			return nil, err
		}
	} else if anchor == messageWindowAnchorStart || direction == messageWindowDirectionAfter {
		startIndex = 0
		items, err = r.queryTimelineItems(query.ConversationID, query.ParentID, "", "created_at ASC, first_id ASC", windowLimit)
	} else {
		items, err = r.queryTimelineItems(query.ConversationID, query.ParentID, "", "created_at DESC, first_id DESC", windowLimit)
		reverseTimelineItems(items)
		startIndex = total - len(items)
	}
	if err != nil {
		return nil, err
	}
	for i := range items {
		items[i].OriginalIndex = startIndex + i
	}

	messages, err := r.fetchMessagesForTimelineItems(query.ConversationID, query.ParentID, items)
	if err != nil {
		return nil, err
	}
	endIndex := startIndex + len(items) - 1
	if len(items) == 0 {
		endIndex = -1
	}

	return &MessageWindowResult{
		Items:      items,
		Messages:   messages,
		TotalCount: total,
		StartIndex: startIndex,
		EndIndex:   endIndex,
		HasBefore:  startIndex > 0,
		HasAfter:   computeMessageWindowHasAfter(total, startIndex, endIndex, len(items)),
	}, nil
}

// GetAllConversationMessagesWithContext retorna todas as mensagens de uma
// conversa (incluindo filhas) pertencente ao usuário do contexto.
func GetAllConversationMessagesWithContext(ctx context.Context, conversationID string) ([]ChatMessage, error) {
	return NewMessageRepository(db).GetAllConversationMessagesWithContext(ctx, conversationID)
}

func (r *MessageRepository) GetAllConversationMessagesWithContext(ctx context.Context, conversationID string) ([]ChatMessage, error) {
	db := r.db
	if _, err := RequireUserID(ctx); err != nil {
		return nil, err
	}
	var messages []ChatMessage
	err := scopedMessageQuery(ctx, db.Model(&ChatMessage{})).
		Where("chat_messages.conversation_id = ?", conversationID).
		Order("chat_messages.created_at ASC, chat_messages.id ASC").
		Find(&messages).Error
	return messages, err
}

// CountChildrenWithContext retorna a contagem de filhos para cada mensagem do
// usuário do contexto.
func CountChildrenWithContext(ctx context.Context, messageIDs []string) (map[string]int, error) {
	return NewMessageRepository(db).CountChildrenWithContext(ctx, messageIDs)
}

func (r *MessageRepository) CountChildrenWithContext(ctx context.Context, messageIDs []string) (map[string]int, error) {
	db := r.db
	if _, err := RequireUserID(ctx); err != nil {
		return nil, err
	}
	if len(messageIDs) == 0 {
		return make(map[string]int), nil
	}

	type countResult struct {
		ParentID string
		Count    int
	}

	var results []countResult
	err := scopedMessageQuery(ctx, db.Model(&ChatMessage{})).
		Select("chat_messages.parent_id, COUNT(*) as count").
		Where("chat_messages.parent_id IN ?", messageIDs).
		Group("chat_messages.parent_id").
		Scan(&results).Error

	if err != nil {
		return nil, err
	}

	counts := make(map[string]int)
	for _, r := range results {
		counts[r.ParentID] = r.Count
	}

	return counts, nil
}

// GetMessageTreeWithContext retorna uma mensagem do usuário do contexto com
// todos os seus descendentes.
// GetMessageTreeWithContext retorna a mensagem raiz e todos os descendentes
// pertencentes ao usuário do contexto.
//
// SECURITY: fail-closed (AEP-0052 / B11). Sem userID no ctx retorna
// ErrUserScopeRequired — sem isso, scopedMessageQuery passa fail-open via
// ScopeByUser e qualquer messageID poderia ser usado para enumerar
// estruturas de conversas alheias.
func GetMessageTreeWithContext(ctx context.Context, messageID string) (*ChatMessage, []ChatMessage, error) {
	return NewMessageRepository(db).GetMessageTreeWithContext(ctx, messageID)
}

func (r *MessageRepository) GetMessageTreeWithContext(ctx context.Context, messageID string) (*ChatMessage, []ChatMessage, error) {
	db := r.db
	if _, err := RequireUserID(ctx); err != nil {
		return nil, nil, err
	}
	var message ChatMessage
	if err := scopedMessageQuery(ctx, db.Model(&ChatMessage{})).
		First(&message, "chat_messages.id = ?", messageID).Error; err != nil {
		return nil, nil, err
	}

	descendants, err := r.getDescendantsWithContext(ctx, messageID)
	if err != nil {
		return nil, nil, err
	}

	return &message, descendants, nil
}

// maxDescendantDepth limita a profundidade da CTE recursiva como guarda contra
// ciclos de parent_id (que não devem existir, mas a recursão precisa ser
// segura — issue #21).
const maxDescendantDepth = 10000

// descendantsCTE busca, em UMA única consulta, todos os descendentes de
// parentID via CTE recursiva do SQLite. O escopo por usuário é preservado com
// JOIN em conversations + filtro user_id em cada passo da recursão, espelhando
// scopedMessageQuery. A coluna auxiliar __depth serve apenas de guarda contra
// ciclos e é ignorada ao escanear para ChatMessage.
const descendantsCTE = `
WITH RECURSIVE descendants AS (
	SELECT cm.*, 0 AS __depth
	FROM chat_messages cm
	JOIN conversations c ON c.id = cm.conversation_id
	WHERE cm.parent_id = ? AND c.user_id = ?
	UNION ALL
	SELECT cm.*, d.__depth + 1 AS __depth
	FROM chat_messages cm
	JOIN conversations c ON c.id = cm.conversation_id
	JOIN descendants d ON cm.parent_id = d.id
	WHERE c.user_id = ? AND d.__depth < ?
)
SELECT * FROM descendants`

// getDescendantsWithContext retorna todos os descendentes de parentID na ordem
// de uma travessia em pré-ordem (DFS), com os filhos de cada nível ordenados
// por created_at ASC e id ASC como desempate determinístico.
//
// Antes (issue #21) a obtenção era uma recursão em Go que emitia uma query por
// nível (N queries em hierarquias profundas). Agora uma única CTE recursiva
// traz todos os registros do banco e a ordenação hierárquica é reconstruída em
// memória, reproduzindo o comportamento anterior sem o custo de N consultas.
func (r *MessageRepository) getDescendantsWithContext(ctx context.Context, parentID string) ([]ChatMessage, error) {
	userID, err := RequireUserID(ctx)
	if err != nil {
		return nil, err
	}

	var rows []ChatMessage
	// Usa o db injetado (r.db) para preservar a consistência transacional
	// quando o repositório é construído com um tx (AEP-0040), em vez da
	// global db.
	if err := r.db.WithContext(ctx).
		Raw(descendantsCTE, parentID, userID, userID, maxDescendantDepth).
		Scan(&rows).Error; err != nil {
		return nil, err
	}

	// Agrupa por parent_id e ordena cada grupo de irmãos (created_at ASC,
	// id ASC como desempate) para uma travessia determinística.
	childrenByParent := make(map[string][]ChatMessage, len(rows))
	for _, row := range rows {
		if row.ParentID == nil {
			continue
		}
		childrenByParent[*row.ParentID] = append(childrenByParent[*row.ParentID], row)
	}
	for pid := range childrenByParent {
		siblings := childrenByParent[pid]
		sort.SliceStable(siblings, func(i, j int) bool {
			if siblings[i].CreatedAt.Equal(siblings[j].CreatedAt) {
				return siblings[i].ID < siblings[j].ID
			}
			return siblings[i].CreatedAt.Before(siblings[j].CreatedAt)
		})
	}

	// Travessia em pré-ordem a partir de parentID. O conjunto visited evita
	// loop caso exista um ciclo de parent_id entre os registros retornados.
	ordered := make([]ChatMessage, 0, len(rows))
	visited := make(map[string]bool, len(rows))
	var walk func(parent string)
	walk = func(parent string) {
		for _, child := range childrenByParent[parent] {
			if visited[child.ID] {
				continue
			}
			visited[child.ID] = true
			ordered = append(ordered, child)
			walk(child.ID)
		}
	}
	walk(parentID)

	return ordered, nil
}

// GetMessagesAfterIDWithContext retorna mensagens raiz de uma conversa do
// usuário do contexto, criadas após a mensagem afterID. Usa posição na lista
// ordenada por created_at em vez de comparação lexicográfica de IDs, evitando
// problemas com UUIDs gerados no mesmo milissegundo. Se afterID for vazio,
// retorna todas as mensagens raiz.
//
// SECURITY: fail-closed (AEP-0052). Sem userID no ctx retorna
// ErrUserScopeRequired.
func GetMessagesAfterIDWithContext(ctx context.Context, conversationID string, afterID string) ([]ChatMessage, error) {
	return NewMessageRepository(db).GetMessagesAfterIDWithContext(ctx, conversationID, afterID)
}

func (r *MessageRepository) GetMessagesAfterIDWithContext(ctx context.Context, conversationID string, afterID string) ([]ChatMessage, error) {
	db := r.db
	if _, err := RequireUserID(ctx); err != nil {
		return nil, err
	}
	var messages []ChatMessage
	err := scopedMessageQuery(ctx, db.Model(&ChatMessage{})).
		Where("chat_messages.conversation_id = ? AND chat_messages.parent_id IS NULL", conversationID).
		Order("chat_messages.created_at ASC").Find(&messages).Error
	if err != nil {
		return nil, err
	}
	if afterID == "" {
		return messages, nil
	}
	for i, m := range messages {
		if m.ID == afterID {
			return messages[i+1:], nil
		}
	}
	// afterID não encontrado: retorna todas
	return messages, nil
}

// GetMessagesBetweenIDsWithContext retorna mensagens raiz do usuário do
// contexto criadas após startAfterID até endID (inclusive). Usa posição na
// lista ordenada por created_at em vez de comparação lexicográfica de IDs.
//
// SECURITY: fail-closed (AEP-0052 / B11). Sem userID = ErrUserScopeRequired.
func GetMessagesBetweenIDsWithContext(ctx context.Context, conversationID string, startAfterID string, endID string) ([]ChatMessage, error) {
	return NewMessageRepository(db).GetMessagesBetweenIDsWithContext(ctx, conversationID, startAfterID, endID)
}

func (r *MessageRepository) GetMessagesBetweenIDsWithContext(ctx context.Context, conversationID string, startAfterID string, endID string) ([]ChatMessage, error) {
	db := r.db
	if _, err := RequireUserID(ctx); err != nil {
		return nil, err
	}
	var messages []ChatMessage
	err := scopedMessageQuery(ctx, db.Model(&ChatMessage{})).
		Where("chat_messages.conversation_id = ? AND chat_messages.parent_id IS NULL", conversationID).
		Order("chat_messages.created_at ASC").Find(&messages).Error
	if err != nil {
		return nil, err
	}
	startIdx := 0
	if startAfterID != "" {
		for i, m := range messages {
			if m.ID == startAfterID {
				startIdx = i + 1
				break
			}
		}
	}
	var result []ChatMessage
	for _, m := range messages[startIdx:] {
		result = append(result, m)
		if m.ID == endID {
			break
		}
	}
	return result, nil
}
