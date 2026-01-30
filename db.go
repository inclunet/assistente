package main

import (
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"time"

	"assistente/internal/agentmanager"
	"assistente/internal/agents"
	"assistente/internal/config"
	"assistente/internal/database"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// Re-exporta tipos do pacote database para manter compatibilidade
type Conversation = database.Conversation
type ChatMessage = database.ChatMessage
type ChatPreferences = database.ChatPreferences
type Memory = database.Memory
type FAQ = database.FAQ
type AgentConfig = database.AgentConfig
type HTTPAgent = database.HTTPAgent
type HTTPEndpoint = database.HTTPEndpoint
type MCPAgentDB = database.MCPAgentDB
type ModelCapability = database.ModelCapability
type OAuthConnection = database.OAuthConnection
type VoiceProfile = database.VoiceProfile

// Re-exporta funções que não dependem de App
var (
	InitDatabase  = database.Init
	GenerateTitle = database.GenerateTitle
)

// ==================== Conversation ====================

// enrichMessage converte ChatMessage para EnrichedMessage com campos calculados
func (a *App) enrichMessage(msg database.ChatMessage) EnrichedMessage {
	// Converte ParentID *uint para *string
	var parentIDStr *string
	if msg.ParentID != nil {
		pidStr := fmt.Sprintf("%d", *msg.ParentID)
		parentIDStr = &pidStr
	}

	enriched := EnrichedMessage{
		// Campos do ChatMessage
		ID:               fmt.Sprintf("%d", msg.ID),
		ConversationID:   msg.ConversationID,
		ParentID:         parentIDStr,
		Role:             msg.Role,
		Content:          msg.Content,
		Media:            msg.Media,
		ToolCalls:        msg.ToolCalls,
		ToolResults:      msg.ToolResults,
		ToolCallID:       msg.ToolCallID,
		AgentName:        msg.AgentName,
		PromptTokens:     msg.PromptTokens,
		CompletionTokens: msg.CompletionTokens,
		TotalTokens:      msg.TotalTokens,
		Model:            msg.Model,
		CreatedAt:        msg.CreatedAt,
		// Campos derivados
		Timestamp:   msg.CreatedAt.UnixMilli(),
		IsStreaming: false,
		Internal:    msg.ParentID != nil,
	}

	// Extrai toolName do primeiro tool call se existir
	if msg.ToolCalls != "" {
		var toolCalls []ToolCall
		if err := json.Unmarshal([]byte(msg.ToolCalls), &toolCalls); err == nil {
			if len(toolCalls) > 0 {
				enriched.ToolName = toolCalls[0].Function.Name
			}
		}
	}

	return enriched
}

func (a *App) CreateConversation(title, model string) (*Conversation, error) {
	return database.CreateConversation(title, model)
}

func (a *App) GetConversations() ([]Conversation, error) {
	return database.GetConversations()
}

func (a *App) GetConversation(id uint) (*Conversation, error) {
	return database.GetConversation(id)
}

// GetMessages retorna mensagens com filtro por parent (API unificada com LAZY LOADING)
// - conversationID > 0 e parentID == nil: mensagens RAIZ com childCount (não carrega filhos)
// - parentID != nil: filhos diretos da mensagem especificada
//
// LAZY LOADING: Retorna apenas o nível solicitado, nunca carrega recursivamente
// Frontend deve chamar novamente para carregar filhos quando usuário expandir thread
func (a *App) GetMessages(conversationID uint, parentID *uint) ([]MessageNode, error) {
	// Busca apenas mensagens do nível solicitado (raízes OU filhos diretos)
	messages, err := database.GetMessages(conversationID, parentID)
	if err != nil {
		return nil, err
	}

	// Coleta IDs para contar filhos
	msgIDs := make([]uint, len(messages))
	for i, msg := range messages {
		msgIDs[i] = msg.ID
	}

	// Conta filhos de cada mensagem (para mostrar indicadores)
	childCounts, err := database.CountChildren(msgIDs)
	if err != nil {
		childCounts = make(map[uint]int)
	}

	// Determina o nível baseado no contexto
	level := 0
	if parentID != nil {
		level = 1 // Filhos são pelo menos nível 1
	}

	// Converte para MessageNode com childCount (SEM filhos carregados - lazy loading)
	result := make([]MessageNode, 0, len(messages))
	for _, msg := range messages {
		childCount := childCounts[msg.ID]
		node := MessageNode{
			Message:    a.enrichMessage(msg),
			Children:   nil, // LAZY LOADING - não carrega filhos
			Level:      level,
			ChildCount: childCount,
		}
		result = append(result, node)
	}

	return result, nil
}

// GetConversationInfo retorna apenas metadados da conversa (sem mensagens)
func (a *App) GetConversationInfo(id uint) (*Conversation, error) {
	return database.GetConversationInfo(id)
}

// GetConversationWithThreads retorna conversa com mensagens raiz (lazy loading)
// Deprecated: Use GetConversationInfo + GetMessages instead
func (a *App) GetConversationWithThreads(id uint) (*ConversationWithThreads, error) {
	conv, err := database.GetConversationInfo(id)
	if err != nil {
		return nil, err
	}

	// Usa a nova API para buscar mensagens raiz
	threads, err := a.GetMessages(id, nil)
	if err != nil {
		return nil, err
	}

	return &ConversationWithThreads{
		ID:          conv.ID,
		Title:       conv.Title,
		Preferences: conv.GetPreferences(),
		Threads:     threads,
	}, nil
}

// GetMessageChildren retorna os filhos de uma mensagem (lazy loading)
// Deprecated: Use GetMessages(0, &parentID) instead
func (a *App) GetMessageChildren(messageID uint) ([]MessageNode, error) {
	return a.GetMessages(0, &messageID)
}

// buildMessageTree organiza mensagens planas em uma árvore hierárquica
// Mensagens com ParentID=nil são raízes (nível 0)
// Mensagens com ParentID apontam para seu pai
func (a *App) buildMessageTree(messages []database.ChatMessage) []MessageNode {
	fmt.Printf("🌳 [TREE] Construindo árvore com %d mensagens\n", len(messages))

	// Passo 1: Cria mapa de filhos (parentID -> lista de mensagens filhas)
	childrenMap := make(map[uint][]database.ChatMessage)
	var rootMessages []database.ChatMessage

	for _, msg := range messages {
		if msg.ParentID == nil {
			rootMessages = append(rootMessages, msg)
		} else {
			childrenMap[*msg.ParentID] = append(childrenMap[*msg.ParentID], msg)
		}
	}

	// Passo 2: Ordena raízes e filhos por ID
	sort.Slice(rootMessages, func(i, j int) bool {
		return rootMessages[i].ID < rootMessages[j].ID
	})
	for parentID := range childrenMap {
		sort.Slice(childrenMap[parentID], func(i, j int) bool {
			return childrenMap[parentID][i].ID < childrenMap[parentID][j].ID
		})
	}

	// Passo 3: Função recursiva para construir nó com todos os descendentes
	var buildNode func(msg database.ChatMessage, level int) MessageNode
	buildNode = func(msg database.ChatMessage, level int) MessageNode {
		node := MessageNode{
			Message:  a.enrichMessage(msg), // Usa método compartilhado
			Children: []MessageNode{},
			Level:    level,
		}

		// Adiciona filhos recursivamente
		children := childrenMap[msg.ID]
		node.ChildCount = len(children) // Define o count de filhos diretos

		for _, child := range children {
			childNode := buildNode(child, level+1)
			node.Children = append(node.Children, childNode)
		}

		return node
	}

	// Passo 4: Constrói árvore a partir das raízes
	result := make([]MessageNode, 0, len(rootMessages))
	for _, rootMsg := range rootMessages {
		node := buildNode(rootMsg, 0)
		result = append(result, node)
	}

	// Log do resultado
	fmt.Printf("🌳 [TREE] Resultado: %d raízes\n", len(result))
	var logTree func(nodes []MessageNode, indent string)
	logTree = func(nodes []MessageNode, indent string) {
		for _, n := range nodes {
			fmt.Printf("🌳 [TREE] %sID=%d, role=%s, children=%d\n",
				indent, n.Message.ID, n.Message.Role, len(n.Children))
			if len(n.Children) > 0 {
				logTree(n.Children, indent+"  ")
			}
		}
	}
	logTree(result, "  ")

	return result
}

func (a *App) UpdateConversation(id uint, title, model string) error {
	if err := database.UpdateConversation(id, title, model); err != nil {
		return err
	}

	// Emite evento unificado para atualizar todas as referências (tabs, etc.)
	if title != "" {
		runtime.EventsEmit(a.ctx, "conversation:renamed", map[string]interface{}{
			"conversation_id": id,
			"new_title":       title,
		})
	}

	return nil
}

func (a *App) DeleteConversation(id uint) error {
	// Antes de deletar, limpa as abas que referenciam essa conversa
	tabs, err := database.GetAllTabs()
	if err == nil {
		for _, tab := range tabs {
			if tab.ConversationID != nil && *tab.ConversationID == id {
				database.ClearTab(tab.ID)
			}
		}
	}

	// Deleta conversa normalmente
	if err := database.DeleteConversation(id); err != nil {
		return err
	}

	// Emite evento
	runtime.EventsEmit(a.ctx, "conversation:deleted", map[string]interface{}{
		"conversation_id": id,
	})

	return nil
}

// DeleteMessage exclui uma mensagem e todas as suas filhas (respostas)
func (a *App) DeleteMessage(messageID uint) error {
	if err := database.DeleteMessage(messageID); err != nil {
		return err
	}

	// Emite evento para o frontend atualizar a UI
	runtime.EventsEmit(a.ctx, "message:deleted", map[string]interface{}{
		"message_id": messageID,
	})

	return nil
}

func (a *App) UpdateConversationModel(id uint, model string) error {
	return database.UpdateConversationModel(id, model)
}

func (a *App) UpdateConversationSettings(id uint, showInternalMessages bool) error {
	return database.UpdateConversationSettings(id, showInternalMessages)
}

// UpdateConversationPreferences atualiza as preferências locais de uma conversa
func (a *App) UpdateConversationPreferences(id uint, prefs *ChatPreferences) error {
	return database.UpdateConversationPreferences(id, prefs)
}

// GetConversationPreferences retorna as preferências de uma conversa
func (a *App) GetConversationPreferences(id uint) (*ChatPreferences, error) {
	return database.GetConversationPreferences(id)
}

// ==================== ChatMessage ====================

func (a *App) AddMessage(conversationID uint, role, content, toolCalls, toolResults string) (*ChatMessage, error) {
	return database.AddMessage(conversationID, role, content, toolCalls, toolResults)
}

func (a *App) AddMessageWithMedia(conversationID uint, role, content, media, toolCalls, toolResults string) (*ChatMessage, error) {
	return database.AddMessageWithMedia(conversationID, role, content, media, toolCalls, toolResults)
}

func (a *App) AddMessageWithTokens(conversationID uint, role, content, toolCalls, toolResults string, promptTokens, completionTokens, totalTokens int, model string) (*ChatMessage, error) {
	return database.AddMessageWithTokens(conversationID, role, content, toolCalls, toolResults, promptTokens, completionTokens, totalTokens, model)
}

func (a *App) AddMessageWithTokensAndMedia(conversationID uint, role, content, media, toolCalls, toolResults string, promptTokens, completionTokens, totalTokens int, model string) (*ChatMessage, error) {
	return database.AddMessageWithTokensAndMedia(conversationID, role, content, media, toolCalls, toolResults, promptTokens, completionTokens, totalTokens, model)
}

func (a *App) GetConversationTokenStats(conversationID uint) (map[string]int, error) {
	return database.GetConversationTokenStats(conversationID)
}

func (a *App) GetAllTokenStats() (map[string]int, error) {
	return database.GetAllTokenStats()
}

// ==================== Memory ====================

func (a *App) CreateMemory(title, content, category string) (*Memory, error) {
	memory, err := database.CreateMemory(title, content, category)
	if err != nil {
		return nil, err
	}
	// Gera embedding em background
	go func() {
		if err := a.GenerateMemoryEmbedding(memory.ID); err != nil {
			fmt.Printf("Aviso: erro ao gerar embedding para Memory %d: %v\n", memory.ID, err)
		}
	}()
	return memory, nil
}

func (a *App) GetAllMemories() ([]Memory, error) {
	return database.GetAllMemories()
}

func (a *App) GetMemoriesByCategory(category string) ([]Memory, error) {
	return database.GetMemoriesByCategory(category)
}

func (a *App) SearchMemories(query string) ([]Memory, error) {
	return database.SearchMemories(query)
}

func (a *App) UpdateMemory(id uint, title, content, category string) (*Memory, error) {
	memory, err := database.UpdateMemory(id, title, content, category)
	if err != nil {
		return nil, err
	}
	// Regenera embedding em background
	go func() {
		if err := a.GenerateMemoryEmbedding(id); err != nil {
			fmt.Printf("Aviso: erro ao regenerar embedding para Memory %d: %v\n", id, err)
		}
	}()
	return memory, nil
}

func (a *App) DeleteMemory(id uint) error {
	return database.DeleteMemory(id)
}

func (a *App) GetCoreMemories() ([]Memory, error) {
	return database.GetCoreMemories()
}

// ==================== Memory Embeddings ====================

func (a *App) GenerateMemoryEmbedding(memoryID uint) error {
	return database.GenerateMemoryEmbedding(memoryID)
}

func (a *App) GenerateAllMemoryEmbeddings() (int, error) {
	return database.GenerateAllMemoryEmbeddings()
}

// GetMemoriesWithoutEmbedding retorna memórias que ainda não têm embedding
func (a *App) GetMemoriesWithoutEmbedding() ([]Memory, error) {
	return database.GetMemoriesWithoutEmbedding()
}

// RegenerateSingleMemoryEmbedding regenera o embedding de uma memória específica
func (a *App) RegenerateSingleMemoryEmbedding(memoryID uint) error {
	return a.GenerateMemoryEmbedding(memoryID)
}

// MemoryEmbeddingStatus representa o status dos embeddings de memórias
type MemoryEmbeddingStatus struct {
	Total            int `json:"total"`
	WithEmbeddings   int `json:"with_embeddings"`
	WithoutEmbedding int `json:"without_embedding"`
}

// GetMemoryEmbeddingStatus retorna o status dos embeddings de memórias
func (a *App) GetMemoryEmbeddingStatus() (*MemoryEmbeddingStatus, error) {
	memories, err := a.GetAllMemories()
	if err != nil {
		return nil, err
	}

	withEmb := 0
	for _, memory := range memories {
		if memory.Embedding != "" {
			withEmb++
		}
	}

	return &MemoryEmbeddingStatus{
		Total:            len(memories),
		WithEmbeddings:   withEmb,
		WithoutEmbedding: len(memories) - withEmb,
	}, nil
}

// RegenerateMemoryEmbeddings regenera embeddings de todas as memórias que não têm
func (a *App) RegenerateMemoryEmbeddings() (int, error) {
	return database.GenerateAllMemoryEmbeddings()
}

// ==================== Conversation Embeddings ====================

// GenerateConversationEmbedding gera o embedding de uma conversa específica
func (a *App) GenerateConversationEmbedding(conversationID uint) error {
	return database.GenerateConversationEmbedding(conversationID)
}

// GenerateAllConversationEmbeddings gera embeddings para todas as conversas que não têm
func (a *App) GenerateAllConversationEmbeddings() (int, error) {
	return database.GenerateAllConversationEmbeddings()
}

// SearchConversationsSemantic busca conversas usando similaridade semântica
func (a *App) SearchConversationsSemantic(query string, topK int, minSimilarity float32) ([]Conversation, error) {
	return database.SearchConversationsSemantic(query, topK, minSimilarity)
}

// ConversationEmbeddingStatus representa o status dos embeddings de conversas
type ConversationEmbeddingStatus struct {
	Total            int `json:"total"`
	WithEmbeddings   int `json:"with_embeddings"`
	WithoutEmbedding int `json:"without_embedding"`
}

// GetConversationEmbeddingStatus retorna o status dos embeddings de conversas
func (a *App) GetConversationEmbeddingStatus() (*ConversationEmbeddingStatus, error) {
	conversations, err := database.GetConversations()
	if err != nil {
		return nil, err
	}

	withEmb := 0
	for _, conv := range conversations {
		if conv.Embedding != "" {
			withEmb++
		}
	}

	return &ConversationEmbeddingStatus{
		Total:            len(conversations),
		WithEmbeddings:   withEmb,
		WithoutEmbedding: len(conversations) - withEmb,
	}, nil
}

// OnTabInactive é chamado quando uma aba fica inativa
// Gera o embedding da conversa em background se necessário
func (a *App) OnTabInactive(tabID uint) error {
	// Busca a aba
	tab, err := database.GetTab(tabID)
	if err != nil {
		return err
	}

	// Se não tem conversa associada, ignora
	if tab.ConversationID == nil {
		return nil
	}

	// Gera embedding em background
	go func() {
		if err := database.GenerateConversationEmbedding(*tab.ConversationID); err != nil {
			fmt.Printf("Aviso: erro ao gerar embedding para conversa %d: %v\n", *tab.ConversationID, err)
		}
	}()

	return nil
}

// OnTabClosed é chamado quando uma aba é fechada
// Garante que o embedding final seja gerado
func (a *App) OnTabClosed(conversationID uint) error {
	if conversationID == 0 {
		return nil
	}

	// Gera embedding de forma síncrona para garantir que seja salvo
	return database.GenerateConversationEmbedding(conversationID)
}

// ==================== Context Navigation (for agents) ====================

// SearchOpenTabs busca em abas abertas usando similaridade semântica
// Implementa agents.ContextNavigator
func (a *App) SearchOpenTabs(query string, minSimilarity float32) ([]agents.OpenTabResult, error) {
	if query == "" {
		return nil, fmt.Errorf("query não pode ser vazia")
	}

	// Busca todas as abas
	tabs, err := database.GetAllTabs()
	if err != nil {
		return nil, err
	}

	// Gera embedding da query
	if a.embeddingsService == nil {
		return nil, fmt.Errorf("serviço de embeddings não configurado")
	}

	queryEmbedding, err := a.embeddingsService.Generate(query)
	if err != nil {
		return nil, fmt.Errorf("erro ao gerar embedding da query: %w", err)
	}

	var results []agents.OpenTabResult

	for _, tab := range tabs {
		if tab.ConversationID == nil {
			continue // Aba sem conversa
		}

		// Busca a conversa para pegar o embedding
		conv, err := database.GetConversation(*tab.ConversationID)
		if err != nil {
			continue
		}

		convEmbedding := conv.GetEmbedding()
		if len(convEmbedding) == 0 {
			continue // Sem embedding
		}

		// Calcula similaridade
		similarity := database.CosineSimilarity(queryEmbedding, convEmbedding)
		if similarity >= minSimilarity {
			results = append(results, agents.OpenTabResult{
				TabID:          tab.ID,
				ConversationID: *tab.ConversationID,
				Title:          tab.Title,
				Summary:        conv.Summary,
				Similarity:     similarity,
				IsActive:       tab.IsActive,
			})
		}
	}

	// Ordena por similaridade decrescente
	for i := 0; i < len(results)-1; i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].Similarity > results[i].Similarity {
				results[i], results[j] = results[j], results[i]
			}
		}
	}

	return results, nil
}

