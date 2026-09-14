package database

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"assistente/internal/logging"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var errToolLedgerResourceDeleted = errors.New("recurso do backfill removido")

const (
	toolLedgerStatePending    = "pending"
	toolLedgerStateBackfilled = "backfilled"

	toolLedgerResourceConversation = "conversation"
	toolLedgerResourceJobRun       = "job_run"

	toolLedgerMigrationProvenance = "aep-0104-v18"
	toolLedgerArchiveOrigin       = "archival"
	toolLedgerArchiveUnavailable  = "unavailable"
)

type legacyToolCall struct {
	CallID             string
	Name               string
	Arguments          string
	Result             string
	TurnID             string
	AssistantMessageID string
	Iteration          int
	SourceKey          string
	QueuedAt           time.Time
	CompletedAt        time.Time
}

type toolLedgerBackfillReport struct {
	LegacyRows          int
	LedgerRows          int
	LegacyInputHashes   []string
	LegacyOutputHashes  []string
	LedgerInputHashes   []string
	LedgerOutputHashes  []string
	AmbiguousCount      int
	LastErrorCode       string
	LastProcessedKey    string
	LegacyHighWatermark *time.Time
}

// migrateToolLedgerBackfill é retomável por conversa/run. Cada recurso fecha
// em uma transação própria; crash entre recursos preserva os já marcados como
// backfilled e o próximo boot continua somente os pendentes.
func migrateToolLedgerBackfill(database *gorm.DB) error {
	if database == nil {
		return nil
	}
	statements := []string{
		`CREATE UNIQUE INDEX IF NOT EXISTS ux_tool_invocations_migration_source
		   ON tool_invocations (user_id, migration_source_key)
		 WHERE migration_source_key <> ''`,
		`CREATE UNIQUE INDEX IF NOT EXISTS ux_tool_catalog_archival_user_name
		   ON tool_catalog (user_id, name)
		 WHERE origin = 'archival' AND user_id IS NOT NULL`,
	}
	for _, statement := range statements {
		if err := database.Exec(statement).Error; err != nil {
			return err
		}
	}

	deferredOwners, err := seedToolLedgerMigrationStates(database)
	if err != nil {
		return err
	}
	started := time.Now()
	var rows int
	var bytes int64
	processedResources := 0
	cursor := ""
	for {
		var states []ToolLedgerMigrationState
		query := database.Where(`
			state = ?
			OR (
				resource_type = 'conversation'
				AND EXISTS (
					SELECT 1 FROM chat_messages
					 WHERE chat_messages.conversation_id = tool_ledger_migration_states.resource_id
					   AND chat_messages.updated_at > COALESCE(tool_ledger_migration_states.legacy_high_watermark, '0001-01-01')
					   AND (
					     chat_messages.role = 'tool'
					     OR (chat_messages.role = 'assistant'
					         AND trim(COALESCE(chat_messages.tool_calls, '')) NOT IN ('', '[]', 'null'))
					   )
				)
			)
			OR (
				resource_type = 'job_run'
				AND EXISTS (
					SELECT 1 FROM job_runs
					 WHERE job_runs.id = tool_ledger_migration_states.resource_id
					   AND job_runs.updated_at > COALESCE(tool_ledger_migration_states.legacy_high_watermark, '0001-01-01')
				)
			)`, toolLedgerStatePending)
		if cursor != "" {
			query = query.Where("id > ?", cursor)
		}
		if err := query.Order("id").Limit(100).Find(&states).Error; err != nil {
			return err
		}
		if len(states) == 0 {
			break
		}
		for _, state := range states {
			var report toolLedgerBackfillReport
			resourceDeleted := false
			err := database.Transaction(func(tx *gorm.DB) error {
				var processErr error
				switch state.ResourceType {
				case toolLedgerResourceConversation:
					report, processErr = backfillConversationToolLedger(tx, state)
				case toolLedgerResourceJobRun:
					report, processErr = backfillJobRunToolLedger(tx, state)
				default:
					processErr = fmt.Errorf("tipo de recurso de backfill desconhecido: %s", state.ResourceType)
				}
				if processErr != nil {
					if errors.Is(processErr, errToolLedgerResourceDeleted) {
						resourceDeleted = true
						return tx.Delete(&state).Error
					}
					return processErr
				}
				rows += report.LedgerRows
				state.LegacyRows = report.LegacyRows
				state.LedgerRows = report.LedgerRows
				state.LegacyInputDigest = digestHashes(report.LegacyInputHashes)
				state.LegacyOutputDigest = digestHashes(report.LegacyOutputHashes)
				state.LedgerInputDigest = digestHashes(report.LedgerInputHashes)
				state.LedgerOutputDigest = digestHashes(report.LedgerOutputHashes)
				if report.LastErrorCode == "" &&
					state.ResourceType == toolLedgerResourceConversation &&
					state.LegacyRows != state.LedgerRows {
					report.LastErrorCode = "count_mismatch"
				} else if report.LastErrorCode == "" &&
					(state.LegacyInputDigest != state.LedgerInputDigest ||
						state.LegacyOutputDigest != state.LedgerOutputDigest) {
					report.LastErrorCode = "hash_mismatch"
				}
				state.AmbiguousCount = report.AmbiguousCount
				state.LastErrorCode = report.LastErrorCode
				state.LastProcessedKey = report.LastProcessedKey
				state.LegacyHighWatermark = report.LegacyHighWatermark
				if report.AmbiguousCount == 0 && report.LastErrorCode == "" {
					state.State = toolLedgerStateBackfilled
				} else {
					state.State = toolLedgerStatePending
				}
				return tx.Save(&state).Error
			})
			if err != nil {
				return err
			}
			if resourceDeleted {
				processedResources++
				cursor = state.ID
				continue
			}
			processedResources++
			cursor = state.ID
		}
	}
	metadataRows, err := backfillExistingInvocationMetadata(database)
	if err != nil {
		return err
	}
	rows += metadataRows

	if err := database.Model(&ToolInvocation{}).
		Where("migration_provenance = ?", toolLedgerMigrationProvenance).
		Select("COALESCE(SUM(input_bytes + output_bytes), 0)").
		Scan(&bytes).Error; err != nil {
		return err
	}
	var pending int64
	if err := database.Model(&ToolLedgerMigrationState{}).
		Where("state <> ? OR ambiguous_count > 0 OR last_error_code <> ''", toolLedgerStateBackfilled).
		Count(&pending).Error; err != nil {
		return err
	}
	logging.Debugf(
		context.Background(),
		"database.tool-ledger.backfill",
		"phase=backfill resources=%d rows=%d bytes=%d duration_ms=%d pending=%d deferred_owners=%d",
		processedResources, rows, bytes, time.Since(started).Milliseconds(), pending, deferredOwners,
	)
	if pending > 0 || deferredOwners > 0 {
		return errMigrationDeferred
	}
	return nil
}

