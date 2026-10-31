package toolinvocations

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"assistente/internal/database"
	"assistente/internal/tools"
)

type ChatToolInvocationDisplay struct {
	ExecutionID        string `json:"-"`
	ExecutionDepth     int    `json:"-"`
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

// LoadChatToolInvocationDisplaysForTurnIDsWithUser carrega o snapshot exibível de
// tool_invocations para turns de chat, organizado como turnID -> chamadas.
func LoadChatToolInvocationDisplaysForTurnIDsWithUser(ctx context.Context, userID string, turnIDs []string) (map[string][]ChatToolInvocationDisplay, error) {
	return loadChatToolInvocationDisplays(ctx, userID, turnIDs, false, nil)
}

// LoadChatToolInvocationDisplaysForExecutionWithUser mantém o escopo da mensagem
// histórica solicitada. nil identifica legado sem atribuição, nunca a atual.
func LoadChatToolInvocationDisplaysForExecutionWithUser(ctx context.Context, userID, turnID string, executionID *string) ([]ChatToolInvocationDisplay, error) {
	result, err := loadChatToolInvocationDisplays(ctx, userID, []string{turnID}, true, executionID)
	return result[turnID], err
}

func loadChatToolInvocationDisplays(ctx context.Context, userID string, turnIDs []string, historical bool, executionID *string) (map[string][]ChatToolInvocationDisplay, error) {
	selection := database.ChatInvocationSelection{Historical: historical}

	turnIDs = uniqueNonEmptyIDs(turnIDs)
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
	indexByTurnCall := make(map[string]map[invocationCallKey]int, len(turnIDs))
	const resolvedTurnSQL = "tool_invocations.turn_id"
	for start := 0; start < len(turnIDs); start += maxTurnIDsPerBatch {
		end := start + maxTurnIDsPerBatch
		if end > len(turnIDs) {
			end = len(turnIDs)
		}
		batch := turnIDs[start:end]

		attemptByTurnCall := make(map[string]map[invocationCallKey]int)
		var cursorQueuedAt *time.Time
		cursorID := ""
		cursorTurnID := ""
		for {
			type invocationDisplayRow struct {
				database.ToolInvocation
				ExecutionID           string
				ExecutionDepth        int
				PresentationMessageID string
				ToolName              string
				ToolDisplayName       string
				ToolOrigin            string
				ResolvedTurnID        string `gorm:"column:resolved_turn_id"`
			}
			q := db.WithContext(ctx).
				Model(&database.ToolInvocation{}).
				Select("tool_invocations.chat_execution_id AS execution_id, "+selection.Depth("tool_invocations")+" AS execution_depth, "+selection.PresentationMessage("tool_invocations", assistantMessageSQL)+" AS presentation_message_id, tool_invocations.id, tool_invocations.created_at, tool_invocations.origin_id, tool_invocations.tool_call_id, tool_invocations.attempt, tool_invocations.output, tool_invocations.error_message, tool_invocations.metadata, tool_invocations.queued_at, tool_invocations.duration_ms, tool_catalog.name AS tool_name, tool_catalog.display_name AS tool_display_name, tool_catalog.origin AS tool_origin, "+resolvedTurnSQL+" AS resolved_turn_id").
				Joins("LEFT JOIN tool_catalog ON tool_catalog.id = tool_invocations.tool_catalog_id").
				Joins(selection.Join("tool_invocations")).
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
			if historical {
				q = q.Joins("JOIN (SELECT ? AS execution_id) requested_chat_execution ON 1=1", executionID)
			}
			q = q.Where(selection.Predicate("tool_invocations"))
			if cursorQueuedAt != nil {
				q = q.Where("(tool_invocations.turn_id, tool_invocations.queued_at, tool_invocations.id) > (?, ?, ?)", cursorTurnID, *cursorQueuedAt, cursorID)
			}

			var rows []invocationDisplayRow
			err := q.Order("tool_invocations.turn_id ASC, tool_invocations.queued_at ASC, tool_invocations.id ASC").Limit(pageSize).Find(&rows).Error
			if err != nil {
				return nil, fmt.Errorf("erro ao buscar tool invocations: %w", err)
			}
			if len(rows) == 0 {
				break
			}

			for _, row := range rows {
				turnID := row.ResolvedTurnID
				callID := row.ToolCallID
				if strings.TrimSpace(turnID) == "" || strings.TrimSpace(callID) == "" {
					continue
				}
				key := invocationCallKey{row.ExecutionID, callID}
				indexByCall := indexByTurnCall[turnID]
				if indexByCall == nil {
					indexByCall = map[invocationCallKey]int{}
					indexByTurnCall[turnID] = indexByCall
					attemptByTurnCall[turnID] = map[invocationCallKey]int{}
				}
				display := toolInvocationRowToDisplay(row.ToolInvocation, row.ToolName, row.ToolDisplayName, row.ToolOrigin)
				display.ExecutionID, display.ExecutionDepth = row.ExecutionID, row.ExecutionDepth
				display.AssistantMessageID = row.PresentationMessageID
				if idx, ok := indexByCall[key]; ok {
					// Attempt é autoritativo para retries; em empate (dados legados),
					// a ordem cronológica mantém a linha mais recente.
					if row.Attempt >= attemptByTurnCall[turnID][key] {
						results[turnID][idx] = display
						attemptByTurnCall[turnID][key] = row.Attempt
					}
					continue
				}
				indexByCall[key] = len(results[turnID])
				attemptByTurnCall[turnID][key] = row.Attempt
				results[turnID] = append(results[turnID], display)
			}

			last := rows[len(rows)-1]
			cursorQueuedAt = &last.QueuedAt
			cursorID = last.ID
			cursorTurnID = last.ResolvedTurnID
			if len(rows) < pageSize {
				break
			}
		}
	}
	for _, calls := range results {
		sort.SliceStable(calls, func(i, j int) bool { return calls[i].ExecutionDepth > calls[j].ExecutionDepth })
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
	modelResult := tools.ContentForModel(result)
	if result.IsError && row.ErrorMessage != "" && row.ErrorMessage != result.Content {
		// O ledger mantém saída e diagnóstico separados. A projeção para o
		// modelo precisa dos dois, sem substituir a saída original exibível.
		encoded, _ := json.Marshal(struct {
			Output string `json:"output"`
			Error  string `json:"error"`
		}{Output: modelResult, Error: row.ErrorMessage})
		modelResult = string(encoded)
	}
	return ChatToolInvocationDisplay{
		LedgerID:           row.ID,
		ID:                 row.ToolCallID,
		Type:               tipo,
		Name:               name,
		Arguments:          meta.Display.Arguments,
		Result:             result.Content,
		ModelResult:        modelResult,
		Origin:             origin,
		ServerLabel:        meta.Display.ServerLabel,
		Iteration:          meta.Display.Iteration,
		DurationMs:         durationMs,
		AssistantMessageID: meta.Display.AssistantMessageID,
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