// SearchConversationHistory busca em histórico de conversas (não abertas em abas)
// Implementa agents.ContextNavigator
func (a *App) SearchConversationHistory(query string, topK int, minSimilarity float32) ([]agents.ConversationResult, error) {
	conversations, err := database.SearchConversationsSemantic(query, topK, minSimilarity)
	if err != nil {
		return nil, err
	}

	// Pega IDs das conversas abertas em abas para excluí-las
	tabs, _ := database.GetAllTabs()
	openConvIDs := make(map[uint]bool)
	for _, tab := range tabs {
		if tab.ConversationID != nil {
			openConvIDs[*tab.ConversationID] = true
		}
	}

	var results []agents.ConversationResult
	for _, conv := range conversations {
		// Exclui conversas que já estão abertas em abas
		if openConvIDs[conv.ID] {
			continue
		}

		results = append(results, agents.ConversationResult{
			ConversationID: conv.ID,
			Title:          conv.Title,
			Summary:        conv.Summary,
			CreatedAt:      conv.CreatedAt.Format("02/01/2006"),
			MessageCount:   conv.MessageCount,
		})
	}

	return results, nil
}

// SwitchToTab muda para uma aba específica
func (a *App) SwitchToTab(tabID uint) error {
	return database.SetActiveTab(tabID)
}