func backfillExistingInvocationMetadata(database *gorm.DB) (int, error) {
	cursor := ""
	updated := 0
	for {
		var invocations []ToolInvocation
		query := database.
			Where(`input_hash = '' OR output_hash = '' OR attempt < 1`)
		if cursor != "" {
			query = query.Where("id > ?", cursor)
		}
		if err := query.Order("id").Limit(100).Find(&invocations).Error; err != nil {
			return updated, err
		}
		if len(invocations) == 0 {
			return updated, nil
		}
		catalogIDs := make([]string, 0, len(invocations))
		for index := range invocations {
			catalogIDs = append(catalogIDs, invocations[index].ToolCatalogID)
		}
		var catalogs []ToolCatalog
		if err := database.Select("id", "name", "display_name").Where("id IN ?", catalogIDs).Find(&catalogs).Error; err != nil {
			return updated, err
		}
		displayNames := make(map[string]string, len(catalogs))
		for _, catalog := range catalogs {
			displayName := strings.TrimSpace(catalog.DisplayName)
			if displayName == "" {
				displayName = catalog.Name
			}
			displayNames[catalog.ID] = displayName
		}
		if err := database.Transaction(func(tx *gorm.DB) error {
			for index := range invocations {
				invocation := &invocations[index]
				availability := "missing"
				if strings.TrimSpace(invocation.Output) != "" {
					availability = "available"
				}
				if err := withoutPayloadLogging(tx).Model(invocation).Updates(map[string]any{
					"attempt":              maxInt(1, invocation.Attempt),
					"display_name":         displayNames[invocation.ToolCatalogID],
					"input_preview":        structuralPreview(invocation.Input),
					"output_preview":       structuralPreview(invocation.Output),
					"input_bytes":          len([]byte(invocation.Input)),
					"output_bytes":         len([]byte(invocation.Output)),
					"input_hash":           digestValue(invocation.Input),
					"output_hash":          digestValue(invocation.Output),
					"result_availability":  availability,
					"migration_provenance": toolLedgerMigrationProvenance,
				}).Error; err != nil {
					return err
				}
				updated++
			}
			return nil
		}); err != nil {
			return updated, err
		}
		cursor = invocations[len(invocations)-1].ID
	}
}

