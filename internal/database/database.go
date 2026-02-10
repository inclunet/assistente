package database

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"assistente/internal/configdir"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

var db *gorm.DB

// ErrConversationDeleted é retornado quando se tenta salvar mensagem em conversa que foi deletada
// Os chamadores devem verificar esse erro e abortar o processamento graciosamente
var ErrConversationDeleted = errors.New("conversa foi deletada")

// ErrParentMessageDeleted é retornado quando se tenta criar mensagem com parentId que não existe mais
// Isso acontece quando a conversa foi limpa (clear) - as mensagens foram deletadas mas a conversa ainda existe
var ErrParentMessageDeleted = errors.New("mensagem pai foi deletada")

// DB retorna a instância do banco de dados
func DB() *gorm.DB {
	return db
}

// Close fecha a conexão com o banco de dados
func Close() error {
	if db == nil {
		return nil
	}

	sqlDB, err := db.DB()
	if err != nil {
		return err
	}

	return sqlDB.Close()
}

// Init inicializa o banco de dados
// Resolve conversations.db nos 3 diretórios (exe > home > workdir).
// Se não existir em nenhum, cria em ~/.assistente/
func Init() error {
	rootResolver := configdir.NewResolver("")

	var dbPath string
	resolved, err := rootResolver.Resolve("conversations.db")
	if err != nil {
		// Não existe em nenhum diretório — criar no home
		if err := rootResolver.EnsureHomeDir(); err != nil {
			return err
		}
		dbPath = filepath.Join(configdir.GetHomeDir(), "conversations.db")
	} else {
		dbPath = resolved.Path
	}

	db, err = gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		return err
	}

	// Ativa modo WAL para melhor performance com arquivos grandes
	db.Exec("PRAGMA journal_mode=WAL")
	db.Exec("PRAGMA synchronous=NORMAL")

	// Auto migrate - apenas tabelas de conversas, mensagens e abas
	// Perfis agora são gerenciados via arquivos JSON em .assistente/profiles/
	if err := db.AutoMigrate(
		&Conversation{},
		&ChatMessage{},
		&ChatTab{},
	); err != nil {
		return err
	}

	return nil
}

// ==================== Conversation ====================

// CreateConversation cria uma nova conversa
func CreateConversation(title, model string) (*Conversation, error) {
	conv := &Conversation{
		Title: title,
	}

	if err := db.Create(conv).Error; err != nil {
		return nil, err
	}
	return conv, nil
}

// GetConversations retorna todas as conversas ordenadas por data
func GetConversations() ([]Conversation, error) {
	var conversations []Conversation

	// Usa subquery para contar mensagens em uma única query (evita N+1)
	err := db.Table("conversations").
		Select("conversations.*, COALESCE(msg_counts.count, 0) as message_count").
		Joins("LEFT JOIN (SELECT conversation_id, COUNT(*) as count FROM chat_messages GROUP BY conversation_id) as msg_counts ON msg_counts.conversation_id = conversations.id").
		Order("conversations.updated_at DESC").
		Find(&conversations).Error

	if err != nil {
		return nil, err
	}

	return conversations, nil
}

// GetConversation retorna uma conversa com suas mensagens
// Deprecated: Use GetConversationInfo + GetMessages for lazy loading
func GetConversation(id uint) (*Conversation, error) {
	var conv Conversation
	err := db.Preload("Messages", func(db *gorm.DB) *gorm.DB {
		return db.Order("created_at ASC")
	}).First(&conv, id).Error
	if err != nil {
		return nil, err
	}
	return &conv, nil
}

// GetConversationInfo retorna apenas metadados da conversa (sem mensagens)
func GetConversationInfo(id uint) (*Conversation, error) {
	var conv Conversation
	err := db.First(&conv, id).Error
	if err != nil {
		return nil, err
	}
	return &conv, nil
}

// UpdateConversation atualiza título da conversa
func UpdateConversation(id uint, title, model string) error {
	updates := map[string]interface{}{
		"title":      title,
		"updated_at": time.Now(),
	}

	return db.Model(&Conversation{}).Where("id = ?", id).Updates(updates).Error
}

// DeleteConversation deleta uma conversa e suas mensagens
func DeleteConversation(id uint) error {
	if err := db.Where("conversation_id = ?", id).Delete(&ChatMessage{}).Error; err != nil {
		return err
	}
	return db.Delete(&Conversation{}, id).Error
}

// ==================== ChatMessage ====================

// MessageOptions contém opções para criar uma mensagem
type MessageOptions struct {
	ConversationID   uint
	ParentID         *uint  // ID da mensagem pai (define hierarquia)
	Role             string // user, assistant, tool, system
	Content          string
	Reasoning        string // Reasoning/thinking do modelo
	Media            string // JSON com mídias
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	Model            string
}