// OpenConversationInNewTab abre uma conversa do histórico em uma nova aba
func (a *App) OpenConversationInNewTab(conversationID uint) (uint, error) {
	// Verifica se a conversa existe
	conv, err := database.GetConversation(conversationID)
	if err != nil {
		return 0, fmt.Errorf("conversa não encontrada: %w", err)
	}

	// Cria nova aba
	tab, err := database.CreateTab(conv.Title, "💬", true)
	if err != nil {
		return 0, fmt.Errorf("erro ao criar aba: %w", err)
	}

	// Carrega a conversa na aba
	if err := database.LoadConversationInTab(tab.ID, conversationID); err != nil {
		return 0, fmt.Errorf("erro ao carregar conversa: %w", err)
	}

	return tab.ID, nil
}

// OpenConversationInCurrentTab abre uma conversa na aba atual
func (a *App) OpenConversationInCurrentTab(conversationID uint) error {
	// Verifica se a conversa existe
	_, err := database.GetConversation(conversationID)
	if err != nil {
		return fmt.Errorf("conversa não encontrada: %w", err)
	}

	// Pega a aba ativa
	tabs, err := database.GetAllTabs()
	if err != nil {
		return fmt.Errorf("erro ao obter abas: %w", err)
	}

	var activeTabID uint
	for _, tab := range tabs {
		if tab.IsActive {
			activeTabID = tab.ID
			break
		}
	}

	if activeTabID == 0 {
		return fmt.Errorf("nenhuma aba ativa encontrada")
	}

	// Carrega a conversa na aba ativa
	return database.LoadConversationInTab(activeTabID, conversationID)
}

