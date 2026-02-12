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
	Type       string    `json:"type"` // "conversations"
	Count      int       `json:"count"`
}

// ConversationExport representa uma conversa exportada com todas as mensagens
type ConversationExport struct {
	ID        uint                   `json:"id"`
	Title     string                 `json:"title"`
	CreatedAt time.Time              `json:"created_at"`
	UpdatedAt time.Time              `json:"updated_at"`
	Messages  []database.ChatMessage `json:"messages"`
}

// ConversationsExportFile representa o arquivo de exportação de conversas
type ConversationsExportFile struct {
	Metadata      ExportMetadata       `json:"metadata"`
	Conversations []ConversationExport `json:"conversations"`
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
			ID:        conv.ID,
			Title:     conv.Title,
			CreatedAt: conv.CreatedAt,
			UpdatedAt: conv.UpdatedAt,
			Messages:  conv.Messages,
		}
		conversations = append(conversations, export)
	}

	exportFile := ConversationsExportFile{
		Metadata: ExportMetadata{
			Version:    "2.0",
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
