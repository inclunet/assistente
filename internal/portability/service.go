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
	"mcpServers":    {},
	"taskLists":     {},
	"credentials":   {},
}

func ExportConversations(ids []string, credMgr *credentials.Manager, req ExportRequest, appVersion string) (string, error) {
	return ExportConversationsWithContext(context.Background(), ids, credMgr, req, appVersion)
}

func ExportConversationsWithContext(ctx context.Context, ids []string, credMgr *credentials.Manager, req ExportRequest, appVersion string) (string, error) {
	return ExportPortableDataWithContext(ctx, ids, nil, nil, credMgr, req, appVersion)
}

func ExportPortableData(conversationIDs []string, providerIDs []string, taskListIDs []string, credMgr *credentials.Manager, req ExportRequest, appVersion string) (string, error) {
	return ExportPortableDataWithContext(context.Background(), conversationIDs, providerIDs, taskListIDs, credMgr, req, appVersion)
}

func ExportPortableDataWithContext(ctx context.Context, conversationIDs []string, providerIDs []string, taskListIDs []string, credMgr *credentials.Manager, req ExportRequest, appVersion string) (string, error) {
	file, err := BuildExportFileWithContext(ctx, conversationIDs, providerIDs, taskListIDs, credMgr, req, appVersion)
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
	return BuildConversationExportFileWithContext(context.Background(), ids, credMgr, req, appVersion)
}

func BuildConversationExportFileWithContext(ctx context.Context, ids []string, credMgr *credentials.Manager, req ExportRequest, appVersion string) (*ExportFile, error) {
	return BuildExportFileWithContext(ctx, ids, nil, nil, credMgr, req, appVersion)
}

func BuildExportFile(conversationIDs []string, providerIDs []string, taskListIDs []string, credMgr *credentials.Manager, req ExportRequest, appVersion string) (*ExportFile, error) {
	return BuildExportFileWithContext(context.Background(), conversationIDs, providerIDs, taskListIDs, credMgr, req, appVersion)
}

