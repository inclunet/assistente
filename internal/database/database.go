package database

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"assistente/internal/config"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

var db *gorm.DB

// EmbeddingGenerator interface para gerar embeddings
type EmbeddingGenerator interface {
	Generate(text string) ([]float32, error)
}

// embeddingGenerator é o gerador de embeddings configurado
var embeddingGenerator EmbeddingGenerator

// SetEmbeddingGenerator configura o gerador de embeddings
func SetEmbeddingGenerator(gen EmbeddingGenerator) {
	embeddingGenerator = gen
}

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
func Init() error {
	configPath, err := config.GetConfigPath()
	if err != nil {
		return err
	}

	dbPath := filepath.Join(filepath.Dir(configPath), "conversations.db")

	db, err = gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		return err
	}

	// Ativa modo WAL para melhor performance com arquivos grandes
	db.Exec("PRAGMA journal_mode=WAL")
	db.Exec("PRAGMA synchronous=NORMAL")

	// Auto migrate
	if err := db.AutoMigrate(
		&Conversation{},
		&ChatMessage{},
		&ChatTab{},
		&FAQ{},
		&Memory{},
		&AgentConfig{},
		&HTTPAgent{},
		&HTTPEndpoint{},
		&MCPAgentDB{},
		&OAuthConnection{},
		&ModelCapability{},
		&FileAgentAuthorizedPath{},
		&VoiceProfile{},
	); err != nil {
		return err
	}

	// Seed: cria perfil de voz padrão "Desativado" se não existir
	if err := seedDefaultVoiceProfile(); err != nil {
		fmt.Printf("Aviso: erro ao criar perfil de voz padrão: %v\n", err)
	}

	return nil
}

// seedDefaultVoiceProfile cria o perfil de voz padrão "Desativado" se não existir
func seedDefaultVoiceProfile() error {
	// Verifica se já existe um perfil padrão
	var count int64
	if err := db.Model(&VoiceProfile{}).Where("is_default = ?", true).Count(&count).Error; err != nil {
		return err
	}

	if count > 0 {
		// Já existe um perfil padrão
		return nil
	}

	// Cria o perfil padrão "Desativado"
	profile := &VoiceProfile{
		Name:            "Desativado",
		Description:     "Perfil padrão sem síntese de voz. Usa aria-live para leitores de tela.",
		Provider:        "disabled",
		VoiceID:         "",
		Rate:            1.0,
		Pitch:           1.0,
		Volume:          1.0,
		EnabledForAgent: false,
		EnabledForUser:  false,
		IsDefault:       true,
	}

	if err := db.Create(profile).Error; err != nil {
		return err
	}

	fmt.Println("[Database] Perfil de voz padrão 'Desativado' criado com sucesso")
	return nil
}

// ==================== Conversation ====================

// CreateConversation cria uma nova conversa
func CreateConversation(title, model string) (*Conversation, error) {
	conv := &Conversation{
		Title: title,
	}

	// Se modelo fornecido, salva nas preferências
	if model != "" {
		conv.SetPreferences(&ChatPreferences{Model: model})
	}

	if err := db.Create(conv).Error; err != nil {
		return nil, err
	}
	return conv, nil
}

// CreateConversationWithPreferences cria uma nova conversa com preferências iniciais
func CreateConversationWithPreferences(title string, prefs *ChatPreferences) (*Conversation, error) {
	conv := &Conversation{
		Title: title,
	}

	if prefs != nil {
		conv.SetPreferences(prefs)
	}

	if err := db.Create(conv).Error; err != nil {
		return nil, err
	}
	return conv, nil
}

