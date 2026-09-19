package toolinvocations

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"assistente/internal/database"
	"assistente/internal/tools"
)

type ChatToolInvocationDisplay struct {
	LedgerID           string
	ID                 string
	Type               string
	Name               string
	Arguments          string
	Result             string
	ModelResult        string `json:"-"`
	Origin             string
	ServerLabel        string
	Iteration          int
	DurationMs         int64
	AssistantMessageID string
	CreatedAt          time.Time
}

type toolInvocationDisplayMetadata struct {
	Display struct {
		Version            int    `json:"version,omitempty"`
		Type               string `json:"type,omitempty"`
		Name               string `json:"name,omitempty"`
		Arguments          string `json:"arguments,omitempty"`
		Origin             string `json:"origin,omitempty"`
		ServerLabel        string `json:"server_label,omitempty"`
		Iteration          int    `json:"iteration,omitempty"`
		DurationMs         int64  `json:"duration_ms,omitempty"`
		AssistantMessageID string `json:"assistant_message_id,omitempty"`
	} `json:"display,omitempty"`
	External bool `json:"external,omitempty"`
}

// LoadChatToolInvocationResultsForTurnIDsWithUser carrega outputs de tool_invocations
// para turns de chat, organizados como turnID -> callID -> content.
//
// Observação: retornos são best-effort; o chamador decide log/propagação.
func LoadChatToolInvocationResultsForTurnIDsWithUser(ctx context.Context, userID string, turnIDs []string) (map[string]map[string]string, error) {
	displays, err := LoadChatToolInvocationDisplaysForTurnIDsWithUser(ctx, userID, turnIDs)
	if err != nil {
		return nil, err
	}
	results := make(map[string]map[string]string, len(displays))
	for turnID, calls := range displays {
		for _, call := range calls {
			callID := strings.TrimSpace(call.ID)
			if callID == "" {
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
			byCall[callID] = call.ModelResult
		}
	}
	return results, nil
}

// LoadChatToolInvocationDisplaysForTurnIDsWithUser carrega o snapshot exibível de
// tool_invocations para turns de chat, organizado como turnID -> chamadas.
func LoadChatToolInvocationDisplaysForTurnIDsWithUser(ctx context.Context, userID string, turnIDs []string) (map[string][]ChatToolInvocationDisplay, error) {
	if len(turnIDs) == 0 {
		return map[string][]ChatToolInvocationDisplay{}, nil
	}

	db := database.DB()
	if db == nil {
		// Best-effort: em alguns cenários (ex.: testes com repos mockados) o DB pode não estar inicializado.
		return map[string][]ChatToolInvocationDisplay{}, nil
	}
	if !db.Migrator().HasTable(&database.ToolInvocation{}) {
		return map[string][]ChatToolInvocationDisplay{}, nil
	}

	// SQLite tem limite de variáveis (tipicamente 999).
	const maxTurnIDsPerBatch = 400
	const pageSize = 2000

	results := make(map[string][]ChatToolInvocationDisplay, len(turnIDs))
	indexByTurnCall := make(map[string]map[string]int, len(turnIDs))
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
	for start := 0; start < len(turnIDs); start += maxTurnIDsPerBatch {
		end := start + maxTurnIDsPerBatch
		if end > len(turnIDs) {
			end = len(turnIDs)
		}
		batch := turnIDs[start:end]

		attemptByTurnCall := make(map[string]map[string]int)
		var cursorQueuedAt *time.Time
		cursorID := ""
		for {
			type invocationDisplayRow struct {
				database.ToolInvocation
				ToolName        string
				ToolDisplayName string
				ToolOrigin      string
				ResolvedTurnID  string `gorm:"column:resolved_turn_id"`
			}
			q := db.WithContext(ctx).
				Model(&database.ToolInvocation{}).
				Select("tool_invocations.id, tool_invocations.created_at, tool_invocations.origin_id, tool_invocations.tool_call_id, tool_invocations.attempt, tool_invocations.output, tool_invocations.metadata, tool_invocations.queued_at, tool_invocations.duration_ms, tool_catalog.name AS tool_name, tool_catalog.display_name AS tool_display_name, tool_catalog.origin AS tool_origin, "+resolvedTurnSQL+" AS resolved_turn_id").
				Joins("LEFT JOIN tool_catalog ON tool_catalog.id = tool_invocations.tool_catalog_id").
				Where(
					"tool_invocations.user_id = ? AND tool_invocations.origin_type = ? AND "+resolvedTurnSQL+" IN ? AND TRIM(tool_invocations.tool_call_id) <> '' AND (tool_invocations.completed_at IS NOT NULL OR tool_invocations.status IN (?, ?, ?, ?, ?, ?))",
					userID,
					OriginChat,
					batch,
					StatusQueued,
					StatusRunning,
					StatusSucceeded,
					StatusFailed,
					StatusCancelled,
					StatusTimedOut,
				)
			if cursorQueuedAt != nil {
				q = q.Where("(tool_invocations.queued_at > ?) OR (tool_invocations.queued_at = ? AND tool_invocations.id > ?)", *cursorQueuedAt, *cursorQueuedAt, cursorID)
			}

			var rows []invocationDisplayRow
			err := q.Order("tool_invocations.queued_at ASC, tool_invocations.id ASC").Limit(pageSize).Find(&rows).Error
			if err != nil {
				return nil, fmt.Errorf("erro ao buscar tool invocations: %w", err)
			}
			if len(rows) == 0 {
				break
			}

			for _, row := range rows {
				turnID := strings.TrimSpace(row.ResolvedTurnID)
				callID := strings.TrimSpace(row.ToolCallID)
				if turnID == "" || callID == "" {
					continue
				}
				indexByCall := indexByTurnCall[turnID]
				if indexByCall == nil {
					indexByCall = map[string]int{}
					indexByTurnCall[turnID] = indexByCall
					attemptByTurnCall[turnID] = map[string]int{}
				}
				display := toolInvocationRowToDisplay(row.ToolInvocation, row.ToolName, row.ToolDisplayName, row.ToolOrigin)
				if idx, ok := indexByCall[callID]; ok {
					// Attempt é autoritativo para retries; em empate (dados legados),
					// a ordem cronológica mantém a linha mais recente.
					if row.Attempt >= attemptByTurnCall[turnID][callID] {
						results[turnID][idx] = display
						attemptByTurnCall[turnID][callID] = row.Attempt
					}
					continue
				}
				indexByCall[callID] = len(results[turnID])
				attemptByTurnCall[turnID][callID] = row.Attempt
				results[turnID] = append(results[turnID], display)
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

func toolInvocationRowToDisplay(row database.ToolInvocation, toolName, toolDisplayName, toolOrigin string) ChatToolInvocationDisplay {
	var meta toolInvocationDisplayMetadata
	_ = json.Unmarshal([]byte(strings.TrimSpace(row.Metadata)), &meta)

	tipo := strings.TrimSpace(meta.Display.Type)
	if tipo == "" {
		tipo = "function"
	}
	name := strings.TrimSpace(meta.Display.Name)
	if name == "" {
		name = strings.TrimSpace(toolDisplayName)
	}
	if name == "" {
		name = strings.TrimSpace(toolName)
	}
	if name == "" {
		name = "tool_result"
	}
	origin := strings.TrimSpace(meta.Display.Origin)
	if origin == "" {
		origin = strings.TrimSpace(toolOrigin)
	}
	durationMs := meta.Display.DurationMs
	if durationMs == 0 {
		durationMs = row.DurationMs
	}

	result := ExtractToolInvocationResult(row.Output)
	return ChatToolInvocationDisplay{
		LedgerID:           row.ID,
		ID:                 strings.TrimSpace(row.ToolCallID),
		Type:               tipo,
		Name:               name,
		Arguments:          meta.Display.Arguments,
		Result:             result.Content,
		ModelResult:        tools.ContentForModel(result),
		Origin:             origin,
		ServerLabel:        meta.Display.ServerLabel,
		Iteration:          meta.Display.Iteration,
		DurationMs:         durationMs,
		AssistantMessageID: strings.TrimSpace(meta.Display.AssistantMessageID),
		CreatedAt:          row.CreatedAt,
	}
}

func ExtractToolInvocationContent(raw string) string {
	return ExtractToolInvocationResult(raw).Content
}

const persistenceOmissionSentinel = "0"

func ExtractToolInvocationResult(raw string) tools.ToolResult {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return tools.ToolResult{}
	}
	if raw == persistenceOmissionSentinel {
		return tools.ToolResult{
			Content:  "[result_omitted_for_persistence]",
			IsError:  true,
			Metadata: map[string]any{"omitted_for_persistence": true},
			Failure: &tools.ToolFailure{
				Code: "result_omitted_for_persistence", Kind: tools.ErrorKindUnknown, Retryable: false,
			},
		}
	}
	var payload tools.ToolResult
	if json.Unmarshal([]byte(raw), &payload) == nil {
		return payload
	}
	return tools.ToolResult{Content: raw}
}