func BuildExportFileWithContext(ctx context.Context, conversationIDs []string, providerIDs []string, taskListIDs []string, credMgr *credentials.Manager, req ExportRequest, appVersion string) (*ExportFile, error) {
	conversations, err := buildConversationExports(ctx, conversationIDs, req.IncludeAudio)
	if err != nil {
		return nil, err
	}

	providers := make([]ProviderExport, 0, len(providerIDs))
	for _, id := range providerIDs {
		provider, err := database.GetLLMProviderWithContext(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("erro ao buscar provider %s: %w", id, err)
		}
		providers = append(providers, exportProvider(provider))
	}

	mcpServers, err := buildMCPServerExports(ctx, req.MCPServerSlugs)
	if err != nil {
		return nil, err
	}

	taskLists := make([]TaskListExport, 0, len(taskListIDs))
	for _, id := range taskListIDs {
		taskList, err := exportTaskListWithContext(ctx, id)
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
			MCPServers:    mcpServers,
			TaskLists:     taskLists,
		},
	}

	if req.IncludeCredentials {
		if credMgr == nil || !credMgr.CanPersist() {
			return nil, fmt.Errorf("cofre de credenciais indisponível para exportação")
		}
		creds, err := exportCredentials(ctx, credMgr)
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

func buildConversationExports(ctx context.Context, conversationIDs []string, includeAudio bool) ([]ConversationExport, error) {
	if len(conversationIDs) == 0 {
		return nil, nil
	}

	uniqueIDs := make([]string, 0, len(conversationIDs))
	seen := make(map[string]struct{}, len(conversationIDs))
	for _, rawID := range conversationIDs {
		id := strings.TrimSpace(rawID)
		if id == "" {
			return nil, fmt.Errorf("erro ao buscar conversa %s: id vazio", rawID)
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		uniqueIDs = append(uniqueIDs, id)
	}

	var conversations []database.Conversation
	if err := database.ScopeByUser(ctx, database.DB(), "user_id").
		Where("id IN ?", uniqueIDs).
		Find(&conversations).Error; err != nil {
		return nil, fmt.Errorf("erro ao buscar conversas para exportação: %w", err)
	}

	conversationsByID := make(map[string]*database.Conversation, len(conversations))
	for i := range conversations {
		conversationsByID[conversations[i].ID] = &conversations[i]
	}
	for _, id := range uniqueIDs {
		if _, ok := conversationsByID[id]; !ok {
			return nil, fmt.Errorf("erro ao buscar conversa %s: %w", id, gorm.ErrRecordNotFound)
		}
	}

	var messages []database.ChatMessage
	if err := database.DB().
		Where("conversation_id IN ?", uniqueIDs).
		Order("conversation_id ASC, created_at ASC").
		Find(&messages).Error; err != nil {
		return nil, fmt.Errorf("erro ao buscar mensagens das conversas para exportação: %w", err)
	}
	if err := hydrateToolCallResultsForExport(ctx, messages); err != nil {
		return nil, err
	}

	for _, msg := range messages {
		if conv := conversationsByID[msg.ConversationID]; conv != nil {
			conv.Messages = append(conv.Messages, msg)
		}
	}

	exports := make([]ConversationExport, 0, len(conversationIDs))
	for _, rawID := range conversationIDs {
		id := strings.TrimSpace(rawID)
		exports = append(exports, exportConversation(conversationsByID[id], includeAudio))
	}
	return exports, nil
}

func hydrateToolCallResultsForExport(ctx context.Context, messages []database.ChatMessage) error {
	if len(messages) == 0 {
		return nil
	}
	if !database.DB().Migrator().HasTable(&database.ToolInvocation{}) {
		return nil
	}
	userID, err := database.RequireUserID(ctx)
	if err != nil {
		return err
	}

	turnIDs := make([]string, 0)
	seenTurnIDs := map[string]struct{}{}
	for _, msg := range messages {
		if strings.TrimSpace(msg.ToolCalls) == "" {
			continue
		}
		if msg.TurnID == nil {
			continue
		}
		turnID := strings.TrimSpace(*msg.TurnID)
		if turnID == "" {
			continue
		}
		if _, ok := seenTurnIDs[turnID]; ok {
			continue
		}
		seenTurnIDs[turnID] = struct{}{}
		turnIDs = append(turnIDs, turnID)
	}
	if len(turnIDs) == 0 {
		return nil
	}

	resultsByTurn, err := loadChatToolInvocationResultsForTurnIDs(ctx, userID, turnIDs)
	if err != nil {
		return err
	}
	if len(resultsByTurn) == 0 {
		return nil
	}

	for i := range messages {
		msg := &messages[i]
		if strings.TrimSpace(msg.ToolCalls) == "" || msg.TurnID == nil {
			continue
		}
		turnID := strings.TrimSpace(*msg.TurnID)
		turnResults := resultsByTurn[turnID]
		if len(turnResults) == 0 {
			continue
		}
		calls := parseToolCalls(msg.ToolCalls)
		if len(calls) == 0 {
			continue
		}
		changed := false
		for _, call := range calls {
			callID, _ := call["id"].(string)
			callID = strings.TrimSpace(callID)
			if callID == "" {
				continue
			}
			if existing, ok := call["result"]; ok {
				if s, ok := existing.(string); ok && strings.TrimSpace(s) != "" {
					continue
				}
			}
			if result, ok := turnResults[callID]; ok {
				call["result"] = result
				changed = true
			}
		}
		if changed {
			if encoded, err := json.Marshal(calls); err == nil {
				msg.ToolCalls = string(encoded)
			}
		}
	}
	return nil
}

func parseToolCalls(raw string) []map[string]interface{} {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var calls []map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &calls); err == nil {
		return calls
	}
	var call map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &call); err == nil {
		return []map[string]interface{}{call}
	}
	return nil
}

