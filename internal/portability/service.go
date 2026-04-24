package portability

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"assistente/internal/credentials"
	"assistente/internal/database"
)

func ExportConversations(ids []uint, credMgr *credentials.Manager, req ExportRequest, appVersion string) (string, error) {
	file, err := BuildConversationExportFile(ids, credMgr, req, appVersion)
	if err != nil {
		return "", err
	}

	raw, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return "", fmt.Errorf("erro ao serializar exportação: %w", err)
	}
	return string(raw), nil
}

func BuildConversationExportFile(ids []uint, credMgr *credentials.Manager, req ExportRequest, appVersion string) (*ExportFile, error) {
	conversations := make([]ConversationExport, 0, len(ids))
	for _, id := range ids {
		conv, err := database.GetConversation(id)
		if err != nil {
			return nil, fmt.Errorf("erro ao buscar conversa %d: %w", id, err)
		}
		conversations = append(conversations, exportConversation(conv, req.IncludeAudio))
	}

	file := &ExportFile{
		Version:    1,
		ExportedAt: time.Now().UTC(),
		AppVersion: appVersion,
		Options: ExportOptions{
			IncludeAudio:       req.IncludeAudio,
			IncludeCredentials: req.IncludeCredentials,
		},
		Resources: ExportResources{
			Conversations: conversations,
		},
	}

	if req.IncludeCredentials {
		if credMgr == nil || !credMgr.CanPersist() {
			return nil, fmt.Errorf("cofre de credenciais indisponível para exportação")
		}
		creds, err := exportCredentials(credMgr)
		if err != nil {
			return nil, err
		}
		blob, err := EncryptCredentialsPayload(req.CredentialExportPassword, creds)
		if err != nil {
			return nil, err
		}
		file.Resources.Credentials = blob
	}

	return file, nil
}

func ImportConversations(jsonData string, credMgr *credentials.Manager, credentialPassword string) (*ImportResult, error) {
	var file ExportFile
	if err := json.Unmarshal([]byte(jsonData), &file); err != nil {
		return nil, fmt.Errorf("erro ao parsear JSON: %w", err)
	}
	if file.Version != 1 {
		return nil, fmt.Errorf("versão de exportação não suportada: %d", file.Version)
	}

	result := &ImportResult{
		Success: true,
		Errors:  make([]string, 0),
	}

	for _, conv := range file.Resources.Conversations {
		imported, err := importConversation(conv, file.Options.IncludeAudio)
		if err != nil {
			result.Errors = append(result.Errors, err.Error())
			result.Skipped++
			continue
		}
		if imported {
			result.Imported++
		}
	}

	if file.Options.IncludeCredentials && file.Resources.Credentials != nil {
		if credMgr == nil || !credMgr.CanPersist() {
			result.Errors = append(result.Errors, "cofre de credenciais indisponível para importação")
			result.Success = false
		} else {
			if err := importCredentials(credMgr, file.Resources.Credentials, credentialPassword); err != nil {
				result.Errors = append(result.Errors, err.Error())
				result.Success = false
			}
		}
	}

	result.Message = fmt.Sprintf("Importadas %d conversas, %d ignoradas", result.Imported, result.Skipped)
	if len(result.Errors) > 0 {
		result.Success = false
	}
	return result, nil
}

func exportConversation(conv *database.Conversation, includeAudio bool) ConversationExport {
	indexByMessageID := make(map[uint]int, len(conv.Messages))
	for i, msg := range conv.Messages {
		indexByMessageID[msg.ID] = i
	}

	messages := make([]MessageExport, 0, len(conv.Messages))
	for _, msg := range conv.Messages {
		exported := MessageExport{
			Role:             msg.Role,
			Content:          msg.Content,
			Reasoning:        msg.Reasoning,
			Media:            msg.Media,
			AudioMimeType:    msg.AudioMimeType,
			ToolCalls:        msg.ToolCalls,
			ToolCallID:       msg.ToolCallID,
			PromptTokens:     msg.PromptTokens,
			CompletionTokens: msg.CompletionTokens,
			TotalTokens:      msg.TotalTokens,
			Model:            msg.Model,
			Source:           msg.Source,
			CreatedAt:        msg.CreatedAt,
		}
		if includeAudio {
			exported.Audio = msg.Audio
		}
		if msg.ParentID != nil {
			if idx, ok := indexByMessageID[*msg.ParentID]; ok {
				exported.ParentIndex = intPtr(idx)
			}
		}
		if msg.TurnID != nil {
			if idx, ok := indexByMessageID[*msg.TurnID]; ok {
				exported.TurnIndex = intPtr(idx)
			}
		}
		messages = append(messages, exported)
	}

	return ConversationExport{
		Title:     conv.Title,
		Channel:   conv.Channel,
		ContactID: conv.ContactID,
		Summary:   conv.Summary,
		CreatedAt: conv.CreatedAt,
		Messages:  messages,
	}
}