// GetConversations retorna todas as conversas ordenadas por data
func GetConversations() ([]Conversation, error) {
	var conversations []Conversation
	err := db.Order("updated_at DESC").Find(&conversations).Error
	if err != nil {
		return nil, err
	}

	// Popula a contagem de mensagens para cada conversa
	for i := range conversations {
		var count int64
		db.Model(&ChatMessage{}).Where("conversation_id = ?", conversations[i].ID).Count(&count)
		conversations[i].MessageCount = int(count)
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

// UpdateConversation atualiza título e modelo da conversa
func UpdateConversation(id uint, title, model string) error {
	updates := map[string]interface{}{
		"title":      title,
		"updated_at": time.Now(),
	}

	// Se modelo fornecido, atualiza nas preferências
	if model != "" {
		conv, err := GetConversationInfo(id)
		if err == nil {
			prefs := conv.GetPreferences()
			if prefs == nil {
				prefs = &ChatPreferences{}
			}
			prefs.Model = model
			if prefsJSON, err := json.Marshal(prefs); err == nil {
				updates["preferences"] = string(prefsJSON)
			}
		}
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

// UpdateConversationModel atualiza apenas o modelo da conversa (via preferências)
func UpdateConversationModel(id uint, model string) error {
	conv, err := GetConversationInfo(id)
	if err != nil {
		return err
	}

	prefs := conv.GetPreferences()
	if prefs == nil {
		prefs = &ChatPreferences{}
	}
	prefs.Model = model

	return UpdateConversationPreferences(id, prefs)
}

// UpdateConversationSettings atualiza showInternalMessages (via preferências)
func UpdateConversationSettings(id uint, showInternalMessages bool) error {
	conv, err := GetConversationInfo(id)
	if err != nil {
		return err
	}

	prefs := conv.GetPreferences()
	if prefs == nil {
		prefs = &ChatPreferences{}
	}
	prefs.ShowInternalMessages = &showInternalMessages

	return UpdateConversationPreferences(id, prefs)
}

// UpdateConversationPreferences atualiza as preferências locais de uma conversa
func UpdateConversationPreferences(id uint, prefs *ChatPreferences) error {
	var prefsJSON string
	if prefs != nil {
		data, err := json.Marshal(prefs)
		if err != nil {
			return err
		}
		prefsJSON = string(data)
	}

	return db.Model(&Conversation{}).Where("id = ?", id).Updates(map[string]interface{}{
		"preferences": prefsJSON,
		"updated_at":  time.Now(),
	}).Error
}

// GetConversationPreferences retorna as preferências de uma conversa
func GetConversationPreferences(id uint) (*ChatPreferences, error) {
	conv, err := GetConversationInfo(id)
	if err != nil {
		return nil, err
	}
	return conv.GetPreferences(), nil
}

// ==================== ChatMessage ====================

// MessageOptions contém opções para criar uma mensagem
type MessageOptions struct {
	ConversationID   uint
	ParentID         *uint  // ID da mensagem pai (define hierarquia)
	Role             string // user, assistant, tool
	Content          string
	Media            string // JSON com mídias
	ToolCalls        string // JSON com tool calls
	ToolCallID       string // ID da tool call (para role="tool")
	AgentName        string // Nome do agente
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	Model            string
}

// CreateMessage cria uma mensagem com todas as opções disponíveis
func CreateMessage(opts MessageOptions) (*ChatMessage, error) {
	msg := &ChatMessage{
		ConversationID:   opts.ConversationID,
		ParentID:         opts.ParentID,
		Role:             opts.Role,
		Content:          opts.Content,
		Media:            opts.Media,
		ToolCalls:        opts.ToolCalls,
		ToolCallID:       opts.ToolCallID,
		AgentName:        opts.AgentName,
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
func AddMessage(conversationID uint, role, content, toolCalls, toolResults string) (*ChatMessage, error) {
	return CreateMessage(MessageOptions{
		ConversationID: conversationID,
		Role:           role,
		Content:        content,
		ToolCalls:      toolCalls,
	})
}

// AddMessageWithMedia adiciona uma mensagem com mídias (sem parent - nível 0)
func AddMessageWithMedia(conversationID uint, role, content, media, toolCalls, toolResults string) (*ChatMessage, error) {
	return CreateMessage(MessageOptions{
		ConversationID: conversationID,
		Role:           role,
		Content:        content,
		Media:          media,
		ToolCalls:      toolCalls,
	})
}

// AddMessageWithTokens adiciona uma mensagem com informações de tokens
func AddMessageWithTokens(conversationID uint, role, content, toolCalls, toolResults string, promptTokens, completionTokens, totalTokens int, model string) (*ChatMessage, error) {
	return CreateMessage(MessageOptions{
		ConversationID:   conversationID,
		Role:             role,
		Content:          content,
		ToolCalls:        toolCalls,
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		TotalTokens:      totalTokens,
		Model:            model,
	})
}

// AddMessageWithTokensAndMedia adiciona uma mensagem com mídias e informações de tokens
func AddMessageWithTokensAndMedia(conversationID uint, role, content, media, toolCalls, toolResults string, promptTokens, completionTokens, totalTokens int, model string) (*ChatMessage, error) {
	return CreateMessage(MessageOptions{
		ConversationID:   conversationID,
		Role:             role,
		Content:          content,
		Media:            media,
		ToolCalls:        toolCalls,
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		TotalTokens:      totalTokens,
		Model:            model,
	})
}

// AddToolMessage adiciona uma mensagem de role="tool" (resposta de tool ao orquestrador)
func AddToolMessage(conversationID uint, content, toolCallID string) (*ChatMessage, error) {
	return CreateMessage(MessageOptions{
		ConversationID: conversationID,
		Role:           "tool",
		Content:        content,
		ToolCallID:     toolCallID,
	})
}

// AddChildMessage adiciona uma mensagem filha (com ParentID definido)
// Usada para mensagens internas de agentes e tools
func AddChildMessage(conversationID uint, parentID uint, role, content, toolCalls, toolCallID, agentName, model string) (*ChatMessage, error) {
	return CreateMessage(MessageOptions{
		ConversationID: conversationID,
		ParentID:       &parentID,
		Role:           role,
		Content:        content,
		ToolCalls:      toolCalls,
		ToolCallID:     toolCallID,
		AgentName:      agentName,
		Model:          model,
	})
}

// UpdateMessageContent atualiza o conteúdo e tokens de uma mensagem existente
// Usado para completar mensagens de delegação com a resposta final
func UpdateMessageContent(messageID uint, content string, promptTokens, completionTokens, totalTokens int, model string) error {
	return db.Model(&ChatMessage{}).Where("id = ?", messageID).Updates(map[string]interface{}{
		"content":           content,
		"prompt_tokens":     promptTokens,
		"completion_tokens": completionTokens,
		"total_tokens":      totalTokens,
		"model":             model,
	}).Error
}

// UpdateMessageToolCalls atualiza uma mensagem com tool_calls
// Usado quando o assistant decide chamar ferramentas
func UpdateMessageToolCalls(messageID uint, toolCalls string, agentName string) error {
	return db.Model(&ChatMessage{}).Where("id = ?", messageID).Updates(map[string]interface{}{
		"tool_calls": toolCalls,
		"agent_name": agentName,
	}).Error
}

// GetMessageChildren retorna todas as mensagens filhas de uma mensagem
// Deprecated: Use GetMessages instead
func GetMessageChildren(parentID uint) ([]ChatMessage, error) {
	return GetMessages(0, &parentID)
}

// GetMessages retorna mensagens de uma conversa com filtro opcional por parent
// - conversationID > 0: filtra por conversa (obrigatório para raízes)
// - parentID == nil: retorna mensagens raiz (parent_id IS NULL)
// - parentID != nil: retorna filhos da mensagem especificada
//
// Exemplos:
//
//	GetMessages(convID, nil)      → mensagens raiz da conversa
//	GetMessages(0, &parentID)     → filhos de uma mensagem
func GetMessages(conversationID uint, parentID *uint) ([]ChatMessage, error) {
	var messages []ChatMessage
	query := db.Order("created_at ASC")

	if parentID != nil {
		// Busca filhos de uma mensagem específica
		query = query.Where("parent_id = ?", *parentID)
	} else {
		// Busca mensagens raiz de uma conversa
		if conversationID == 0 {
			return nil, fmt.Errorf("conversationID é obrigatório para buscar mensagens raiz")
		}
		query = query.Where("conversation_id = ? AND parent_id IS NULL", conversationID)
	}

	err := query.Find(&messages).Error
	return messages, err
}

// CountChildren retorna a contagem de filhos para cada mensagem
func CountChildren(messageIDs []uint) (map[uint]int, error) {
	if len(messageIDs) == 0 {
		return make(map[uint]int), nil
	}

	fmt.Printf("🔍 [CountChildren] Contando filhos para IDs: %v\n", messageIDs)

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
		fmt.Printf("❌ [CountChildren] Erro: %v\n", err)
		return nil, err
	}

	fmt.Printf("📊 [CountChildren] Resultados SQL: %+v\n", results)

	counts := make(map[uint]int)
	for _, r := range results {
		counts[r.ParentID] = r.Count
	}

	fmt.Printf("✅ [CountChildren] Mapa final: %v\n", counts)
	return counts, nil
}

// GetMessageTree retorna uma mensagem com todos os seus descendentes
func GetMessageTree(messageID uint) (*ChatMessage, []ChatMessage, error) {
	var message ChatMessage
	if err := db.First(&message, messageID).Error; err != nil {
		return nil, nil, err
	}

	// Busca todos os descendentes recursivamente
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

// ==================== Memory ====================

// CreateMemory cria uma nova memória
func CreateMemory(title, content, category string) (*Memory, error) {
	memory := &Memory{
		Title:    title,
		Content:  content,
		Category: category,
	}
	if err := db.Create(memory).Error; err != nil {
		return nil, err
	}
	return memory, nil
}

// GetAllMemories retorna todas as memórias
func GetAllMemories() ([]Memory, error) {
	var memories []Memory
	err := db.Order("updated_at DESC").Find(&memories).Error
	return memories, err
}

// GetMemoriesByCategory retorna memórias de uma categoria
func GetMemoriesByCategory(category string) ([]Memory, error) {
	var memories []Memory
	err := db.Where("LOWER(category) = LOWER(?)", category).Order("updated_at DESC").Find(&memories).Error
	return memories, err
}

// SearchMemories busca memórias por texto
func SearchMemories(query string) ([]Memory, error) {
	var memories []Memory
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return GetAllMemories()
	}
	words := strings.Fields(query)
	tx := db.Model(&Memory{})
	for _, word := range words {
		searchTerm := "%" + word + "%"
		tx = tx.Where(
			"LOWER(title) LIKE ? OR LOWER(content) LIKE ? OR LOWER(category) LIKE ?",
			searchTerm, searchTerm, searchTerm,
		)
	}
	err := tx.Order("updated_at DESC").Find(&memories).Error
	return memories, err
}

// UpdateMemory atualiza uma memória
func UpdateMemory(id uint, title, content, category string) (*Memory, error) {
	var memory Memory
	if err := db.First(&memory, id).Error; err != nil {
		return nil, err
	}
	memory.Title = title
	memory.Content = content
	memory.Category = category
	memory.UpdatedAt = time.Now()
	if err := db.Save(&memory).Error; err != nil {
		return nil, err
	}
	return &memory, nil
}

// DeleteMemory deleta uma memória
func DeleteMemory(id uint) error {
	return db.Delete(&Memory{}, id).Error
}

// GetCoreMemories retorna memórias marcadas como "core"
func GetCoreMemories() ([]Memory, error) {
	var memories []Memory
	err := db.Where("LOWER(category) = ?", "core").Order("created_at ASC").Find(&memories).Error
	return memories, err
}

// GetMemory retorna uma memória por ID
func GetMemory(id uint) (*Memory, error) {
	var memory Memory
	err := db.First(&memory, id).Error
	if err != nil {
		return nil, err
	}
	return &memory, nil
}

// ==================== FAQ ====================

// CreateFAQ cria uma nova entrada no FAQ
func CreateFAQ(question, answer, tags string) (*FAQ, error) {
	faq := &FAQ{
		Question: question,
		Answer:   answer,
		Tags:     tags,
	}
	if err := db.Create(faq).Error; err != nil {
		return nil, err
	}
	return faq, nil
}

// GetFAQ retorna uma entrada do FAQ por ID
func GetFAQ(id uint) (*FAQ, error) {
	var faq FAQ
	err := db.First(&faq, id).Error
	if err != nil {
		return nil, err
	}
	return &faq, nil
}

// GetAllFAQs retorna todas as entradas do FAQ
func GetAllFAQs() ([]FAQ, error) {
	var faqs []FAQ
	err := db.Order("updated_at DESC").Find(&faqs).Error
	return faqs, err
}

// UpdateFAQ atualiza uma entrada do FAQ
func UpdateFAQ(id uint, question, answer, tags string) (*FAQ, error) {
	var faq FAQ
	if err := db.First(&faq, id).Error; err != nil {
		return nil, err
	}
	faq.Question = question
	faq.Answer = answer
	faq.Tags = tags
	faq.UpdatedAt = time.Now()
	if err := db.Save(&faq).Error; err != nil {
		return nil, err
	}
	return &faq, nil
}

// DeleteFAQ deleta uma entrada do FAQ
func DeleteFAQ(id uint) error {
	return db.Delete(&FAQ{}, id).Error
}

// SearchFAQ busca FAQs por texto
func SearchFAQ(query string) ([]FAQ, error) {
	var faqs []FAQ
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return faqs, nil
	}
	words := strings.Fields(query)
	tx := db.Model(&FAQ{})
	for _, word := range words {
		searchTerm := "%" + word + "%"
		tx = tx.Where(
			"LOWER(question) LIKE ? OR LOWER(answer) LIKE ? OR LOWER(tags) LIKE ?",
			searchTerm, searchTerm, searchTerm,
		)
	}
	err := tx.Order("updated_at DESC").Find(&faqs).Error
	return faqs, err
}

// GetFAQsWithEmbedding retorna FAQs que têm embedding
func GetFAQsWithEmbedding() ([]FAQ, error) {
	var faqs []FAQ
	err := db.Where("embedding IS NOT NULL AND embedding != ''").Find(&faqs).Error
	return faqs, err
}

// GetFAQsWithoutEmbedding retorna FAQs sem embedding
func GetFAQsWithoutEmbedding() ([]FAQ, error) {
	var faqs []FAQ
	err := db.Where("embedding IS NULL OR embedding = ''").Find(&faqs).Error
	return faqs, err
}

// UpdateFAQEmbedding atualiza o embedding de uma FAQ
func UpdateFAQEmbedding(id uint, embedding string) error {
	return db.Model(&FAQ{}).Where("id = ?", id).Update("embedding", embedding).Error
}

// GenerateFAQEmbedding gera e salva o embedding de uma FAQ
func GenerateFAQEmbedding(faqID uint) error {
	if embeddingGenerator == nil {
		return fmt.Errorf("serviço de embeddings não configurado")
	}

	faq, err := GetFAQ(faqID)
	if err != nil {
		return err
	}

	text := faq.Question + " " + faq.Answer
	if faq.Tags != "" {
		text += " " + faq.Tags
	}

	embedding, err := embeddingGenerator.Generate(text)
	if err != nil {
		return fmt.Errorf("erro ao gerar embedding: %w", err)
	}

	faq.SetEmbedding(embedding)
	return UpdateFAQEmbedding(faqID, faq.Embedding)
}

// GenerateAllFAQEmbeddings gera embeddings para todas as FAQs que não têm
func GenerateAllFAQEmbeddings() (int, error) {
	if embeddingGenerator == nil {
		return 0, fmt.Errorf("serviço de embeddings não configurado")
	}

	faqs, err := GetFAQsWithoutEmbedding()
	if err != nil {
		return 0, err
	}

	count := 0
	for _, faq := range faqs {
		if err := GenerateFAQEmbedding(faq.ID); err != nil {
			fmt.Printf("Erro ao gerar embedding para FAQ %d: %v\n", faq.ID, err)
			continue
		}
		count++
	}

	return count, nil
}

// SearchFAQSemantic busca FAQs usando similaridade de embeddings
func SearchFAQSemantic(query string, topK int, minSimilarity float32) ([]FAQ, error) {
	if embeddingGenerator == nil {
		return SearchFAQ(query)
	}

	if query == "" {
		return nil, nil
	}

	queryEmbedding, err := embeddingGenerator.Generate(query)
	if err != nil {
		fmt.Printf("Erro ao gerar embedding da query, usando busca textual: %v\n", err)
		return SearchFAQ(query)
	}

	faqs, err := GetFAQsWithEmbedding()
	if err != nil {
		return nil, err
	}

	if len(faqs) == 0 {
		return SearchFAQ(query)
	}

	type faqWithSimilarity struct {
		faq        FAQ
		similarity float32
	}

	results := make([]faqWithSimilarity, 0, len(faqs))
	for _, faq := range faqs {
		faqEmbedding := faq.GetEmbedding()
		if len(faqEmbedding) == 0 {
			continue
		}

		similarity := CosineSimilarity(queryEmbedding, faqEmbedding)
		if similarity >= minSimilarity {
			results = append(results, faqWithSimilarity{
				faq:        faq,
				similarity: similarity,
			})
		}
	}

	// Ordena por similaridade decrescente
	for i := 0; i < len(results)-1; i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].similarity > results[i].similarity {
				results[i], results[j] = results[j], results[i]
			}
		}
	}

	if topK > len(results) {
		topK = len(results)
	}

	finalResults := make([]FAQ, topK)
	for i := 0; i < topK; i++ {
		finalResults[i] = results[i].faq
	}

	return finalResults, nil
}

// ==================== AgentConfig ====================

// GetAgentConfig retorna a configuração de um agente pelo nome
func GetAgentConfig(name string) (*AgentConfig, error) {
	var cfg AgentConfig
	err := db.Where("name = ?", name).First(&cfg).Error
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}

// GetAgentConfigByID retorna a configuração de um agente pelo ID
func GetAgentConfigByID(id uint) (*AgentConfig, error) {
	var cfg AgentConfig
	err := db.First(&cfg, id).Error
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}

// GetAllAgentConfigs retorna todas as configurações de agentes
func GetAllAgentConfigs() ([]AgentConfig, error) {
	var configs []AgentConfig
	err := db.Order("name ASC").Find(&configs).Error
	return configs, err
}

// CreateAgentConfig cria uma nova configuração de agente
func CreateAgentConfig(name, displayName, description, agentType, model, systemPrompt, cfg string, enabled bool) (*AgentConfig, error) {
	agentConfig := &AgentConfig{
		Name:         name,
		DisplayName:  displayName,
		Description:  description,
		AgentType:    agentType,
		Model:        model,
		SystemPrompt: systemPrompt,
		Config:       cfg,
		Enabled:      enabled,
	}
	if err := db.Create(agentConfig).Error; err != nil {
		return nil, err
	}
	return agentConfig, nil
}

// UpdateAgentConfig atualiza a configuração de um agente
func UpdateAgentConfig(id uint, displayName, description, model, systemPrompt, cfg string, enabled bool) (*AgentConfig, error) {
	var agentConfig AgentConfig
	if err := db.First(&agentConfig, id).Error; err != nil {
		return nil, err
	}
	agentConfig.DisplayName = displayName
	agentConfig.Description = description
	agentConfig.Model = model
	agentConfig.SystemPrompt = systemPrompt
	agentConfig.Config = cfg
	agentConfig.Enabled = enabled
	agentConfig.UpdatedAt = time.Now()
	if err := db.Save(&agentConfig).Error; err != nil {
		return nil, err
	}
	return &agentConfig, nil
}

// DeleteAgentConfig deleta uma configuração de agente
func DeleteAgentConfig(id uint) error {
	return db.Delete(&AgentConfig{}, id).Error
}

// SaveOrUpdateAgentConfig salva ou atualiza configuração de um agente pelo nome
func SaveOrUpdateAgentConfig(name, displayName, description, agentType, model, systemPrompt, cfg string, enabled bool) (*AgentConfig, error) {
	var agentConfig AgentConfig
	err := db.Where("name = ?", name).First(&agentConfig).Error
	if err != nil {
		return CreateAgentConfig(name, displayName, description, agentType, model, systemPrompt, cfg, enabled)
	}
	agentConfig.DisplayName = displayName
	agentConfig.Description = description
	agentConfig.AgentType = agentType
	agentConfig.Model = model
	agentConfig.SystemPrompt = systemPrompt
	agentConfig.Config = cfg
	agentConfig.Enabled = enabled
	agentConfig.UpdatedAt = time.Now()
	if err := db.Save(&agentConfig).Error; err != nil {
		return nil, err
	}
	return &agentConfig, nil
}

// ==================== HTTPAgent ====================

// CreateHTTPAgent cria um novo HTTP Agent
func CreateHTTPAgent(agentConfigID uint, baseURL, authType, authConfig, defaultHeaders string, timeoutSeconds, retryCount int) (*HTTPAgent, error) {
	httpAgent := &HTTPAgent{
		AgentConfigID:  agentConfigID,
		BaseURL:        baseURL,
		AuthType:       authType,
		AuthConfig:     authConfig,
		DefaultHeaders: defaultHeaders,
		TimeoutSeconds: timeoutSeconds,
		RetryCount:     retryCount,
	}
	if err := db.Create(httpAgent).Error; err != nil {
		return nil, err
	}
	return httpAgent, nil
}

// GetHTTPAgent retorna um HTTP Agent por ID
func GetHTTPAgent(id uint) (*HTTPAgent, error) {
	var httpAgent HTTPAgent
	err := db.Preload("Endpoints").First(&httpAgent, id).Error
	if err != nil {
		return nil, err
	}
	return &httpAgent, nil
}

// GetHTTPAgentByConfigID retorna um HTTP Agent pelo AgentConfigID
func GetHTTPAgentByConfigID(agentConfigID uint) (*HTTPAgent, error) {
	var httpAgent HTTPAgent
	err := db.Preload("Endpoints").Where("agent_config_id = ?", agentConfigID).First(&httpAgent).Error
	if err != nil {
		return nil, err
	}
	return &httpAgent, nil
}

// GetAllHTTPAgents retorna todos os HTTP Agents
func GetAllHTTPAgents() ([]HTTPAgent, error) {
	var httpAgents []HTTPAgent
	err := db.Preload("Endpoints").Find(&httpAgents).Error
	return httpAgents, err
}

// UpdateHTTPAgent atualiza um HTTP Agent
func UpdateHTTPAgent(id uint, baseURL, authType, authConfig, defaultHeaders string, timeoutSeconds, retryCount int) (*HTTPAgent, error) {
	var httpAgent HTTPAgent
	if err := db.First(&httpAgent, id).Error; err != nil {
		return nil, err
	}
	httpAgent.BaseURL = baseURL
	httpAgent.AuthType = authType
	httpAgent.AuthConfig = authConfig
	httpAgent.DefaultHeaders = defaultHeaders
	httpAgent.TimeoutSeconds = timeoutSeconds
	httpAgent.RetryCount = retryCount
	httpAgent.UpdatedAt = time.Now()
	if err := db.Save(&httpAgent).Error; err != nil {
		return nil, err
	}
	return &httpAgent, nil
}

// DeleteHTTPAgent deleta um HTTP Agent e seus endpoints
func DeleteHTTPAgent(id uint) error {
	if err := db.Where("http_agent_id = ?", id).Delete(&HTTPEndpoint{}).Error; err != nil {
		return err
	}
	return db.Delete(&HTTPAgent{}, id).Error
}

// ==================== HTTPEndpoint ====================

// CreateHTTPEndpoint cria um novo endpoint
func CreateHTTPEndpoint(httpAgentID uint, name, description, method, pathTemplate, queryTemplate, headersJSON, bodyTemplate, parameters, responseTemplate string) (*HTTPEndpoint, error) {
	endpoint := &HTTPEndpoint{
		HTTPAgentID:      httpAgentID,
		Name:             name,
		Description:      description,
		Method:           method,
		PathTemplate:     pathTemplate,
		QueryTemplate:    queryTemplate,
		HeadersJSON:      headersJSON,
		BodyTemplate:     bodyTemplate,
		Parameters:       parameters,
		ResponseTemplate: responseTemplate,
	}
	if err := db.Create(endpoint).Error; err != nil {
		return nil, err
	}
	return endpoint, nil
}

// GetHTTPEndpoint retorna um endpoint por ID
func GetHTTPEndpoint(id uint) (*HTTPEndpoint, error) {
	var endpoint HTTPEndpoint
	err := db.First(&endpoint, id).Error
	if err != nil {
		return nil, err
	}
	return &endpoint, nil
}

// GetHTTPEndpointsByAgentID retorna todos os endpoints de um agent
func GetHTTPEndpointsByAgentID(httpAgentID uint) ([]HTTPEndpoint, error) {
	var endpoints []HTTPEndpoint
	err := db.Where("http_agent_id = ?", httpAgentID).Find(&endpoints).Error
	return endpoints, err
}

// UpdateHTTPEndpoint atualiza um endpoint
func UpdateHTTPEndpoint(id uint, name, description, method, pathTemplate, queryTemplate, headersJSON, bodyTemplate, parameters, responseTemplate string) (*HTTPEndpoint, error) {
	var endpoint HTTPEndpoint
	if err := db.First(&endpoint, id).Error; err != nil {
		return nil, err
	}
	endpoint.Name = name
	endpoint.Description = description
	endpoint.Method = method
	endpoint.PathTemplate = pathTemplate
	endpoint.QueryTemplate = queryTemplate
	endpoint.HeadersJSON = headersJSON
	endpoint.BodyTemplate = bodyTemplate
	endpoint.Parameters = parameters
	endpoint.ResponseTemplate = responseTemplate
	endpoint.UpdatedAt = time.Now()
	if err := db.Save(&endpoint).Error; err != nil {
		return nil, err
	}
	return &endpoint, nil
}

// DeleteHTTPEndpoint deleta um endpoint
func DeleteHTTPEndpoint(id uint) error {
	return db.Delete(&HTTPEndpoint{}, id).Error
}

// ==================== MCPAgentDB ====================

// CreateMCPAgent cria um novo MCP Agent
func CreateMCPAgent(agentConfigID uint, transportType, serverCommand, serverArgs, serverEnv, workingDir, serverURL, authType, authValue, httpHeaders, executionMode string, autoConnect bool) (*MCPAgentDB, error) {
	if transportType == "" {
		transportType = "stdio"
	}
	if executionMode == "" {
		executionMode = "convert"
	}
	mcpAgent := &MCPAgentDB{
		AgentConfigID: agentConfigID,
		TransportType: transportType,
		ServerCommand: serverCommand,
		ServerArgs:    serverArgs,
		ServerEnv:     serverEnv,
		WorkingDir:    workingDir,
		ServerURL:     serverURL,
		AuthType:      authType,
		AuthValue:     authValue,
		HTTPHeaders:   httpHeaders,
		ExecutionMode: executionMode,
		AutoConnect:   autoConnect,
	}
	if err := db.Create(mcpAgent).Error; err != nil {
		return nil, err
	}
	return mcpAgent, nil
}

// GetMCPAgent retorna um MCP Agent por ID
func GetMCPAgent(id uint) (*MCPAgentDB, error) {
	var mcpAgent MCPAgentDB
	err := db.First(&mcpAgent, id).Error
	if err != nil {
		return nil, err
	}
	return &mcpAgent, nil
}

// GetMCPAgentByConfigID retorna um MCP Agent pelo AgentConfigID
func GetMCPAgentByConfigID(agentConfigID uint) (*MCPAgentDB, error) {
	var mcpAgent MCPAgentDB
	err := db.Where("agent_config_id = ?", agentConfigID).First(&mcpAgent).Error
	if err != nil {
		return nil, err
	}
	return &mcpAgent, nil
}

// GetAllMCPAgents retorna todos os MCP Agents
func GetAllMCPAgents() ([]MCPAgentDB, error) {
	var mcpAgents []MCPAgentDB
	err := db.Find(&mcpAgents).Error
	return mcpAgents, err
}

// UpdateMCPAgent atualiza um MCP Agent
func UpdateMCPAgent(id uint, transportType, serverCommand, serverArgs, serverEnv, workingDir, serverURL, authType, authValue, httpHeaders, executionMode string, autoConnect bool) (*MCPAgentDB, error) {
	var mcpAgent MCPAgentDB
	if err := db.First(&mcpAgent, id).Error; err != nil {
		return nil, err
	}
	mcpAgent.TransportType = transportType
	mcpAgent.ServerCommand = serverCommand
	mcpAgent.ServerArgs = serverArgs
	mcpAgent.ServerEnv = serverEnv
	mcpAgent.WorkingDir = workingDir
	mcpAgent.ServerURL = serverURL
	mcpAgent.AuthType = authType
	mcpAgent.AuthValue = authValue
	mcpAgent.HTTPHeaders = httpHeaders
	mcpAgent.ExecutionMode = executionMode
	mcpAgent.AutoConnect = autoConnect
	mcpAgent.UpdatedAt = time.Now()
	if err := db.Save(&mcpAgent).Error; err != nil {
		return nil, err
	}
	return &mcpAgent, nil
}

// DeleteMCPAgent deleta um MCP Agent
func DeleteMCPAgent(id uint) error {
	return db.Delete(&MCPAgentDB{}, id).Error
}

// GetAllMCPAgentsFull retorna todos os MCP Agents com suas configurações de agente
func GetAllMCPAgentsFull() ([]map[string]interface{}, error) {
	var mcpAgents []MCPAgentDB
	if err := db.Find(&mcpAgents).Error; err != nil {
		return nil, err
	}
	result := make([]map[string]interface{}, 0, len(mcpAgents))
	for _, mcp := range mcpAgents {
		var agentConfig AgentConfig
		if err := db.First(&agentConfig, mcp.AgentConfigID).Error; err != nil {
			continue
		}
		result = append(result, map[string]interface{}{
			"id":             mcp.ID,
			"agent_config":   agentConfig,
			"transport_type": mcp.TransportType,
			"server_command": mcp.ServerCommand,
			"server_args":    mcp.ServerArgs,
			"server_env":     mcp.ServerEnv,
			"working_dir":    mcp.WorkingDir,
			"server_url":     mcp.ServerURL,
			"auth_type":      mcp.AuthType,
			"auth_value":     mcp.AuthValue,
			"http_headers":   mcp.HTTPHeaders,
			"execution_mode": mcp.ExecutionMode,
			"auto_connect":   mcp.AutoConnect,
			"created_at":     mcp.CreatedAt,
			"updated_at":     mcp.UpdatedAt,
		})
	}
	return result, nil
}

// ==================== ModelCapability ====================

// GetOrCreateModelCapability retorna ou cria capacidades para um modelo
func GetOrCreateModelCapability(modelName string) (*ModelCapability, error) {
	var cap ModelCapability
	err := db.Where("model_name = ?", modelName).First(&cap).Error
	if err == nil {
		return &cap, nil
	}
	cap = ModelCapability{
		ModelName: modelName,
	}
	if err := db.Create(&cap).Error; err != nil {
		return nil, err
	}
	return &cap, nil
}

// GetModelCapability retorna as capacidades de um modelo
func GetModelCapability(modelName string) (*ModelCapability, error) {
	var cap ModelCapability
	err := db.Where("model_name = ?", modelName).First(&cap).Error
	if err != nil {
		return nil, err
	}
	return &cap, nil
}

// GetAllModelCapabilities retorna todas as capacidades conhecidas
func GetAllModelCapabilities() ([]ModelCapability, error) {
	var caps []ModelCapability
	err := db.Order("times_used DESC, model_name ASC").Find(&caps).Error
	return caps, err
}

// UpdateModelCapability atualiza as capacidades de um modelo
func UpdateModelCapability(modelName string, supportsVision, supportsAudio, supportsVideo, supportsDocuments, supportsTools, supportsStreaming, supportsJSON *bool) (*ModelCapability, error) {
	cap, err := GetOrCreateModelCapability(modelName)
	if err != nil {
		return nil, err
	}
	if supportsVision != nil {
		cap.SupportsVision = supportsVision
	}
	if supportsAudio != nil {
		cap.SupportsAudio = supportsAudio
	}
	if supportsVideo != nil {
		cap.SupportsVideo = supportsVideo
	}
	if supportsDocuments != nil {
		cap.SupportsDocuments = supportsDocuments
	}
	if supportsTools != nil {
		cap.SupportsTools = supportsTools
	}
	if supportsStreaming != nil {
		cap.SupportsStreaming = supportsStreaming
	}
	if supportsJSON != nil {
		cap.SupportsJSON = supportsJSON
	}
	cap.LastTested = time.Now()
	cap.UpdatedAt = time.Now()
	if err := db.Save(cap).Error; err != nil {
		return nil, err
	}
	return cap, nil
}

// SetModelVisionSupport define se um modelo suporta visão
func SetModelVisionSupport(modelName string, supported bool) error {
	_, err := UpdateModelCapability(modelName, &supported, nil, nil, nil, nil, nil, nil)
	return err
}

// SetModelToolsSupport define se um modelo suporta tools
func SetModelToolsSupport(modelName string, supported bool) error {
	_, err := UpdateModelCapability(modelName, nil, nil, nil, nil, &supported, nil, nil)
	return err
}

// IncrementModelUsage incrementa o contador de uso de um modelo
func IncrementModelUsage(modelName string) error {
	cap, err := GetOrCreateModelCapability(modelName)
	if err != nil {
		return err
	}
	cap.TimesUsed++
	return db.Model(cap).Update("times_used", cap.TimesUsed).Error
}

// SetModelError registra um erro em um modelo
func SetModelError(modelName, errorMsg string) error {
	cap, err := GetOrCreateModelCapability(modelName)
	if err != nil {
		return err
	}
	cap.LastError = errorMsg
	cap.LastTested = time.Now()
	return db.Save(cap).Error
}

// GetVisionCapableModels retorna modelos que suportam visão
func GetVisionCapableModels() ([]ModelCapability, error) {
	var caps []ModelCapability
	err := db.Where("supports_vision = ?", true).Order("times_used DESC").Find(&caps).Error
	return caps, err
}

// ModelSupportsVision verifica se um modelo suporta visão
func ModelSupportsVision(modelName string) (bool, error) {
	cap, err := GetModelCapability(modelName)
	if err != nil {
		return false, nil
	}
	if cap.SupportsVision == nil {
		return false, nil
	}
	return *cap.SupportsVision, nil
}

// ==================== OAuthConnection ====================

// CreateOAuthConnection cria uma nova conexão OAuth
func CreateOAuthConnection(providerID, providerName, userEmail, userName, userID, accessToken, refreshToken, tokenType, scopes string, expiresAt time.Time) (*OAuthConnection, error) {
	conn := &OAuthConnection{
		ProviderID:   providerID,
		ProviderName: providerName,
		UserEmail:    userEmail,
		UserName:     userName,
		UserID:       userID,
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		TokenType:    tokenType,
		Scopes:       scopes,
		ExpiresAt:    expiresAt,
		IsActive:     true,
		LastUsedAt:   time.Now(),
	}
	if err := db.Create(conn).Error; err != nil {
		return nil, err
	}
	return conn, nil
}

// GetOAuthConnection retorna uma conexão OAuth por ID
func GetOAuthConnection(id uint) (*OAuthConnection, error) {
	var conn OAuthConnection
	err := db.First(&conn, id).Error
	if err != nil {
		return nil, err
	}
	return &conn, nil
}

// GetOAuthConnectionByProvider retorna conexões de um provider específico
func GetOAuthConnectionByProvider(providerID string) ([]OAuthConnection, error) {
	var conns []OAuthConnection
	err := db.Where("provider_id = ? AND is_active = ?", providerID, true).
		Order("updated_at DESC").Find(&conns).Error
	return conns, err
}

// GetAllOAuthConnections retorna todas as conexões OAuth ativas
func GetAllOAuthConnections() ([]OAuthConnection, error) {
	var conns []OAuthConnection
	err := db.Where("is_active = ?", true).Order("provider_id ASC, updated_at DESC").Find(&conns).Error
	return conns, err
}

// UpdateOAuthTokens atualiza os tokens de uma conexão
func UpdateOAuthTokens(id uint, accessToken, refreshToken string, expiresAt time.Time) error {
	return db.Model(&OAuthConnection{}).Where("id = ?", id).Updates(map[string]interface{}{
		"access_token":  accessToken,
		"refresh_token": refreshToken,
		"expires_at":    expiresAt,
		"last_used_at":  time.Now(),
		"updated_at":    time.Now(),
	}).Error
}

// UpdateOAuthConnectionLastUsed atualiza o timestamp de último uso
func UpdateOAuthConnectionLastUsed(id uint) error {
	return db.Model(&OAuthConnection{}).Where("id = ?", id).Updates(map[string]interface{}{
		"last_used_at": time.Now(),
	}).Error
}

// DeleteOAuthConnection desativa uma conexão OAuth (soft delete)
func DeleteOAuthConnection(id uint) error {
	return db.Model(&OAuthConnection{}).Where("id = ?", id).Updates(map[string]interface{}{
		"is_active":  false,
		"updated_at": time.Now(),
	}).Error
}

// HardDeleteOAuthConnection remove permanentemente uma conexão
func HardDeleteOAuthConnection(id uint) error {
	return db.Delete(&OAuthConnection{}, id).Error
}

// GetActiveOAuthConnectionForProvider retorna a conexão ativa mais recente para um provider
func GetActiveOAuthConnectionForProvider(providerID string) (*OAuthConnection, error) {
	var conn OAuthConnection
	err := db.Where("provider_id = ? AND is_active = ?", providerID, true).
		Order("updated_at DESC").First(&conn).Error
	if err != nil {
		return nil, err
	}
	return &conn, nil
}

// ==================== FileAgentAuthorizedPath ====================

// CreateFileAgentAuthorizedPath cria uma nova pasta autorizada
func CreateFileAgentAuthorizedPath(path string, allowDelete, allowWrite, recursive bool) (*FileAgentAuthorizedPath, error) {
	authPath := &FileAgentAuthorizedPath{
		Path:        path,
		AllowDelete: allowDelete,
		AllowWrite:  allowWrite,
		Recursive:   recursive,
	}
	if err := db.Create(authPath).Error; err != nil {
		return nil, err
	}
	return authPath, nil
}

// GetFileAgentAuthorizedPath retorna uma pasta autorizada por ID
func GetFileAgentAuthorizedPath(id uint) (*FileAgentAuthorizedPath, error) {
	var authPath FileAgentAuthorizedPath
	err := db.First(&authPath, id).Error
	if err != nil {
		return nil, err
	}
	return &authPath, nil
}

// GetAllFileAgentAuthorizedPaths retorna todas as pastas autorizadas
func GetAllFileAgentAuthorizedPaths() ([]FileAgentAuthorizedPath, error) {
	var authPaths []FileAgentAuthorizedPath
	err := db.Order("path ASC").Find(&authPaths).Error
	return authPaths, err
}

// UpdateFileAgentAuthorizedPath atualiza uma pasta autorizada
func UpdateFileAgentAuthorizedPath(id uint, path string, allowDelete, allowWrite, recursive bool) (*FileAgentAuthorizedPath, error) {
	var authPath FileAgentAuthorizedPath
	if err := db.First(&authPath, id).Error; err != nil {
		return nil, err
	}
	authPath.Path = path
	authPath.AllowDelete = allowDelete
	authPath.AllowWrite = allowWrite
	authPath.Recursive = recursive
	if err := db.Save(&authPath).Error; err != nil {
		return nil, err
	}
	return &authPath, nil
}

// DeleteFileAgentAuthorizedPath deleta uma pasta autorizada
func DeleteFileAgentAuthorizedPath(id uint) error {
	return db.Delete(&FileAgentAuthorizedPath{}, id).Error
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

// CosineSimilarity calcula a similaridade de cosseno entre dois vetores
func CosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dotProduct, normA, normB float64
	for i := range a {
		dotProduct += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return float32(dotProduct / (sqrtFloat64(normA) * sqrtFloat64(normB)))
}

func sqrtFloat64(x float64) float64 {
	if x <= 0 {
		return 0
	}
	z := x / 2
	for i := 0; i < 10; i++ {
		z = z - (z*z-x)/(2*z)
	}
	return z
}

// ==================== VoiceProfile ====================

// CreateVoiceProfile cria um novo perfil de voz
// VoiceProfileOptions contém opções para criar/atualizar um perfil de voz
type VoiceProfileOptions struct {
	Name            string
	Description     string
	Provider        string
	VoiceID         string
	Rate            float64
	Pitch           float64
	Volume          float64
	EnabledForAgent bool
	EnabledForUser  bool
	IsDefault       bool
}

// CreateVoiceProfile cria um novo perfil de voz (versão simplificada para compatibilidade)
func CreateVoiceProfile(name, description, provider, voiceID string, rate, pitch, volume float64, isDefault bool) (*VoiceProfile, error) {
	return CreateVoiceProfileFull(VoiceProfileOptions{
		Name:            name,
		Description:     description,
		Provider:        provider,
		VoiceID:         voiceID,
		Rate:            rate,
		Pitch:           pitch,
		Volume:          volume,
		EnabledForAgent: provider != "disabled",
		EnabledForUser:  false,
		IsDefault:       isDefault,
	})
}

// CreateVoiceProfileFull cria um novo perfil de voz com todas as opções
func CreateVoiceProfileFull(opts VoiceProfileOptions) (*VoiceProfile, error) {
	profile := &VoiceProfile{
		Name:            opts.Name,
		Description:     opts.Description,
		Provider:        opts.Provider,
		VoiceID:         opts.VoiceID,
		Rate:            opts.Rate,
		Pitch:           opts.Pitch,
		Volume:          opts.Volume,
		EnabledForAgent: opts.EnabledForAgent,
		EnabledForUser:  opts.EnabledForUser,
		IsDefault:       opts.IsDefault,
	}

	// Valida o perfil
	if err := profile.Validate(); err != nil {
		return nil, err
	}

	// Se marcado como default, remove o default anterior
	if opts.IsDefault {
		if err := db.Model(&VoiceProfile{}).Where("is_default = ?", true).Update("is_default", false).Error; err != nil {
			return nil, err
		}
	}

	if err := db.Create(profile).Error; err != nil {
		return nil, err
	}
	return profile, nil
}

// GetVoiceProfile retorna um perfil de voz por ID
func GetVoiceProfile(id uint) (*VoiceProfile, error) {
	var profile VoiceProfile
	err := db.First(&profile, id).Error
	if err != nil {
		return nil, err
	}
	return &profile, nil
}

// GetVoiceProfileByName retorna um perfil de voz por nome
func GetVoiceProfileByName(name string) (*VoiceProfile, error) {
	var profile VoiceProfile
	err := db.Where("name = ?", name).First(&profile).Error
	if err != nil {
		return nil, err
	}
	return &profile, nil
}

// GetAllVoiceProfiles retorna todos os perfis de voz
func GetAllVoiceProfiles() ([]VoiceProfile, error) {
	var profiles []VoiceProfile
	err := db.Order("name ASC").Find(&profiles).Error
	return profiles, err
}

// GetDefaultVoiceProfile retorna o perfil de voz padrão
func GetDefaultVoiceProfile() (*VoiceProfile, error) {
	var profile VoiceProfile
	err := db.Where("is_default = ?", true).First(&profile).Error
	if err != nil {
		return nil, err
	}
	return &profile, nil
}

// UpdateVoiceProfile atualiza um perfil de voz
// UpdateVoiceProfile atualiza um perfil de voz (versão simplificada para compatibilidade)
func UpdateVoiceProfile(id uint, name, description, provider, voiceID string, rate, pitch, volume float64, isDefault bool) (*VoiceProfile, error) {
	// Busca o perfil existente para manter os valores dos novos campos
	var existing VoiceProfile
	if err := db.First(&existing, id).Error; err != nil {
		return nil, err
	}

	return UpdateVoiceProfileFull(id, VoiceProfileOptions{
		Name:            name,
		Description:     description,
		Provider:        provider,
		VoiceID:         voiceID,
		Rate:            rate,
		Pitch:           pitch,
		Volume:          volume,
		EnabledForAgent: existing.EnabledForAgent,
		EnabledForUser:  existing.EnabledForUser,
		IsDefault:       isDefault,
	})
}

// UpdateVoiceProfileFull atualiza um perfil de voz com todas as opções
func UpdateVoiceProfileFull(id uint, opts VoiceProfileOptions) (*VoiceProfile, error) {
	var profile VoiceProfile
	if err := db.First(&profile, id).Error; err != nil {
		return nil, err
	}

	profile.Name = opts.Name
	profile.Description = opts.Description
	profile.Provider = opts.Provider
	profile.VoiceID = opts.VoiceID
	profile.Rate = opts.Rate
	profile.Pitch = opts.Pitch
	profile.Volume = opts.Volume
	profile.EnabledForAgent = opts.EnabledForAgent
	profile.EnabledForUser = opts.EnabledForUser
	profile.UpdatedAt = time.Now()

	// Valida o perfil
	if err := profile.Validate(); err != nil {
		return nil, err
	}

	// Se marcado como default, remove o default anterior
	if opts.IsDefault && !profile.IsDefault {
		if err := db.Model(&VoiceProfile{}).Where("is_default = ? AND id != ?", true, id).Update("is_default", false).Error; err != nil {
			return nil, err
		}
	}
	profile.IsDefault = opts.IsDefault

	if err := db.Save(&profile).Error; err != nil {
		return nil, err
	}
	return &profile, nil
}

// DeleteVoiceProfile deleta um perfil de voz
func DeleteVoiceProfile(id uint) error {
	return db.Delete(&VoiceProfile{}, id).Error
}

// SetDefaultVoiceProfile define um perfil como padrão
func SetDefaultVoiceProfile(id uint) error {
	// Remove default anterior
	if err := db.Model(&VoiceProfile{}).Where("is_default = ?", true).Update("is_default", false).Error; err != nil {
		return err
	}
	// Define o novo default
	return db.Model(&VoiceProfile{}).Where("id = ?", id).Update("is_default", true).Error
}

// SearchVoiceProfiles busca perfis por nome ou descrição
func SearchVoiceProfiles(query string) ([]VoiceProfile, error) {
	var profiles []VoiceProfile
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return GetAllVoiceProfiles()
	}
	searchTerm := "%" + query + "%"
	err := db.Where(
		"LOWER(name) LIKE ? OR LOWER(description) LIKE ? OR LOWER(provider) LIKE ?",
		searchTerm, searchTerm, searchTerm,
	).Order("name ASC").Find(&profiles).Error
	return profiles, err
}