// RenameTab renomeia uma aba (usa UpdateTabTitle que emite eventos)
func (a *App) RenameTab(tabID uint, newTitle string) error {
	return a.UpdateTabTitle(tabID, newTitle)
}

// GetCurrentTabID retorna o ID da aba ativa
func (a *App) GetCurrentTabID() (uint, error) {
	tabs, err := database.GetAllTabs()
	if err != nil {
		return 0, err
	}

	for _, tab := range tabs {
		if tab.IsActive {
			return tab.ID, nil
		}
	}

	return 0, fmt.Errorf("nenhuma aba ativa encontrada")
}

// GetCurrentConversationID retorna o ID da conversa da aba ativa
func (a *App) GetCurrentConversationID() (uint, error) {
	tabs, err := database.GetAllTabs()
	if err != nil {
		return 0, err
	}

	for _, tab := range tabs {
		if tab.IsActive {
			if tab.ConversationID != nil && *tab.ConversationID > 0 {
				return *tab.ConversationID, nil
			}
			return 0, fmt.Errorf("aba ativa não tem conversa associada")
		}
	}

	return 0, fmt.Errorf("nenhuma aba ativa encontrada")
}

// CreateNewConversation cria uma nova conversa e abre em nova aba
func (a *App) CreateNewConversation(title string) (uint, error) {
	// Cria a conversa
	conv, err := database.CreateConversation(title, "gpt-4o-mini")
	if err != nil {
		return 0, fmt.Errorf("erro ao criar conversa: %w", err)
	}

	// Cria nova aba com a conversa
	tab, err := database.CreateTab(title, "💬", true)
	if err != nil {
		return 0, fmt.Errorf("erro ao criar aba: %w", err)
	}

	// Carrega a conversa na aba
	if err := database.LoadConversationInTab(tab.ID, conv.ID); err != nil {
		return 0, fmt.Errorf("erro ao carregar conversa: %w", err)
	}

	return conv.ID, nil
}

// RenameConversation renomeia uma conversa (usa UpdateConversation que já emite o evento)
func (a *App) RenameConversation(conversationID uint, newTitle string) error {
	return a.UpdateConversation(conversationID, newTitle, "")
}

// ClearConversation remove todas as mensagens de uma conversa
func (a *App) ClearConversation(conversationID uint) error {
	if err := database.DeleteAllMessages(conversationID); err != nil {
		return err
	}

	// Emite evento para frontend atualizar
	runtime.EventsEmit(a.ctx, "conversation:cleared", map[string]interface{}{
		"conversation_id": conversationID,
	})

	return nil
}

// DeleteMessages remove mensagens específicas de uma conversa
func (a *App) DeleteMessages(conversationID uint, messageIDs []uint) error {
	for _, msgID := range messageIDs {
		if err := database.DeleteMessage(msgID); err != nil {
			return fmt.Errorf("erro ao deletar mensagem %d: %w", msgID, err)
		}
	}
	return nil
}

