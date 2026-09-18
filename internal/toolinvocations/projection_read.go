package toolinvocations

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"assistente/internal/database"
)

const MaxDetailBatchSize = 100

type Summary struct {
	InvocationID       string `json:"invocationId"`
	CallID             string `json:"callId"`
	Name               string `json:"name"`
	Origin             string `json:"origin,omitempty"`
	ServerLabel        string `json:"serverLabel,omitempty"`
	Status             string `json:"status"`
	Iteration          int    `json:"iteration,omitempty"`
	DurationMs         int64  `json:"durationMs,omitempty"`
	InputPreview       string `json:"inputPreview,omitempty"`
	OutputPreview      string `json:"outputPreview,omitempty"`
	InputBytes         int64  `json:"inputBytes,omitempty"`
	OutputBytes        int64  `json:"outputBytes,omitempty"`
	HasDetails         bool   `json:"hasDetails"`
	ResultAvailability string `json:"resultAvailability"`
	HasSearchResults   bool   `json:"hasSearchResults,omitempty"`
	SearchResultCount  int    `json:"searchResultCount,omitempty"`
	SecurityOutcome    string `json:"securityOutcome,omitempty"`
	AssistantMessageID string `json:"-"`
}

type Detail struct {
	InvocationID       string     `json:"invocationId"`
	CallID             string     `json:"callId"`
	Name               string     `json:"name"`
	DisplayName        string     `json:"displayName,omitempty"`
	Origin             string     `json:"origin,omitempty"`
	Status             string     `json:"status"`
	Attempt            int        `json:"attempt"`
	DryRun             bool       `json:"dryRun"`
	Input              string     `json:"input,omitempty"`
	Output             string     `json:"output,omitempty"`
	Metadata           string     `json:"metadata,omitempty"`
	InputBytes         int64      `json:"inputBytes,omitempty"`
	OutputBytes        int64      `json:"outputBytes,omitempty"`
	InputHash          string     `json:"inputHash,omitempty"`
	OutputHash         string     `json:"outputHash,omitempty"`
	ResultAvailability string     `json:"resultAvailability"`
	ErrorKind          string     `json:"errorKind,omitempty"`
	ErrorCode          string     `json:"errorCode,omitempty"`
	ErrorMessage       string     `json:"errorMessage,omitempty"`
	Retryable          bool       `json:"retryable"`
	RetryabilityKnown  bool       `json:"retryabilityKnown"`
	QueuedAt           time.Time  `json:"queuedAt"`
	StartedAt          *time.Time `json:"startedAt,omitempty"`
	CompletedAt        *time.Time `json:"completedAt,omitempty"`
	DurationMs         int64      `json:"durationMs,omitempty"`
}

type projectionMetadata struct {
	Display struct {
		Name               string `json:"name"`
		Origin             string `json:"origin"`
		ServerLabel        string `json:"server_label"`
		Iteration          int    `json:"iteration"`
		AssistantMessageID string `json:"assistant_message_id"`
	} `json:"display"`
}

