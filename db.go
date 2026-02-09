package main

import (
	"fmt"
	"log"
	"sort"

	"assistente/internal/config"
	"assistente/internal/database"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// Re-exporta tipos do pacote database para manter compatibilidade
type Conversation = database.Conversation
type ChatMessage = database.ChatMessage
type ChatPreferences = database.ChatPreferences
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
		Reasoning:        msg.Reasoning,
		Media:            msg.Media,
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
			fmt.Printf("🌳 [TREE] %sID=%s, role=%s, children=%d\n",
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
	fmt.Printf("🗑️ [DeleteConversation] Iniciando deleção da conversa %d...\n", id)

	// Antes de deletar, limpa as abas que referenciam essa conversa
	tabs, err := database.GetAllTabs()
	if err == nil {
		for _, tab := range tabs {
			if tab.ConversationID != nil && *tab.ConversationID == id {
				fmt.Printf("🗑️ [DeleteConversation] Limpando tab %d que referencia conversa %d\n", tab.ID, id)
				database.ClearTab(tab.ID)
			}
		}
	}

	// Deleta conversa normalmente
	if err := database.DeleteConversation(id); err != nil {
		fmt.Printf("🗑️ [DeleteConversation] ERRO ao deletar: %v\n", err)
		return err
	}

	fmt.Printf("🗑️ [DeleteConversation] Conversa deletada, emitindo evento...\n")

	// Emite evento
	runtime.EventsEmit(a.ctx, "conversation:deleted", map[string]interface{}{
		"conversation_id": id,
	})

	fmt.Printf("🗑️ [DeleteConversation] Evento 'conversation:deleted' emitido para conversa %d\n", id)

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

// UpdateMessage atualiza o conteúdo de uma mensagem existente
func (a *App) UpdateMessage(messageID uint, newContent string) error {
	// Atualiza apenas o conteúdo, mantendo tokens como 0
	// (tokens não são críticos para mensagens editadas manualmente)
	if err := database.UpdateMessageContent(
		messageID,
		newContent,
		0,  // prompt_tokens
		0,  // completion_tokens
		0,  // total_tokens
		"", // model (mantém o original)
	); err != nil {
		return err
	}

	// Emite evento para o frontend atualizar a UI
	runtime.EventsEmit(a.ctx, "message:updated", map[string]interface{}{
		"message_id": messageID,
		"content":    newContent,
	})

	return nil
}

func (a *App) UpdateConversationModel(id uint, model string) error {
	return database.UpdateConversationModel(id, model)
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

func (a *App) CreateMessage(conversationID uint, role, content string) (*ChatMessage, error) {
	return database.CreateMessage(database.MessageOptions{
		ConversationID: conversationID,
		Role:           role,
		Content:        content,
	})
}

func (a *App) AddMessage(conversationID uint, role, content string) (*ChatMessage, error) {
	return database.AddMessage(conversationID, role, content)
}

func (a *App) AddMessageWithMedia(conversationID uint, role, content, media string) (*ChatMessage, error) {
	return database.AddMessageWithMedia(conversationID, role, content, media)
}

func (a *App) AddMessageWithTokens(conversationID uint, role, content string, promptTokens, completionTokens, totalTokens int, model string) (*ChatMessage, error) {
	return database.AddMessageWithTokens(conversationID, role, content, promptTokens, completionTokens, totalTokens, model)
}

func (a *App) AddMessageWithTokensAndMedia(conversationID uint, role, content, media string, promptTokens, completionTokens, totalTokens int, model string) (*ChatMessage, error) {
	return database.AddMessageWithTokensAndMedia(conversationID, role, content, media, promptTokens, completionTokens, totalTokens, model)
}

func (a *App) AddChildMessage(conversationID uint, parentID uint, role, content, model string) (*ChatMessage, error) {
	return database.AddChildMessage(conversationID, parentID, role, content, model)
}

func (a *App) GetConversationTokenStats(conversationID uint) (map[string]int, error) {
	return database.GetConversationTokenStats(conversationID)
}

func (a *App) GetAllTokenStats() (map[string]int, error) {
	return database.GetAllTokenStats()
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
	fmt.Printf("🧹 [ClearConversation] Limpando conversa %d...\n", conversationID)

	if err := database.DeleteAllMessages(conversationID); err != nil {
		fmt.Printf("🧹 [ClearConversation] ERRO ao limpar: %v\n", err)
		return err
	}

	fmt.Printf("🧹 [ClearConversation] Mensagens deletadas, emitindo evento...\n")

	// Emite evento para frontend atualizar
	runtime.EventsEmit(a.ctx, "conversation:cleared", map[string]interface{}{
		"conversation_id": conversationID,
	})

	fmt.Printf("🧹 [ClearConversation] Evento 'conversation:cleared' emitido para conversa %d\n", conversationID)

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

// SetConversationModel define o modelo de uma conversa
func (a *App) SetConversationModel(conversationID uint, model string) error {
	prefs, err := database.GetConversationPreferences(conversationID)
	if err != nil {
		prefs = &database.ChatPreferences{}
	}
	if prefs == nil {
		prefs = &database.ChatPreferences{}
	}

	// Se model é vazio, remove o modelo customizado (usa o padrão)
	prefs.Model = model

	return database.UpdateConversationPreferences(conversationID, prefs)
}

// GetConversationModel retorna o modelo de uma conversa (ou vazio se não definido)
func (a *App) GetConversationModel(conversationID uint) (string, error) {
	prefs, err := database.GetConversationPreferences(conversationID)
	if err != nil {
		return "", nil // Sem preferências = usar padrão
	}
	if prefs == nil {
		return "", nil
	}
	return prefs.Model, nil
}

// GetEffectiveModel retorna o modelo efetivo de uma conversa (da conversa ou padrão)
func (a *App) GetEffectiveModel(conversationID uint) (string, error) {
	// Primeiro tenta obter da conversa
	model, _ := a.GetConversationModel(conversationID)
	if model != "" {
		return model, nil
	}

	// Se não tem na conversa, usa o padrão do config
	cfg, err := config.Load()
	if err != nil {
		return "", err
	}
	return cfg.DefaultModel, nil
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
	previewLen := len(sampleText)
	if previewLen > 50 {
		previewLen = 50
	}
	log.Printf("[PreviewVoiceSettings] Sintetizando texto: %s", sampleText[:previewLen])
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