// GetConversationSummary retorna o resumo de uma conversa
func (a *App) GetConversationSummary(conversationID uint) (string, error) {
	conv, err := database.GetConversation(conversationID)
	if err != nil {
		return "", err
	}
	return conv.Summary, nil
}

// ==================== FAQ ====================

func (a *App) CreateFAQ(question, answer, tags string) (*FAQ, error) {
	faq, err := database.CreateFAQ(question, answer, tags)
	if err != nil {
		return nil, err
	}
	// Gera embedding em background
	go func() {
		if err := a.GenerateFAQEmbedding(faq.ID); err != nil {
			fmt.Printf("Aviso: erro ao gerar embedding para FAQ %d: %v\n", faq.ID, err)
		}
	}()
	return faq, nil
}

func (a *App) GetFAQ(id uint) (*FAQ, error) {
	return database.GetFAQ(id)
}

func (a *App) GetAllFAQs() ([]FAQ, error) {
	return database.GetAllFAQs()
}

func (a *App) UpdateFAQ(id uint, question, answer, tags string) (*FAQ, error) {
	faq, err := database.UpdateFAQ(id, question, answer, tags)
	if err != nil {
		return nil, err
	}
	// Regenera embedding em background
	go func() {
		if err := a.GenerateFAQEmbedding(id); err != nil {
			fmt.Printf("Aviso: erro ao regenerar embedding para FAQ %d: %v\n", id, err)
		}
	}()
	return faq, nil
}

func (a *App) DeleteFAQ(id uint) error {
	return database.DeleteFAQ(id)
}

func (a *App) SearchFAQ(query string) ([]FAQ, error) {
	return database.SearchFAQ(query)
}

// ==================== FAQ Embeddings ====================

func (a *App) GenerateFAQEmbedding(faqID uint) error {
	return database.GenerateFAQEmbedding(faqID)
}

func (a *App) GenerateAllFAQEmbeddings() (int, error) {
	return database.GenerateAllFAQEmbeddings()
}

func (a *App) SearchFAQSemantic(query string, topK int, minSimilarity float32) ([]FAQ, error) {
	return database.SearchFAQSemantic(query, topK, minSimilarity)
}

// FAQEmbeddingStatus representa o status de embeddings das FAQs
type FAQEmbeddingStatus struct {
	TotalFAQs        int `json:"total_faqs"`
	WithEmbedding    int `json:"with_embedding"`
	WithoutEmbedding int `json:"without_embedding"`
}

// GetFAQEmbeddingStatus retorna o status dos embeddings de FAQs
func (a *App) GetFAQEmbeddingStatus() (*FAQEmbeddingStatus, error) {
	faqs, err := a.GetAllFAQs()
	if err != nil {
		return nil, err
	}

	withEmb := 0
	for _, faq := range faqs {
		if faq.Embedding != "" {
			withEmb++
		}
	}

	return &FAQEmbeddingStatus{
		TotalFAQs:        len(faqs),
		WithEmbedding:    withEmb,
		WithoutEmbedding: len(faqs) - withEmb,
	}, nil
}

// RegenerateFAQEmbeddings regenera embeddings para todas as FAQs sem embedding
func (a *App) RegenerateFAQEmbeddings() (int, error) {
	return a.GenerateAllFAQEmbeddings()
}

// RegenerateSingleFAQEmbedding regenera o embedding de uma FAQ específica
func (a *App) RegenerateSingleFAQEmbedding(faqID uint) error {
	return a.GenerateFAQEmbedding(faqID)
}

// ==================== AgentConfig ====================

func (a *App) GetAgentConfig(name string) (*AgentConfig, error) {
	return database.GetAgentConfig(name)
}

func (a *App) GetAgentConfigByID(id uint) (*AgentConfig, error) {
	return database.GetAgentConfigByID(id)
}

func (a *App) GetAllAgentConfigs() ([]AgentConfig, error) {
	return database.GetAllAgentConfigs()
}

func (a *App) CreateAgentConfig(name, displayName, description, agentType, model, systemPrompt, config string, enabled bool) (*AgentConfig, error) {
	return database.CreateAgentConfig(name, displayName, description, agentType, model, systemPrompt, config, enabled)
}

func (a *App) UpdateAgentConfig(id uint, displayName, description, model, systemPrompt, config string, enabled bool) (*AgentConfig, error) {
	return database.UpdateAgentConfig(id, displayName, description, model, systemPrompt, config, enabled)
}

func (a *App) DeleteAgentConfig(id uint) error {
	return database.DeleteAgentConfig(id)
}

func (a *App) SaveOrUpdateAgentConfig(name, displayName, description, agentType, model, systemPrompt, config string, enabled bool) (*AgentConfig, error) {
	return database.SaveOrUpdateAgentConfig(name, displayName, description, agentType, model, systemPrompt, config, enabled)
}

// ==================== HTTPAgent ====================

func (a *App) CreateHTTPAgent(agentConfigID uint, baseURL, authType, authConfig, defaultHeaders string, timeoutSeconds, retryCount int) (*HTTPAgent, error) {
	return database.CreateHTTPAgent(agentConfigID, baseURL, authType, authConfig, defaultHeaders, timeoutSeconds, retryCount)
}

func (a *App) GetHTTPAgent(id uint) (*HTTPAgent, error) {
	return database.GetHTTPAgent(id)
}

func (a *App) GetHTTPAgentByConfigID(agentConfigID uint) (*HTTPAgent, error) {
	return database.GetHTTPAgentByConfigID(agentConfigID)
}

func (a *App) GetAllHTTPAgents() ([]HTTPAgent, error) {
	return database.GetAllHTTPAgents()
}

func (a *App) UpdateHTTPAgent(id uint, baseURL, authType, authConfig, defaultHeaders string, timeoutSeconds, retryCount int) (*HTTPAgent, error) {
	return database.UpdateHTTPAgent(id, baseURL, authType, authConfig, defaultHeaders, timeoutSeconds, retryCount)
}

func (a *App) DeleteHTTPAgent(id uint) error {
	return database.DeleteHTTPAgent(id)
}

// ==================== HTTPEndpoint ====================