func LoadSummariesForTurnIDsWithUser(ctx context.Context, userID string, turnIDs []string) (map[string][]Summary, error) {
	userID = strings.TrimSpace(userID)
	turnIDs = uniqueNonEmptyIDs(turnIDs)
	result := make(map[string][]Summary, len(turnIDs))
	if userID == "" {
		return nil, database.ErrUserScopeRequired
	}
	if len(turnIDs) == 0 {
		return result, nil
	}
	db := database.DB()
	if db == nil || !db.Migrator().HasTable(&database.ToolInvocation{}) {
		return result, nil
	}
	resolvedTurnSQL := "tool_invocations.origin_id"
	if db.Migrator().HasTable(&database.ChatMessage{}) && db.Migrator().HasTable(&database.Conversation{}) {
		resolvedTurnSQL = `COALESCE(
			tool_invocations.turn_id,
			(
				SELECT COALESCE(legacy_message.turn_id, legacy_message.id)
				FROM chat_messages legacy_message
				JOIN conversations legacy_conversation
					ON legacy_conversation.id = legacy_message.conversation_id
					AND legacy_conversation.user_id = tool_invocations.user_id
				WHERE legacy_message.id = tool_invocations.origin_id
				LIMIT 1
			),
			tool_invocations.origin_id
		)`
	}
	type row struct {
		ID                   string
		ToolCallID           string
		Status               string
		DisplayName          string
		MetadataName         string
		MetadataOrigin       string
		MetadataServer       string
		MetadataIteration    int
		SearchResultsVersion int
		SearchResultCount    int
		SecurityOutcome      string
		AssistantMessageID   string
		InputPreview         string
		OutputPreview        string
		InputBytes           int64
		OutputBytes          int64
		ResultAvailability   string
		DurationMs           int64
		QueuedAt             time.Time
		ToolName             string
		ToolDisplayName      string
		ToolOrigin           string
		ResolvedTurnID       string `gorm:"column:resolved_turn_id"`
	}
	const batchSize = 400
	started := time.Now()
	queryCount := uint64(0)
	projectionBytes := uint64(0)
	for start := 0; start < len(turnIDs); start += batchSize {
		end := start + batchSize
		if end > len(turnIDs) {
			end = len(turnIDs)
		}
		var rows []row
		queryCount++
		if err := db.WithContext(ctx).
			Model(&database.ToolInvocation{}).
			Select(
				"tool_invocations.id, tool_invocations.tool_call_id, tool_invocations.status, "+
					"tool_invocations.display_name, "+
					"CASE WHEN json_valid(tool_invocations.metadata) THEN COALESCE(CAST(json_extract(tool_invocations.metadata, '$.display.name') AS TEXT), '') ELSE '' END AS metadata_name, "+
					"CASE WHEN json_valid(tool_invocations.metadata) THEN COALESCE(CAST(json_extract(tool_invocations.metadata, '$.display.origin') AS TEXT), '') ELSE '' END AS metadata_origin, "+
					"CASE WHEN json_valid(tool_invocations.metadata) THEN COALESCE(CAST(json_extract(tool_invocations.metadata, '$.display.server_label') AS TEXT), '') ELSE '' END AS metadata_server, "+
					"CASE WHEN json_valid(tool_invocations.metadata) THEN CAST(COALESCE(json_extract(tool_invocations.metadata, '$.display.iteration'), 0) AS INTEGER) ELSE 0 END AS metadata_iteration, "+
					"CASE WHEN json_valid(tool_invocations.metadata) THEN CAST(COALESCE(json_extract(tool_invocations.metadata, '$.search_result_presentation.version'), 0) AS INTEGER) ELSE 0 END AS search_results_version, "+
					"CASE WHEN json_valid(tool_invocations.metadata) THEN CAST(COALESCE(json_extract(tool_invocations.metadata, '$.search_result_presentation.total'), 0) AS INTEGER) ELSE 0 END AS search_result_count, "+
					"CASE WHEN json_valid(tool_invocations.metadata) THEN COALESCE(CAST(json_extract(tool_invocations.metadata, '$.security_signals[0].outcome') AS TEXT), '') ELSE '' END AS security_outcome, "+
					"CASE WHEN json_valid(tool_invocations.metadata) THEN COALESCE(CAST(json_extract(tool_invocations.metadata, '$.display.assistant_message_id') AS TEXT), '') ELSE '' END AS assistant_message_id, "+
					"tool_invocations.input_preview, tool_invocations.output_preview, "+
					"tool_invocations.input_bytes, tool_invocations.output_bytes, tool_invocations.result_availability, "+
					"tool_invocations.duration_ms, tool_invocations.queued_at, "+
					"tool_catalog.name AS tool_name, tool_catalog.display_name AS tool_display_name, "+
					"tool_catalog.origin AS tool_origin, "+resolvedTurnSQL+" AS resolved_turn_id",
			).
			Joins("LEFT JOIN tool_catalog ON tool_catalog.id = tool_invocations.tool_catalog_id").
			Where(
				"tool_invocations.user_id = ? AND tool_invocations.origin_type = ? AND "+resolvedTurnSQL+" IN ? AND TRIM(tool_invocations.tool_call_id) <> ''",
				userID,
				OriginChat,
				turnIDs[start:end],
			).
			Order("resolved_turn_id, tool_invocations.queued_at, tool_invocations.id").
			Find(&rows).Error; err != nil {
			return nil, fmt.Errorf("erro ao buscar resumos de tool invocations: %w", err)
		}
		for _, item := range rows {
			name := projectionFirstNonEmpty(item.MetadataName, item.DisplayName, item.ToolDisplayName, item.ToolName, "tool")
			origin := projectionFirstNonEmpty(item.MetadataOrigin, item.ToolOrigin)
			availability := strings.TrimSpace(item.ResultAvailability)
			if availability == "" {
				availability = "available"
			}
			result[item.ResolvedTurnID] = append(result[item.ResolvedTurnID], Summary{
				InvocationID:       item.ID,
				CallID:             item.ToolCallID,
				Name:               name,
				Origin:             origin,
				ServerLabel:        item.MetadataServer,
				Status:             item.Status,
				Iteration:          item.MetadataIteration,
				DurationMs:         item.DurationMs,
				InputPreview:       item.InputPreview,
				OutputPreview:      item.OutputPreview,
				InputBytes:         item.InputBytes,
				OutputBytes:        item.OutputBytes,
				HasDetails:         item.InputBytes > 0 || item.OutputBytes > 0 || availability == "available",
				ResultAvailability: availability,
				HasSearchResults:   item.SearchResultsVersion == 1,
				SearchResultCount:  item.SearchResultCount,
				SecurityOutcome:    item.SecurityOutcome,
				AssistantMessageID: item.AssistantMessageID,
			})
			projectionBytes += uint64(len(item.ID) + len(item.ToolCallID) + len(name) + len(origin) +
				len(item.InputPreview) + len(item.OutputPreview) + len(availability))
		}
	}
	RuntimeMetrics().ObserveTimeline(queryCount, projectionBytes, uint64(time.Since(started)))
	return result, nil
}