func seedToolLedgerMigrationStates(database *gorm.DB) (int, error) {
	type resource struct {
		UserID       string
		ResourceType string
		ResourceID   string
	}
	var resources []resource
	if err := database.Raw(`
		SELECT conversations.user_id, 'conversation' AS resource_type, conversations.id AS resource_id
		  FROM conversations
		 WHERE EXISTS (
		       SELECT 1 FROM chat_messages
		        WHERE chat_messages.conversation_id = conversations.id
		          AND (
		            chat_messages.role = 'tool'
		            OR (chat_messages.role = 'assistant'
		                AND trim(COALESCE(chat_messages.tool_calls, '')) NOT IN ('', '[]', 'null'))
		          )
		 )
		UNION ALL
		SELECT job_runs.user_id, 'job_run', job_runs.id
		  FROM job_runs
		 WHERE trim(COALESCE(job_runs.tool_name, '')) <> ''
		    OR trim(COALESCE(job_runs.inputs, '')) <> ''
		    OR trim(COALESCE(job_runs.output, '')) <> ''`).Scan(&resources).Error; err != nil {
		return 0, err
	}
	deferred := 0
	var existingStates []ToolLedgerMigrationState
	if err := database.Select("user_id", "resource_type", "resource_id").Find(&existingStates).Error; err != nil {
		return 0, err
	}
	existing := make(map[string]struct{}, len(existingStates))
	for _, state := range existingStates {
		existing[toolLedgerResourceKey(state.UserID, state.ResourceType, state.ResourceID)] = struct{}{}
	}
	for _, resource := range resources {
		resource.UserID = strings.TrimSpace(resource.UserID)
		if resource.UserID == "" {
			deferred++
			continue
		}
		key := toolLedgerResourceKey(resource.UserID, resource.ResourceType, resource.ResourceID)
		if _, found := existing[key]; found {
			continue
		}
		state := ToolLedgerMigrationState{
			UserID:       resource.UserID,
			ResourceType: resource.ResourceType,
			ResourceID:   resource.ResourceID,
			State:        toolLedgerStatePending,
		}
		if err := database.Create(&state).Error; err != nil {
			return 0, err
		}
		existing[key] = struct{}{}
	}
	return deferred, nil
}

func toolLedgerResourceKey(userID, resourceType, resourceID string) string {
	return userID + "\x00" + resourceType + "\x00" + resourceID
}