func loadChatToolInvocationResultsForTurnIDs(ctx context.Context, userID string, turnIDs []string) (map[string]map[string]string, error) {
	// SQLite tem limite de variáveis (tipicamente 999).
	const maxTurnIDsPerBatch = 400
	const pageSize = 2000
	results := make(map[string]map[string]string, len(turnIDs))
	for start := 0; start < len(turnIDs); start += maxTurnIDsPerBatch {
		end := start + maxTurnIDsPerBatch
		if end > len(turnIDs) {
			end = len(turnIDs)
		}
		batch := turnIDs[start:end]

		var cursorQueuedAt *time.Time
		cursorID := ""
		for {
			q := database.DB().WithContext(ctx).
				Where(
					"user_id = ? AND origin_type = ? AND origin_id IN ? AND tool_call_id <> '' AND (completed_at IS NOT NULL OR status IN (?, ?, ?, ?))",
					userID,
					"chat",
					batch,
					"succeeded",
					"failed",
					"cancelled",
					"timed_out",
				)
			if cursorQueuedAt != nil {
				q = q.Where("(queued_at < ?) OR (queued_at = ? AND id < ?)", *cursorQueuedAt, *cursorQueuedAt, cursorID)
			}
			var rows []database.ToolInvocation
			err := q.
				Order("queued_at DESC, id DESC").
				Limit(pageSize).
				Find(&rows).Error
			if err != nil {
				return nil, fmt.Errorf("erro ao buscar tool invocations para exportação: %w", err)
			}
			if len(rows) == 0 {
				break
			}

			for _, row := range rows {
				turnID := strings.TrimSpace(row.OriginID)
				callID := strings.TrimSpace(row.ToolCallID)
				if turnID == "" || callID == "" {
					continue
				}
				byCall := results[turnID]
				if byCall == nil {
					byCall = make(map[string]string)
					results[turnID] = byCall
				}
				if _, ok := byCall[callID]; ok {
					continue
				}
				byCall[callID] = extractToolInvocationContent(row.Output)
			}

			last := rows[len(rows)-1]
			cursorQueuedAt = &last.QueuedAt
			cursorID = last.ID
			if len(rows) < pageSize {
				break
			}
		}
	}
	return results, nil
}

