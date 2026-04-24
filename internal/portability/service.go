package portability

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"assistente/internal/credentials"
	"assistente/internal/database"

	"gorm.io/gorm"
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
	return ImportConversationsWithContext(context.Background(), jsonData, credMgr, credentialPassword)
}

func ImportConversationsWithContext(ctx context.Context, jsonData string, credMgr *credentials.Manager, credentialPassword string) (*ImportResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	var file ExportFile
	if err := json.Unmarshal([]byte(jsonData), &file); err != nil {
		return nil, fmt.Errorf("erro ao parsear JSON: %w", err)
	}
	if file.Version != 1 {
		return nil, fmt.Errorf("versão de exportação não suportada: %d", file.Version)
	}
	analysis, err := analyzeImportFile(&file, credMgr, credentialPassword)
	if err != nil {
		return nil, err
	}

	conversationConflictKeys := make(map[string]struct{}, len(analysis.ConversationConflicts))
	for _, conflict := range analysis.ConversationConflicts {
		conversationConflictKeys[conflict.Identifier] = struct{}{}
	}
	credentialConflictPatterns := make(map[string]struct{}, len(analysis.CredentialConflicts))
	for _, conflict := range analysis.CredentialConflicts {
		credentialConflictPatterns[conflict.Identifier] = struct{}{}
	}

	result := &ImportResult{
		Success: true,
		Errors:  make([]string, 0),
	}

	for _, conv := range file.Resources.Conversations {
		if isEmptyConversation(conv) {
			result.Skipped++
			result.SkippedEmptyConversations++
			continue
		}
		if _, exists := conversationConflictKeys[conversationConflictIdentifier(conv)]; exists {
			result.Skipped++
			result.SkippedConversationConflict++
			continue
		}
		imported, err := importConversation(conv, file.Options.IncludeAudio)
		if err != nil {
			result.Errors = append(result.Errors, err.Error())
			result.Failed++
			continue
		}
		if imported {
			result.Imported++
		}
	}

	if file.Options.IncludeCredentials && file.Resources.Credentials != nil {
		if credMgr == nil || !credMgr.CanPersist() {
			result.Errors = append(result.Errors, "cofre de credenciais indisponível para importação")
			result.Failed++
			result.Success = false
		} else {
			skipped, err := importCredentials(ctx, credMgr, file.Resources.Credentials, credentialPassword, credentialConflictPatterns)
			result.Skipped += skipped
			result.SkippedCredentialConflict += skipped
			if err != nil {
				result.Errors = append(result.Errors, err.Error())
				result.Failed++
				result.Success = false
			}
		}
	}

	result.SkippedOther = maxInt(result.Skipped-result.SkippedEmptyConversations-result.SkippedConversationConflict-result.SkippedCredentialConflict, 0)
	if result.Failed > 0 {
		result.Message = fmt.Sprintf("Importadas %d conversas, %d itens ignorados e %d falha(s)", result.Imported, result.Skipped, result.Failed)
	} else {
		result.Message = fmt.Sprintf("Importadas %d conversas, %d itens ignorados", result.Imported, result.Skipped)
	}
	if len(result.Errors) > 0 {
		result.Success = false
	}
	return result, nil
}

func AnalyzeImportData(jsonData string, credMgr *credentials.Manager, credentialPassword string) (*ImportAnalysis, error) {
	var file ExportFile
	if err := json.Unmarshal([]byte(jsonData), &file); err != nil {
		return nil, fmt.Errorf("erro ao parsear JSON: %w", err)
	}
	if file.Version != 1 {
		return nil, fmt.Errorf("versão de exportação não suportada: %d", file.Version)
	}
	return analyzeImportFile(&file, credMgr, credentialPassword)
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
	err := database.DB().Transaction(func(tx *gorm.DB) error {
		newConv := &database.Conversation{
			Title:     conv.Title,
			Channel:   conv.Channel,
			ContactID: conv.ContactID,
			Summary:   conv.Summary,
		}
		if !conv.CreatedAt.IsZero() {
			newConv.CreatedAt = conv.CreatedAt
			newConv.UpdatedAt = conv.CreatedAt
		}
		if err := tx.Create(newConv).Error; err != nil {
			return fmt.Errorf("erro ao criar conversa '%s': %w", conv.Title, err)
		}

		idMap := make(map[int]uint, len(conv.Messages))
		for i, msg := range conv.Messages {
			parentID, err := resolveImportedMessageReference(msg.ParentIndex, idMap, "pai")
			if err != nil {
				return fmt.Errorf("erro ao importar mensagem %d da conversa '%s': %w", i, conv.Title, err)
			}

			turnID, err := resolveImportedMessageReference(msg.TurnIndex, idMap, "turno")
			if err != nil {
				return fmt.Errorf("erro ao importar mensagem %d da conversa '%s': %w", i, conv.Title, err)
			}

			audio := ""
			if includeAudio {
				audio = msg.Audio
			}

			newMsg := &database.ChatMessage{
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
			}
			if !msg.CreatedAt.IsZero() {
				newMsg.CreatedAt = msg.CreatedAt
			}

			if err := tx.Create(newMsg).Error; err != nil {
				return fmt.Errorf("erro ao importar mensagem %d da conversa '%s': %w", i, conv.Title, err)
			}
			idMap[i] = newMsg.ID
		}

		return nil
	})
	if err != nil {
		return false, err
	}

	return true, nil
}

