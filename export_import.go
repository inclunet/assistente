package main

import (
	"encoding/json"
	"fmt"
	"time"

	"assistente/internal/database"
)

// ==================== Export/Import Types ====================

// ExportMetadata contém metadados do arquivo de exportação
type ExportMetadata struct {
	Version    string    `json:"version"`
	ExportedAt time.Time `json:"exported_at"`
	Type       string    `json:"type"` // "conversations", "faqs", "memories", "agents"
	Count      int       `json:"count"`
}

// ConversationExport representa uma conversa exportada com todas as mensagens
type ConversationExport struct {
	ID          uint                       `json:"id"`
	Title       string                     `json:"title"`
	Preferences *database.ChatPreferences  `json:"preferences,omitempty"`
	CreatedAt   time.Time                  `json:"created_at"`
	UpdatedAt   time.Time                  `json:"updated_at"`
	Messages    []database.ChatMessage     `json:"messages"`
}

// ConversationsExportFile representa o arquivo de exportação de conversas
type ConversationsExportFile struct {
	Metadata      ExportMetadata       `json:"metadata"`
	Conversations []ConversationExport `json:"conversations"`
}

// FAQExport representa uma FAQ exportada
type FAQExport struct {
	ID        uint      `json:"id"`
	Question  string    `json:"question"`
	Answer    string    `json:"answer"`
	Tags      string    `json:"tags,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// FAQsExportFile representa o arquivo de exportação de FAQs
type FAQsExportFile struct {
	Metadata ExportMetadata `json:"metadata"`
	FAQs     []FAQExport    `json:"faqs"`
}

// MemoryExport representa uma memória exportada
type MemoryExport struct {
	ID        uint      `json:"id"`
	Title     string    `json:"title"`
	Content   string    `json:"content"`
	Category  string    `json:"category,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// MemoriesExportFile representa o arquivo de exportação de memórias
type MemoriesExportFile struct {
	Metadata ExportMetadata `json:"metadata"`
	Memories []MemoryExport `json:"memories"`
}

// HTTPAgentExport representa um HTTP Agent exportado
type HTTPAgentExport struct {
	// AgentConfig fields
	Name         string `json:"name"`
	DisplayName  string `json:"display_name"`
	Description  string `json:"description"`
	Model        string `json:"model"`
	SystemPrompt string `json:"system_prompt"`
	Enabled      bool   `json:"enabled"`
	// HTTPAgent fields
	BaseURL        string                   `json:"base_url"`
	AuthType       string                   `json:"auth_type"`
	AuthConfig     string                   `json:"auth_config,omitempty"`
	DefaultHeaders string                   `json:"default_headers,omitempty"`
	TimeoutSeconds int                      `json:"timeout_seconds"`
	RetryCount     int                      `json:"retry_count"`
	Endpoints      []HTTPEndpointExport     `json:"endpoints"`
}

// HTTPEndpointExport representa um endpoint HTTP exportado
type HTTPEndpointExport struct {
	Name             string `json:"name"`
	Description      string `json:"description"`
	Method           string `json:"method"`
	PathTemplate     string `json:"path_template"`
	QueryTemplate    string `json:"query_template,omitempty"`
	HeadersJSON      string `json:"headers_json,omitempty"`
	BodyTemplate     string `json:"body_template,omitempty"`
	Parameters       string `json:"parameters,omitempty"`
	ResponseTemplate string `json:"response_template,omitempty"`
}

// MCPAgentExport representa um MCP Agent exportado
type MCPAgentExport struct {
	// AgentConfig fields
	Name         string `json:"name"`
	DisplayName  string `json:"display_name"`
	Description  string `json:"description"`
	Model        string `json:"model"`
	SystemPrompt string `json:"system_prompt"`
	Enabled      bool   `json:"enabled"`
	// MCPAgent fields
	TransportType string `json:"transport_type"`
	ServerCommand string `json:"server_command,omitempty"`
	ServerArgs    string `json:"server_args,omitempty"`
	ServerEnv     string `json:"server_env,omitempty"`
	WorkingDir    string `json:"working_dir,omitempty"`
	ServerURL     string `json:"server_url,omitempty"`
	AuthType      string `json:"auth_type,omitempty"`
	AuthValue     string `json:"auth_value,omitempty"`
	HTTPHeaders   string `json:"http_headers,omitempty"`
	ExecutionMode string `json:"execution_mode"`
	AutoConnect   bool   `json:"auto_connect"`
}

// AgentsExportFile representa o arquivo de exportação de agentes
type AgentsExportFile struct {
	Metadata   ExportMetadata    `json:"metadata"`
	HTTPAgents []HTTPAgentExport `json:"http_agents,omitempty"`
	MCPAgents  []MCPAgentExport  `json:"mcp_agents,omitempty"`
}

// VoiceProfileExport representa um perfil de voz exportado
type VoiceProfileExport struct {
	ID              uint      `json:"id"`
	Name            string    `json:"name"`
	Description     string    `json:"description,omitempty"`
	Provider        string    `json:"provider"`
	VoiceID         string    `json:"voice_id"`
	Rate            float64   `json:"rate"`
	Pitch           float64   `json:"pitch"`
	Volume          float64   `json:"volume"`
	EnabledForAgent bool      `json:"enabled_for_agent"`
	EnabledForUser  bool      `json:"enabled_for_user"`
	IsDefault       bool      `json:"is_default"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// VoiceProfilesExportFile representa o arquivo de exportação de perfis de voz
type VoiceProfilesExportFile struct {
	Metadata      ExportMetadata       `json:"metadata"`
	VoiceProfiles []VoiceProfileExport `json:"voice_profiles"`
}

// ImportResult representa o resultado de uma importação
type ImportResult struct {
	Success   bool     `json:"success"`
	Imported  int      `json:"imported"`
	Skipped   int      `json:"skipped"`
	Errors    []string `json:"errors,omitempty"`
	Message   string   `json:"message"`
}

// ==================== Export Functions ====================

// ExportConversations exporta conversas selecionadas
func (a *App) ExportConversations(ids []uint) (string, error) {
	conversations := make([]ConversationExport, 0, len(ids))

	for _, id := range ids {
		conv, err := database.GetConversation(id)
		if err != nil {
			return "", fmt.Errorf("erro ao buscar conversa %d: %w", id, err)
		}

		export := ConversationExport{
			ID:          conv.ID,
			Title:       conv.Title,
			Preferences: conv.GetPreferences(),
			CreatedAt:   conv.CreatedAt,
			UpdatedAt:   conv.UpdatedAt,
			Messages:    conv.Messages,
		}
		conversations = append(conversations, export)
	}

	exportFile := ConversationsExportFile{
		Metadata: ExportMetadata{
			Version:    "1.0",
			ExportedAt: time.Now(),
			Type:       "conversations",
			Count:      len(conversations),
		},
		Conversations: conversations,
	}

	jsonData, err := json.MarshalIndent(exportFile, "", "  ")
	if err != nil {
		return "", fmt.Errorf("erro ao serializar conversas: %w", err)
	}

	return string(jsonData), nil
}

// ExportFAQs exporta FAQs selecionadas
func (a *App) ExportFAQs(ids []uint) (string, error) {
	faqs := make([]FAQExport, 0, len(ids))

	for _, id := range ids {
		faq, err := database.GetFAQ(id)
		if err != nil {
			return "", fmt.Errorf("erro ao buscar FAQ %d: %w", id, err)
		}

		export := FAQExport{
			ID:        faq.ID,
			Question:  faq.Question,
			Answer:    faq.Answer,
			Tags:      faq.Tags,
			CreatedAt: faq.CreatedAt,
			UpdatedAt: faq.UpdatedAt,
		}
		faqs = append(faqs, export)
	}

	exportFile := FAQsExportFile{
		Metadata: ExportMetadata{
			Version:    "1.0",
			ExportedAt: time.Now(),
			Type:       "faqs",
			Count:      len(faqs),
		},
		FAQs: faqs,
	}

	jsonData, err := json.MarshalIndent(exportFile, "", "  ")
	if err != nil {
		return "", fmt.Errorf("erro ao serializar FAQs: %w", err)
	}

	return string(jsonData), nil
}

// ExportMemories exporta memórias selecionadas
func (a *App) ExportMemories(ids []uint) (string, error) {
	memories := make([]MemoryExport, 0, len(ids))

	for _, id := range ids {
		mem, err := database.GetMemory(id)
		if err != nil {
			return "", fmt.Errorf("erro ao buscar memória %d: %w", id, err)
		}

		export := MemoryExport{
			ID:        mem.ID,
			Title:     mem.Title,
			Content:   mem.Content,
			Category:  mem.Category,
			CreatedAt: mem.CreatedAt,
			UpdatedAt: mem.UpdatedAt,
		}
		memories = append(memories, export)
	}

	exportFile := MemoriesExportFile{
		Metadata: ExportMetadata{
			Version:    "1.0",
			ExportedAt: time.Now(),
			Type:       "memories",
			Count:      len(memories),
		},
		Memories: memories,
	}

	jsonData, err := json.MarshalIndent(exportFile, "", "  ")
	if err != nil {
		return "", fmt.Errorf("erro ao serializar memórias: %w", err)
	}

	return string(jsonData), nil
}

// ExportAgents exporta agentes selecionados (HTTP e MCP)
func (a *App) ExportAgents(httpAgentIDs []uint, mcpAgentIDs []uint) (string, error) {
	httpAgents := make([]HTTPAgentExport, 0)
	mcpAgents := make([]MCPAgentExport, 0)

	// Exporta HTTP Agents
	for _, id := range httpAgentIDs {
		httpAgent, err := database.GetHTTPAgent(id)
		if err != nil {
			return "", fmt.Errorf("erro ao buscar HTTP Agent %d: %w", id, err)
		}

		agentConfig, err := database.GetAgentConfigByID(httpAgent.AgentConfigID)
		if err != nil {
			return "", fmt.Errorf("erro ao buscar config do HTTP Agent %d: %w", id, err)
		}

		endpoints, err := a.agentManager.GetHTTPEndpointsByAgentID(id)
		if err != nil {
			return "", fmt.Errorf("erro ao buscar endpoints do HTTP Agent %d: %w", id, err)
		}

		exportEndpoints := make([]HTTPEndpointExport, len(endpoints))
		for i, ep := range endpoints {
			exportEndpoints[i] = HTTPEndpointExport{
				Name:             ep.Name,
				Description:      ep.Description,
				Method:           ep.Method,
				PathTemplate:     ep.PathTemplate,
				QueryTemplate:    ep.QueryTemplate,
				HeadersJSON:      ep.HeadersJSON,
				BodyTemplate:     ep.BodyTemplate,
				Parameters:       ep.Parameters,
				ResponseTemplate: ep.ResponseTemplate,
			}
		}

		export := HTTPAgentExport{
			Name:           agentConfig.Name,
			DisplayName:    agentConfig.DisplayName,
			Description:    agentConfig.Description,
			Model:          agentConfig.Model,
			SystemPrompt:   agentConfig.SystemPrompt,
			Enabled:        agentConfig.Enabled,
			BaseURL:        httpAgent.BaseURL,
			AuthType:       httpAgent.AuthType,
			AuthConfig:     httpAgent.AuthConfig,
			DefaultHeaders: httpAgent.DefaultHeaders,
			TimeoutSeconds: httpAgent.TimeoutSeconds,
			RetryCount:     httpAgent.RetryCount,
			Endpoints:      exportEndpoints,
		}
		httpAgents = append(httpAgents, export)
	}

	// Exporta MCP Agents
	for _, id := range mcpAgentIDs {
		mcpAgent, err := database.GetMCPAgent(id)
		if err != nil {
			return "", fmt.Errorf("erro ao buscar MCP Agent %d: %w", id, err)
		}

		agentConfig, err := database.GetAgentConfigByID(mcpAgent.AgentConfigID)
		if err != nil {
			return "", fmt.Errorf("erro ao buscar config do MCP Agent %d: %w", id, err)
		}

		export := MCPAgentExport{
			Name:          agentConfig.Name,
			DisplayName:   agentConfig.DisplayName,
			Description:   agentConfig.Description,
			Model:         agentConfig.Model,
			SystemPrompt:  agentConfig.SystemPrompt,
			Enabled:       agentConfig.Enabled,
			TransportType: mcpAgent.TransportType,
			ServerCommand: mcpAgent.ServerCommand,
			ServerArgs:    mcpAgent.ServerArgs,
			ServerEnv:     mcpAgent.ServerEnv,
			WorkingDir:    mcpAgent.WorkingDir,
			ServerURL:     mcpAgent.ServerURL,
			AuthType:      mcpAgent.AuthType,
			AuthValue:     mcpAgent.AuthValue,
			HTTPHeaders:   mcpAgent.HTTPHeaders,
			ExecutionMode: mcpAgent.ExecutionMode,
			AutoConnect:   mcpAgent.AutoConnect,
		}
		mcpAgents = append(mcpAgents, export)
	}

	exportFile := AgentsExportFile{
		Metadata: ExportMetadata{
			Version:    "1.0",
			ExportedAt: time.Now(),
			Type:       "agents",
			Count:      len(httpAgents) + len(mcpAgents),
		},
		HTTPAgents: httpAgents,
		MCPAgents:  mcpAgents,
	}

	jsonData, err := json.MarshalIndent(exportFile, "", "  ")
	if err != nil {
		return "", fmt.Errorf("erro ao serializar agentes: %w", err)
	}

	return string(jsonData), nil
}

// ==================== Import Functions ====================

// ImportConversations importa conversas de um JSON
func (a *App) ImportConversations(jsonData string) (*ImportResult, error) {
	var exportFile ConversationsExportFile
	if err := json.Unmarshal([]byte(jsonData), &exportFile); err != nil {
		return nil, fmt.Errorf("erro ao parsear JSON: %w", err)
	}

	result := &ImportResult{
		Success: true,
		Errors:  make([]string, 0),
	}

	for _, conv := range exportFile.Conversations {
		// Cria nova conversa
		newConv, err := database.CreateConversation(conv.Title, "")
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("Erro ao criar conversa '%s': %v", conv.Title, err))
			result.Skipped++
			continue
		}

		// Atualiza preferências se existirem
		if conv.Preferences != nil {
			database.UpdateConversationPreferences(newConv.ID, conv.Preferences)
		}

		// Mapeia IDs antigos para novos (para reconstruir hierarquia)
		idMap := make(map[uint]uint)

		// Importa mensagens mantendo a ordem e hierarquia
		for _, msg := range conv.Messages {
			var parentID *uint
			if msg.ParentID != nil {
				if newParentID, ok := idMap[*msg.ParentID]; ok {
					parentID = &newParentID
				}
			}

			newMsg, err := database.CreateMessage(database.MessageOptions{
				ConversationID:   newConv.ID,
				ParentID:         parentID,
				Role:             msg.Role,
				Content:          msg.Content,
				Media:            msg.Media,
				ToolCalls:        msg.ToolCalls,
				ToolCallID:       msg.ToolCallID,
				AgentName:        msg.AgentName,
				PromptTokens:     msg.PromptTokens,
				CompletionTokens: msg.CompletionTokens,
				TotalTokens:      msg.TotalTokens,
				Model:            msg.Model,
			})

			if err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("Erro ao importar mensagem: %v", err))
				continue
			}

			idMap[msg.ID] = newMsg.ID
		}

		result.Imported++
	}

	result.Message = fmt.Sprintf("Importadas %d conversas, %d ignoradas", result.Imported, result.Skipped)
	if len(result.Errors) > 0 {
		result.Success = false
	}

	return result, nil
}

// ImportFAQs importa FAQs de um JSON
func (a *App) ImportFAQs(jsonData string) (*ImportResult, error) {
	var exportFile FAQsExportFile
	if err := json.Unmarshal([]byte(jsonData), &exportFile); err != nil {
		return nil, fmt.Errorf("erro ao parsear JSON: %w", err)
	}

	result := &ImportResult{
		Success: true,
		Errors:  make([]string, 0),
	}

	for _, faq := range exportFile.FAQs {
		_, err := a.CreateFAQ(faq.Question, faq.Answer, faq.Tags)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("Erro ao criar FAQ '%s': %v", faq.Question[:min(50, len(faq.Question))], err))
			result.Skipped++
			continue
		}
		result.Imported++
	}

	result.Message = fmt.Sprintf("Importadas %d FAQs, %d ignoradas", result.Imported, result.Skipped)
	if len(result.Errors) > 0 {
		result.Success = false
	}

	return result, nil
}

// ImportMemories importa memórias de um JSON
func (a *App) ImportMemories(jsonData string) (*ImportResult, error) {
	var exportFile MemoriesExportFile
	if err := json.Unmarshal([]byte(jsonData), &exportFile); err != nil {
		return nil, fmt.Errorf("erro ao parsear JSON: %w", err)
	}

	result := &ImportResult{
		Success: true,
		Errors:  make([]string, 0),
	}

	for _, mem := range exportFile.Memories {
		_, err := database.CreateMemory(mem.Title, mem.Content, mem.Category)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("Erro ao criar memória '%s': %v", mem.Title, err))
			result.Skipped++
			continue
		}
		result.Imported++
	}

	result.Message = fmt.Sprintf("Importadas %d memórias, %d ignoradas", result.Imported, result.Skipped)
	if len(result.Errors) > 0 {
		result.Success = false
	}

	return result, nil
}

// ImportAgents importa agentes de um JSON
func (a *App) ImportAgents(jsonData string) (*ImportResult, error) {
	var exportFile AgentsExportFile
	if err := json.Unmarshal([]byte(jsonData), &exportFile); err != nil {
		return nil, fmt.Errorf("erro ao parsear JSON: %w", err)
	}

	result := &ImportResult{
		Success: true,
		Errors:  make([]string, 0),
	}

	// Importa HTTP Agents
	for _, httpExport := range exportFile.HTTPAgents {
		// Verifica se já existe um agente com esse nome
		existing, _ := database.GetAgentConfig(httpExport.Name)
		if existing != nil {
			// Gera nome único
			httpExport.Name = fmt.Sprintf("%s_imported_%d", httpExport.Name, time.Now().Unix())
		}

		// Cria o agente HTTP completo
		httpFull, err := a.CreateHTTPAgentFull(
			httpExport.Name,
			httpExport.DisplayName,
			httpExport.Description,
			httpExport.Model,
			httpExport.SystemPrompt,
			httpExport.Enabled,
			httpExport.BaseURL,
			httpExport.AuthType,
			httpExport.AuthConfig,
			httpExport.DefaultHeaders,
			httpExport.TimeoutSeconds,
			httpExport.RetryCount,
		)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("Erro ao criar HTTP Agent '%s': %v", httpExport.Name, err))
			result.Skipped++
			continue
		}

		// Cria os endpoints
		httpAgentID := httpFull.HTTPAgentID
		for _, ep := range httpExport.Endpoints {
			_, err := a.CreateHTTPEndpoint(
				httpAgentID,
				ep.Name,
				ep.Description,
				ep.Method,
				ep.PathTemplate,
				ep.QueryTemplate,
				ep.HeadersJSON,
				ep.BodyTemplate,
				ep.Parameters,
				ep.ResponseTemplate,
			)
			if err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("Erro ao criar endpoint '%s': %v", ep.Name, err))
			}
		}

		result.Imported++
	}

	// Importa MCP Agents
	for _, mcpExport := range exportFile.MCPAgents {
		// Verifica se já existe um agente com esse nome
		existing, _ := database.GetAgentConfig(mcpExport.Name)
		if existing != nil {
			// Gera nome único
			mcpExport.Name = fmt.Sprintf("%s_imported_%d", mcpExport.Name, time.Now().Unix())
		}

		_, err := a.CreateMCPAgentFull(
			mcpExport.Name,
			mcpExport.DisplayName,
			mcpExport.Description,
			mcpExport.Model,
			mcpExport.SystemPrompt,
			mcpExport.TransportType,
			mcpExport.ServerCommand,
			mcpExport.ServerArgs,
			mcpExport.ServerEnv,
			mcpExport.WorkingDir,
			mcpExport.ServerURL,
			mcpExport.AuthType,
			mcpExport.AuthValue,
			mcpExport.HTTPHeaders,
			mcpExport.ExecutionMode,
			mcpExport.AutoConnect,
			mcpExport.Enabled,
		)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("Erro ao criar MCP Agent '%s': %v", mcpExport.Name, err))
			result.Skipped++
			continue
		}

		result.Imported++
	}

	result.Message = fmt.Sprintf("Importados %d agentes, %d ignorados", result.Imported, result.Skipped)
	if len(result.Errors) > 0 {
		result.Success = false
	}

	return result, nil
}

// ExportVoiceProfiles exporta perfis de voz selecionados
func (a *App) ExportVoiceProfiles(ids []uint) (string, error) {
	profiles := make([]VoiceProfileExport, 0, len(ids))

	for _, id := range ids {
		profile, err := database.GetVoiceProfile(id)
		if err != nil {
			return "", fmt.Errorf("erro ao buscar perfil de voz %d: %w", id, err)
		}

		profiles = append(profiles, VoiceProfileExport{
			ID:              profile.ID,
			Name:            profile.Name,
			Description:     profile.Description,
			Provider:        profile.Provider,
			VoiceID:         profile.VoiceID,
			Rate:            profile.Rate,
			Pitch:           profile.Pitch,
			Volume:          profile.Volume,
			EnabledForAgent: profile.EnabledForAgent,
			EnabledForUser:  profile.EnabledForUser,
			IsDefault:       profile.IsDefault,
			CreatedAt:       profile.CreatedAt,
			UpdatedAt:       profile.UpdatedAt,
		})
	}

	exportFile := VoiceProfilesExportFile{
		Metadata: ExportMetadata{
			Version:    "1.0",
			ExportedAt: time.Now(),
			Type:       "voice_profiles",
			Count:      len(profiles),
		},
		VoiceProfiles: profiles,
	}

	jsonData, err := json.MarshalIndent(exportFile, "", "  ")
	if err != nil {
		return "", fmt.Errorf("erro ao serializar JSON: %w", err)
	}

	return string(jsonData), nil
}

// ImportVoiceProfiles importa perfis de voz de um JSON
func (a *App) ImportVoiceProfiles(jsonData string) (*ImportResult, error) {
	var exportFile VoiceProfilesExportFile
	if err := json.Unmarshal([]byte(jsonData), &exportFile); err != nil {
		return nil, fmt.Errorf("erro ao parsear JSON: %w", err)
	}

	result := &ImportResult{
		Success: true,
		Errors:  make([]string, 0),
	}

	for _, profileExport := range exportFile.VoiceProfiles {
		// Verifica se já existe um perfil com esse nome
		existing, _ := database.GetVoiceProfileByName(profileExport.Name)
		name := profileExport.Name
		if existing != nil {
			// Gera nome único
			name = fmt.Sprintf("%s_imported_%d", profileExport.Name, time.Now().Unix())
		}

		// Não importa como default se já existe um default
		isDefault := profileExport.IsDefault
		if isDefault {
			existingDefault, _ := database.GetDefaultVoiceProfile()
			if existingDefault != nil {
				isDefault = false // Não sobrescreve o default existente
			}
		}

		_, err := database.CreateVoiceProfileFull(database.VoiceProfileOptions{
			Name:            name,
			Description:     profileExport.Description,
			Provider:        profileExport.Provider,
			VoiceID:         profileExport.VoiceID,
			Rate:            profileExport.Rate,
			Pitch:           profileExport.Pitch,
			Volume:          profileExport.Volume,
			EnabledForAgent: profileExport.EnabledForAgent,
			EnabledForUser:  profileExport.EnabledForUser,
			IsDefault:       isDefault,
		})
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("Erro ao criar perfil '%s': %v", profileExport.Name, err))
			result.Skipped++
			continue
		}
		result.Imported++
	}

	result.Message = fmt.Sprintf("Importados %d perfis de voz, %d ignorados", result.Imported, result.Skipped)
	if len(result.Errors) > 0 {
		result.Success = false
	}

	return result, nil
}

// min retorna o menor de dois inteiros
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