func LoadDetailsWithUser(ctx context.Context, userID string, invocationIDs []string) ([]Detail, error) {
	userID = strings.TrimSpace(userID)
	if len(invocationIDs) > MaxDetailBatchSize {
		return nil, fmt.Errorf("no máximo %d invocationIds por lote", MaxDetailBatchSize)
	}
	ids := uniqueNonEmptyIDs(invocationIDs)
	if userID == "" {
		return nil, database.ErrUserScopeRequired
	}
	if len(ids) == 0 {
		return []Detail{}, nil
	}
	for _, id := range ids {
		if len(id) > 128 {
			return nil, fmt.Errorf("invocationId excede 128 bytes")
		}
	}
	db := database.DB()
	if db == nil || !db.Migrator().HasTable(&database.ToolInvocation{}) {
		return []Detail{}, nil
	}
	type row struct {
		database.ToolInvocation
		ToolName        string
		ToolDisplayName string
		ToolOrigin      string
	}
	var rows []row
	started := time.Now()
	query := db.WithContext(ctx).
		Model(&database.ToolInvocation{}).
		Select("tool_invocations.*, tool_catalog.name AS tool_name, tool_catalog.display_name AS tool_display_name, tool_catalog.origin AS tool_origin").
		Joins("LEFT JOIN tool_catalog ON tool_catalog.id = tool_invocations.tool_catalog_id").
		Where("tool_invocations.user_id = ? AND tool_invocations.id IN ?", userID, ids)
	if db.Migrator().HasTable(&database.Conversation{}) && db.Migrator().HasTable(&database.ChatMessage{}) {
		query = query.Where(`(
			tool_invocations.origin_type <> ?
			OR EXISTS (
				SELECT 1
				FROM conversations owned_conversation
				WHERE owned_conversation.id = tool_invocations.conversation_id
					AND owned_conversation.user_id = tool_invocations.user_id
			)
			OR (
				tool_invocations.conversation_id IS NULL
				AND EXISTS (
					SELECT 1
					FROM chat_messages legacy_message
					JOIN conversations legacy_conversation
						ON legacy_conversation.id = legacy_message.conversation_id
						AND legacy_conversation.user_id = tool_invocations.user_id
					WHERE legacy_message.id = tool_invocations.origin_id
				)
			)
		)`, OriginChat)
	} else {
		query = query.Where("tool_invocations.origin_type <> ?", OriginChat)
	}
	if db.Migrator().HasTable(&database.JobRun{}) {
		query = query.Where(`(
			tool_invocations.origin_type <> ?
			OR EXISTS (
				SELECT 1
				FROM job_runs owned_run
				WHERE owned_run.id = tool_invocations.origin_id
					AND owned_run.user_id = tool_invocations.user_id
			)
		)`, OriginJobRun)
	} else {
		query = query.Where("tool_invocations.origin_type <> ?", OriginJobRun)
	}
	if err := query.Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("erro ao buscar detalhes de tool invocations: %w", err)
	}
	byID := make(map[string]Detail, len(rows))
	for _, item := range rows {
		var metadata projectionMetadata
		_ = json.Unmarshal([]byte(item.Metadata), &metadata)
		byID[item.ID] = Detail{
			InvocationID:       item.ID,
			CallID:             item.ToolCallID,
			Name:               projectionFirstNonEmpty(metadata.Display.Name, item.DisplayName, item.ToolDisplayName, item.ToolName, "tool"),
			DisplayName:        projectionFirstNonEmpty(item.DisplayName, item.ToolDisplayName),
			Origin:             projectionFirstNonEmpty(metadata.Display.Origin, item.ToolOrigin),
			Status:             item.Status,
			Attempt:            item.Attempt,
			DryRun:             item.DryRun,
			Input:              item.Input,
			Output:             item.Output,
			Metadata:           item.Metadata,
			InputBytes:         item.InputBytes,
			OutputBytes:        item.OutputBytes,
			InputHash:          item.InputHash,
			OutputHash:         item.OutputHash,
			ResultAvailability: item.ResultAvailability,
			ErrorKind:          item.ErrorKind,
			ErrorCode:          item.ErrorCode,
			ErrorMessage:       item.ErrorMessage,
			Retryable:          item.Retryable,
			RetryabilityKnown:  item.RetryabilityKnown,
			QueuedAt:           item.QueuedAt,
			StartedAt:          item.StartedAt,
			CompletedAt:        item.CompletedAt,
			DurationMs:         item.DurationMs,
		}
	}
	result := make([]Detail, 0, len(rows))
	for _, id := range ids {
		if item, ok := byID[id]; ok {
			result = append(result, item)
		}
	}
	RuntimeMetrics().ObserveDetails(uint64(len(ids)), uint64(time.Since(started)))
	return result, nil
}

func uniqueNonEmptyIDs(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func projectionFirstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}