func resolveImportedMessageReference(index *int, idMap map[int]uint, label string) (*uint, error) {
	if index == nil {
		return nil, nil
	}

	mapped, ok := idMap[*index]
	if !ok {
		return nil, fmt.Errorf("referência de %s inválida: índice %d", label, *index)
	}
	return &mapped, nil
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

func importCredentials(ctx context.Context, credMgr *credentials.Manager, blob *CredentialCipher, credentialPassword string, skipPatterns map[string]struct{}) (int, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	creds, err := decodeCredentialExports(blob, credentialPassword)
	if err != nil {
		return 0, err
	}

	skipped := 0

	for _, cred := range creds {
		if _, shouldSkip := skipPatterns[cred.Pattern]; shouldSkip {
			skipped++
			continue
		}
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
		if err := credMgr.RegisterPatternWithContext(ctx, cred.Pattern, auth); err != nil {
			return skipped, fmt.Errorf("erro ao importar credencial '%s': %w", cred.Pattern, err)
		}
	}

	return skipped, nil
}

func intPtr(v int) *int {
	return &v
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func analyzeImportFile(file *ExportFile, credMgr *credentials.Manager, credentialPassword string) (*ImportAnalysis, error) {
	analysis := &ImportAnalysis{
		Version:               file.Version,
		AppVersion:            file.AppVersion,
		ConversationCount:     len(file.Resources.Conversations),
		IncludesCredentials:   file.Options.IncludeCredentials && file.Resources.Credentials != nil,
		ConversationConflicts: make([]ImportConflict, 0),
		CredentialConflicts:   make([]ImportConflict, 0),
		Warnings:              make([]string, 0),
	}

	for _, conv := range file.Resources.Conversations {
		analysis.MessageCount += len(conv.Messages)
	}

	existingConversations, err := database.GetConversations()
	if err != nil {
		return nil, fmt.Errorf("erro ao analisar conversas existentes: %w", err)
	}
	existingConversationKeys := make(map[string]struct{}, len(existingConversations))
	for _, conv := range existingConversations {
		existingConversationKeys[conversationConflictKey(conv.Title, conv.Channel, conv.CreatedAt)] = struct{}{}
	}

	for _, conv := range file.Resources.Conversations {
		if isEmptyConversation(conv) {
			continue
		}
		if _, exists := existingConversationKeys[conversationConflictKey(conv.Title, conv.Channel, conv.CreatedAt)]; !exists {
			continue
		}
		analysis.ConversationConflicts = append(analysis.ConversationConflicts, ImportConflict{
			ResourceType: "conversation",
			Identifier:   conversationConflictIdentifier(conv),
			Reason:       "Já existe uma conversa com o mesmo título, canal e data de criação.",
		})
	}

	if analysis.IncludesCredentials {
		analysis.RequiresCredentialPassword = true
		if credMgr == nil {
			analysis.Warnings = append(analysis.Warnings, "O cofre de credenciais atual não está disponível para analisar conflitos de credenciais.")
		} else {
			if strings.TrimSpace(credentialPassword) == "" {
				analysis.Warnings = append(analysis.Warnings, "Informe a senha de exportação para analisar conflitos de credenciais.")
			} else {
				creds, err := decodeCredentialExports(file.Resources.Credentials, credentialPassword)
				if err != nil {
					analysis.CredentialAnalysisError = err.Error()
					analysis.Warnings = append(analysis.Warnings, "Não foi possível analisar as credenciais com a senha informada.")
				} else {
					analysis.CredentialCount = len(creds)
					existingPatterns := make(map[string]struct{}, len(credMgr.ListPatterns()))
					for _, pattern := range credMgr.ListPatterns() {
						existingPatterns[pattern] = struct{}{}
					}
					for _, cred := range creds {
						if _, exists := existingPatterns[cred.Pattern]; !exists {
							continue
						}
						analysis.CredentialConflicts = append(analysis.CredentialConflicts, ImportConflict{
							ResourceType: "credential",
							Identifier:   cred.Pattern,
							Reason:       "Já existe uma credencial registrada com o mesmo pattern.",
						})
					}
				}
			}
		}
	}

	analysis.ConflictCount = len(analysis.ConversationConflicts) + len(analysis.CredentialConflicts)
	if emptyCount := countEmptyConversations(file.Resources.Conversations); emptyCount > 0 {
		analysis.Warnings = append(analysis.Warnings, fmt.Sprintf("%d conversa(s) vazia(s) serão descartadas na importação.", emptyCount))
	}
	return analysis, nil
}

func decodeCredentialExports(blob *CredentialCipher, credentialPassword string) ([]CredentialExport, error) {
	if blob == nil {
		return nil, fmt.Errorf("bloco de credenciais ausente")
	}
	var creds []CredentialExport
	if err := DecryptCredentialsPayload(credentialPassword, blob, &creds); err != nil {
		return nil, fmt.Errorf("erro ao descriptografar credenciais do arquivo: %w", err)
	}
	return creds, nil
}

func conversationConflictKey(title, channel string, createdAt time.Time) string {
	return strings.TrimSpace(title) + "|" + strings.TrimSpace(channel) + "|" + createdAt.UTC().Format(time.RFC3339Nano)
}

func conversationConflictIdentifier(conv ConversationExport) string {
	return conversationConflictKey(conv.Title, conv.Channel, conv.CreatedAt)
}

func isEmptyConversation(conv ConversationExport) bool {
	return len(conv.Messages) == 0
}

func countEmptyConversations(conversations []ConversationExport) int {
	count := 0
	for _, conv := range conversations {
		if isEmptyConversation(conv) {
			count++
		}
	}
	return count
}