// CreateMessage cria uma mensagem com todas as opções disponíveis
func CreateMessage(opts MessageOptions) (*ChatMessage, error) {
	// Verifica se a conversa ainda existe antes de criar a mensagem
	var conv Conversation
	if err := db.First(&conv, opts.ConversationID).Error; err != nil {
		return nil, fmt.Errorf("%w: conversa %d", ErrConversationDeleted, opts.ConversationID)
	}

	// Verifica se a mensagem pai existe (se parentId foi fornecido)
	if opts.ParentID != nil && *opts.ParentID > 0 {
		var parentMsg ChatMessage
		if err := db.First(&parentMsg, *opts.ParentID).Error; err != nil {
			return nil, fmt.Errorf("%w: mensagem %d", ErrParentMessageDeleted, *opts.ParentID)
		}
	}

	msg := &ChatMessage{
		ConversationID:   opts.ConversationID,
		ParentID:         opts.ParentID,
		Role:             opts.Role,
		Content:          opts.Content,
		Reasoning:        opts.Reasoning,
		Media:            opts.Media,
		PromptTokens:     opts.PromptTokens,
		CompletionTokens: opts.CompletionTokens,
		TotalTokens:      opts.TotalTokens,
		Model:            opts.Model,
	}
	if err := db.Create(msg).Error; err != nil {
		return nil, err
	}
	db.Model(&Conversation{}).Where("id = ?", opts.ConversationID).Update("updated_at", time.Now())
	return msg, nil
}

// AddMessage adiciona uma mensagem simples (sem parent - nível 0)
func AddMessage(conversationID uint, role, content string) (*ChatMessage, error) {
	return CreateMessage(MessageOptions{
		ConversationID: conversationID,
		Role:           role,
		Content:        content,
	})
}

// AddMessageWithMedia adiciona uma mensagem com mídias (sem parent - nível 0)
func AddMessageWithMedia(conversationID uint, role, content, media string) (*ChatMessage, error) {
	return CreateMessage(MessageOptions{
		ConversationID: conversationID,
		Role:           role,
		Content:        content,
		Media:          media,
	})
}

// AddMessageWithTokens adiciona uma mensagem com informações de tokens
func AddMessageWithTokens(conversationID uint, role, content string, promptTokens, completionTokens, totalTokens int, model string) (*ChatMessage, error) {
	return CreateMessage(MessageOptions{
		ConversationID:   conversationID,
		Role:             role,
		Content:          content,
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		TotalTokens:      totalTokens,
		Model:            model,
	})
}

