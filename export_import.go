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
	Type       string    `json:"type"` // "conversations", "voice_profiles", "interaction_profiles"
	Count      int       `json:"count"`
}

// ConversationExport representa uma conversa exportada com todas as mensagens
type ConversationExport struct {
	ID          uint                      `json:"id"`
	Title       string                    `json:"title"`
	Preferences *database.ChatPreferences `json:"preferences,omitempty"`
	CreatedAt   time.Time                 `json:"created_at"`
	UpdatedAt   time.Time                 `json:"updated_at"`
	Messages    []database.ChatMessage    `json:"messages"`
}

// ConversationsExportFile representa o arquivo de exportação de conversas
type ConversationsExportFile struct {
	Metadata      ExportMetadata       `json:"metadata"`
	Conversations []ConversationExport `json:"conversations"`
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

// InteractionTriggerExport representa um trigger de interação exportado
type InteractionTriggerExport struct {
	Type                 string  `json:"type"`
	Enabled              bool    `json:"enabled"`
	AutoStop             bool    `json:"auto_stop"`
	Hotkey               string  `json:"hotkey,omitempty"`
	HotkeyGlobal         bool    `json:"hotkey_global"`
	HotkeyBringToFront   bool    `json:"hotkey_bring_to_front"`
	WakewordKeyword      string  `json:"wakeword_keyword,omitempty"`
	WakewordProvider     string  `json:"wakeword_provider,omitempty"`
	WakewordSensitivity  float64 `json:"wakeword_sensitivity"`
	VADSilenceThreshold  float64 `json:"vad_silence_threshold"`
	VADSilenceDuration   int     `json:"vad_silence_duration"`
	VADActivityThreshold float64 `json:"vad_activity_threshold"`
	VADActivityDuration  int     `json:"vad_activity_duration"`
}

// InteractionProfileExport representa um perfil de interação exportado
type InteractionProfileExport struct {
	ID             uint                       `json:"id"`
	Name           string                     `json:"name"`
	Description    string                     `json:"description,omitempty"`
	IsDefault      bool                       `json:"is_default"`
	STTProvider    string                     `json:"stt_provider"`
	Language       string                     `json:"language"`
	FeedbackSounds bool                       `json:"feedback_sounds"`
	Triggers       []InteractionTriggerExport `json:"triggers,omitempty"`
	CreatedAt      time.Time                  `json:"created_at"`
	UpdatedAt      time.Time                  `json:"updated_at"`
}

// InteractionProfilesExportFile representa o arquivo de exportação de perfis de interação
type InteractionProfilesExportFile struct {
	Metadata            ExportMetadata             `json:"metadata"`
	InteractionProfiles []InteractionProfileExport `json:"interaction_profiles"`
}

// ImportResult representa o resultado de uma importação
type ImportResult struct {
	Success  bool     `json:"success"`
	Imported int      `json:"imported"`
	Skipped  int      `json:"skipped"`
	Errors   []string `json:"errors,omitempty"`
	Message  string   `json:"message"`
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

// ExportInteractionProfiles exporta perfis de interação selecionados
func (a *App) ExportInteractionProfiles(ids []uint) (string, error) {
	profiles := make([]InteractionProfileExport, 0, len(ids))

	for _, id := range ids {
		profile, err := database.GetInteractionProfile(id)
		if err != nil {
			return "", fmt.Errorf("erro ao buscar perfil de interação %d: %w", id, err)
		}

		// Exporta triggers
		triggers := make([]InteractionTriggerExport, len(profile.Triggers))
		for i, t := range profile.Triggers {
			triggers[i] = InteractionTriggerExport{
				Type:                 t.Type,
				Enabled:              t.Enabled,
				AutoStop:             t.AutoStop,
				Hotkey:               t.Hotkey,
				HotkeyGlobal:         t.HotkeyGlobal,
				HotkeyBringToFront:   t.HotkeyBringToFront,
				WakewordKeyword:      t.WakewordKeyword,
				WakewordProvider:     t.WakewordProvider,
				WakewordSensitivity:  t.WakewordSensitivity,
				VADSilenceThreshold:  t.VADSilenceThreshold,
				VADSilenceDuration:   t.VADSilenceDuration,
				VADActivityThreshold: t.VADActivityThreshold,
				VADActivityDuration:  t.VADActivityDuration,
			}
		}

		profiles = append(profiles, InteractionProfileExport{
			ID:             profile.ID,
			Name:           profile.Name,
			Description:    profile.Description,
			IsDefault:      profile.IsDefault,
			STTProvider:    profile.STTProvider,
			Language:       profile.Language,
			FeedbackSounds: profile.FeedbackSounds,
			Triggers:       triggers,
			CreatedAt:      profile.CreatedAt,
			UpdatedAt:      profile.UpdatedAt,
		})
	}

	exportFile := InteractionProfilesExportFile{
		Metadata: ExportMetadata{
			Version:    "1.0",
			ExportedAt: time.Now(),
			Type:       "interaction_profiles",
			Count:      len(profiles),
		},
		InteractionProfiles: profiles,
	}

	jsonData, err := json.MarshalIndent(exportFile, "", "  ")
	if err != nil {
		return "", fmt.Errorf("erro ao serializar JSON: %w", err)
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

// ImportInteractionProfiles importa perfis de interação de um JSON
func (a *App) ImportInteractionProfiles(jsonData string) (*ImportResult, error) {
	var exportFile InteractionProfilesExportFile
	if err := json.Unmarshal([]byte(jsonData), &exportFile); err != nil {
		return nil, fmt.Errorf("erro ao parsear JSON: %w", err)
	}

	result := &ImportResult{
		Success: true,
		Errors:  make([]string, 0),
	}

	for _, profileExport := range exportFile.InteractionProfiles {
		// Verifica se já existe um perfil com esse nome
		existing, _ := database.GetInteractionProfileByName(profileExport.Name)
		name := profileExport.Name
		if existing != nil {
			// Gera nome único
			name = fmt.Sprintf("%s_imported_%d", profileExport.Name, time.Now().Unix())
		}

		// Não importa como default se já existe um default
		isDefault := profileExport.IsDefault
		if isDefault {
			existingDefault, _ := database.GetDefaultInteractionProfile()
			if existingDefault != nil {
				isDefault = false // Não sobrescreve o default existente
			}
		}

		// Cria o perfil
		newProfile := &database.InteractionProfile{
			Name:           name,
			Description:    profileExport.Description,
			IsDefault:      isDefault,
			STTProvider:    profileExport.STTProvider,
			Language:       profileExport.Language,
			FeedbackSounds: profileExport.FeedbackSounds,
		}
		profile, err := database.CreateInteractionProfile(newProfile)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("Erro ao criar perfil '%s': %v", profileExport.Name, err))
			result.Skipped++
			continue
		}

		// Cria os triggers
		for _, triggerExport := range profileExport.Triggers {
			trigger := &database.InteractionTrigger{
				ProfileID:            profile.ID,
				Type:                 triggerExport.Type,
				Enabled:              triggerExport.Enabled,
				AutoStop:             triggerExport.AutoStop,
				Hotkey:               triggerExport.Hotkey,
				HotkeyGlobal:         triggerExport.HotkeyGlobal,
				HotkeyBringToFront:   triggerExport.HotkeyBringToFront,
				WakewordKeyword:      triggerExport.WakewordKeyword,
				WakewordProvider:     triggerExport.WakewordProvider,
				WakewordSensitivity:  triggerExport.WakewordSensitivity,
				VADSilenceThreshold:  triggerExport.VADSilenceThreshold,
				VADSilenceDuration:   triggerExport.VADSilenceDuration,
				VADActivityThreshold: triggerExport.VADActivityThreshold,
				VADActivityDuration:  triggerExport.VADActivityDuration,
			}
			if _, err := database.CreateInteractionTrigger(trigger); err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("Erro ao criar trigger para '%s': %v", name, err))
			}
		}

		result.Imported++
	}

	result.Message = fmt.Sprintf("Importados %d perfis de interação, %d ignorados", result.Imported, result.Skipped)
	if len(result.Errors) > 0 {
		result.Success = false
	}

	return result, nil
}