func (a *App) CreateHTTPEndpoint(httpAgentID uint, name, description, method, pathTemplate, queryTemplate, headersJSON, bodyTemplate, parameters, responseTemplate string) (*HTTPEndpoint, error) {
	req := agentmanager.CreateEndpointRequest{
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
	data, err := a.agentManager.CreateHTTPEndpoint(httpAgentID, req)
	if err != nil {
		return nil, err
	}
	// Converte para tipo UI
	return &HTTPEndpoint{
		ID:               data.ID,
		HTTPAgentID:      data.HTTPAgentID,
		Name:             data.Name,
		Description:      data.Description,
		Method:           data.Method,
		PathTemplate:     data.PathTemplate,
		QueryTemplate:    data.QueryTemplate,
		HeadersJSON:      data.HeadersJSON,
		BodyTemplate:     data.BodyTemplate,
		Parameters:       data.Parameters,
		ResponseTemplate: data.ResponseTemplate,
	}, nil
}

func (a *App) GetHTTPEndpoint(id uint) (*HTTPEndpoint, error) {
	data, err := a.agentManager.GetHTTPEndpoint(id)
	if err != nil {
		return nil, err
	}
	return &HTTPEndpoint{
		ID:               data.ID,
		HTTPAgentID:      data.HTTPAgentID,
		Name:             data.Name,
		Description:      data.Description,
		Method:           data.Method,
		PathTemplate:     data.PathTemplate,
		QueryTemplate:    data.QueryTemplate,
		HeadersJSON:      data.HeadersJSON,
		BodyTemplate:     data.BodyTemplate,
		Parameters:       data.Parameters,
		ResponseTemplate: data.ResponseTemplate,
	}, nil
}

func (a *App) GetHTTPEndpointsByAgentID(httpAgentID uint) ([]HTTPEndpoint, error) {
	data, err := a.agentManager.GetHTTPEndpointsByAgentID(httpAgentID)
	if err != nil {
		return nil, err
	}
	result := make([]HTTPEndpoint, 0, len(data))
	for _, d := range data {
		result = append(result, HTTPEndpoint{
			ID:               d.ID,
			HTTPAgentID:      d.HTTPAgentID,
			Name:             d.Name,
			Description:      d.Description,
			Method:           d.Method,
			PathTemplate:     d.PathTemplate,
			QueryTemplate:    d.QueryTemplate,
			HeadersJSON:      d.HeadersJSON,
			BodyTemplate:     d.BodyTemplate,
			Parameters:       d.Parameters,
			ResponseTemplate: d.ResponseTemplate,
		})
	}
	return result, nil
}

func (a *App) UpdateHTTPEndpoint(id uint, name, description, method, pathTemplate, queryTemplate, headersJSON, bodyTemplate, parameters, responseTemplate string) (*HTTPEndpoint, error) {
	req := agentmanager.CreateEndpointRequest{
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
	data, err := a.agentManager.UpdateHTTPEndpoint(id, req)
	if err != nil {
		return nil, err
	}
	endpoint := &HTTPEndpoint{
		ID:               data.ID,
		HTTPAgentID:      data.HTTPAgentID,
		Name:             data.Name,
		Description:      data.Description,
		Method:           data.Method,
		PathTemplate:     data.PathTemplate,
		QueryTemplate:    data.QueryTemplate,
		HeadersJSON:      data.HeadersJSON,
		BodyTemplate:     data.BodyTemplate,
		Parameters:       data.Parameters,
		ResponseTemplate: data.ResponseTemplate,
	}

	// Hot reload: recarrega o agente pai no registry
	go func() {
		// Busca o HTTP agent para pegar o AgentConfigID
		httpAgent, err := a.agentManager.GetHTTPAgent(data.HTTPAgentID)
		if err == nil && httpAgent.AgentConfigID > 0 {
			a.ReloadHTTPAgent(httpAgent.AgentConfigID)
		}
	}()

	return endpoint, nil
}

func (a *App) DeleteHTTPEndpoint(id uint) error {
	// Busca o endpoint para saber qual agente recarregar
	endpoint, err := a.agentManager.GetHTTPEndpoint(id)
	if err == nil && endpoint.HTTPAgentID > 0 {
		// Deleta
		if err := a.agentManager.DeleteHTTPEndpoint(id); err != nil {
			return err
		}
		// Hot reload: recarrega o agente pai no registry
		go func() {
			httpAgent, err := a.agentManager.GetHTTPAgent(endpoint.HTTPAgentID)
			if err == nil && httpAgent.AgentConfigID > 0 {
				a.ReloadHTTPAgent(httpAgent.AgentConfigID)
			}
		}()
		return nil
	}
	return a.agentManager.DeleteHTTPEndpoint(id)
}

// ==================== MCPAgentDB ====================

func (a *App) CreateMCPAgent(agentConfigID uint, transportType, serverCommand, serverArgs, serverEnv, workingDir, serverURL, authType, authValue, httpHeaders, executionMode string, autoConnect bool) (*MCPAgentDB, error) {
	return database.CreateMCPAgent(agentConfigID, transportType, serverCommand, serverArgs, serverEnv, workingDir, serverURL, authType, authValue, httpHeaders, executionMode, autoConnect)
}

func (a *App) GetMCPAgent(id uint) (*MCPAgentDB, error) {
	return database.GetMCPAgent(id)
}

func (a *App) GetMCPAgentByConfigID(agentConfigID uint) (*MCPAgentDB, error) {
	return database.GetMCPAgentByConfigID(agentConfigID)
}

func (a *App) GetAllMCPAgents() ([]MCPAgentDB, error) {
	return database.GetAllMCPAgents()
}

func (a *App) UpdateMCPAgent(id uint, transportType, serverCommand, serverArgs, serverEnv, workingDir, serverURL, authType, authValue, httpHeaders, executionMode string, autoConnect bool) (*MCPAgentDB, error) {
	return database.UpdateMCPAgent(id, transportType, serverCommand, serverArgs, serverEnv, workingDir, serverURL, authType, authValue, httpHeaders, executionMode, autoConnect)
}

func (a *App) DeleteMCPAgent(id uint) error {
	return database.DeleteMCPAgent(id)
}

// ==================== Chat Tabs ====================

type TabsResponse struct {
	Tabs        []database.ChatTab `json:"tabs"`
	ActiveTabId uint               `json:"active_tab_id"`
}

// GetTabs retorna todas as abas de chat
func (a *App) GetTabs() (TabsResponse, error) {
	tabs, err := database.GetAllTabs()
	if err != nil {
		return TabsResponse{}, err
	}

	// Se não há abas, cria uma padrão
	if len(tabs) == 0 {
		if err := database.InitializeDefaultTab(); err != nil {
			return TabsResponse{}, err
		}
		tabs, err = database.GetAllTabs()
		if err != nil {
			return TabsResponse{}, err
		}
	}

	// Encontra aba ativa
	var activeId uint
	for _, tab := range tabs {
		if tab.IsActive {
			activeId = tab.ID
			break
		}
	}

	return TabsResponse{
		Tabs:        tabs,
		ActiveTabId: activeId,
	}, nil
}

func (a *App) GetAllMCPAgentsFull() ([]map[string]interface{}, error) {
	return database.GetAllMCPAgentsFull()
}

// ==================== ModelCapability ====================

func (a *App) GetOrCreateModelCapability(modelName string) (*ModelCapability, error) {
	return database.GetOrCreateModelCapability(modelName)
}

func (a *App) GetModelCapability(modelName string) (*ModelCapability, error) {
	return database.GetModelCapability(modelName)
}

func (a *App) GetAllModelCapabilities() ([]ModelCapability, error) {
	return database.GetAllModelCapabilities()
}

func (a *App) UpdateModelCapability(modelName string, supportsVision, supportsAudio, supportsVideo, supportsDocuments, supportsTools, supportsStreaming, supportsJSON *bool) (*ModelCapability, error) {
	return database.UpdateModelCapability(modelName, supportsVision, supportsAudio, supportsVideo, supportsDocuments, supportsTools, supportsStreaming, supportsJSON)
}

func (a *App) SetModelVisionSupport(modelName string, supported bool) error {
	return database.SetModelVisionSupport(modelName, supported)
}

func (a *App) SetModelToolsSupport(modelName string, supported bool) error {
	return database.SetModelToolsSupport(modelName, supported)
}

func (a *App) IncrementModelUsage(modelName string) error {
	return database.IncrementModelUsage(modelName)
}

func (a *App) SetModelError(modelName, errorMsg string) error {
	return database.SetModelError(modelName, errorMsg)
}

func (a *App) GetVisionCapableModels() ([]ModelCapability, error) {
	return database.GetVisionCapableModels()
}

func (a *App) ModelSupportsVision(modelName string) (bool, error) {
	return database.ModelSupportsVision(modelName)
}

// ==================== OAuthConnection ====================

func (a *App) CreateOAuthConnection(providerID, providerName, userEmail, userName, userID, accessToken, refreshToken, tokenType, scopes string, expiresAt time.Time) (*OAuthConnection, error) {
	return database.CreateOAuthConnection(providerID, providerName, userEmail, userName, userID, accessToken, refreshToken, tokenType, scopes, expiresAt)
}

func (a *App) GetOAuthConnection(id uint) (*OAuthConnection, error) {
	return database.GetOAuthConnection(id)
}

func (a *App) GetOAuthConnectionByProvider(providerID string) ([]OAuthConnection, error) {
	return database.GetOAuthConnectionByProvider(providerID)
}

func (a *App) GetAllOAuthConnections() ([]OAuthConnection, error) {
	return database.GetAllOAuthConnections()
}

func (a *App) UpdateOAuthTokens(id uint, accessToken, refreshToken string, expiresAt time.Time) error {
	return database.UpdateOAuthTokens(id, accessToken, refreshToken, expiresAt)
}

func (a *App) UpdateOAuthConnectionLastUsed(id uint) error {
	return database.UpdateOAuthConnectionLastUsed(id)
}

func (a *App) DeleteOAuthConnection(id uint) error {
	return database.DeleteOAuthConnection(id)
}

func (a *App) HardDeleteOAuthConnection(id uint) error {
	return database.HardDeleteOAuthConnection(id)
}

func (a *App) GetActiveOAuthConnectionForProvider(providerID string) (*OAuthConnection, error) {
	return database.GetActiveOAuthConnectionForProvider(providerID)
}

// ==================== VoiceProfile ====================

// CreateVoiceProfile cria um novo perfil de voz
func (a *App) CreateVoiceProfile(name, description, provider, voiceID string, rate, pitch, volume float64, isDefault bool) (*VoiceProfile, error) {
	return database.CreateVoiceProfile(name, description, provider, voiceID, rate, pitch, volume, isDefault)
}

// CreateVoiceProfileFull cria um novo perfil de voz com todas as opções
func (a *App) CreateVoiceProfileFull(name, description, provider, voiceID string, rate, pitch, volume float64, enabledForAgent, enabledForUser, isDefault bool) (*VoiceProfile, error) {
	return database.CreateVoiceProfileFull(database.VoiceProfileOptions{
		Name:            name,
		Description:     description,
		Provider:        provider,
		VoiceID:         voiceID,
		Rate:            rate,
		Pitch:           pitch,
		Volume:          volume,
		EnabledForAgent: enabledForAgent,
		EnabledForUser:  enabledForUser,
		IsDefault:       isDefault,
	})
}

// GetVoiceProfile retorna um perfil de voz por ID
func (a *App) GetVoiceProfile(id uint) (*VoiceProfile, error) {
	return database.GetVoiceProfile(id)
}

// GetVoiceProfileByName retorna um perfil de voz por nome
func (a *App) GetVoiceProfileByName(name string) (*VoiceProfile, error) {
	return database.GetVoiceProfileByName(name)
}

// GetAllVoiceProfiles retorna todos os perfis de voz
func (a *App) GetAllVoiceProfiles() ([]VoiceProfile, error) {
	return database.GetAllVoiceProfiles()
}

// GetDefaultVoiceProfile retorna o perfil de voz padrão
func (a *App) GetDefaultVoiceProfile() (*VoiceProfile, error) {
	return database.GetDefaultVoiceProfile()
}

// UpdateVoiceProfile atualiza um perfil de voz
func (a *App) UpdateVoiceProfile(id uint, name, description, provider, voiceID string, rate, pitch, volume float64, isDefault bool) (*VoiceProfile, error) {
	return database.UpdateVoiceProfile(id, name, description, provider, voiceID, rate, pitch, volume, isDefault)
}

// UpdateVoiceProfileFull atualiza um perfil de voz com todas as opções
func (a *App) UpdateVoiceProfileFull(id uint, name, description, provider, voiceID string, rate, pitch, volume float64, enabledForAgent, enabledForUser, isDefault bool) (*VoiceProfile, error) {
	return database.UpdateVoiceProfileFull(id, database.VoiceProfileOptions{
		Name:            name,
		Description:     description,
		Provider:        provider,
		VoiceID:         voiceID,
		Rate:            rate,
		Pitch:           pitch,
		Volume:          volume,
		EnabledForAgent: enabledForAgent,
		EnabledForUser:  enabledForUser,
		IsDefault:       isDefault,
	})
}

// DeleteVoiceProfile deleta um perfil de voz
func (a *App) DeleteVoiceProfile(id uint) error {
	return database.DeleteVoiceProfile(id)
}

// SetDefaultVoiceProfile define um perfil como padrão
func (a *App) SetDefaultVoiceProfile(id uint) error {
	return database.SetDefaultVoiceProfile(id)
}

// SearchVoiceProfiles busca perfis por nome ou descrição
func (a *App) SearchVoiceProfiles(query string) ([]VoiceProfile, error) {
	return database.SearchVoiceProfiles(query)
}

// PreviewVoiceProfile reproduz um texto de teste com as configurações de um perfil
func (a *App) PreviewVoiceProfile(id uint, sampleText string) error {
	profile, err := database.GetVoiceProfile(id)
	if err != nil {
		return fmt.Errorf("perfil não encontrado: %w", err)
	}

	if sampleText == "" {
		sampleText = "Este é um teste do perfil de voz " + profile.Name
	}

	// Usa o SpeechManager para sintetizar
	if a.speechManager == nil {
		return fmt.Errorf("speech manager não configurado")
	}

	// Configura as opções do perfil temporariamente
	if profile.Provider == "openai" {
		a.speechManager.SetTTSVoice(profile.VoiceID)
		// Converte rate para escala SAPI5 para o manager
		var sapi5Rate int
		if profile.Rate < 1 {
			sapi5Rate = int((profile.Rate - 1) / 0.075)
		} else {
			sapi5Rate = int((profile.Rate - 1) / 0.3)
		}
		a.speechManager.SetTTSSpeed(sapi5Rate)
	}

	// Sintetiza e retorna (o frontend vai tocar)
	result, err := a.speechManager.Synthesize(sampleText)
	if err != nil {
		return fmt.Errorf("erro ao sintetizar: %w", err)
	}

	// Emite evento com o áudio para o frontend tocar
	runtime.EventsEmit(a.ctx, "voice_profile:preview", map[string]interface{}{
		"audio_base64": result.AudioBase64,
		"format":       result.Format,
	})

	return nil
}

// GetEffectiveVoiceProfile retorna o perfil de voz efetivo para uma conversa
// Respeita fallback: perfil da conversa -> perfil padrão -> nil
func (a *App) GetEffectiveVoiceProfile(conversationID uint) (*VoiceProfile, error) {
	// Primeiro, tenta obter o perfil configurado na conversa
	prefs, err := database.GetConversationPreferences(conversationID)
	if err == nil && prefs != nil && prefs.VoiceProfileID != nil {
		// Tenta carregar o perfil específico
		profile, err := database.GetVoiceProfile(*prefs.VoiceProfileID)
		if err == nil {
			return profile, nil
		}
		// Se falhou (perfil foi deletado?), cai para o padrão
		fmt.Printf("[GetEffectiveVoiceProfile] Perfil %d não encontrado, usando padrão\n", *prefs.VoiceProfileID)
	}

	// Tenta obter o perfil padrão
	defaultProfile, err := database.GetDefaultVoiceProfile()
	if err == nil {
		return defaultProfile, nil
	}

	// Sem perfil configurado
	return nil, nil
}

// SetConversationVoiceProfile define o perfil de voz de uma conversa
func (a *App) SetConversationVoiceProfile(conversationID uint, profileID uint) error {
	prefs, err := database.GetConversationPreferences(conversationID)
	if err != nil {
		// Se não existe preferências, cria uma nova
		prefs = &database.ChatPreferences{}
	}
	if prefs == nil {
		prefs = &database.ChatPreferences{}
	}

	// Se profileID é 0, remove o perfil customizado (usa nil para indicar "usar padrão")
	if profileID == 0 {
		prefs.VoiceProfileID = nil
	} else {
		prefs.VoiceProfileID = &profileID
	}

	return database.UpdateConversationPreferences(conversationID, prefs)
}

// PreviewVoiceSettings reproduz um texto de teste com configurações ad-hoc (sem salvar perfil)
func (a *App) PreviewVoiceSettings(provider, voiceID string, rate, pitch, volume float64, sampleText string) error {
	if sampleText == "" {
		sampleText = "Este é um teste das configurações de voz"
	}

	log.Printf("[PreviewVoiceSettings] provider=%s, voiceID=%s, rate=%.2f", provider, voiceID, rate)

	// Inicializa speechManager se necessário
	if a.speechManager == nil {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("erro ao carregar config: %w", err)
		}
		if cfg.APIKey == "" {
			return fmt.Errorf("API key não configurada")
		}
		log.Printf("[PreviewVoiceSettings] Inicializando speechManager...")
		a.InitSpeechManager(cfg.APIKey, cfg.APIBaseURL, "pt", voiceID, "tts-1")
	}

	// Configura as opções temporariamente
	if provider == "openai" {
		a.speechManager.SetTTSVoice(voiceID)
		// OpenAI TTS usa rate diretamente (0.25 a 4.0), não precisa converter
		log.Printf("[PreviewVoiceSettings] Configurando voz OpenAI: %s, rate=%.2f", voiceID, rate)
	}

	// Sintetiza usando a voz específica
	log.Printf("[PreviewVoiceSettings] Sintetizando texto: %s", sampleText[:min(50, len(sampleText))])
	result, err := a.speechManager.SynthesizeWithVoice(sampleText, voiceID)
	if err != nil {
		log.Printf("[PreviewVoiceSettings] Erro ao sintetizar: %v", err)
		return fmt.Errorf("erro ao sintetizar: %w", err)
	}

	log.Printf("[PreviewVoiceSettings] Áudio gerado, emitindo evento...")

	// Emite evento com o áudio para o frontend tocar
	runtime.EventsEmit(a.ctx, "voice_profile:preview", map[string]interface{}{
		"audio_base64": result.AudioBase64,
		"format":       result.Format,
	})

	log.Printf("[PreviewVoiceSettings] Evento emitido com sucesso")
	return nil
}