func backfillConversationToolLedger(tx *gorm.DB, state ToolLedgerMigrationState) (toolLedgerBackfillReport, error) {
	var conversation Conversation
	if err := tx.Where("id = ? AND user_id = ?", state.ResourceID, state.UserID).First(&conversation).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return toolLedgerBackfillReport{}, errToolLedgerResourceDeleted
		}
		return toolLedgerBackfillReport{}, err
	}
	var messages []ChatMessage
	if err := tx.Where("conversation_id = ?", conversation.ID).Order("created_at, id").Find(&messages).Error; err != nil {
		return toolLedgerBackfillReport{}, err
	}

	resultsByTurnCall := make(map[string][]ChatMessage)
	calls := make([]legacyToolCall, 0)
	for _, message := range messages {
		if message.Role == "tool" && strings.TrimSpace(message.ToolCallID) != "" {
			key := legacyTurnCallKey(messageTurnID(message), message.ToolCallID)
			resultsByTurnCall[key] = append(resultsByTurnCall[key], message)
			continue
		}
		if message.Role != "assistant" || legacyToolCallsEmpty(message.ToolCalls) {
			continue
		}
		parsed, err := parseLegacyToolCalls(message)
		if err != nil {
			return blockedConversationReport(messages, "invalid_tool_calls"), nil
		}
		calls = append(calls, parsed...)
	}

	report := toolLedgerBackfillReport{}
	usedResultMessages := make(map[string]struct{})
	for _, message := range messages {
		if message.Role == "tool" || (message.Role == "assistant" && !legacyToolCallsEmpty(message.ToolCalls)) {
			report.LegacyHighWatermark = laterTime(report.LegacyHighWatermark, message.UpdatedAt)
		}
	}
	callKeys := make(map[string]struct{}, len(calls))
	callIDs := make(map[string]int, len(calls))
	for _, call := range calls {
		callKeys[legacyTurnCallKey(call.TurnID, call.CallID)] = struct{}{}
		callIDs[call.CallID]++
	}
	for _, message := range messages {
		if message.Role != "tool" {
			continue
		}
		callID := strings.TrimSpace(message.ToolCallID)
		if callID == "" {
			report.LegacyRows++
			report.AmbiguousCount++
			report.LastErrorCode = "missing_call_identity"
			continue
		}
		if _, exact := callKeys[legacyTurnCallKey(messageTurnID(message), callID)]; !exact && callIDs[callID] > 0 {
			report.AmbiguousCount++
			report.LastErrorCode = "tool_result_turn_mismatch"
		}
	}
	for _, call := range calls {
		report.LegacyRows++
		if call.CallID == "" || call.Name == "" {
			report.AmbiguousCount++
			report.LastErrorCode = "missing_call_identity"
			continue
		}
		results := resultsByTurnCall[legacyTurnCallKey(call.TurnID, call.CallID)]
		if len(results) > 1 {
			report.AmbiguousCount++
			report.LastErrorCode = "duplicate_tool_result"
		}
		var existingCount int64
		if err := tx.Model(&ToolInvocation{}).Where(
			`user_id = ? AND origin_type = 'chat' AND tool_call_id = ?
			 AND (migration_source_key = ? OR origin_id IN (?, ?))`,
			state.UserID, call.CallID, call.SourceKey, call.TurnID, call.AssistantMessageID,
		).Count(&existingCount).Error; err != nil {
			return report, err
		}
		if existingCount > 1 {
			report.AmbiguousCount++
			report.LastErrorCode = "duplicate_ledger_invocation"
		}
	}
	for _, results := range resultsByTurnCall {
		if len(results) > 1 && report.LastErrorCode != "duplicate_tool_result" {
			report.AmbiguousCount++
			report.LastErrorCode = "duplicate_tool_result"
		}
	}
	if report.AmbiguousCount > 0 || report.LastErrorCode != "" {
		return report, nil
	}

	for _, call := range calls {
		results := resultsByTurnCall[legacyTurnCallKey(call.TurnID, call.CallID)]
		result := call.Result
		if len(results) == 1 {
			result = results[0].Content
			call.CompletedAt = results[0].CreatedAt
			usedResultMessages[results[0].ID] = struct{}{}
		}
		invocation, err := upsertMigratedChatInvocation(tx, state.UserID, conversation.ID, call, result)
		if err != nil {
			return report, err
		}
		report.LedgerRows++
		report.LegacyInputHashes = append(report.LegacyInputHashes, digestValue(call.Arguments))
		report.LegacyOutputHashes = append(report.LegacyOutputHashes, digestValue(result))
		report.LedgerInputHashes = append(report.LedgerInputHashes, digestValue(invocation.Input))
		report.LedgerOutputHashes = append(report.LedgerOutputHashes, digestCanonicalOutput(invocation.Output))
		report.LastProcessedKey = call.SourceKey
	}

	for _, message := range messages {
		if message.Role != "tool" || strings.TrimSpace(message.ToolCallID) == "" {
			continue
		}
		if _, used := usedResultMessages[message.ID]; used {
			continue
		}
		call := legacyToolCall{
			CallID:             strings.TrimSpace(message.ToolCallID),
			Name:               "tool_result",
			Result:             message.Content,
			TurnID:             messageTurnID(message),
			AssistantMessageID: "",
			SourceKey:          "chat-result:" + message.ID,
			QueuedAt:           message.CreatedAt,
			CompletedAt:        message.CreatedAt,
		}
		invocation, err := upsertMigratedChatInvocation(tx, state.UserID, conversation.ID, call, call.Result)
		if err != nil {
			return report, err
		}
		report.LegacyRows++
		report.LedgerRows++
		report.LegacyInputHashes = append(report.LegacyInputHashes, digestValue(""))
		report.LegacyOutputHashes = append(report.LegacyOutputHashes, digestValue(call.Result))
		report.LedgerInputHashes = append(report.LedgerInputHashes, digestValue(invocation.Input))
		report.LedgerOutputHashes = append(report.LedgerOutputHashes, digestCanonicalOutput(invocation.Output))
		report.LastProcessedKey = call.SourceKey
	}
	return report, nil
}

