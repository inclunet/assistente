package database

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"time"

	"assistente/internal/config"

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
		&VoiceProfile{},
		&InteractionProfile{},
		&InteractionTrigger{},
		&ChatProfile{},
	); err != nil {
		return err
	}

	// Seed: cria perfil de voz padrão "Desativado" se não existir
	if err := seedDefaultVoiceProfile(); err != nil {
		fmt.Printf("Aviso: erro ao criar perfil de voz padrão: %v\n", err)
	}

	// Seed: cria perfil de interação padrão "Manual" se não existir
	if err := seedDefaultInteractionProfile(); err != nil {
		fmt.Printf("Aviso: erro ao criar perfil de interação padrão: %v\n", err)
	}

	// Seed: cria perfis de conversa padrão se não existirem
	if err := seedDefaultChatProfiles(); err != nil {
		fmt.Printf("Aviso: erro ao criar perfis de conversa padrão: %v\n", err)
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
		// Conversa não existe (foi deletada) - retorna erro especial
		// Os chamadores devem verificar ErrConversationDeleted e abortar graciosamente
		return nil, fmt.Errorf("%w: conversa %d", ErrConversationDeleted, opts.ConversationID)
	}

	// Verifica se a mensagem pai existe (se parentId foi fornecido)
	if opts.ParentID != nil && *opts.ParentID > 0 {
		var parentMsg ChatMessage
		if err := db.First(&parentMsg, *opts.ParentID).Error; err != nil {
			// Mensagem pai não existe (foi deletada no clear) - retorna erro especial
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
// Usada para mensagens internas de agentes e tools
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

// DeleteMessage exclui uma mensagem e todas as suas filhas (respostas)
func DeleteMessage(messageID uint) error {
	// Primeiro, exclui recursivamente todas as mensagens filhas
	var childIDs []uint
	if err := db.Model(&ChatMessage{}).Where("parent_id = ?", messageID).Pluck("id", &childIDs).Error; err != nil {
		return err
	}
	for _, childID := range childIDs {
		if err := DeleteMessage(childID); err != nil {
			return err
		}
	}
	// Exclui a mensagem em si
	return db.Delete(&ChatMessage{}, messageID).Error
}

// DeleteAllMessages remove todas as mensagens de uma conversa
func DeleteAllMessages(conversationID uint) error {
	return db.Where("conversation_id = ?", conversationID).Delete(&ChatMessage{}).Error
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

// ==================== Interaction Profile CRUD ====================

// migrateInteractionProfilesToTriggers migra perfis antigos para a nova estrutura com triggers
func migrateInteractionProfilesToTriggers() error {
	// Verifica se existem perfis sem triggers
	var profiles []InteractionProfile
	if err := db.Find(&profiles).Error; err != nil {
		return err
	}

	for _, profile := range profiles {
		// Verifica se o perfil tem triggers
		var triggerCount int64
		if err := db.Model(&InteractionTrigger{}).Where("profile_id = ?", profile.ID).Count(&triggerCount).Error; err != nil {
			continue
		}

		if triggerCount == 0 {
			// Perfil sem triggers - cria trigger padrão PTT (push-to-talk)
			log.Printf("[Database] Migrando perfil '%s' (ID: %d) - criando trigger PTT padrão", profile.Name, profile.ID)
			trigger := InteractionTrigger{
				ProfileID: profile.ID,
				Type:      TriggerTypeButtonPTT,
				Enabled:   true,
				AutoStop:  false, // PTT não usa VAD
			}
			if err := db.Create(&trigger).Error; err != nil {
				log.Printf("[Database] Erro ao criar trigger para perfil %d: %v", profile.ID, err)
			}
		}
	}

	return nil
}

// seedDefaultInteractionProfile cria os perfis de interação padrão
func seedDefaultInteractionProfile() error {
	// Primeiro, migra perfis existentes para a nova estrutura
	if err := migrateInteractionProfilesToTriggers(); err != nil {
		log.Printf("[Database] Erro na migração de triggers: %v", err)
	}

	// Verifica se já existe algum perfil
	var count int64
	if err := db.Model(&InteractionProfile{}).Count(&count).Error; err != nil {
		return err
	}

	if count > 0 {
		// Já existem perfis (possivelmente migrados)
		return nil
	}

	// Cria perfil PTT (padrão de fábrica e ativo por padrão)
	pttProfile := InteractionProfile{
		Name:           "PTT (Push-to-Talk)",
		Description:    "Segure o botão para gravar. Modo padrão de fábrica.",
		IsDefault:      true,
		IsActive:       true, // Perfil ativo inicial
		STTProvider:    "webspeech",
		Language:       "pt-BR",
		FeedbackSounds: true,
	}
	if err := db.Create(&pttProfile).Error; err != nil {
		return err
	}
	// Adiciona trigger button_ptt
	db.Create(&InteractionTrigger{
		ProfileID: pttProfile.ID,
		Type:      TriggerTypeButtonPTT,
		Enabled:   true,
		AutoStop:  false, // PTT não usa VAD, solta o botão para parar
	})

	// Cria perfil Toggle (clique para iniciar/parar)
	toggleProfile := InteractionProfile{
		Name:           "Toggle",
		Description:    "Clique para iniciar, clique novamente para parar.",
		IsDefault:      false,
		STTProvider:    "webspeech",
		Language:       "pt-BR",
		FeedbackSounds: true,
	}
	if err := db.Create(&toggleProfile).Error; err != nil {
		return err
	}
	// Adiciona trigger button_toggle
	db.Create(&InteractionTrigger{
		ProfileID: toggleProfile.ID,
		Type:      TriggerTypeButtonToggle,
		Enabled:   true,
		AutoStop:  false,
	})

	// Cria perfil Desktop Rápido (com hotkey)
	desktopProfile := InteractionProfile{
		Name:           "Desktop Rápido",
		Description:    "Atalho Ctrl+Shift+Space traz janela e ativa gravação com VAD.",
		IsDefault:      false,
		STTProvider:    "webspeech",
		Language:       "pt-BR",
		FeedbackSounds: true,
	}
	if err := db.Create(&desktopProfile).Error; err != nil {
		return err
	}
	// Adiciona triggers
	db.Create(&InteractionTrigger{
		ProfileID:          desktopProfile.ID,
		Type:               TriggerTypeHotkey,
		Enabled:            true,
		AutoStop:           true,
		Hotkey:             "Ctrl+Shift+Space",
		HotkeyGlobal:       true,
		HotkeyBringToFront: true,
		VADSilenceDuration: 1500,
	})
	db.Create(&InteractionTrigger{
		ProfileID:          desktopProfile.ID,
		Type:               TriggerTypeButtonToggle,
		Enabled:            true,
		AutoStop:           true,
		VADSilenceDuration: 1500,
	})

	// Cria perfil Conversa com Wake Word
	wakewordProfile := InteractionProfile{
		Name:           "Conversa por Voz",
		Description:    "Diga 'assistente' para iniciar. Ctrl+W liga/desliga escuta.",
		IsDefault:      false,
		STTProvider:    "webspeech",
		Language:       "pt-BR",
		FeedbackSounds: true,
	}
	if err := db.Create(&wakewordProfile).Error; err != nil {
		return err
	}
	// Adiciona triggers
	db.Create(&InteractionTrigger{
		ProfileID:           wakewordProfile.ID,
		Type:                TriggerTypeWakeword,
		Enabled:             true,
		WakewordKeyword:     "assistente",
		WakewordProvider:    "webspeech",
		WakewordSensitivity: 0.5,
		Hotkey:              "Ctrl+W",
		HotkeyGlobal:        true,
		HotkeyBringToFront:  false,
		VADSilenceDuration:  1500,
	})
	db.Create(&InteractionTrigger{
		ProfileID:          wakewordProfile.ID,
		Type:               TriggerTypeButtonToggle,
		Enabled:            true,
		AutoStop:           true,
		VADSilenceDuration: 1500,
	})

	return nil
}

// seedDefaultChatProfiles cria os perfis de conversa padrão
func seedDefaultChatProfiles() error {
	// Verifica se já existe algum perfil
	var count int64
	if err := db.Model(&ChatProfile{}).Count(&count).Error; err != nil {
		return err
	}

	if count > 0 {
		// Já existem perfis
		return nil
	}

	// 1. Perfil "Padrão"
	defaultProfile := ChatProfile{
		Name:                 "Padrão",
		Description:          "Configuração padrão.",
		Icon:                 "💬",
		IsDefault:            true,
		Model:                "", // Será definido automaticamente ao configurar API
		Temperature:          0.7,
		MaxTokens:            4096,
		TopP:                 1.0,
		ResponseTimeout:      180,
		SystemPromptPosition: "after",
	}
	if err := db.Create(&defaultProfile).Error; err != nil {
		return err
	}
	fmt.Println("[Database] Perfil de conversa 'Padrão' criado com sucesso")

	// 2. Perfil "Modelo Local" - para modelos locais
	localProfile := ChatProfile{
		Name:                 "Modelo Local",
		Description:          "Para modelos locais (Ollama, LM Studio, etc.).",
		Icon:                 "🏠",
		IsDefault:            false,
		Model:                "",
		Temperature:          0.7,
		MaxTokens:            4096,
		TopP:                 1.0,
		ResponseTimeout:      300, // Modelos locais podem ser mais lentos
		SystemPromptPosition: "after",
	}
	if err := db.Create(&localProfile).Error; err != nil {
		return err
	}
	fmt.Println("[Database] Perfil de conversa 'Modelo Local' criado com sucesso")

	// 3. Perfil "Programação" - focado em código
	codeProfile := ChatProfile{
		Name:                 "Programação",
		Description:          "Otimizado para tarefas de desenvolvimento de software.",
		Icon:                 "💻",
		IsDefault:            false,
		Model:                "",
		Temperature:          0.3, // Mais determinístico para código
		MaxTokens:            8192,
		TopP:                 1.0,
		ResponseTimeout:      180,
		SystemPrompt:         "You are a programming assistant. Always provide code examples when relevant. Use markdown to format code. Prefer simple and idiomatic solutions.",
		SystemPromptPosition: "after",
	}
	if err := db.Create(&codeProfile).Error; err != nil {
		return err
	}
	fmt.Println("[Database] Perfil de conversa 'Programação' criado com sucesso")

	return nil
}

// CreateInteractionProfile cria um novo perfil de interação
func CreateInteractionProfile(profile *InteractionProfile) (*InteractionProfile, error) {
	if err := profile.Validate(); err != nil {
		return nil, err
	}

	// Se é default, remove default anterior
	if profile.IsDefault {
		db.Model(&InteractionProfile{}).Where("is_default = ?", true).Update("is_default", false)
	}

	if err := db.Create(profile).Error; err != nil {
		return nil, err
	}

	return profile, nil
}

// GetInteractionProfile retorna um perfil por ID com seus triggers
func GetInteractionProfile(id uint) (*InteractionProfile, error) {
	var profile InteractionProfile
	if err := db.Preload("Triggers").First(&profile, id).Error; err != nil {
		return nil, err
	}
	return &profile, nil
}

// GetInteractionProfileByName retorna um perfil por nome
func GetInteractionProfileByName(name string) (*InteractionProfile, error) {
	var profile InteractionProfile
	if err := db.Preload("Triggers").Where("name = ?", name).First(&profile).Error; err != nil {
		return nil, err
	}
	return &profile, nil
}

// GetAllInteractionProfiles retorna todos os perfis de interação com triggers
func GetAllInteractionProfiles() ([]InteractionProfile, error) {
	var profiles []InteractionProfile
	err := db.Preload("Triggers").Order("name ASC").Find(&profiles).Error
	return profiles, err
}

// GetDefaultInteractionProfile retorna o perfil de interação padrão
func GetDefaultInteractionProfile() (*InteractionProfile, error) {
	var profile InteractionProfile
	if err := db.Preload("Triggers").Where("is_default = ?", true).First(&profile).Error; err != nil {
		// Se não encontrou, retorna o primeiro
		if err := db.Preload("Triggers").First(&profile).Error; err != nil {
			return nil, err
		}
	}
	return &profile, nil
}

// GetActiveInteractionProfile retorna o perfil de interação atualmente ativo
func GetActiveInteractionProfile() (*InteractionProfile, error) {
	var profile InteractionProfile
	if err := db.Preload("Triggers").Where("is_active = ?", true).First(&profile).Error; err != nil {
		// Se não encontrou perfil ativo, retorna nil sem erro
		return nil, nil
	}
	return &profile, nil
}

// SetActiveInteractionProfile define qual perfil está ativo (persiste no banco)
func SetActiveInteractionProfile(profileID uint) error {
	// Desativa todos os perfis
	if err := db.Model(&InteractionProfile{}).Where("is_active = ?", true).Update("is_active", false).Error; err != nil {
		return err
	}

	// Ativa o perfil selecionado (0 = nenhum perfil ativo)
	if profileID > 0 {
		if err := db.Model(&InteractionProfile{}).Where("id = ?", profileID).Update("is_active", true).Error; err != nil {
			return err
		}
	}

	return nil
}

// UpdateInteractionProfile atualiza um perfil de interação
func UpdateInteractionProfile(id uint, profile *InteractionProfile) (*InteractionProfile, error) {
	if err := profile.Validate(); err != nil {
		return nil, err
	}

	// Busca o perfil existente
	var existing InteractionProfile
	if err := db.First(&existing, id).Error; err != nil {
		return nil, err
	}

	// Se está se tornando default, remove default anterior
	if profile.IsDefault && !existing.IsDefault {
		db.Model(&InteractionProfile{}).Where("is_default = ?", true).Update("is_default", false)
	}

	// Atualiza todos os campos
	profile.ID = id
	profile.CreatedAt = existing.CreatedAt

	if err := db.Save(profile).Error; err != nil {
		return nil, err
	}

	// Retorna com triggers
	return GetInteractionProfile(id)
}

// DeleteInteractionProfile deleta um perfil de interação (triggers são deletados em cascata)
func DeleteInteractionProfile(id uint) error {
	return db.Delete(&InteractionProfile{}, id).Error
}

// SetDefaultInteractionProfile define um perfil como padrão
func SetDefaultInteractionProfile(id uint) error {
	// Remove default anterior
	if err := db.Model(&InteractionProfile{}).Where("is_default = ?", true).Update("is_default", false).Error; err != nil {
		return err
	}
	// Define o novo default
	return db.Model(&InteractionProfile{}).Where("id = ?", id).Update("is_default", true).Error
}

// SearchInteractionProfiles busca perfis por nome ou descrição
func SearchInteractionProfiles(query string) ([]InteractionProfile, error) {
	var profiles []InteractionProfile
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return GetAllInteractionProfiles()
	}
	searchTerm := "%" + query + "%"
	err := db.Preload("Triggers").Where(
		"LOWER(name) LIKE ? OR LOWER(description) LIKE ?",
		searchTerm, searchTerm,
	).Order("name ASC").Find(&profiles).Error
	return profiles, err
}

// ==================== Interaction Trigger CRUD ====================

// CreateInteractionTrigger cria um novo trigger
func CreateInteractionTrigger(trigger *InteractionTrigger) (*InteractionTrigger, error) {
	if err := trigger.Validate(); err != nil {
		return nil, err
	}

	if err := db.Create(trigger).Error; err != nil {
		return nil, err
	}

	return trigger, nil
}

// GetInteractionTrigger retorna um trigger por ID
func GetInteractionTrigger(id uint) (*InteractionTrigger, error) {
	var trigger InteractionTrigger
	if err := db.First(&trigger, id).Error; err != nil {
		return nil, err
	}
	return &trigger, nil
}

// GetTriggersByProfile retorna todos os triggers de um perfil
func GetTriggersByProfile(profileID uint) ([]InteractionTrigger, error) {
	var triggers []InteractionTrigger
	err := db.Where("profile_id = ?", profileID).Find(&triggers).Error
	return triggers, err
}

// UpdateInteractionTrigger atualiza um trigger
func UpdateInteractionTrigger(id uint, trigger *InteractionTrigger) (*InteractionTrigger, error) {
	if err := trigger.Validate(); err != nil {
		return nil, err
	}

	// Busca o trigger existente
	var existing InteractionTrigger
	if err := db.First(&existing, id).Error; err != nil {
		return nil, err
	}

	// Atualiza todos os campos
	trigger.ID = id
	trigger.ProfileID = existing.ProfileID
	trigger.CreatedAt = existing.CreatedAt

	if err := db.Save(trigger).Error; err != nil {
		return nil, err
	}

	return trigger, nil
}

// DeleteInteractionTrigger deleta um trigger
func DeleteInteractionTrigger(id uint) error {
	return db.Delete(&InteractionTrigger{}, id).Error
}

// DeleteTriggersByProfile deleta todos os triggers de um perfil
func DeleteTriggersByProfile(profileID uint) error {
	return db.Where("profile_id = ?", profileID).Delete(&InteractionTrigger{}).Error
}

// ==================== Chat Profile CRUD ====================

// CreateChatProfile cria um novo perfil de conversa
func CreateChatProfile(profile *ChatProfile) (*ChatProfile, error) {
	if err := profile.Validate(); err != nil {
		return nil, err
	}

	// Se é default, remove default anterior
	if profile.IsDefault {
		db.Model(&ChatProfile{}).Where("is_default = ?", true).Update("is_default", false)
	}

	if err := db.Create(profile).Error; err != nil {
		return nil, err
	}

	return profile, nil
}

// GetChatProfile retorna um perfil de conversa por ID
func GetChatProfile(id uint) (*ChatProfile, error) {
	var profile ChatProfile
	if err := db.First(&profile, id).Error; err != nil {
		return nil, err
	}
	return &profile, nil
}

// GetAllChatProfiles retorna todos os perfis de conversa
func GetAllChatProfiles() ([]ChatProfile, error) {
	var profiles []ChatProfile
	err := db.Order("is_default DESC, name ASC").Find(&profiles).Error
	return profiles, err
}

// GetDefaultChatProfile retorna o perfil de conversa padrão
func GetDefaultChatProfile() (*ChatProfile, error) {
	var profile ChatProfile
	if err := db.Where("is_default = ?", true).First(&profile).Error; err != nil {
		return nil, err
	}
	return &profile, nil
}

// UpdateChatProfile atualiza um perfil de conversa
func UpdateChatProfile(id uint, profile *ChatProfile) (*ChatProfile, error) {
	if err := profile.Validate(); err != nil {
		return nil, err
	}

	// Busca o perfil existente
	var existing ChatProfile
	if err := db.First(&existing, id).Error; err != nil {
		return nil, err
	}

	// Se está se tornando default, remove default anterior
	if profile.IsDefault && !existing.IsDefault {
		db.Model(&ChatProfile{}).Where("is_default = ?", true).Update("is_default", false)
	}

	// Atualiza todos os campos
	profile.ID = id
	profile.CreatedAt = existing.CreatedAt

	if err := db.Save(profile).Error; err != nil {
		return nil, err
	}

	return GetChatProfile(id)
}

// DeleteChatProfile deleta um perfil de conversa
func DeleteChatProfile(id uint) error {
	// Não permite deletar o perfil padrão
	var profile ChatProfile
	if err := db.First(&profile, id).Error; err != nil {
		return err
	}
	if profile.IsDefault {
		return fmt.Errorf("não é possível deletar o perfil padrão")
	}

	// Remove referências em conversas
	db.Model(&Conversation{}).Where("chat_profile_id = ?", id).Update("chat_profile_id", nil)

	return db.Delete(&ChatProfile{}, id).Error
}

// SetDefaultChatProfile define um perfil como padrão
func SetDefaultChatProfile(id uint) error {
	// Remove default anterior
	if err := db.Model(&ChatProfile{}).Where("is_default = ?", true).Update("is_default", false).Error; err != nil {
		return err
	}
	// Define o novo default
	return db.Model(&ChatProfile{}).Where("id = ?", id).Update("is_default", true).Error
}

// SearchChatProfiles busca perfis por nome ou descrição
func SearchChatProfiles(query string) ([]ChatProfile, error) {
	var profiles []ChatProfile
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return GetAllChatProfiles()
	}
	searchTerm := "%" + query + "%"
	err := db.Where(
		"LOWER(name) LIKE ? OR LOWER(description) LIKE ?",
		searchTerm, searchTerm,
	).Order("is_default DESC, name ASC").Find(&profiles).Error
	return profiles, err
}

// ==================== Chat Profile - Conversation Integration ====================

// SetConversationChatProfile define o perfil de conversa para uma conversa
func SetConversationChatProfile(conversationID uint, profileID uint) error {
	return db.Model(&Conversation{}).Where("id = ?", conversationID).Update("chat_profile_id", profileID).Error
}

// ClearConversationChatProfile remove o perfil customizado de uma conversa (usa padrão)
func ClearConversationChatProfile(conversationID uint) error {
	return db.Model(&Conversation{}).Where("id = ?", conversationID).Update("chat_profile_id", nil).Error
}

// GetConversationChatProfile retorna o perfil de conversa de uma conversa (ou nil se usar padrão)
func GetConversationChatProfile(conversationID uint) (*ChatProfile, error) {
	var conversation Conversation
	if err := db.First(&conversation, conversationID).Error; err != nil {
		return nil, err
	}

	if conversation.ChatProfileID == nil {
		return nil, nil // Usa perfil padrão
	}

	return GetChatProfile(*conversation.ChatProfileID)
}

// GetEffectiveChatProfile retorna o perfil efetivo de uma conversa (da conversa ou padrão)
func GetEffectiveChatProfile(conversationID uint) (*ChatProfile, error) {
	// Tenta obter perfil da conversa
	profile, err := GetConversationChatProfile(conversationID)
	if err != nil {
		return nil, err
	}

	if profile != nil {
		return profile, nil
	}

	// Usa perfil padrão
	return GetDefaultChatProfile()
}
