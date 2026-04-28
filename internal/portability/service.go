package portability

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"assistente/internal/credentials"
	"assistente/internal/database"

	"gorm.io/gorm"
)

var supportedPortableResourceTypes = map[string]struct{}{
	"conversations": {},
	"providers":     {},
	"taskLists":     {},
	"credentials":   {},
}

func ExportConversations(ids []string, credMgr *credentials.Manager, req ExportRequest, appVersion string) (string, error) {
	return ExportPortableData(ids, nil, nil, credMgr, req, appVersion)
}

func ExportPortableData(conversationIDs []string, providerIDs []string, taskListIDs []string, credMgr *credentials.Manager, req ExportRequest, appVersion string) (string, error) {
	file, err := BuildExportFile(conversationIDs, providerIDs, taskListIDs, credMgr, req, appVersion)
	if err != nil {
		return "", err
	}

	raw, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return "", fmt.Errorf("erro ao serializar exportação: %w", err)
	}
	return string(raw), nil
}

func BuildConversationExportFile(ids []string, credMgr *credentials.Manager, req ExportRequest, appVersion string) (*ExportFile, error) {
	return BuildExportFile(ids, nil, nil, credMgr, req, appVersion)
}

func BuildExportFile(conversationIDs []string, providerIDs []string, taskListIDs []string, credMgr *credentials.Manager, req ExportRequest, appVersion string) (*ExportFile, error) {
	conversations := make([]ConversationExport, 0, len(conversationIDs))
	for _, id := range conversationIDs {
		conv, err := database.GetConversation(id)
		if err != nil {
			return nil, fmt.Errorf("erro ao buscar conversa %s: %w", id, err)
		}
		conversations = append(conversations, exportConversation(conv, req.IncludeAudio))
	}

	providers := make([]ProviderExport, 0, len(providerIDs))
	for _, id := range providerIDs {
		provider, err := database.GetLLMProvider(id)
		if err != nil {
			return nil, fmt.Errorf("erro ao buscar provider %s: %w", id, err)
		}
		providers = append(providers, exportProvider(provider))
	}

	taskLists := make([]TaskListExport, 0, len(taskListIDs))
	for _, id := range taskListIDs {
		taskList, err := exportTaskList(id)
		if err != nil {
			return nil, fmt.Errorf("erro ao buscar tasklist %s: %w", id, err)
		}
		taskLists = append(taskLists, taskList)
	}

	file := &ExportFile{
		Version:    ExportVersion,
		ExportedAt: time.Now().UTC(),
		AppVersion: appVersion,
		Options: ExportOptions{
			IncludeAudio:       req.IncludeAudio,
			IncludeCredentials: req.IncludeCredentials,
		},
		Resources: ExportResources{
			Conversations: conversations,
			Providers:     providers,
			TaskLists:     taskLists,
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
	return ImportConversationsWithResolutions(context.Background(), jsonData, credMgr, credentialPassword, nil)
}

func ImportConversationsWithContext(ctx context.Context, jsonData string, credMgr *credentials.Manager, credentialPassword string) (*ImportResult, error) {
	return ImportConversationsWithResolutions(ctx, jsonData, credMgr, credentialPassword, nil)
}

func ImportConversationsWithResolutions(
	ctx context.Context,
	jsonData string,
	credMgr *credentials.Manager,
	credentialPassword string,
	resolutions []ImportResolution,
) (*ImportResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	file, unsupportedResourceTypes, err := parseExportFile(jsonData)
	if err != nil {
		return nil, err
	}
	if file.Version != ExportVersion {
		return nil, fmt.Errorf("versão de exportação não suportada: %d", file.Version)
	}
	analysis, err := analyzeImportFile(file, credMgr, credentialPassword)
	if err != nil {
		return nil, err
	}
	resolutionMap := buildImportResolutionMap(resolutions)

	conversationConflictKeys := make(map[string]struct{}, len(analysis.ConversationConflicts))
	for _, conflict := range analysis.ConversationConflicts {
		conversationConflictKeys[conflict.Identifier] = struct{}{}
	}
	credentialConflictIdentifiers := make(map[string]struct{}, len(analysis.CredentialConflicts))
	for _, conflict := range analysis.CredentialConflicts {
		credentialConflictIdentifiers[conflict.Identifier] = struct{}{}
	}
	providerConflictKeys := make(map[string]struct{}, len(analysis.ProviderConflicts))
	for _, conflict := range analysis.ProviderConflicts {
		providerConflictKeys[conflict.Identifier] = struct{}{}
	}
	taskListConflictKeys := make(map[string]struct{}, len(analysis.TaskListConflicts))
	for _, conflict := range analysis.TaskListConflicts {
		taskListConflictKeys[conflict.Identifier] = struct{}{}
	}

	result := &ImportResult{
		Success:                  true,
		Errors:                   make([]string, 0),
		Warnings:                 make([]string, 0),
		UnsupportedResourceTypes: unsupportedResourceTypes,
	}
	if warning := unsupportedResourcesWarning(unsupportedResourceTypes); warning != "" {
		result.Warnings = append(result.Warnings, warning)
	}

	for _, conv := range file.Resources.Conversations {
		if isEmptyConversation(conv) {
			result.Skipped++
			result.SkippedEmptyConversations++
			continue
		}
		if _, exists := conversationConflictKeys[conversationConflictIdentifier(conv)]; exists {
			resolution, hasResolution := resolutionMap.lookup("conversation", conversationConflictIdentifier(conv))
			if !hasResolution || resolution.Strategy == ConflictResolutionSkip {
				result.Skipped++
				result.SkippedConversationConflict++
				continue
			}
			resolvedConv, err := applyConversationResolution(conv, resolution)
			if err != nil {
				result.Errors = append(result.Errors, err.Error())
				result.Failed++
				continue
			}
			if resolution.Strategy == ConflictResolutionOverwrite {
				imported, err := overwriteConversation(resolvedConv, file.Options.IncludeAudio)
				if err != nil {
					result.Errors = append(result.Errors, err.Error())
					result.Failed++
					continue
				}
				if imported {
					result.Imported++
				}
				continue
			}
			conv = resolvedConv
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

	for _, provider := range file.Resources.Providers {
		if _, exists := providerConflictKeys[providerConflictIdentifier(provider)]; exists {
			resolution, hasResolution := resolutionMap.lookup("provider", providerConflictIdentifier(provider))
			if !hasResolution || resolution.Strategy == ConflictResolutionSkip {
				result.Skipped++
				result.SkippedProviderConflict++
				continue
			}
			resolvedProvider, err := applyProviderResolution(provider, resolution)
			if err != nil {
				result.Errors = append(result.Errors, err.Error())
				result.Failed++
				continue
			}
			if resolution.Strategy == ConflictResolutionOverwrite {
				imported, err := overwriteProvider(resolvedProvider)
				if err != nil {
					result.Errors = append(result.Errors, err.Error())
					result.Failed++
					continue
				}
				if imported {
					result.Imported++
				}
				continue
			}
			provider = resolvedProvider
		}
		imported, err := importProvider(provider)
		if err != nil {
			result.Errors = append(result.Errors, err.Error())
			result.Failed++
			continue
		}
		if imported {
			result.Imported++
		}
	}

	for _, taskList := range file.Resources.TaskLists {
		if _, exists := taskListConflictKeys[taskListConflictIdentifier(taskList)]; exists {
			resolution, hasResolution := resolutionMap.lookup("taskList", taskListConflictIdentifier(taskList))
			if !hasResolution || resolution.Strategy == ConflictResolutionSkip {
				result.Skipped++
				result.SkippedTaskListConflict++
				continue
			}
			resolvedTaskList, err := applyTaskListResolution(taskList, resolution)
			if err != nil {
				result.Errors = append(result.Errors, err.Error())
				result.Failed++
				continue
			}
			if resolution.Strategy == ConflictResolutionOverwrite {
				imported, err := overwriteTaskList(resolvedTaskList)
				if err != nil {
					result.Errors = append(result.Errors, err.Error())
					result.Failed++
					continue
				}
				if imported {
					result.Imported++
				}
				continue
			}
			taskList = resolvedTaskList
		}
		imported, err := importTaskList(taskList)
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
			imported, skipped, err := importCredentials(ctx, credMgr, file.Resources.Credentials, credentialPassword, credentialConflictIdentifiers, resolutionMap)
			result.Imported += imported
			result.Skipped += skipped
			result.SkippedCredentialConflict += skipped
			if err != nil {
				result.Errors = append(result.Errors, err.Error())
				result.Failed++
				result.Success = false
			}
		}
	}

	result.SkippedOther = maxInt(
		result.Skipped-
			result.SkippedEmptyConversations-
			result.SkippedConversationConflict-
			result.SkippedProviderConflict-
			result.SkippedTaskListConflict-
			result.SkippedCredentialConflict,
		0,
	)
	if result.Failed > 0 {
		result.Message = fmt.Sprintf("Importados %d recursos, %d itens ignorados e %d falha(s)", result.Imported, result.Skipped, result.Failed)
	} else {
		result.Message = fmt.Sprintf("Importados %d recursos, %d itens ignorados", result.Imported, result.Skipped)
	}
	if len(result.Errors) > 0 {
		result.Success = false
	}
	return result, nil
}

func AnalyzeImportData(jsonData string, credMgr *credentials.Manager, credentialPassword string) (*ImportAnalysis, error) {
	file, unsupportedResourceTypes, err := parseExportFile(jsonData)
	if err != nil {
		return nil, err
	}
	if file.Version != ExportVersion {
		return nil, fmt.Errorf("versão de exportação não suportada: %d", file.Version)
	}
	analysis, err := analyzeImportFile(file, credMgr, credentialPassword)
	if err != nil {
		return nil, err
	}
	analysis.UnsupportedResourceTypes = unsupportedResourceTypes
	return analysis, nil
}

func exportConversation(conv *database.Conversation, includeAudio bool) ConversationExport {
	indexByMessageID := make(map[string]int, len(conv.Messages))
	for i, msg := range conv.Messages {
		indexByMessageID[msg.ID] = i
	}

	messages := make([]MessageExport, 0, len(conv.Messages))
	for _, msg := range conv.Messages {
		exported := MessageExport{
			ID:               msg.ID,
			ConversationID:   msg.ConversationID,
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
		if msg.ParentID != nil {
			exported.ParentID = *msg.ParentID
		}
		if msg.TurnID != nil {
			exported.TurnID = *msg.TurnID
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
		ID:        conv.ID,
		Title:     conv.Title,
		Channel:   conv.Channel,
		ContactID: conv.ContactID,
		Summary:   conv.Summary,
		CreatedAt: conv.CreatedAt,
		Messages:  messages,
	}
}

func importConversation(conv ConversationExport, includeAudio bool) (bool, error) {
	if existing, err := findExistingConversationForImport(conv); err != nil {
		return false, err
	} else if existing != nil {
		return overwriteConversationByExisting(conv, includeAudio, existing)
	}

	err := database.DB().Transaction(func(tx *gorm.DB) error {
		newConv, err := createImportedConversation(tx, conv)
		if err != nil {
			return err
		}
		return importConversationMessages(tx, newConv.ID, conv, includeAudio)
	})
	if err != nil {
		return false, err
	}

	return true, nil
}

func overwriteConversation(conv ConversationExport, includeAudio bool) (bool, error) {
	existing, err := findExistingConversationForImport(conv)
	if err != nil {
		return false, err
	}
	if existing == nil {
		return importConversation(conv, includeAudio)
	}

	return overwriteConversationByExisting(conv, includeAudio, existing)
}

func overwriteConversationByExisting(conv ConversationExport, includeAudio bool, existing *database.Conversation) (bool, error) {
	err := database.DB().Transaction(func(tx *gorm.DB) error {
		updatedAt := conv.CreatedAt
		if updatedAt.IsZero() {
			updatedAt = time.Now()
		}
		existing.Title = conv.Title
		existing.Channel = conv.Channel
		existing.ContactID = conv.ContactID
		existing.Summary = conv.Summary
		if !conv.CreatedAt.IsZero() {
			existing.CreatedAt = conv.CreatedAt
		}
		existing.UpdatedAt = updatedAt
		if err := tx.Save(existing).Error; err != nil {
			return fmt.Errorf("erro ao atualizar conversa '%s': %w", conv.Title, err)
		}
		if err := tx.Where("conversation_id = ?", existing.ID).Delete(&database.ChatMessage{}).Error; err != nil {
			return fmt.Errorf("erro ao limpar mensagens da conversa '%s': %w", conv.Title, err)
		}
		return importConversationMessages(tx, existing.ID, conv, includeAudio)
	})
	if err != nil {
		return false, err
	}

	return true, nil
}

func createImportedConversation(tx *gorm.DB, conv ConversationExport) (*database.Conversation, error) {
	newConv := &database.Conversation{
		UUIDModel: database.UUIDModel{
			ID: conv.ID,
		},
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
		return nil, fmt.Errorf("erro ao criar conversa '%s': %w", conv.Title, err)
	}
	return newConv, nil
}

func importConversationMessages(tx *gorm.DB, conversationID string, conv ConversationExport, includeAudio bool) error {
	exportedMessageIDs := make(map[string]struct{}, len(conv.Messages))
	for _, msg := range conv.Messages {
		if id := strings.TrimSpace(msg.ID); id != "" {
			exportedMessageIDs[id] = struct{}{}
		}
	}

	idMap := make(map[int]string, len(conv.Messages))
	for i, msg := range conv.Messages {
		parentID, err := resolveImportedMessageLink(msg.ParentID, msg.ParentIndex, exportedMessageIDs, idMap, "pai")
		if err != nil {
			return fmt.Errorf("erro ao importar mensagem %d da conversa '%s': %w", i, conv.Title, err)
		}

		turnID, err := resolveImportedMessageLink(msg.TurnID, msg.TurnIndex, exportedMessageIDs, idMap, "turno")
		if err != nil {
			return fmt.Errorf("erro ao importar mensagem %d da conversa '%s': %w", i, conv.Title, err)
		}

		audio := ""
		if includeAudio {
			audio = msg.Audio
		}

		newMsg := &database.ChatMessage{
			UUIDModel: database.UUIDModel{
				ID: strings.TrimSpace(msg.ID),
			},
			ConversationID:   conversationID,
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
}

func resolveImportedMessageReference(index *int, idMap map[int]string, label string) (*string, error) {
	if index == nil {
		return nil, nil
	}

	mapped, ok := idMap[*index]
	if !ok {
		return nil, fmt.Errorf("referência de %s inválida: índice %d", label, *index)
	}
	return &mapped, nil
}

func resolveImportedMessageLink(
	stableID string,
	index *int,
	exportedIDs map[string]struct{},
	idMap map[int]string,
	label string,
) (*string, error) {
	if trimmed := strings.TrimSpace(stableID); trimmed != "" {
		if _, ok := exportedIDs[trimmed]; !ok {
			return nil, fmt.Errorf("referência de %s inválida: id %q", label, trimmed)
		}
		return &trimmed, nil
	}
	return resolveImportedMessageReference(index, idMap, label)
}

func exportCredentials(credMgr *credentials.Manager) ([]CredentialExport, error) {
	list, err := credMgr.ListCredentials()
	if err != nil {
		return nil, err
	}
	idByPattern := make(map[string]string)
	var entries []database.CredentialEntry
	if err := database.DB().Find(&entries).Error; err != nil {
		return nil, fmt.Errorf("erro ao carregar credenciais persistidas para exportação: %w", err)
	}
	for _, entry := range entries {
		pattern := strings.TrimSpace(entry.Pattern)
		if pattern == "" || strings.TrimSpace(entry.ID) == "" {
			continue
		}
		idByPattern[pattern] = strings.TrimSpace(entry.ID)
	}

	result := make([]CredentialExport, 0, len(list))
	for _, entry := range list {
		if entry.Auth == nil {
			continue
		}
		id := strings.TrimSpace(entry.ID)
		if id == "" {
			id = idByPattern[strings.TrimSpace(entry.Pattern)]
		}
		result = append(result, CredentialExport{
			ID:           id,
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

func importCredentials(
	ctx context.Context,
	credMgr *credentials.Manager,
	blob *CredentialCipher,
	credentialPassword string,
	conflictIdentifiers map[string]struct{},
	resolutionMap importResolutionMap,
) (int, int, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	creds, err := decodeCredentialExports(blob, credentialPassword)
	if err != nil {
		return 0, 0, err
	}

	skipped := 0
	imported := 0

	for _, cred := range creds {
		identifier := strings.TrimSpace(cred.ID)
		if identifier == "" {
			identifier = cred.Pattern
		}
		if _, hasConflict := conflictIdentifiers[identifier]; hasConflict {
			resolution, hasResolution := resolutionMap.lookup("credential", identifier)
			if !hasResolution || resolution.Strategy == ConflictResolutionSkip {
				skipped++
				continue
			}
			if resolution.Strategy != ConflictResolutionOverwrite {
				return imported, skipped, fmt.Errorf("estratégia de conflito não suportada para credencial %q: %s", identifier, resolution.Strategy)
			}
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
		if err := credMgr.RegisterStoredCredentialWithContext(ctx, credentials.StoredCredential{
			ID:      strings.TrimSpace(cred.ID),
			Pattern: cred.Pattern,
			Auth:    auth,
		}); err != nil {
			return imported, skipped, fmt.Errorf("erro ao importar credencial '%s': %w", cred.Pattern, err)
		}
		imported++
	}

	return imported, skipped, nil
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
		ProviderCount:         len(file.Resources.Providers),
		TaskListCount:         len(file.Resources.TaskLists),
		IncludesCredentials:   file.Options.IncludeCredentials && file.Resources.Credentials != nil,
		ConversationConflicts: make([]ImportConflict, 0),
		ProviderConflicts:     make([]ImportConflict, 0),
		TaskListConflicts:     make([]ImportConflict, 0),
		CredentialConflicts:   make([]ImportConflict, 0),
		Warnings:              make([]string, 0),
	}

	for _, conv := range file.Resources.Conversations {
		analysis.MessageCount += len(conv.Messages)
	}
	analysis.TaskCount, analysis.TaskNoteCount = countExportedTasks(file.Resources.TaskLists)

	existingConversations, err := database.GetConversations()
	if err != nil {
		return nil, fmt.Errorf("erro ao analisar conversas existentes: %w", err)
	}
	existingConversationIDs := make(map[string]struct{}, len(existingConversations))
	existingConversationKeys := make(map[string]struct{}, len(existingConversations))
	for _, conv := range existingConversations {
		existingConversationIDs[strings.TrimSpace(conv.ID)] = struct{}{}
		existingConversationKeys[conversationConflictKey(conv.Title, conv.Channel, conv.CreatedAt)] = struct{}{}
	}

	for _, conv := range file.Resources.Conversations {
		if isEmptyConversation(conv) {
			continue
		}
		if id := strings.TrimSpace(conv.ID); id != "" {
			if _, exists := existingConversationIDs[id]; exists {
				continue
			}
		}
		if _, exists := existingConversationKeys[conversationConflictKey(conv.Title, conv.Channel, conv.CreatedAt)]; !exists {
			continue
		}
		analysis.ConversationConflicts = append(analysis.ConversationConflicts, ImportConflict{
			ResourceType:        "conversation",
			Identifier:          conversationConflictIdentifier(conv),
			Reason:              "Já existe uma conversa com o mesmo título, canal e data de criação.",
			SupportedStrategies: []ConflictResolutionStrategy{ConflictResolutionSkip, ConflictResolutionOverwrite, ConflictResolutionRename},
		})
	}

	existingProviders, err := database.GetLLMProviders()
	if err != nil {
		return nil, fmt.Errorf("erro ao analisar providers existentes: %w", err)
	}
	existingProviderIDs := make(map[string]struct{}, len(existingProviders))
	for _, provider := range existingProviders {
		existingProviderIDs[strings.TrimSpace(provider.ID)] = struct{}{}
	}
	for _, provider := range file.Resources.Providers {
		if strings.TrimSpace(provider.ID) != "" {
			if _, exists := existingProviderIDs[providerConflictIdentifier(provider)]; exists {
				continue
			}
		}
		if _, exists := existingProviderIDs[providerConflictIdentifier(provider)]; !exists {
			continue
		}
		analysis.ProviderConflicts = append(analysis.ProviderConflicts, ImportConflict{
			ResourceType:        "provider",
			Identifier:          providerConflictIdentifier(provider),
			Reason:              "Já existe um provider com o mesmo id.",
			SupportedStrategies: []ConflictResolutionStrategy{ConflictResolutionSkip, ConflictResolutionOverwrite, ConflictResolutionRename},
		})
	}

	existingTaskLists, err := database.GetAllTaskLists()
	if err != nil {
		return nil, fmt.Errorf("erro ao analisar tasklists existentes: %w", err)
	}
	existingTaskListKeys := existingTaskListConflictKeys(existingTaskLists)
	existingTaskListIDs := make(map[string]struct{}, len(existingTaskLists))
	for _, taskList := range existingTaskLists {
		existingTaskListIDs[strings.TrimSpace(taskList.ID)] = struct{}{}
	}
	for _, taskList := range file.Resources.TaskLists {
		if id := strings.TrimSpace(taskList.ID); id != "" {
			if _, exists := existingTaskListIDs[id]; exists {
				continue
			}
		}
		if _, exists := existingTaskListKeys[taskListConflictLookupKey(taskList)]; !exists {
			continue
		}
		analysis.TaskListConflicts = append(analysis.TaskListConflicts, ImportConflict{
			ResourceType:        "taskList",
			Identifier:          taskListConflictIdentifier(taskList),
			Reason:              "Já existe uma tasklist com o mesmo slug, ou com o mesmo título quando o slug está ausente.",
			SupportedStrategies: []ConflictResolutionStrategy{ConflictResolutionSkip, ConflictResolutionOverwrite, ConflictResolutionRename},
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
					existingCredentialIDs, existingCredentialPatterns, err := loadExistingCredentialIdentifiers()
					if err != nil {
						return nil, fmt.Errorf("erro ao analisar credenciais existentes: %w", err)
					}
					for _, cred := range creds {
						if id := strings.TrimSpace(cred.ID); id != "" {
							if _, exists := existingCredentialIDs[id]; exists {
								continue
							}
						}
						if _, exists := existingCredentialPatterns[cred.Pattern]; !exists {
							continue
						}
						analysis.CredentialConflicts = append(analysis.CredentialConflicts, ImportConflict{
							ResourceType:        "credential",
							Identifier:          cred.Pattern,
							Reason:              "Já existe uma credencial registrada com o mesmo pattern.",
							SupportedStrategies: []ConflictResolutionStrategy{ConflictResolutionSkip, ConflictResolutionOverwrite},
						})
					}
				}
			}
		}
	}

	analysis.ConflictCount = len(analysis.ConversationConflicts) + len(analysis.ProviderConflicts) + len(analysis.TaskListConflicts) + len(analysis.CredentialConflicts)
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

func loadExistingCredentialIdentifiers() (map[string]struct{}, map[string]struct{}, error) {
	var entries []database.CredentialEntry
	if err := database.DB().Find(&entries).Error; err != nil {
		return nil, nil, err
	}

	ids := make(map[string]struct{}, len(entries))
	patterns := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		if id := strings.TrimSpace(entry.ID); id != "" {
			ids[id] = struct{}{}
		}
		if pattern := strings.TrimSpace(entry.Pattern); pattern != "" {
			patterns[pattern] = struct{}{}
		}
	}
	return ids, patterns, nil
}

func parseExportFile(jsonData string) (*ExportFile, []string, error) {
	var envelope struct {
		Resources map[string]json.RawMessage `json:"resources"`
	}
	if err := json.Unmarshal([]byte(jsonData), &envelope); err != nil {
		return nil, nil, fmt.Errorf("erro ao parsear JSON: %w", err)
	}

	var file ExportFile
	if err := json.Unmarshal([]byte(jsonData), &file); err != nil {
		return nil, nil, fmt.Errorf("erro ao parsear JSON: %w", err)
	}

	return &file, collectUnsupportedResourceTypes(envelope.Resources), nil
}

func collectUnsupportedResourceTypes(resources map[string]json.RawMessage) []string {
	if len(resources) == 0 {
		return nil
	}

	unsupported := make([]string, 0)
	for resourceType, raw := range resources {
		if _, supported := supportedPortableResourceTypes[resourceType]; supported {
			continue
		}
		if !hasPortableResourcePayload(raw) {
			continue
		}
		unsupported = append(unsupported, resourceType)
	}

	if len(unsupported) == 0 {
		return nil
	}

	sort.Strings(unsupported)
	return unsupported
}

func hasPortableResourcePayload(raw json.RawMessage) bool {
	compact := strings.TrimSpace(string(raw))
	switch compact {
	case "", "null", "[]", "{}":
		return false
	default:
		return true
	}
}

func unsupportedResourcesWarning(resourceTypes []string) string {
	if len(resourceTypes) == 0 {
		return ""
	}
	return fmt.Sprintf(
		"Este arquivo inclui recursos fora do escopo atual (%s). Eles serão ignorados nesta fase e poderão ser suportados após as migrações planejadas nas AEP-0046, AEP-0048, AEP-0050, AEP-0051 e AEP-0052.",
		strings.Join(resourceTypes, ", "),
	)
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

type importResolutionMap map[string]ImportResolution

func buildImportResolutionMap(resolutions []ImportResolution) importResolutionMap {
	result := make(importResolutionMap, len(resolutions))
	for _, resolution := range resolutions {
		resourceType := strings.TrimSpace(resolution.ResourceType)
		identifier := strings.TrimSpace(resolution.Identifier)
		if resourceType == "" || identifier == "" {
			continue
		}
		result[importResolutionMapKey(resourceType, identifier)] = resolution
	}
	return result
}

func (m importResolutionMap) lookup(resourceType, identifier string) (ImportResolution, bool) {
	resolution, ok := m[importResolutionMapKey(resourceType, identifier)]
	return resolution, ok
}

func importResolutionMapKey(resourceType, identifier string) string {
	return strings.TrimSpace(resourceType) + "|" + strings.TrimSpace(identifier)
}

func applyConversationResolution(conv ConversationExport, resolution ImportResolution) (ConversationExport, error) {
	switch resolution.Strategy {
	case ConflictResolutionOverwrite:
		return conv, nil
	case ConflictResolutionRename:
		renamedTitle := strings.TrimSpace(resolution.RenameValue)
		if renamedTitle == "" {
			return ConversationExport{}, fmt.Errorf("resolução de conflito da conversa %q requer um novo título", resolution.Identifier)
		}
		conv.Title = renamedTitle
		if existing, err := findExistingConversationByExport(conv); err != nil {
			return ConversationExport{}, err
		} else if existing != nil {
			return ConversationExport{}, fmt.Errorf("já existe uma conversa com o novo título %q para a mesma data e canal", renamedTitle)
		}
		return conv, nil
	default:
		return ConversationExport{}, fmt.Errorf("estratégia de conflito não suportada para conversa %q: %s", resolution.Identifier, resolution.Strategy)
	}
}

func applyProviderResolution(provider ProviderExport, resolution ImportResolution) (ProviderExport, error) {
	switch resolution.Strategy {
	case ConflictResolutionOverwrite:
		return provider, nil
	case ConflictResolutionRename:
		renamedID := strings.TrimSpace(resolution.RenameValue)
		if renamedID == "" {
			return ProviderExport{}, fmt.Errorf("resolução de conflito do provider %q requer um novo id", resolution.Identifier)
		}
		provider.ID = renamedID
		if existing, err := findExistingProviderByID(provider.ID); err != nil {
			return ProviderExport{}, err
		} else if existing != nil {
			return ProviderExport{}, fmt.Errorf("já existe um provider com o novo id %q", renamedID)
		}
		return provider, nil
	default:
		return ProviderExport{}, fmt.Errorf("estratégia de conflito não suportada para provider %q: %s", resolution.Identifier, resolution.Strategy)
	}
}

func applyTaskListResolution(taskList TaskListExport, resolution ImportResolution) (TaskListExport, error) {
	switch resolution.Strategy {
	case ConflictResolutionOverwrite:
		return taskList, nil
	case ConflictResolutionRename:
		renameValue := strings.TrimSpace(resolution.RenameValue)
		if renameValue == "" {
			return TaskListExport{}, fmt.Errorf("resolução de conflito da tasklist %q requer um novo identificador", resolution.Identifier)
		}
		if strings.TrimSpace(taskList.Slug) != "" {
			taskList.Slug = renameValue
		} else {
			taskList.Title = renameValue
		}
		if existing, err := findExistingTaskListByExport(taskList); err != nil {
			return TaskListExport{}, err
		} else if existing != nil {
			return TaskListExport{}, fmt.Errorf("já existe uma tasklist com o novo identificador %q", renameValue)
		}
		return taskList, nil
	default:
		return TaskListExport{}, fmt.Errorf("estratégia de conflito não suportada para tasklist %q: %s", resolution.Identifier, resolution.Strategy)
	}
}

func findExistingConversationForImport(conv ConversationExport) (*database.Conversation, error) {
	if id := strings.TrimSpace(conv.ID); id != "" {
		return findExistingConversationByID(id)
	}
	return findExistingConversationByExport(conv)
}

func findExistingConversationByID(id string) (*database.Conversation, error) {
	var existing database.Conversation
	err := database.DB().Where("id = ?", strings.TrimSpace(id)).First(&existing).Error
	if err == nil {
		return &existing, nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return nil, fmt.Errorf("erro ao localizar conversa %q: %w", id, err)
}

func findExistingConversationByExport(conv ConversationExport) (*database.Conversation, error) {
	var existing database.Conversation
	err := database.DB().
		Where("title = ? AND channel = ? AND created_at = ?", strings.TrimSpace(conv.Title), strings.TrimSpace(conv.Channel), conv.CreatedAt).
		First(&existing).Error
	if err == nil {
		return &existing, nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return nil, fmt.Errorf("erro ao localizar conversa em conflito '%s': %w", conv.Title, err)
}