func importConversation(conv ConversationExport, includeAudio bool) (bool, error) {
	newConv, err := database.CreateConversation(conv.Title, "")
	if err != nil {
		return false, fmt.Errorf("erro ao criar conversa '%s': %w", conv.Title, err)
	}
	if conv.Channel != "" || conv.ContactID != "" {
		if err := database.UpdateConversationChannel(newConv.ID, conv.Channel, conv.ContactID); err != nil {
			return false, fmt.Errorf("erro ao vincular canal da conversa '%s': %w", conv.Title, err)
		}
	}
	if conv.Summary != "" {
		if err := database.DB().Model(&database.Conversation{}).Where("id = ?", newConv.ID).Update("summary", conv.Summary).Error; err != nil {
			return false, fmt.Errorf("erro ao salvar resumo da conversa '%s': %w", conv.Title, err)
		}
	}

	idMap := make(map[int]uint, len(conv.Messages))
	for i, msg := range conv.Messages {
		var parentID *uint
		if msg.ParentIndex != nil {
			if mapped, ok := idMap[*msg.ParentIndex]; ok {
				parentID = &mapped
			}
		}

		var turnID *uint
		if msg.TurnIndex != nil {
			if mapped, ok := idMap[*msg.TurnIndex]; ok {
				turnID = &mapped
			}
		}

		audio := ""
		if includeAudio {
			audio = msg.Audio
		}

		newMsg, err := database.CreateMessage(database.MessageOptions{
			ConversationID:   newConv.ID,
			ParentID:         parentID,
			TurnID:           turnID,
			Role:             msg.Role,
			Content:          msg.Content,
			Reasoning:        msg.Reasoning,
			Media:            msg.Media,
			Audio:            audio,
			AudioMimeType:    msg.AudioMimeType,
			ToolCalls:        msg.ToolCalls,
			ToolCallID:       msg.ToolCallID,
			PromptTokens:     msg.PromptTokens,
			CompletionTokens: msg.CompletionTokens,
			TotalTokens:      msg.TotalTokens,
			Model:            msg.Model,
			Source:           msg.Source,
		})
		if err != nil {
			return false, fmt.Errorf("erro ao importar mensagem %d da conversa '%s': %w", i, conv.Title, err)
		}
		idMap[i] = newMsg.ID
	}

	return true, nil
}

func exportCredentials(credMgr *credentials.Manager) ([]CredentialExport, error) {
	list, err := credMgr.ListCredentials()
	if err != nil {
		return nil, err
	}

	result := make([]CredentialExport, 0, len(list))
	for _, entry := range list {
		if entry.Auth == nil {
			continue
		}
		result = append(result, CredentialExport{
			Pattern:      entry.Pattern,
			AuthType:     entry.Auth.Type,
			Token:        entry.Auth.Token,
			Username:     entry.Auth.Username,
			Password:     entry.Auth.Password,
			Headers:      entry.Auth.Headers,
			ExpiresAt:    entry.Auth.ExpiresAt,
			RefreshURL:   entry.Auth.RefreshURL,
			ClientID:     entry.Auth.ClientID,
			ClientSecret: entry.Auth.ClientSecret,
		})
	}
	return result, nil
}

func importCredentials(credMgr *credentials.Manager, raw any, credentialPassword string) error {
	data, err := json.Marshal(raw)
	if err != nil {
		return err
	}

	var blob CredentialCipher
	if err := json.Unmarshal(data, &blob); err != nil {
		return err
	}

	var creds []CredentialExport
	if err := DecryptCredentialsPayload(credentialPassword, &blob, &creds); err != nil {
		return fmt.Errorf("erro ao descriptografar credenciais do arquivo: %w", err)
	}

	for _, cred := range creds {
		auth := &credentials.AuthConfig{
			Type:         cred.AuthType,
			Token:        cred.Token,
			Username:     cred.Username,
			Password:     cred.Password,
			Headers:      cred.Headers,
			ExpiresAt:    cred.ExpiresAt,
			RefreshURL:   cred.RefreshURL,
			ClientID:     cred.ClientID,
			ClientSecret: cred.ClientSecret,
		}
		if err := credMgr.RegisterPatternWithContext(context.Background(), cred.Pattern, auth); err != nil {
			return fmt.Errorf("erro ao importar credencial '%s': %w", cred.Pattern, err)
		}
	}

	return nil
}

func intPtr(v int) *int {
	return &v
}