func blockedConversationReport(messages []ChatMessage, code string) toolLedgerBackfillReport {
	legacyRows := 0
	var highWatermark *time.Time
	for _, message := range messages {
		if message.Role == "tool" || (message.Role == "assistant" && !legacyToolCallsEmpty(message.ToolCalls)) {
			legacyRows++
			highWatermark = laterTime(highWatermark, message.UpdatedAt)
		}
	}
	return toolLedgerBackfillReport{
		LegacyRows:          legacyRows,
		AmbiguousCount:      1,
		LastErrorCode:       code,
		LegacyHighWatermark: highWatermark,
	}
}

func parseLegacyToolCalls(message ChatMessage) ([]legacyToolCall, error) {
	raw := strings.TrimSpace(message.ToolCalls)
	var items []map[string]any
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		var single map[string]any
		if singleErr := json.Unmarshal([]byte(raw), &single); singleErr != nil {
			return nil, err
		}
		items = []map[string]any{single}
	}
	calls := make([]legacyToolCall, 0, len(items))
	for index, item := range items {
		callID, _ := item["id"].(string)
		name, arguments := legacyFunction(item)
		result := legacyJSONField(item["result"])
		completedAt := time.Time{}
		if result != "" {
			completedAt = message.CreatedAt
		}
		calls = append(calls, legacyToolCall{
			CallID:             strings.TrimSpace(callID),
			Name:               strings.TrimSpace(name),
			Arguments:          arguments,
			Result:             result,
			TurnID:             messageTurnID(message),
			AssistantMessageID: message.ID,
			Iteration:          index + 1,
			SourceKey:          fmt.Sprintf("chat:%s:%d:%s", message.ID, index, strings.TrimSpace(callID)),
			QueuedAt:           message.CreatedAt,
			CompletedAt:        completedAt,
		})
	}
	return calls, nil
}

func legacyFunction(item map[string]any) (string, string) {
	container, _ := item["function"].(map[string]any)
	name, _ := container["name"].(string)
	arguments := legacyJSONField(container["arguments"])
	if name == "" {
		name, _ = item["name"].(string)
	}
	if arguments == "" {
		arguments = legacyJSONField(item["arguments"])
	}
	return name, arguments
}

func legacyJSONField(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case nil:
		return ""
	default:
		encoded, _ := json.Marshal(typed)
		return string(encoded)
	}
}

func upsertMigratedChatInvocation(tx *gorm.DB, userID, conversationID string, call legacyToolCall, result string) (ToolInvocation, error) {
	var existing []ToolInvocation
	err := tx.Where(
		`user_id = ? AND origin_type = 'chat' AND tool_call_id = ?
		 AND (migration_source_key = ? OR origin_id IN (?, ?))`,
		userID, call.CallID, call.SourceKey, call.TurnID, call.AssistantMessageID,
	).Order("queued_at, id").Find(&existing).Error
	if err != nil {
		return ToolInvocation{}, err
	}
	if len(existing) > 1 {
		return ToolInvocation{}, fmt.Errorf("ambiguous existing chat invocations for source key %s", call.SourceKey)
	}
	catalogID, err := resolveOrCreateArchivalCatalog(tx, userID, call.Name)
	if err != nil {
		return ToolInvocation{}, err
	}
	input := normalizeLegacyValue(call.Arguments)
	output := migratedOutput(result)
	conversation := conversationID
	turn := call.TurnID
	status := "succeeded"
	availability := "available"
	if result == "" {
		status = "failed"
		availability = "missing"
	}
	if len(existing) == 1 {
		canonicalInput := existing[0].Input
		if strings.TrimSpace(canonicalInput) == "" {
			canonicalInput = input
		}
		canonicalOutput := existing[0].Output
		if strings.TrimSpace(canonicalOutput) == "" {
			canonicalOutput = output
		}
		canonicalAvailability := availability
		if strings.TrimSpace(canonicalOutput) != "" {
			canonicalAvailability = "available"
		}
		additiveFields := map[string]any{
			"conversation_id":      &conversation,
			"turn_id":              &turn,
			"attempt":              maxInt(1, existing[0].Attempt),
			"display_name":         call.Name,
			"input":                canonicalInput,
			"output":               canonicalOutput,
			"input_preview":        structuralPreview(canonicalInput),
			"output_preview":       structuralPreview(result),
			"input_bytes":          len([]byte(canonicalInput)),
			"output_bytes":         len([]byte(canonicalOutput)),
			"input_hash":           digestValue(canonicalInput),
			"output_hash":          digestValue(canonicalOutput),
			"result_availability":  canonicalAvailability,
			"migration_source_key": call.SourceKey,
			"migration_provenance": toolLedgerMigrationProvenance,
		}
		if err := withoutPayloadLogging(tx).Model(&existing[0]).Updates(additiveFields).Error; err != nil {
			return ToolInvocation{}, err
		}
		return existing[0], tx.First(&existing[0], "id = ?", existing[0].ID).Error
	}
	now := call.QueuedAt
	if now.IsZero() {
		now = time.Now().UTC()
	}
	var completedAt *time.Time
	if !call.CompletedAt.IsZero() {
		completed := call.CompletedAt
		completedAt = &completed
	}
	invocation := ToolInvocation{
		UserID:              userID,
		ToolCatalogID:       catalogID,
		OriginType:          "chat",
		OriginID:            call.TurnID,
		ConversationID:      &conversation,
		TurnID:              &turn,
		ToolCallID:          call.CallID,
		Attempt:             1,
		Status:              status,
		Input:               input,
		Output:              output,
		Metadata:            migratedDisplayMetadata(call),
		DisplayName:         call.Name,
		InputPreview:        structuralPreview(input),
		OutputPreview:       structuralPreview(result),
		InputBytes:          int64(len([]byte(input))),
		OutputBytes:         int64(len([]byte(output))),
		InputHash:           digestValue(input),
		OutputHash:          digestValue(output),
		ResultAvailability:  availability,
		MigrationSourceKey:  call.SourceKey,
		MigrationProvenance: toolLedgerMigrationProvenance,
		QueuedAt:            now,
		StartedAt:           &now,
		CompletedAt:         completedAt,
	}
	return invocation, withoutPayloadLogging(tx).Create(&invocation).Error
}

