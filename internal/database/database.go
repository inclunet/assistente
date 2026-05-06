package database

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"time"

	"assistente/internal/configdir"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var db *gorm.DB

// gormLogLevel controla o nível de log do GORM. Padrão: Warn.
// Use SetLogLevel(logger.Silent) para silenciar completamente (ex.: CLI sem --verbose).
var gormLogLevel = logger.Warn

// SetLogLevel define o nível de log do GORM antes de Init().
func SetLogLevel(level logger.LogLevel) {
	gormLogLevel = level
}

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

// SetDB define a instância do banco de dados (usado em testes)
func SetDB(database *gorm.DB) {
	db = database
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

	db, err = gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(gormLogLevel),
	})
	if err != nil {
		return err
	}

	// Ativa modo WAL para melhor performance com arquivos grandes
	db.Exec("PRAGMA journal_mode=WAL")
	db.Exec("PRAGMA synchronous=NORMAL")

	// Migração: converter IDs INTEGER → UUIDv7 (AEP-0046)
	if err := migrateToUUIDv7(); err != nil {
		return fmt.Errorf("erro na migração UUIDv7: %w", err)
	}

	// Auto migrate - apenas tabelas de conversas, mensagens e abas
	// Perfis agora são gerenciados via arquivos JSON em .assistente/profiles/
	if err := db.AutoMigrate(
		&Conversation{},
		&ChatMessage{},
		&CredentialEntry{},
		&CredentialKeyWrap{},
		&LLMProvider{},
		&TaskListWorkflow{},
		&TaskList{},
		&Task{},
		&TaskNote{},
	); err != nil {
		return err
	}

	ensureTaskNoteExternalUniqueIndex()
	ensureTaskListSlugUniqueIndex()
	ensureChatMessageWindowIndex()

	// Normalizar campos booleanos: SQLite armazena bool como INTEGER 0/1,
	// mas valores corrompidos (ex: 4) causam erro no GORM Scan.
	db.Exec(`UPDATE conversations SET summarizing_in_progress = CASE WHEN summarizing_in_progress > 0 THEN 1 ELSE 0 END WHERE summarizing_in_progress NOT IN (0, 1)`)

	// Migração: mover refresh_url → refresh_token_enc (coluna renomeada)
	if db.Migrator().HasColumn(&CredentialEntry{}, "refresh_url") {
		db.Exec(`UPDATE credential_entries SET refresh_token_enc = refresh_url WHERE refresh_url != '' AND (refresh_token_enc IS NULL OR refresh_token_enc = '')`)
		_ = db.Migrator().DropColumn(&CredentialEntry{}, "refresh_url")
	}

	// Inicializa FTS5 (full-text search) para busca em mensagens
	if err := initFTS5(); err != nil {
		return fmt.Errorf("erro ao inicializar FTS5: %w", err)
	}

	// Verifica se o índice FTS5 está desatualizado e precisa de rebuild
	sqlDB, err := db.DB()
	if err == nil {
		var ftsCount, msgCount int
		_ = sqlDB.QueryRow(`SELECT count(*) FROM chat_messages_fts`).Scan(&ftsCount)
		_ = sqlDB.QueryRow(`SELECT count(*) FROM chat_messages WHERE role IN ('user','assistant') AND content != ''`).Scan(&msgCount)
		if msgCount > 0 && ftsCount < msgCount {
			log.Printf("[Database] Índice FTS5 desatualizado (%d/%d), reconstruindo...", ftsCount, msgCount)
			if err := RebuildFTSIndex(); err != nil {
				log.Printf("[Database] ERRO: falha ao reconstruir FTS5 — busca de histórico pode estar incompleta. Será retentado no próximo startup. Erro: %v", err)
			} else {
				log.Printf("[Database] Índice FTS5 reconstruído (%d mensagens)", msgCount)
			}
		}
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

// RecycleOrCreateConversation busca uma conversa vazia (0 mensagens, sem canal,
// não vinculada a nenhuma tab aberta) e a recicla, resetando título e timestamps.
// Se não encontrar candidata, cria uma nova. Evita acumular registros orfãos no banco.
func RecycleOrCreateConversation(title string) (*Conversation, error) {
	var candidate Conversation
	err := db.
		Where("channel = '' AND contact_id = ''").
		Where("id NOT IN (?)",
			db.Model(&ChatMessage{}).Select("DISTINCT conversation_id"),
		).
		Order("created_at ASC").
		First(&candidate).Error

	if err == nil {
		now := time.Now()
		candidate.Title = title
		candidate.Summary = ""
		candidate.SummaryUpToMessageID = ""
		candidate.SummarizingInProgress = false
		candidate.CreatedAt = now
		candidate.UpdatedAt = now
		if err := db.Save(&candidate).Error; err != nil {
			return nil, err
		}
		return &candidate, nil
	}

	return CreateConversation(title, "")
}

// FindOrCreateChannelConversation busca uma conversa existente para um canal+contato.
// Se não existir, cria uma nova. Retorna a conversa e se foi criada (true) ou encontrada (false).
func FindOrCreateChannelConversation(channel, contactID, contactName string) (*Conversation, bool, error) {
	var conv Conversation
	err := db.Where("channel = ? AND contact_id = ?", channel, contactID).First(&conv).Error
	if err == nil {
		return &conv, false, nil
	}

	// Cria nova conversa dedicada para este contato
	title := contactName
	if title == "" {
		title = contactID
	}
	conv = Conversation{
		Title:     title,
		Channel:   channel,
		ContactID: contactID,
	}
	if err := db.Create(&conv).Error; err != nil {
		return nil, false, err
	}
	return &conv, true, nil
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
func GetConversation(id string) (*Conversation, error) {
	var conv Conversation
	err := db.Preload("Messages", func(db *gorm.DB) *gorm.DB {
		return db.Order("created_at ASC")
	}).First(&conv, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &conv, nil
}

// GetConversationInfo retorna apenas metadados da conversa (sem mensagens)
func GetConversationInfo(id string) (*Conversation, error) {
	var conv Conversation
	err := db.First(&conv, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &conv, nil
}

// UpdateConversation atualiza título da conversa
func UpdateConversation(id string, title, model string) error {
	updates := map[string]interface{}{
		"title":      title,
		"updated_at": time.Now(),
	}

	return db.Model(&Conversation{}).Where("id = ?", id).Updates(updates).Error
}

// UpdateConversationChannel atualiza o canal e contato vinculados a uma conversa.
// Passar channel="" e contactID="" desvincula a conversa do canal.
func UpdateConversationChannel(id string, channel, contactID string) error {
	return db.Model(&Conversation{}).Where("id = ?", id).Updates(map[string]interface{}{
		"channel":    channel,
		"contact_id": contactID,
		"updated_at": time.Now(),
	}).Error
}

// DeleteConversation deleta uma conversa e suas mensagens
func DeleteConversation(id string) error {
	if err := db.Where("conversation_id = ?", id).Delete(&ChatMessage{}).Error; err != nil {
		return err
	}
	return db.Where("id = ?", id).Delete(&Conversation{}).Error
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
	Model            string
	Source           string // Origem da mensagem: "wails", "telegram", "signal", etc.
}

// CreateMessage cria uma mensagem com todas as opções disponíveis
func CreateMessage(opts MessageOptions) (*ChatMessage, error) {
	// Verifica se a conversa ainda existe antes de criar a mensagem
	var conv Conversation
	if err := db.First(&conv, "id = ?", opts.ConversationID).Error; err != nil {
		return nil, fmt.Errorf("%w: conversa %s", ErrConversationDeleted, opts.ConversationID)
	}

	// Verifica se a mensagem pai existe (se parentId foi fornecido)
	if opts.ParentID != nil && *opts.ParentID != "" {
		var parentMsg ChatMessage
		if err := db.First(&parentMsg, "id = ?", *opts.ParentID).Error; err != nil {
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
		Model:            opts.Model,
		Source:           opts.Source,
	}
	if err := db.Create(msg).Error; err != nil {
		return nil, err
	}
	db.Model(&Conversation{}).Where("id = ?", opts.ConversationID).Update("updated_at", time.Now())
	return msg, nil
}

// AddMessage adiciona uma mensagem simples (sem parent - nível 0)
func AddMessage(conversationID string, role, content string) (*ChatMessage, error) {
	return CreateMessage(MessageOptions{
		ConversationID: conversationID,
		Role:           role,
		Content:        content,
	})
}

// AddMessageWithMedia adiciona uma mensagem com mídias (sem parent - nível 0)
func AddMessageWithMedia(conversationID string, role, content, media string) (*ChatMessage, error) {
	return CreateMessage(MessageOptions{
		ConversationID: conversationID,
		Role:           role,
		Content:        content,
		Media:          media,
	})
}

// AddMessageWithTokens adiciona uma mensagem com informações de tokens
func AddMessageWithTokens(conversationID string, role, content string, promptTokens, completionTokens, totalTokens int, model string) (*ChatMessage, error) {
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
func AddMessageWithTokensAndMedia(conversationID string, role, content, media string, promptTokens, completionTokens, totalTokens int, model string) (*ChatMessage, error) {
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

// GetMessageAudio retorna o áudio base64 e MIME de uma mensagem.
// Retorna ("", "", nil) se a mensagem não tem áudio.
func GetMessageAudio(messageID string) (string, string, error) {
	var msg ChatMessage
	if err := db.Select("audio", "audio_mime_type").First(&msg, "id = ?", messageID).Error; err != nil {
		return "", "", err
	}
	return msg.Audio, msg.AudioMimeType, nil
}

// SaveMessageAudio salva áudio (base64) numa mensagem existente.
func SaveMessageAudio(messageID string, audioBase64 string, mimeType string) error {
	return db.Model(&ChatMessage{}).Where("id = ?", messageID).Updates(map[string]interface{}{
		"audio":           audioBase64,
		"audio_mime_type": mimeType,
	}).Error
}

// HasMessageAudio verifica se uma mensagem tem áudio salvo.
func HasMessageAudio(messageID string) bool {
	var count int64
	db.Model(&ChatMessage{}).Where("id = ? AND audio != '' AND audio IS NOT NULL", messageID).Count(&count)
	return count > 0
}

// GetMessageContent retorna o conteúdo textual de uma mensagem.
func GetMessageContent(messageID string) (string, error) {
	var msg ChatMessage
	if err := db.Select("content").First(&msg, "id = ?", messageID).Error; err != nil {
		return "", err
	}
	return msg.Content, nil
}

// GetMessage retorna a mensagem completa pelo ID.
func GetMessage(messageID string) (*ChatMessage, error) {
	var msg ChatMessage
	if err := db.First(&msg, "id = ?", messageID).Error; err != nil {
		return nil, err
	}
	return &msg, nil
}

// AddToolMessage adiciona uma mensagem de role="tool" (resposta de tool ao orquestrador)
func AddToolMessage(conversationID string, content string) (*ChatMessage, error) {
	return CreateMessage(MessageOptions{
		ConversationID: conversationID,
		Role:           "tool",
		Content:        content,
	})
}

// AddToolResultMessage adiciona uma mensagem de resultado de tool com TurnID e ToolCallID.
// Usado pelo agentic loop para salvar o resultado de uma execução de ferramenta.
func AddToolResultMessage(conversationID string, turnID string, content, toolCallID string) (*ChatMessage, error) {
	return CreateMessage(MessageOptions{
		ConversationID: conversationID,
		TurnID:         &turnID,
		Role:           "tool",
		Content:        content,
		ToolCallID:     toolCallID,
	})
}

// AddAssistantToolMessage adiciona uma mensagem do assistente que contém tool_calls.
// Usada quando o LLM responde com texto + pedidos de ferramentas.
func AddAssistantToolMessage(conversationID string, turnID string, content, toolCalls, reasoning, model string) (*ChatMessage, error) {
	return CreateMessage(MessageOptions{
		ConversationID: conversationID,
		TurnID:         &turnID,
		Role:           "assistant",
		Content:        content,
		ToolCalls:      toolCalls,
		Reasoning:      reasoning,
		Model:          model,
	})
}

// GetTurnMessages retorna todas as mensagens de um turno (mesmo TurnID), ordenadas por criação.
func GetTurnMessages(turnID string) ([]ChatMessage, error) {
	var messages []ChatMessage
	err := db.Where("turn_id = ?", turnID).Order("created_at ASC, id ASC").Find(&messages).Error
	return messages, err
}

// GetMessagesByTurnID retorna mensagens de um turno específico.
// Mantém o mesmo escopo de parent da janela para não misturar raiz e threads.
func GetMessagesByTurnID(conversationID string, parentID *string, turnID string, limit int) ([]ChatMessage, error) {
	if turnID == "" {
		return []ChatMessage{}, nil
	}
	query := db.Where("conversation_id = ? AND turn_id = ?", conversationID, turnID)
	if parentID != nil {
		query = query.Where("parent_id = ?", *parentID)
	} else {
		query = query.Where("parent_id IS NULL")
	}
	if limit > 0 {
		query = query.Limit(limit)
	}
	var messages []ChatMessage
	err := query.Order("created_at ASC, id ASC").Find(&messages).Error
	return messages, err
}

// AddChildMessage adiciona uma mensagem filha (com ParentID definido)
func AddChildMessage(conversationID string, parentID string, role, content, model string) (*ChatMessage, error) {
	return CreateMessage(MessageOptions{
		ConversationID: conversationID,
		ParentID:       &parentID,
		Role:           role,
		Content:        content,
		Model:          model,
	})
}

// UpdateMessageContent atualiza o conteúdo e tokens de uma mensagem existente
func UpdateMessageContent(messageID string, content string, promptTokens, completionTokens, totalTokens int, model string) error {
	return db.Model(&ChatMessage{}).Where("id = ?", messageID).Updates(map[string]interface{}{
		"content":           content,
		"prompt_tokens":     promptTokens,
		"completion_tokens": completionTokens,
		"total_tokens":      totalTokens,
		"model":             model,
	}).Error
}

// DeleteMessage exclui uma mensagem e todas as suas filhas (respostas)
func DeleteMessage(messageID string) error {
	var childIDs []string
	if err := db.Model(&ChatMessage{}).Where("parent_id = ?", messageID).Pluck("id", &childIDs).Error; err != nil {
		return err
	}
	for _, childID := range childIDs {
		if err := DeleteMessage(childID); err != nil {
			return err
		}
	}
	return db.Where("id = ?", messageID).Delete(&ChatMessage{}).Error
}

// DeleteAllMessages remove todas as mensagens de uma conversa
func DeleteAllMessages(conversationID string) error {
	return db.Where("conversation_id = ?", conversationID).Delete(&ChatMessage{}).Error
}

func ClearAllConversations() error {
	if err := db.Where("1 = 1").Delete(&ChatMessage{}).Error; err != nil {
		return fmt.Errorf("erro ao limpar mensagens: %w", err)
	}
	if err := db.Where("1 = 1").Delete(&Conversation{}).Error; err != nil {
		return fmt.Errorf("erro ao limpar conversas: %w", err)
	}
	return nil
}

// GetMessages retorna mensagens de uma conversa com filtro opcional por parent
func GetMessages(conversationID string, parentID *string) ([]ChatMessage, error) {
	var messages []ChatMessage
	query := db.Order("created_at ASC, id ASC")

	if parentID != nil {
		query = query.Where("parent_id = ?", *parentID)
	} else {
		if conversationID == "" {
			return nil, fmt.Errorf("conversationID é obrigatório para buscar mensagens raiz")
		}
		query = query.Where("conversation_id = ? AND parent_id IS NULL", conversationID)
	}

	err := query.Find(&messages).Error
	return messages, err
}

// GetRecentRootMessages retorna as mensagens raiz mais recentes de uma conversa,
// preservando ordem cronológica no retorno.
func GetRecentRootMessages(conversationID string, limit int) ([]ChatMessage, error) {
	if conversationID == "" {
		return nil, fmt.Errorf("conversationID é obrigatório para buscar mensagens recentes")
	}
	if limit <= 0 {
		return []ChatMessage{}, nil
	}

	var messages []ChatMessage
	err := db.Where("conversation_id = ? AND parent_id IS NULL", conversationID).
		Order("created_at DESC, id DESC").
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

// GetRootMessagesBefore retorna mensagens raiz anteriores a beforeID,
// preservando ordem cronológica no retorno.
func GetRootMessagesBefore(conversationID string, beforeID string, limit int) ([]ChatMessage, error) {
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
	if err := db.Select("id", "created_at").First(&before, "id = ? AND conversation_id = ?", beforeID, conversationID).Error; err != nil {
		return nil, err
	}

	var messages []ChatMessage
	err := db.Where(
		"conversation_id = ? AND parent_id IS NULL AND (created_at < ? OR (created_at = ? AND id < ?))",
		conversationID,
		before.CreatedAt,
		before.CreatedAt,
		before.ID,
	).
		Order("created_at DESC, id DESC").
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
	Messages   []ChatMessage
	TotalCount int
	StartIndex int
	EndIndex   int
	HasBefore  bool
	HasAfter   bool
}

const (
	MaxMessageWindowRows = 240

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

func messageScopeQuery(conversationID string, parentID *string) *gorm.DB {
	query := db.Model(&ChatMessage{}).Where("conversation_id = ?", conversationID)
	if parentID != nil {
		return query.Where("parent_id = ?", *parentID)
	}
	return query.Where("parent_id IS NULL")
}

func countMessagesBeforeAnchor(baseQuery *gorm.DB, anchor ChatMessage) (int, error) {
	var count int64
	err := baseQuery.
		Where("(created_at < ? OR (created_at = ? AND id < ?))", anchor.CreatedAt, anchor.CreatedAt, anchor.ID).
		Count(&count).Error
	return int(count), err
}

func reverseChatMessages(messages []ChatMessage) {
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}
}

// GetMessageWindow retorna uma fatia ordenada de mensagens raiz ou filhos diretos,
// acompanhada de metadados absolutos para renderização acessível e navegação incremental.
func GetMessageWindow(query MessageWindowQuery) (*MessageWindowResult, error) {
	if query.ConversationID == "" {
		return nil, fmt.Errorf("conversationID é obrigatório para buscar janela de mensagens")
	}
	anchor, direction, err := normalizeMessageWindowCursor(query)
	if err != nil {
		return nil, err
	}

	limit := normalizeMessageWindowLimit(query.Limit)
	baseQuery := messageScopeQuery(query.ConversationID, query.ParentID)
	var totalCount int64
	if err := baseQuery.Count(&totalCount).Error; err != nil {
		return nil, err
	}
	if limit == 0 {
		return &MessageWindowResult{
			Messages:   []ChatMessage{},
			TotalCount: int(totalCount),
			StartIndex: 0,
			EndIndex:   -1,
			HasBefore:  false,
			HasAfter:   totalCount > 0,
		}, nil
	}
	if totalCount == 0 {
		return &MessageWindowResult{
			Messages:   []ChatMessage{},
			TotalCount: 0,
			StartIndex: 0,
			EndIndex:   -1,
		}, nil
	}

	total := int(totalCount)
	startIndex := 0
	windowLimit := limit
	var anchorMessage ChatMessage
	hasAnchorMessage := false

	switch {
	case query.AnchorMessageID != "":
		if err := db.
			Select("id", "conversation_id", "parent_id", "created_at").
			First(&anchorMessage, "id = ? AND conversation_id = ?", query.AnchorMessageID, query.ConversationID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, fmt.Errorf("anchorMessageId inválido: %s", query.AnchorMessageID)
			}
			return nil, err
		}
		hasAnchorMessage = true
		if query.ParentID == nil && anchorMessage.ParentID != nil {
			return nil, fmt.Errorf("anchorMessageId não pertence à janela raiz da conversa")
		}
		if query.ParentID != nil && (anchorMessage.ParentID == nil || *anchorMessage.ParentID != *query.ParentID) {
			return nil, fmt.Errorf("anchorMessageId não pertence à thread solicitada")
		}
		anchorIndex, err := countMessagesBeforeAnchor(messageScopeQuery(query.ConversationID, query.ParentID), anchorMessage)
		if err != nil {
			return nil, err
		}
		switch direction {
		case "after":
			startIndex = anchorIndex + 1
		case "around":
			startIndex = anchorIndex - (limit / 2)
		default:
			startIndex = anchorIndex - limit
			if anchorIndex < windowLimit {
				windowLimit = anchorIndex
			}
		}
	case anchor == "start" || direction == "after":
		startIndex = 0
	default:
		startIndex = total - limit
	}

	if startIndex < 0 {
		startIndex = 0
	}
	if startIndex > total {
		startIndex = total
	}
	if direction == "around" && windowLimit > 0 && startIndex+windowLimit > total {
		startIndex = total - windowLimit
		if startIndex < 0 {
			startIndex = 0
		}
	}
	if startIndex+windowLimit > total {
		windowLimit = total - startIndex
	}
	if windowLimit < 0 {
		windowLimit = 0
	}

	var messages []ChatMessage
	if windowLimit > 0 {
		var err error
		switch {
		case hasAnchorMessage && direction == "before":
			err = messageScopeQuery(query.ConversationID, query.ParentID).
				Where("(created_at < ? OR (created_at = ? AND id < ?))", anchorMessage.CreatedAt, anchorMessage.CreatedAt, anchorMessage.ID).
				Order("created_at DESC, id DESC").
				Limit(windowLimit).
				Find(&messages).Error
			reverseChatMessages(messages)
		case hasAnchorMessage && direction == "after":
			err = messageScopeQuery(query.ConversationID, query.ParentID).
				Where("(created_at > ? OR (created_at = ? AND id > ?))", anchorMessage.CreatedAt, anchorMessage.CreatedAt, anchorMessage.ID).
				Order("created_at ASC, id ASC").
				Limit(windowLimit).
				Find(&messages).Error
		case !hasAnchorMessage && (anchor == "start" || direction == "after"):
			err = messageScopeQuery(query.ConversationID, query.ParentID).
				Order("created_at ASC, id ASC").
				Limit(windowLimit).
				Find(&messages).Error
		case !hasAnchorMessage:
			err = messageScopeQuery(query.ConversationID, query.ParentID).
				Order("created_at DESC, id DESC").
				Limit(windowLimit).
				Find(&messages).Error
			reverseChatMessages(messages)
		default:
			err = messageScopeQuery(query.ConversationID, query.ParentID).
				Order("created_at ASC, id ASC").
				Offset(startIndex).
				Limit(windowLimit).
				Find(&messages).Error
		}
		if err != nil {
			return nil, err
		}
	}

	endIndex := startIndex + len(messages) - 1
	if len(messages) == 0 {
		endIndex = -1
	}

	return &MessageWindowResult{
		Messages:   messages,
		TotalCount: total,
		StartIndex: startIndex,
		EndIndex:   endIndex,
		HasBefore:  startIndex > 0,
		HasAfter:   (endIndex >= 0 && endIndex < total-1) || (len(messages) == 0 && startIndex < total),
	}, nil
}

// GetAllConversationMessages retorna todas as mensagens de uma conversa (incluindo filhas)
func GetAllConversationMessages(conversationID string) ([]ChatMessage, error) {
	var messages []ChatMessage
	err := db.Where("conversation_id = ?", conversationID).Order("created_at ASC, id ASC").Find(&messages).Error
	return messages, err
}

// CountChildren retorna a contagem de filhos para cada mensagem
func CountChildren(messageIDs []string) (map[string]int, error) {
	if len(messageIDs) == 0 {
		return make(map[string]int), nil
	}

	type countResult struct {
		ParentID string
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

	counts := make(map[string]int)
	for _, r := range results {
		counts[r.ParentID] = r.Count
	}

	return counts, nil
}

// GetMessageTree retorna uma mensagem com todos os seus descendentes
func GetMessageTree(messageID string) (*ChatMessage, []ChatMessage, error) {
	var message ChatMessage
	if err := db.First(&message, "id = ?", messageID).Error; err != nil {
		return nil, nil, err
	}

	var descendants []ChatMessage
	if err := getDescendants(messageID, &descendants); err != nil {
		return nil, nil, err
	}

	return &message, descendants, nil
}

func getDescendants(parentID string, descendants *[]ChatMessage) error {
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
func GetConversationTokenStats(conversationID string) (map[string]int, error) {
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

// TokenStats representa estatísticas detalhadas de tokens
type TokenStats struct {
	PromptTokens     int    `json:"prompt_tokens"`
	CompletionTokens int    `json:"completion_tokens"`
	TotalTokens      int    `json:"total_tokens"`
	MessageCount     int    `json:"message_count"`
	Model            string `json:"model,omitempty"`
}

// ToolUsageBreakdown detalha o uso de um tool específico
type ToolUsageBreakdown struct {
	ToolName              string `json:"tool_name"`
	CallCount             int    `json:"call_count"`
	TotalPromptTokens     int    `json:"total_prompt_tokens"`
	TotalCompletionTokens int    `json:"total_completion_tokens"`
	TotalTokens           int    `json:"total_tokens"`
}

// DetailedTokenStats fornece breakdown detalhado de tokens por categoria
type DetailedTokenStats struct {
	// Dados básicos da conversa
	PromptTokens     int    `json:"prompt_tokens"`
	CompletionTokens int    `json:"completion_tokens"`
	TotalTokens      int    `json:"total_tokens"`
	MessageCount     int    `json:"message_count"`
	Model            string `json:"model,omitempty"`

	// Breakdown de contexto (sistema + resumo + mensagens)
	SystemPromptEstimatedTokens int `json:"system_prompt_estimated_tokens"`
	SummaryTokens               int `json:"summary_tokens"`
	MessagesInContextCount      int `json:"messages_in_context_count"`
	MessagesInContextTokens     int `json:"messages_in_context_tokens"`
	MessagesOutOfContextCount   int `json:"messages_out_of_context_count"`
	MessagesOutOfContextTokens  int `json:"messages_out_of_context_tokens"`

	// Tool calling
	ToolsUsedCount int                  `json:"tools_used_count"`
	ToolBreakdown  []ToolUsageBreakdown `json:"tool_breakdown"`
}

// GetTurnTokenStats retorna estatísticas de tokens para um turno específico
func GetTurnTokenStats(conversationID string, turnID string) (*TokenStats, error) {
	var result struct {
		TotalPromptTokens     int
		TotalCompletionTokens int
		TotalTokens           int
		MessageCount          int
	}
	err := db.Model(&ChatMessage{}).
		Where("conversation_id = ? AND turn_id = ?", conversationID, turnID).
		Select("SUM(prompt_tokens) as total_prompt_tokens, SUM(completion_tokens) as total_completion_tokens, SUM(total_tokens) as total_tokens, COUNT(*) as message_count").
		Scan(&result).Error
	if err != nil {
		return nil, err
	}
	return &TokenStats{
		PromptTokens:     result.TotalPromptTokens,
		CompletionTokens: result.TotalCompletionTokens,
		TotalTokens:      result.TotalTokens,
		MessageCount:     result.MessageCount,
	}, nil
}

// GetConversationDetailedTokenStats retorna estatísticas detalhadas de tokens de uma conversa
func GetConversationDetailedTokenStats(conversationID string) (*TokenStats, error) {
	var result struct {
		TotalPromptTokens     int
		TotalCompletionTokens int
		TotalTokens           int
		MessageCount          int
	}
	err := db.Model(&ChatMessage{}).
		Where("conversation_id = ?", conversationID).
		Select("SUM(prompt_tokens) as total_prompt_tokens, SUM(completion_tokens) as total_completion_tokens, SUM(total_tokens) as total_tokens, COUNT(*) as message_count").
		Scan(&result).Error
	if err != nil {
		return nil, err
	}

	var mostUsedModel string
	db.Model(&ChatMessage{}).
		Where("conversation_id = ? AND model != ''", conversationID).
		Select("model").
		Group("model").
		Order("COUNT(*) DESC").
		Limit(1).
		Scan(&mostUsedModel)

	return &TokenStats{
		PromptTokens:     result.TotalPromptTokens,
		CompletionTokens: result.TotalCompletionTokens,
		TotalTokens:      result.TotalTokens,
		MessageCount:     result.MessageCount,
		Model:            mostUsedModel,
	}, nil
}

// GetDetailedTokenStats retorna agregação completa de tokens com breakdown por categoria
func GetDetailedTokenStats(conversationID string, summaryUpToMessageID string) (*DetailedTokenStats, error) {
	// 1. Dados básicos da conversa
	basicStats, err := GetConversationDetailedTokenStats(conversationID)
	if err != nil {
		return nil, err
	}

	// 2. Recuperar resumo (se houver)
	summaryTokens := 0
	summary, _, err := GetConversationSummary(conversationID)
	if err == nil && summary != "" {
		// Estima tokens do resumo: ~1 token a cada 4 caracteres
		summaryTokens = (len(summary) + 3) / 4
	}

	// 3. Contar mensagens in-context vs out-of-context
	// Usa índice na lista ordenada por created_at (como HistoryLoader.Load)
	// em vez de comparação lexicográfica de IDs, evitando problemas com
	// UUIDs gerados no mesmo milissegundo.
	var messagesInContextCount, messagesOutOfContextCount int
	var messagesInContextTokens, messagesOutOfContextTokens int

	if summaryUpToMessageID != "" {
		var allMessages []ChatMessage
		if err := db.Where("conversation_id = ? AND parent_id IS NULL", conversationID).
			Order("created_at ASC").
			Select("id, total_tokens").
			Find(&allMessages).Error; err == nil {

			cutIdx := -1
			for i, m := range allMessages {
				if m.ID == summaryUpToMessageID {
					cutIdx = i
					break
				}
			}

			if cutIdx >= 0 {
				// Out of context: mensagens até cutIdx (inclusive)
				for _, m := range allMessages[:cutIdx+1] {
					messagesOutOfContextCount++
					messagesOutOfContextTokens += m.TotalTokens
				}
				// In context: mensagens após cutIdx
				for _, m := range allMessages[cutIdx+1:] {
					messagesInContextCount++
					messagesInContextTokens += m.TotalTokens
				}
			} else {
				// summaryUpToMessageID não encontrado: tratar tudo como in-context
				messagesInContextCount = basicStats.MessageCount
				messagesInContextTokens = basicStats.TotalTokens
			}
		}
	} else {
		// Se não há sumarização, todas são in-context
		messagesInContextCount = basicStats.MessageCount
		messagesInContextTokens = basicStats.TotalTokens
	}

	// 4. Breakdown de tool usage
	toolBreakdown, toolsUsedCount := getToolUsageBreakdown(conversationID)

	// Estima tokens do system prompt: ~1 token a cada 4 caracteres
	// O DefaultSystemPrompt tem ~500 caracteres, então ~125 tokens
	systemPromptEstimatedTokens := 125

	return &DetailedTokenStats{
		PromptTokens:                basicStats.PromptTokens,
		CompletionTokens:            basicStats.CompletionTokens,
		TotalTokens:                 basicStats.TotalTokens,
		MessageCount:                basicStats.MessageCount,
		Model:                       basicStats.Model,
		SystemPromptEstimatedTokens: systemPromptEstimatedTokens,
		SummaryTokens:               summaryTokens,
		MessagesInContextCount:      messagesInContextCount,
		MessagesInContextTokens:     messagesInContextTokens,
		MessagesOutOfContextCount:   messagesOutOfContextCount,
		MessagesOutOfContextTokens:  messagesOutOfContextTokens,
		ToolsUsedCount:              toolsUsedCount,
		ToolBreakdown:               toolBreakdown,
	}, nil
}

// getToolUsageBreakdown extrai informações de uso de tools das mensagens
func getToolUsageBreakdown(conversationID string) ([]ToolUsageBreakdown, int) {
	var messages []ChatMessage
	db.Where("conversation_id = ? AND tool_calls != '' AND tool_calls IS NOT NULL", conversationID).
		Select("tool_calls, prompt_tokens, completion_tokens").
		Find(&messages)

	// Map para agregar tool usage
	toolMap := make(map[string]*ToolUsageBreakdown)

	for _, msg := range messages {
		if msg.ToolCalls == "" {
			continue
		}

		// Parse JSON das tool calls
		var toolCalls []map[string]interface{}
		err := json.Unmarshal([]byte(msg.ToolCalls), &toolCalls)
		if err != nil {
			continue
		}

		for _, toolCall := range toolCalls {
			if funcData, ok := toolCall["function"].(map[string]interface{}); ok {
				if toolName, ok := funcData["name"].(string); ok {
					if _, exists := toolMap[toolName]; !exists {
						toolMap[toolName] = &ToolUsageBreakdown{
							ToolName: toolName,
						}
					}
					toolMap[toolName].CallCount++
					// Distribuir tokens igualmente entre tools usados nessa mensagem
					toolCount := len(toolCalls)
					if toolCount > 0 {
						toolMap[toolName].TotalPromptTokens += msg.PromptTokens / toolCount
						toolMap[toolName].TotalCompletionTokens += msg.CompletionTokens / toolCount
					}
				}
			}
		}
	}

	// Converter map para slice
	var result []ToolUsageBreakdown
	for _, breakdown := range toolMap {
		breakdown.TotalTokens = breakdown.TotalPromptTokens + breakdown.TotalCompletionTokens
		result = append(result, *breakdown)
	}

	return result, len(toolMap)
}

// GetContextWindowUsage calcula a porcentagem de uso da janela de contexto
func GetContextWindowUsage(conversationID string, contextLimit int) (float64, int, error) {
	stats, err := GetConversationDetailedTokenStats(conversationID)
	if err != nil {
		return 0, 0, err
	}
	if contextLimit <= 0 {
		return 0, stats.TotalTokens, nil
	}
	percentage := (float64(stats.TotalTokens) / float64(contextLimit)) * 100
	return percentage, stats.TotalTokens, nil
}

// GetRecentMessagesTokenCount retorna o total de tokens das N mensagens mais recentes
func GetRecentMessagesTokenCount(conversationID string, messageLimit int) (int, error) {
	var totalTokens int
	err := db.Model(&ChatMessage{}).
		Where("conversation_id = ?", conversationID).
		Order("created_at DESC").
		Limit(messageLimit).
		Select("SUM(total_tokens)").
		Scan(&totalTokens).Error
	return totalTokens, err
}

// ==================== Rolling Context (Summary) ====================

// GetConversationSummary retorna o resumo e o ID da última mensagem resumida de uma conversa
func GetConversationSummary(conversationID string) (summary string, upToMessageID string, err error) {
	var conv Conversation
	err = db.Select("summary", "summary_up_to_message_id").First(&conv, "id = ?", conversationID).Error
	if err != nil {
		return "", "", err
	}
	return conv.Summary, conv.SummaryUpToMessageID, nil
}

// UpdateConversationSummary atualiza o resumo de uma conversa
func UpdateConversationSummary(conversationID string, summary string, upToMessageID string) error {
	return db.Model(&Conversation{}).Where("id = ?", conversationID).Updates(map[string]interface{}{
		"summary":                  summary,
		"summary_up_to_message_id": upToMessageID,
		"summarizing_in_progress":  false,
	}).Error
}

// SetSummarizingInProgress marca se uma sumarização está em andamento
func SetSummarizingInProgress(conversationID string, inProgress bool) error {
	return db.Model(&Conversation{}).Where("id = ?", conversationID).
		Update("summarizing_in_progress", inProgress).Error
}

// IsSummarizingInProgress verifica se há sumarização em andamento
func IsSummarizingInProgress(conversationID string) (bool, error) {
	var conv Conversation
	err := db.Select("summarizing_in_progress").First(&conv, "id = ?", conversationID).Error
	if err != nil {
		return false, err
	}
	return conv.SummarizingInProgress, nil
}

// GetMessagesAfterID retorna mensagens raiz de uma conversa criadas após a
// mensagem afterID. Usa posição na lista ordenada por created_at em vez de
// comparação lexicográfica de IDs, evitando problemas com UUIDs gerados no
// mesmo milissegundo. Se afterID for vazio, retorna todas as mensagens raiz.
func GetMessagesAfterID(conversationID string, afterID string) ([]ChatMessage, error) {
	var messages []ChatMessage
	err := db.Where("conversation_id = ? AND parent_id IS NULL", conversationID).
		Order("created_at ASC").Find(&messages).Error
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

// GetMessagesBetweenIDs retorna mensagens raiz criadas após startAfterID até
// endID (inclusive). Usa posição na lista ordenada por created_at em vez de
// comparação lexicográfica de IDs.
func GetMessagesBetweenIDs(conversationID string, startAfterID string, endID string) ([]ChatMessage, error) {
	var messages []ChatMessage
	err := db.Where("conversation_id = ? AND parent_id IS NULL", conversationID).
		Order("created_at ASC").Find(&messages).Error
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

// MessageSearchResult representa um resultado de busca no conteúdo de mensagens
type MessageSearchResult struct {
	ConversationID    string    `json:"conversation_id"`
	ConversationTitle string    `json:"conversation_title"`
	MessageID         string    `json:"message_id"`
	Role              string    `json:"role"`
	Snippet           string    `json:"snippet"`
	Rank              float64   `json:"rank"`
	CreatedAt         time.Time `json:"created_at"`
}

// initFTS5 cria a tabela FTS5 e triggers de sincronização.
// Idempotente — pode ser chamada múltiplas vezes sem efeito.
func initFTS5() error {
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("erro ao obter sql.DB: %w", err)
	}

	stmts := []string{
		// Tabela FTS5 virtual (content-sync externo via triggers)
		`CREATE VIRTUAL TABLE IF NOT EXISTS chat_messages_fts USING fts5(
			content,
			role UNINDEXED,
			conversation_id UNINDEXED,
			content='chat_messages',
			content_rowid='rowid',
			tokenize='unicode61 remove_diacritics 2'
		)`,

		// Trigger INSERT: indexa apenas user e assistant
		`CREATE TRIGGER IF NOT EXISTS chat_messages_fts_insert AFTER INSERT ON chat_messages
		WHEN NEW.role IN ('user', 'assistant')
		BEGIN
			INSERT INTO chat_messages_fts(rowid, content, role, conversation_id)
			VALUES (NEW.rowid, NEW.content, NEW.role, NEW.conversation_id);
		END`,

		// Trigger DELETE: remove do índice
		`CREATE TRIGGER IF NOT EXISTS chat_messages_fts_delete AFTER DELETE ON chat_messages
		WHEN OLD.role IN ('user', 'assistant')
		BEGIN
			INSERT INTO chat_messages_fts(chat_messages_fts, rowid, content, role, conversation_id)
			VALUES ('delete', OLD.rowid, OLD.content, OLD.role, OLD.conversation_id);
		END`,

		// Trigger UPDATE: atualiza no índice
		`CREATE TRIGGER IF NOT EXISTS chat_messages_fts_update AFTER UPDATE OF content ON chat_messages
		WHEN NEW.role IN ('user', 'assistant')
		BEGIN
			INSERT INTO chat_messages_fts(chat_messages_fts, rowid, content, role, conversation_id)
			VALUES ('delete', OLD.rowid, OLD.content, OLD.role, OLD.conversation_id);
			INSERT INTO chat_messages_fts(rowid, content, role, conversation_id)
			VALUES (NEW.rowid, NEW.content, NEW.role, NEW.conversation_id);
		END`,
	}

	for _, stmt := range stmts {
		if _, err := sqlDB.Exec(stmt); err != nil {
			return fmt.Errorf("erro FTS5 setup: %w\nSQL: %s", err, stmt)
		}
	}

	return nil
}

// RebuildFTSIndex reconstrói o índice FTS5 a partir das mensagens existentes.
// Limpa o índice e repovoa apenas com mensagens de user/assistant.
func RebuildFTSIndex() error {
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}

	// 'delete-all' é o comando seguro para limpar FTS5 external-content tables
	if _, err := sqlDB.Exec(`INSERT INTO chat_messages_fts(chat_messages_fts) VALUES('delete-all')`); err != nil {
		return fmt.Errorf("erro ao limpar FTS: %w", err)
	}

	if _, err := sqlDB.Exec(`
		INSERT INTO chat_messages_fts(rowid, content, role, conversation_id)
		SELECT rowid, content, role, conversation_id
		FROM chat_messages
		WHERE role IN ('user', 'assistant') AND content != ''
	`); err != nil {
		return fmt.Errorf("erro ao repopular FTS: %w", err)
	}

	return nil
}

// SearchMessageContent busca no conteúdo das mensagens de todas as conversas usando FTS5 + BM25.
// query suporta sintaxe FTS5: palavras, "frases exatas", prefixo*, operadores OR/AND/NOT.
// Retorna até `limit` resultados ranqueados por relevância.
func SearchMessageContent(query string, limit int) ([]MessageSearchResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	var results []MessageSearchResult

	err := db.Raw(`
		SELECT
			m.conversation_id,
			c.title AS conversation_title,
			m.id AS message_id,
			fts.role,
			snippet(chat_messages_fts, 0, '>>>', '<<<', '...', 48) AS snippet,
			bm25(chat_messages_fts) AS rank,
			m.created_at
		FROM chat_messages_fts fts
		JOIN chat_messages m ON m.rowid = fts.rowid
		JOIN conversations c ON c.id = m.conversation_id
		WHERE chat_messages_fts MATCH ?
		ORDER BY bm25(chat_messages_fts)
		LIMIT ?
	`, query, limit).Scan(&results).Error

	if err != nil {
		if strings.Contains(err.Error(), "fts5: syntax error") {
			escapedQuery := `"` + strings.ReplaceAll(query, `"`, `""`) + `"`
			err = db.Raw(`
				SELECT
					m.conversation_id,
					c.title AS conversation_title,
					m.id AS message_id,
					fts.role,
					snippet(chat_messages_fts, 0, '>>>', '<<<', '...', 48) AS snippet,
					bm25(chat_messages_fts) AS rank,
					m.created_at
				FROM chat_messages_fts fts
				JOIN chat_messages m ON m.rowid = fts.rowid
				JOIN conversations c ON c.id = m.conversation_id
				WHERE chat_messages_fts MATCH ?
				ORDER BY bm25(chat_messages_fts)
				LIMIT ?
			`, escapedQuery, limit).Scan(&results).Error
			if err != nil {
				return nil, fmt.Errorf("erro na busca FTS5: %w", err)
			}
		} else {
			return nil, fmt.Errorf("erro na busca FTS5: %w", err)
		}
	}

	return results, nil
}

// ==================== LLM Providers ====================

// SaveLLMProvider salva ou atualiza um provedor
func SaveLLMProvider(provider *LLMProvider) error {
	return db.Save(provider).Error
}

// GetLLMProviders retorna todos os provedores
func GetLLMProviders() ([]*LLMProvider, error) {
	var providers []*LLMProvider
	err := db.Order("created_at ASC").Find(&providers).Error
	return providers, err
}

// GetLLMProvider busca um provedor por ID
func GetLLMProvider(id string) (*LLMProvider, error) {
	var provider LLMProvider
	err := db.First(&provider, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &provider, nil
}

// DeleteLLMProvider remove um provedor
func DeleteLLMProvider(id string) error {
	return db.Delete(&LLMProvider{}, "id = ?", id).Error
}

// CountLLMProviders retorna o número total de provedores
func CountLLMProviders() (int64, error) {
	var count int64
	err := db.Model(&LLMProvider{}).Count(&count).Error
	return count, err
}

// SetDefaultProvider marca um provedor como default (e desmarca os demais).
func SetDefaultProvider(id string) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&LLMProvider{}).Where("is_default = ?", true).Update("is_default", false).Error; err != nil {
			return err
		}
		return tx.Model(&LLMProvider{}).Where("id = ?", id).Update("is_default", true).Error
	})
}

// GetDefaultProvider retorna o provedor marcado como default, ou nil se nenhum.
func GetDefaultProvider() (*LLMProvider, error) {
	var provider LLMProvider
	err := db.First(&provider, "is_default = ?", true).Error
	if err != nil {
		return nil, err
	}
	return &provider, nil
}

// ensureTaskNoteExternalUniqueIndex aplica índice único parcial em (external_source, external_id).
//
// Escolha de modelagem: chave única global por origem (sem task_id na unicidade), alinhada à
// preferência de produto e ao padrão “ID estável no sistema remoto”. O mesmo comentário Jira
// (por exemplo) deve mapear a no máximo uma TaskNote no app, impedindo duplicatas em re-syncs.
// Notas manuais permanecem fora do índice (WHERE ambos os campos não vazios).
//
// Se a mesma referência externa for associada a outra task local, UpsertTaskNoteByExternal
// retorna erro explícito em vez de duplicar linhas.
func ensureTaskNoteExternalUniqueIndex() {
	if db == nil {
		return
	}
	db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS ux_task_notes_external_source_id ON task_notes (external_source, external_id) WHERE external_source <> '' AND external_id <> ''`)
}

func ensureChatMessageWindowIndex() {
	if db == nil {
		return
	}
	db.Exec(`CREATE INDEX IF NOT EXISTS idx_chat_messages_window ON chat_messages (conversation_id, parent_id, created_at, id)`)
}
