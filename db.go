package main

import (
	"fmt"
	"sort"
	"time"

	"assistente/internal/database"
)

// Re-exporta tipos do pacote database para manter compatibilidade
type Conversation = database.Conversation
type ChatMessage = database.ChatMessage
type Memory = database.Memory
type FAQ = database.FAQ
type AgentConfig = database.AgentConfig
type HTTPAgent = database.HTTPAgent
type HTTPEndpoint = database.HTTPEndpoint
type MCPAgentDB = database.MCPAgentDB
type ModelCapability = database.ModelCapability
type OAuthConnection = database.OAuthConnection

// Re-exporta funções que não dependem de App
var (
	InitDatabase  = database.Init
	GenerateTitle = database.GenerateTitle
)

// ==================== Conversation ====================

func (a *App) CreateConversation(title, model string) (*Conversation, error) {
	return database.CreateConversation(title, model)
}

func (a *App) GetConversations() ([]Conversation, error) {
	return database.GetConversations()
}

func (a *App) GetConversation(id uint) (*Conversation, error) {
	return database.GetConversation(id)
}

// GetConversationWithThreads retorna uma conversa com mensagens de nível 0 apenas (lazy loading)
// Os filhos são carregados sob demanda via GetMessageChildren
func (a *App) GetConversationWithThreads(id uint) (*ConversationWithThreads, error) {
	conv, err := database.GetConversation(id)
	if err != nil {
		return nil, err
	}

	// Filtra apenas mensagens raiz (nível 0) - sem ParentID
	var rootMessages []database.ChatMessage
	childCounts := make(map[uint]int) // Conta filhos de cada mensagem
	
	for _, msg := range conv.Messages {
		if msg.ParentID == nil {
			rootMessages = append(rootMessages, msg)
		} else {
			// Conta quantos filhos cada mensagem tem
			childCounts[*msg.ParentID]++
		}
	}

	// Converte para MessageNode com contagem de filhos (sem carregar os filhos)
	threads := make([]MessageNode, 0, len(rootMessages))
	for _, msg := range rootMessages {
		threads = append(threads, MessageNode{
			Message:    msg,
			Children:   nil, // Não carrega filhos - lazy loading
			Level:      0,
			ChildCount: childCounts[msg.ID],
		})
	}

	fmt.Printf("🌳 [LAZY] Conversa %d: %d raízes, filhos serão carregados sob demanda\n", id, len(threads))

	return &ConversationWithThreads{
		ID:                   conv.ID,
		Title:                conv.Title,
		Model:                conv.Model,
		ShowInternalMessages: conv.ShowInternalMessages,
		Threads:              threads,
	}, nil
}

// GetMessageChildren retorna os filhos diretos de uma mensagem (lazy loading)
func (a *App) GetMessageChildren(messageID uint) ([]MessageNode, error) {
	children, err := database.GetMessageChildren(messageID)
	if err != nil {
		return nil, err
	}

	// Conta netos para cada filho
	grandchildCounts := make(map[uint]int)
	for _, child := range children {
		gc, _ := database.GetMessageChildren(child.ID)
		grandchildCounts[child.ID] = len(gc)
	}

	// Converte para MessageNode
	result := make([]MessageNode, 0, len(children))
	for _, msg := range children {
		result = append(result, MessageNode{
			Message:    msg,
			Children:   nil, // Lazy loading
			Level:      1,   // Será ajustado pelo frontend se necessário
			ChildCount: grandchildCounts[msg.ID],
		})
	}

	fmt.Printf("🌳 [LAZY] Mensagem %d: %d filhos carregados\n", messageID, len(result))
	return result, nil
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
			Message:  msg,
			Children: []MessageNode{},
			Level:    level,
		}
		
		// Adiciona filhos recursivamente
		children := childrenMap[msg.ID]
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
	return database.UpdateConversation(id, title, model)
}

func (a *App) DeleteConversation(id uint) error {
	return database.DeleteConversation(id)
}

func (a *App) UpdateConversationModel(id uint, model string) error {
	return database.UpdateConversationModel(id, model)
}

func (a *App) UpdateConversationSettings(id uint, showInternalMessages bool) error {
	return database.UpdateConversationSettings(id, showInternalMessages)
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
	return database.CreateMemory(title, content, category)
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
	return database.UpdateMemory(id, title, content, category)
}

func (a *App) DeleteMemory(id uint) error {
	return database.DeleteMemory(id)
}

func (a *App) GetCoreMemories() ([]Memory, error) {
	return database.GetCoreMemories()
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
	return database.CreateHTTPEndpoint(httpAgentID, name, description, method, pathTemplate, queryTemplate, headersJSON, bodyTemplate, parameters, responseTemplate)
}

func (a *App) GetHTTPEndpoint(id uint) (*HTTPEndpoint, error) {
	return database.GetHTTPEndpoint(id)
}

func (a *App) GetHTTPEndpointsByAgentID(httpAgentID uint) ([]HTTPEndpoint, error) {
	return database.GetHTTPEndpointsByAgentID(httpAgentID)
}

func (a *App) UpdateHTTPEndpoint(id uint, name, description, method, pathTemplate, queryTemplate, headersJSON, bodyTemplate, parameters, responseTemplate string) (*HTTPEndpoint, error) {
	return database.UpdateHTTPEndpoint(id, name, description, method, pathTemplate, queryTemplate, headersJSON, bodyTemplate, parameters, responseTemplate)
}

func (a *App) DeleteHTTPEndpoint(id uint) error {
	return database.DeleteHTTPEndpoint(id)
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