func migratedDisplayMetadata(call legacyToolCall) string {
	payload := map[string]any{
		"display": map[string]any{
			"version":              1,
			"type":                 "function",
			"name":                 call.Name,
			"arguments":            call.Arguments,
			"iteration":            call.Iteration,
			"assistant_message_id": call.AssistantMessageID,
		},
		"migration": toolLedgerMigrationProvenance,
	}
	encoded, _ := json.Marshal(payload)
	return string(encoded)
}

func backfillJobRunToolLedger(tx *gorm.DB, state ToolLedgerMigrationState) (toolLedgerBackfillReport, error) {
	type jobRunRow struct {
		JobRun
		DefinitionToolCatalogID string
		DefinitionToolName      string
	}
	var row jobRunRow
	if err := tx.Model(&JobRun{}).
		Select("job_runs.*, jobs.tool_catalog_id AS definition_tool_catalog_id, jobs.tool_name AS definition_tool_name").
		Joins("JOIN jobs ON jobs.id = job_runs.job_id").
		Where("job_runs.id = ? AND job_runs.user_id = ?", state.ResourceID, state.UserID).
		First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return toolLedgerBackfillReport{}, errToolLedgerResourceDeleted
		}
		return toolLedgerBackfillReport{}, err
	}
	sourceKeyPrefix := "job-run:" + row.ID
	var legacyOriginCount int64
	if err := tx.Model(&ToolInvocation{}).
		Where("user_id = ? AND origin_type = 'job' AND origin_id = ?", state.UserID, row.JobID).
		Count(&legacyOriginCount).Error; err != nil {
		return toolLedgerBackfillReport{}, err
	}
	if legacyOriginCount > 0 {
		var runCount int64
		if err := tx.Model(&JobRun{}).
			Where("user_id = ? AND job_id = ?", state.UserID, row.JobID).
			Count(&runCount).Error; err != nil {
			return toolLedgerBackfillReport{}, err
		}
		if runCount > 1 {
			return toolLedgerBackfillReport{
				LegacyRows:          1,
				AmbiguousCount:      1,
				LastErrorCode:       "ambiguous_legacy_job_origin",
				LegacyHighWatermark: &row.UpdatedAt,
			}, nil
		}
	}
	var existing []ToolInvocation
	if err := tx.Where(
		"user_id = ? AND ((origin_type = 'job_run' AND origin_id = ?) OR (origin_type = 'job' AND origin_id = ?))",
		state.UserID, row.ID, row.JobID,
	).Order("queued_at, id").Find(&existing).Error; err != nil {
		return toolLedgerBackfillReport{}, err
	}
	name := strings.TrimSpace(row.ToolName)
	if name == "" {
		name = strings.TrimSpace(row.DefinitionToolName)
	}
	catalogID := strings.TrimSpace(row.DefinitionToolCatalogID)
	if catalogID == "" {
		var err error
		catalogID, err = resolveOrCreateArchivalCatalog(tx, state.UserID, name)
		if err != nil {
			return toolLedgerBackfillReport{}, err
		}
	}
	input := normalizeLegacyValue(row.Inputs)
	output := migratedOutput(row.Output)
	status := migratedJobStatus(row.Status)
	availability := "available"
	if strings.TrimSpace(row.Output) == "" {
		availability = "missing"
	}
	if len(existing) > 0 {
		report := toolLedgerBackfillReport{
			LegacyRows:          1,
			LedgerRows:          len(existing),
			LegacyHighWatermark: &row.UpdatedAt,
		}
		for index := range existing {
			target := &existing[index]
			canonicalInput := target.Input
			if strings.TrimSpace(canonicalInput) == "" {
				canonicalInput = input
			}
			canonicalOutput := target.Output
			if strings.TrimSpace(canonicalOutput) == "" && index == len(existing)-1 {
				canonicalOutput = output
			}
			canonicalAvailability := "missing"
			if strings.TrimSpace(canonicalOutput) != "" {
				canonicalAvailability = "available"
			}
			sourceKey := fmt.Sprintf("%s:%d", sourceKeyPrefix, index+1)
			err := withoutPayloadLogging(tx).Model(target).Updates(map[string]any{
				"origin_type":          "job_run",
				"origin_id":            row.ID,
				"attempt":              index + 1,
				"status":               migratedJobInvocationStatus(target.Status, status, index == len(existing)-1),
				"dry_run":              row.IsDryRun,
				"display_name":         name,
				"input":                canonicalInput,
				"output":               canonicalOutput,
				"input_preview":        structuralPreview(canonicalInput),
				"output_preview":       structuralPreview(canonicalOutput),
				"input_bytes":          len([]byte(canonicalInput)),
				"output_bytes":         len([]byte(canonicalOutput)),
				"input_hash":           digestValue(canonicalInput),
				"output_hash":          digestValue(canonicalOutput),
				"result_availability":  canonicalAvailability,
				"migration_source_key": sourceKey,
				"migration_provenance": toolLedgerMigrationProvenance,
			}).Error
			if err != nil {
				return toolLedgerBackfillReport{}, err
			}
			if index == len(existing)-1 {
				report.LegacyInputHashes = append(report.LegacyInputHashes, digestValue(input))
				report.LegacyOutputHashes = append(report.LegacyOutputHashes, digestValue(row.Output))
				report.LedgerInputHashes = append(report.LedgerInputHashes, digestValue(canonicalInput))
				report.LedgerOutputHashes = append(report.LedgerOutputHashes, digestCanonicalOutput(canonicalOutput))
			}
			report.LastProcessedKey = sourceKey
		}
		return report, nil
	}
	sourceKey := sourceKeyPrefix + ":1"
	startedAt := row.StartedAt
	invocation := ToolInvocation{
		UserID:              state.UserID,
		ToolCatalogID:       catalogID,
		OriginType:          "job_run",
		OriginID:            row.ID,
		Attempt:             maxInt(1, row.RetryCount+1),
		Status:              status,
		DryRun:              row.IsDryRun,
		Input:               input,
		Output:              output,
		DisplayName:         name,
		InputPreview:        structuralPreview(input),
		OutputPreview:       structuralPreview(row.Output),
		InputBytes:          int64(len([]byte(input))),
		OutputBytes:         int64(len([]byte(output))),
		InputHash:           digestValue(input),
		OutputHash:          digestValue(output),
		ResultAvailability:  availability,
		MigrationSourceKey:  sourceKey,
		MigrationProvenance: toolLedgerMigrationProvenance,
		QueuedAt:            startedAt,
		StartedAt:           &startedAt,
		CompletedAt:         row.CompletedAt,
		DurationMs:          row.DurationMs,
		ErrorMessage:        row.Error,
	}
	if err := withoutPayloadLogging(tx).Create(&invocation).Error; err != nil {
		return toolLedgerBackfillReport{}, err
	}
	return toolLedgerBackfillReport{
		LegacyRows:          1,
		LedgerRows:          1,
		LegacyInputHashes:   []string{digestValue(input)},
		LegacyOutputHashes:  []string{digestValue(row.Output)},
		LedgerInputHashes:   []string{digestValue(invocation.Input)},
		LedgerOutputHashes:  []string{digestCanonicalOutput(invocation.Output)},
		LastProcessedKey:    sourceKey,
		LegacyHighWatermark: &row.UpdatedAt,
	}, nil
}