func extractToolInvocationContent(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	var payload struct {
		Content string `json:"content"`
	}
	if json.Unmarshal([]byte(raw), &payload) == nil {
		return payload.Content
	}
	return raw
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
	if err := validateCredentialEnvelope(file); err != nil {
		return nil, err
	}
	analysis, err := analyzeImportFile(ctx, file, credMgr, credentialPassword)
	if err != nil {
		return nil, err
	}
	resolutionMap := buildImportResolutionMap(resolutions)

	credentialConflictIdentifiers := make(map[string]struct{}, len(analysis.CredentialConflicts))
	for _, conflict := range analysis.CredentialConflicts {
		credentialConflictIdentifiers[conflict.Identifier] = struct{}{}
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
		imported, err := importConversation(ctx, conv, file.Options.IncludeAudio)
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
		imported, err := importProvider(ctx, provider)
		if err != nil {
			result.Errors = append(result.Errors, err.Error())
			result.Failed++
			continue
		}
		if imported {
			result.Imported++
		}
	}

	for _, server := range file.Resources.MCPServers {
		imported, err := importMCPServerWithCredentials(ctx, credMgr, server)
		if err != nil {
			result.Errors = append(result.Errors, err.Error())
			result.Failed++
			continue
		}
		if imported {
			result.Imported++
		} else {
			result.Skipped++
			result.SkippedMCPServerConflict++
		}
	}

	for _, taskList := range file.Resources.TaskLists {
		imported, err := importTaskList(ctx, taskList)
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
			result.SkippedMCPServerConflict-
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
	return AnalyzeImportDataWithContext(context.Background(), jsonData, credMgr, credentialPassword)
}

func AnalyzeImportDataWithContext(ctx context.Context, jsonData string, credMgr *credentials.Manager, credentialPassword string) (*ImportAnalysis, error) {
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
	if err := validateCredentialEnvelope(file); err != nil {
		return nil, err
	}
	analysis, err := analyzeImportFile(ctx, file, credMgr, credentialPassword)
	if err != nil {
		return nil, err
	}
	analysis.UnsupportedResourceTypes = unsupportedResourceTypes
	return analysis, nil
}

func validateCredentialEnvelope(file *ExportFile) error {
	if file.Options.IncludeCredentials && file.Resources.Credentials == nil {
		return fmt.Errorf("export declara includeCredentials=true, mas resources.credentials está ausente")
	}
	return nil
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

func importConversation(ctx context.Context, conv ConversationExport, includeAudio bool) (bool, error) {
	if existing, err := findExistingConversationForImport(ctx, conv); err != nil {
		return false, err
	} else if existing != nil {
		return overwriteConversationByExisting(ctx, conv, includeAudio, existing)
	}

	err := database.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		newConv, err := createImportedConversation(ctx, tx, conv)
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

func overwriteConversationByExisting(ctx context.Context, conv ConversationExport, includeAudio bool, existing *database.Conversation) (bool, error) {
	err := database.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		updatedAt := conv.CreatedAt
		if updatedAt.IsZero() {
			updatedAt = time.Now().UTC()
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
		if err := deleteChatToolInvocationsForConversationTx(ctx, tx, existing.ID); err != nil {
			return err
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

func deleteChatToolInvocationsForConversationTx(ctx context.Context, tx *gorm.DB, conversationID string) error {
	conversationID = strings.TrimSpace(conversationID)
	if conversationID == "" {
		return nil
	}
	if tx == nil {
		return nil
	}
	if !tx.Migrator().HasTable(&database.ToolInvocation{}) {
		return nil
	}
	userID, err := database.RequireUserID(ctx)
	if err != nil {
		return err
	}

	var turnIDs []string
	if err := tx.Model(&database.ChatMessage{}).
		Where("conversation_id = ? AND turn_id IS NOT NULL AND turn_id <> ''", conversationID).
		Distinct().
		Pluck("turn_id", &turnIDs).Error; err != nil {
		return fmt.Errorf("erro ao buscar turn_ids da conversa '%s': %w", conversationID, err)
	}
	var msgIDs []string
	if err := tx.Model(&database.ChatMessage{}).
		Where("conversation_id = ?", conversationID).
		Pluck("id", &msgIDs).Error; err != nil {
		return fmt.Errorf("erro ao buscar message ids da conversa '%s': %w", conversationID, err)
	}

	ids := make([]string, 0, len(turnIDs)+len(msgIDs))
	ids = append(ids, turnIDs...)
	ids = append(ids, msgIDs...)
	if len(ids) == 0 {
		return nil
	}

	const batchSize = 400
	for start := 0; start < len(ids); start += batchSize {
		end := start + batchSize
		if end > len(ids) {
			end = len(ids)
		}
		if err := tx.
			Where("user_id = ? AND origin_type = ? AND origin_id IN ?", userID, "chat", ids[start:end]).
			Delete(&database.ToolInvocation{}).Error; err != nil {
			return fmt.Errorf("erro ao limpar tool invocations da conversa '%s': %w", conversationID, err)
		}
	}
	return nil
}

func createImportedConversation(ctx context.Context, tx *gorm.DB, conv ConversationExport) (*database.Conversation, error) {
	conversationID := strings.TrimSpace(conv.ID)
	if conversationID == "" {
		return nil, fmt.Errorf("conversa %q sem id não pode ser importada no formato version %d", conv.Title, ExportVersion)
	}

	newConv := &database.Conversation{
		UUIDModel: database.UUIDModel{
			ID: conversationID,
		},
		Title:     conv.Title,
		Channel:   conv.Channel,
		ContactID: conv.ContactID,
		Summary:   conv.Summary,
	}
	if userID, ok := database.UserIDFromContext(ctx); ok {
		newConv.UserID = userID
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
	for i, msg := range conv.Messages {
		id := strings.TrimSpace(msg.ID)
		if id == "" {
			return fmt.Errorf("mensagem %d da conversa %q sem id não pode ser importada no formato version %d", i, conv.Title, ExportVersion)
		}
		if _, exists := exportedMessageIDs[id]; exists {
			return fmt.Errorf("mensagem %d da conversa %q usa id duplicado %q", i, conv.Title, id)
		}
		exportedMessageIDs[id] = struct{}{}
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

func exportCredentials(ctx context.Context, credMgr *credentials.Manager) ([]CredentialExport, error) {
	list, err := credMgr.ListVisibleCredentialsWithContext(ctx)
	if err != nil {
		return nil, err
	}
	idByPattern := make(map[string]string)
	var entries []database.CredentialEntry
	if err := database.ScopeByUser(ctx, database.DB(), "user_id").Find(&entries).Error; err != nil {
		return nil, fmt.Errorf("erro ao carregar credenciais persistidas para exportação: %w", err)
	}
	for _, entry := range entries {
		pattern := strings.TrimSpace(entry.Pattern)
		if pattern == "" || strings.TrimSpace(entry.ID) == "" || !isPortableCredentialPattern(pattern) {
			continue
		}
		idByPattern[pattern] = strings.TrimSpace(entry.ID)
	}

	result := make([]CredentialExport, 0, len(list))
	for _, entry := range list {
		pattern := strings.TrimSpace(entry.Pattern)
		if entry.Auth == nil || entry.Unreadable || !isPortableCredentialPattern(pattern) {
			continue
		}
		id := strings.TrimSpace(entry.ID)
		if id == "" {
			id = idByPattern[strings.TrimSpace(entry.Pattern)]
		}
		result = append(result, CredentialExport{
			ID:           id,
			Pattern:      pattern,
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
		if err := validatePortableCredentialExport(cred); err != nil {
			return imported, skipped, err
		}
		identifier := credentialConflictIdentifier(cred)
		credentialID := strings.TrimSpace(cred.ID)
		if _, hasConflict := conflictIdentifiers[identifier]; hasConflict {
			resolution, hasResolution := resolutionMap.lookup("credential", identifier)
			if !hasResolution || resolution.Strategy == ConflictResolutionSkip {
				skipped++
				continue
			}
			if resolution.Strategy != ConflictResolutionOverwrite {
				return imported, skipped, fmt.Errorf("estratégia de conflito não suportada para credencial %q: %s", identifier, resolution.Strategy)
			}
			credentialID = ""
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
			ID:      credentialID,
			Pattern: cred.Pattern,
			Auth:    auth,
		}); err != nil {
			return imported, skipped, fmt.Errorf("erro ao importar credencial '%s': %w", cred.Pattern, err)
		}
		imported++
	}

	return imported, skipped, nil
}

func isPortableCredentialPattern(pattern string) bool {
	pattern = strings.TrimSpace(pattern)
	return pattern != "" && !credentials.IsManagedPattern(pattern)
}

func validatePortableCredentialExport(cred CredentialExport) error {
	pattern := strings.TrimSpace(cred.Pattern)
	if !isPortableCredentialPattern(pattern) {
		return fmt.Errorf("credencial gerenciada/interna não pode ser importada: %q", pattern)
	}
	return nil
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

func analyzeImportFile(ctx context.Context, file *ExportFile, credMgr *credentials.Manager, credentialPassword string) (*ImportAnalysis, error) {
	analysis := &ImportAnalysis{
		Version:               file.Version,
		AppVersion:            file.AppVersion,
		ConversationCount:     len(file.Resources.Conversations),
		ProviderCount:         len(file.Resources.Providers),
		MCPServerCount:        len(file.Resources.MCPServers),
		TaskListCount:         len(file.Resources.TaskLists),
		IncludesCredentials:   file.Options.IncludeCredentials && file.Resources.Credentials != nil,
		ConversationConflicts: make([]ImportConflict, 0),
		ProviderConflicts:     make([]ImportConflict, 0),
		MCPServerConflicts:    make([]ImportConflict, 0),
		TaskListConflicts:     make([]ImportConflict, 0),
		CredentialConflicts:   make([]ImportConflict, 0),
		Warnings:              make([]string, 0),
	}

	for _, conv := range file.Resources.Conversations {
		analysis.MessageCount += len(conv.Messages)
	}
	analysis.TaskCount, analysis.TaskNoteCount = countExportedTasks(file.Resources.TaskLists)
	if len(file.Resources.MCPServers) > 0 {
		existingMCPSlugs, err := loadExistingMCPServerSlugs(ctx)
		if err != nil {
			return nil, fmt.Errorf("erro ao analisar servidores MCP existentes: %w", err)
		}
		for _, server := range file.Resources.MCPServers {
			normalized := normalizeMCPServerExport(server)
			slug := strings.TrimSpace(normalized.Slug)
			if slug == "" {
				continue
			}
			if _, exists := existingMCPSlugs[slug]; !exists {
				continue
			}
			analysis.MCPServerConflicts = append(analysis.MCPServerConflicts, ImportConflict{
				ResourceType:        "mcpServer",
				Identifier:          slug,
				Reason:              "Já existe um servidor MCP registrado com o mesmo slug.",
				SupportedStrategies: []ConflictResolutionStrategy{ConflictResolutionSkip},
			})
		}
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
					existingCredentialIDs, existingCredentialPatterns, err := loadExistingCredentialIdentifiers(ctx)
					if err != nil {
						return nil, fmt.Errorf("erro ao analisar credenciais existentes: %w", err)
					}
					for _, cred := range creds {
						if err := validatePortableCredentialExport(cred); err != nil {
							analysis.CredentialAnalysisError = err.Error()
							analysis.Warnings = append(analysis.Warnings, err.Error())
							continue
						}
						if id := strings.TrimSpace(cred.ID); id != "" {
							if _, exists := existingCredentialIDs[id]; exists {
								continue
							}
						}
						identifier := credentialConflictIdentifier(cred)
						if _, exists := existingCredentialPatterns[identifier]; !exists {
							continue
						}
						analysis.CredentialConflicts = append(analysis.CredentialConflicts, ImportConflict{
							ResourceType:        "credential",
							Identifier:          identifier,
							Reason:              "Já existe uma credencial registrada com o mesmo pattern.",
							SupportedStrategies: []ConflictResolutionStrategy{ConflictResolutionSkip, ConflictResolutionOverwrite},
						})
					}
				}
			}
		}
	}

	analysis.ConflictCount = len(analysis.ConversationConflicts) + len(analysis.ProviderConflicts) + len(analysis.MCPServerConflicts) + len(analysis.TaskListConflicts) + len(analysis.CredentialConflicts)
	if emptyCount := countEmptyConversations(file.Resources.Conversations); emptyCount > 0 {
		analysis.Warnings = append(analysis.Warnings, fmt.Sprintf("%d conversa(s) vazia(s) serão descartadas na importação.", emptyCount))
	}
	return analysis, nil
}

func loadExistingMCPServerSlugs(ctx context.Context) (map[string]struct{}, error) {
	var rows []database.MCPServer
	if err := database.ScopeByUser(ctx, database.DB().WithContext(ctx), "user_id").
		Select("slug").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		if slug := strings.TrimSpace(row.Slug); slug != "" {
			result[slug] = struct{}{}
		}
	}
	return result, nil
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

func loadExistingCredentialIdentifiers(ctx context.Context) (map[string]struct{}, map[string]struct{}, error) {
	var entries []database.CredentialEntry
	query := database.ScopeByUser(ctx, database.DB(), "user_id")
	if err := query.Find(&entries).Error; err != nil {
		return nil, nil, err
	}

	ids := make(map[string]struct{}, len(entries))
	patterns := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		if pattern := strings.TrimSpace(entry.Pattern); pattern != "" {
			if !isPortableCredentialPattern(pattern) {
				continue
			}
			patterns[pattern] = struct{}{}
		}
		if id := strings.TrimSpace(entry.ID); id != "" {
			ids[id] = struct{}{}
		}
	}
	return ids, patterns, nil
}

func credentialConflictIdentifier(cred CredentialExport) string {
	return strings.TrimSpace(cred.Pattern)
}

func parseExportFile(jsonData string) (*ExportFile, []string, error) {
	rawData := []byte(jsonData)
	var envelope struct {
		Resources map[string]json.RawMessage `json:"resources"`
		Version   int                        `json:"version"`
	}
	if err := json.Unmarshal(rawData, &envelope); err != nil {
		return nil, nil, fmt.Errorf("erro ao parsear JSON: %w", err)
	}
	if envelope.Version == 0 && len(envelope.Resources) == 0 {
		if servers, ok, err := parseExternalMCPServers(rawData); err != nil {
			return nil, nil, fmt.Errorf("erro ao parsear MCP JSON: %w", err)
		} else if ok {
			return &ExportFile{
				Version:    ExportVersion,
				ExportedAt: time.Now().UTC(),
				Options:    ExportOptions{},
				Resources:  ExportResources{MCPServers: servers},
			}, nil, nil
		}
	}

	var file ExportFile
	if err := json.Unmarshal(rawData, &file); err != nil {
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

func findExistingConversationForImport(ctx context.Context, conv ConversationExport) (*database.Conversation, error) {
	if id := strings.TrimSpace(conv.ID); id != "" {
		return findExistingConversationByID(ctx, id)
	}
	return nil, nil
}

func findExistingConversationByID(ctx context.Context, id string) (*database.Conversation, error) {
	var existing database.Conversation
	err := database.ScopeByUser(ctx, database.DB(), "user_id").Where("id = ?", strings.TrimSpace(id)).First(&existing).Error
	if err == nil {
		return &existing, nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return nil, fmt.Errorf("erro ao localizar conversa %q: %w", id, err)
}