// AddMessageWithTokensAndMedia adiciona uma mensagem com mídias e informações de tokens
func AddMessageWithTokensAndMedia(conversationID uint, role, content, media string, promptTokens, completionTokens, totalTokens int, model string) (*ChatMessage, error) {
	return CreateMessage(MessageOptions{
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

// AddToolMessage adiciona uma mensagem de role="tool" (resposta de tool ao orquestrador)
func AddToolMessage(conversationID uint, content string) (*ChatMessage, error) {
	return CreateMessage(MessageOptions{
		ConversationID: conversationID,
		Role:           "tool",
		Content:        content,
	})
}

// AddChildMessage adiciona uma mensagem filha (com ParentID definido)
func AddChildMessage(conversationID uint, parentID uint, role, content, model string) (*ChatMessage, error) {
	return CreateMessage(MessageOptions{
		ConversationID: conversationID,
		ParentID:       &parentID,
		Role:           role,
		Content:        content,
		Model:          model,
	})
}

// UpdateMessageContent atualiza o conteúdo e tokens de uma mensagem existente
func UpdateMessageContent(messageID uint, content string, promptTokens, completionTokens, totalTokens int, model string) error {
	return db.Model(&ChatMessage{}).Where("id = ?", messageID).Updates(map[string]interface{}{
		"content":           content,
		"prompt_tokens":     promptTokens,
		"completion_tokens": completionTokens,
		"total_tokens":      totalTokens,
		"model":             model,
	}).Error
}

// DeleteMessage exclui uma mensagem e todas as suas filhas (respostas)
func DeleteMessage(messageID uint) error {
	var childIDs []uint
	if err := db.Model(&ChatMessage{}).Where("parent_id = ?", messageID).Pluck("id", &childIDs).Error; err != nil {
		return err
	}
	for _, childID := range childIDs {
		if err := DeleteMessage(childID); err != nil {
			return err
		}
	}
	return db.Delete(&ChatMessage{}, messageID).Error
}

// DeleteAllMessages remove todas as mensagens de uma conversa
func DeleteAllMessages(conversationID uint) error {
	return db.Where("conversation_id = ?", conversationID).Delete(&ChatMessage{}).Error
}

// GetMessages retorna mensagens de uma conversa com filtro opcional por parent
func GetMessages(conversationID uint, parentID *uint) ([]ChatMessage, error) {
	var messages []ChatMessage
	query := db.Order("created_at ASC")

	if parentID != nil {
		query = query.Where("parent_id = ?", *parentID)
	} else {
		if conversationID == 0 {
			return nil, fmt.Errorf("conversationID é obrigatório para buscar mensagens raiz")
		}
		query = query.Where("conversation_id = ? AND parent_id IS NULL", conversationID)
	}

	err := query.Find(&messages).Error
	return messages, err
}

// GetAllConversationMessages retorna todas as mensagens de uma conversa (incluindo filhas)
func GetAllConversationMessages(conversationID uint) ([]ChatMessage, error) {
	var messages []ChatMessage
	err := db.Where("conversation_id = ?", conversationID).Order("created_at ASC").Find(&messages).Error
	return messages, err
}

// CountChildren retorna a contagem de filhos para cada mensagem
func CountChildren(messageIDs []uint) (map[uint]int, error) {
	if len(messageIDs) == 0 {
		return make(map[uint]int), nil
	}

	type countResult struct {
		ParentID uint
		Count    int
	}

	var results []countResult
	err := db.Model(&ChatMessage{}).
		Select("parent_id, COUNT(*) as count").
		Where("parent_id IN ?", messageIDs).
		Group("parent_id").
		Scan(&results).Error

	if err != nil {
		return nil, err
	}

	counts := make(map[uint]int)
	for _, r := range results {
		counts[r.ParentID] = r.Count
	}

	return counts, nil
}

// GetMessageTree retorna uma mensagem com todos os seus descendentes
func GetMessageTree(messageID uint) (*ChatMessage, []ChatMessage, error) {
	var message ChatMessage
	if err := db.First(&message, messageID).Error; err != nil {
		return nil, nil, err
	}

	var descendants []ChatMessage
	if err := getDescendants(messageID, &descendants); err != nil {
		return nil, nil, err
	}

	return &message, descendants, nil
}

func getDescendants(parentID uint, descendants *[]ChatMessage) error {
	var children []ChatMessage
	if err := db.Where("parent_id = ?", parentID).Order("created_at ASC").Find(&children).Error; err != nil {
		return err
	}
	for _, child := range children {
		*descendants = append(*descendants, child)
		if err := getDescendants(child.ID, descendants); err != nil {
			return err
		}
	}
	return nil
}

// GetConversationTokenStats retorna estatísticas de tokens de uma conversa
func GetConversationTokenStats(conversationID uint) (map[string]int, error) {
	var result struct {
		TotalPromptTokens     int
		TotalCompletionTokens int
		TotalTokens           int
	}
	err := db.Model(&ChatMessage{}).
		Where("conversation_id = ?", conversationID).
		Select("SUM(prompt_tokens) as total_prompt_tokens, SUM(completion_tokens) as total_completion_tokens, SUM(total_tokens) as total_tokens").
		Scan(&result).Error
	if err != nil {
		return nil, err
	}
	return map[string]int{
		"prompt_tokens":     result.TotalPromptTokens,
		"completion_tokens": result.TotalCompletionTokens,
		"total_tokens":      result.TotalTokens,
	}, nil
}

// GetAllTokenStats retorna estatísticas de tokens de todas as conversas
func GetAllTokenStats() (map[string]int, error) {
	var result struct {
		TotalPromptTokens     int
		TotalCompletionTokens int
		TotalTokens           int
	}
	err := db.Model(&ChatMessage{}).
		Select("SUM(prompt_tokens) as total_prompt_tokens, SUM(completion_tokens) as total_completion_tokens, SUM(total_tokens) as total_tokens").
		Scan(&result).Error
	if err != nil {
		return nil, err
	}
	return map[string]int{
		"prompt_tokens":     result.TotalPromptTokens,
		"completion_tokens": result.TotalCompletionTokens,
		"total_tokens":      result.TotalTokens,
	}, nil
}

// ==================== Chat Tab ====================

// CreateChatTab cria uma nova aba de chat
func CreateChatTab(conversationID *uint, title, icon string, position int) (*ChatTab, error) {
	tab := &ChatTab{
		ConversationID: conversationID,
		Title:          title,
		Icon:           icon,
		Position:       position,
		IsActive:       false,
	}
	if err := db.Create(tab).Error; err != nil {
		return nil, err
	}
	return tab, nil
}

// GetChatTab retorna uma aba por ID
func GetChatTab(id uint) (*ChatTab, error) {
	var tab ChatTab
	err := db.Preload("Conversation").First(&tab, id).Error
	if err != nil {
		return nil, err
	}
	return &tab, nil
}

// ==================== Utilities ====================

// GenerateTitle gera um título baseado na primeira mensagem
func GenerateTitle(content string) string {
	if len(content) > 50 {
		return content[:50] + "..."
	}
	if len(content) == 0 {
		return "Nova conversa"
	}
	return content
}

// SearchConversations busca conversas por título
func SearchConversations(query string) ([]Conversation, error) {
	var conversations []Conversation
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return GetConversations()
	}
	searchTerm := "%" + query + "%"
	err := db.Where("LOWER(title) LIKE ?", searchTerm).Order("updated_at DESC").Find(&conversations).Error
	return conversations, err
}