func resolveOrCreateArchivalCatalog(tx *gorm.DB, userID, name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "legacy_tool"
	}
	var catalog ToolCatalog
	result := tx.Where("name = ? AND user_id = ?", name, userID).Limit(1).Find(&catalog)
	if result.Error != nil {
		return "", result.Error
	}
	if result.RowsAffected == 0 {
		result = tx.Where(
			"name = ? AND origin = 'builtin' AND (user_id IS NULL OR user_id = '')",
			name,
		).Limit(1).Find(&catalog)
	}
	if result.Error != nil {
		return "", result.Error
	}
	if result.RowsAffected > 0 {
		return catalog.ID, nil
	}
	owner := userID
	catalog = ToolCatalog{
		UserID:             &owner,
		Name:               name,
		DisplayName:        name,
		Origin:             toolLedgerArchiveOrigin,
		AvailabilityStatus: toolLedgerArchiveUnavailable,
		AvailabilityReason: "historical_backfill",
	}
	if err := tx.Create(&catalog).Error; err != nil {
		if retryErr := tx.Where("user_id = ? AND name = ? AND origin = ?", userID, name, toolLedgerArchiveOrigin).First(&catalog).Error; retryErr != nil {
			return "", err
		}
	}
	return catalog.ID, nil
}

func legacyToolCallsEmpty(raw string) bool {
	switch strings.TrimSpace(raw) {
	case "", "[]", "null":
		return true
	default:
		return false
	}
}

func messageTurnID(message ChatMessage) string {
	if message.TurnID != nil && strings.TrimSpace(*message.TurnID) != "" {
		return strings.TrimSpace(*message.TurnID)
	}
	return message.ID
}

func legacyTurnCallKey(turnID, callID string) string {
	return strings.TrimSpace(turnID) + "\x00" + strings.TrimSpace(callID)
}

func normalizeLegacyValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	var compact bytes.Buffer
	if json.Compact(&compact, []byte(value)) == nil {
		return compact.String()
	}
	return value
}

func migratedOutput(content string) string {
	if strings.TrimSpace(content) == "" {
		return ""
	}
	encoded, _ := json.Marshal(map[string]any{
		"content":  content,
		"is_error": false,
	})
	return string(encoded)
}

func structuralPreview(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	preview := map[string]any{"bytes": len([]byte(value))}
	var object map[string]json.RawMessage
	if json.Unmarshal([]byte(value), &object) == nil {
		keys := make([]string, 0, len(object))
		for key := range object {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		preview["fields"] = keys
	}
	encoded, _ := json.Marshal(preview)
	return string(encoded)
}

func digestValue(value string) string {
	sum := sha256.Sum256([]byte(normalizeLegacyValue(value)))
	return hex.EncodeToString(sum[:])
}

func digestCanonicalOutput(value string) string {
	var envelope struct {
		Content string `json:"content"`
	}
	if json.Unmarshal([]byte(value), &envelope) == nil && envelope.Content != "" {
		return digestValue(envelope.Content)
	}
	return digestValue(value)
}

func digestHashes(hashes []string) string {
	sort.Strings(hashes)
	return digestValue(strings.Join(hashes, "\n"))
}

func migratedJobStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "completed", "succeeded", "success":
		return "succeeded"
	case "cancelled", "canceled":
		return "cancelled"
	case "timed_out", "timeout":
		return "timed_out"
	case "running":
		return "running"
	case "queued", "pending":
		return "queued"
	default:
		return "failed"
	}
}

func migratedJobInvocationStatus(existing, final string, latest bool) string {
	if latest {
		return final
	}
	status := migratedJobStatus(existing)
	if status == "queued" || status == "running" {
		return "failed"
	}
	return status
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}

func laterTime(current *time.Time, candidate time.Time) *time.Time {
	if candidate.IsZero() {
		return current
	}
	if current == nil || candidate.After(*current) {
		value := candidate
		return &value
	}
	return current
}

func withoutPayloadLogging(database *gorm.DB) *gorm.DB {
	return database.Session(&gorm.Session{Logger: logger.Discard})
}
