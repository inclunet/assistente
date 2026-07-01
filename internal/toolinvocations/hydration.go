package toolinvocations

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"assistente/internal/database"
)

type ChatToolInvocationDisplay struct {
	ID          string
	Type        string
	Name        string
	Arguments   string
	Result      string
	Origin      string
	ServerLabel string
	Iteration   int
	DurationMs  int64
}

type toolInvocationDisplayMetadata struct {
	Display struct {
		Version     int    `json:"version,omitempty"`
		Type        string `json:"type,omitempty"`
		Name        string `json:"name,omitempty"`
		Arguments   string `json:"arguments,omitempty"`
		Origin      string `json:"origin,omitempty"`
		ServerLabel string `json:"server_label,omitempty"`
		Iteration   int    `json:"iteration,omitempty"`
		DurationMs  int64  `json:"duration_ms,omitempty"`
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
			byCall[callID] = call.Result
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
	for start := 0; start < len(turnIDs); start += maxTurnIDsPerBatch {
		end := start + maxTurnIDsPerBatch
		if end > len(turnIDs) {
			end = len(turnIDs)
		}
		batch := turnIDs[start:end]

		var cursorQueuedAt *time.Time
		cursorID := ""
		for {
			type invocationDisplayRow struct {
				database.ToolInvocation
				ToolName        string
				ToolDisplayName string
				ToolOrigin      string
			}
			q := db.WithContext(ctx).
				Model(&database.ToolInvocation{}).
				Select("tool_invocations.*, tool_catalog.name AS tool_name, tool_catalog.display_name AS tool_display_name, tool_catalog.origin AS tool_origin").
				Joins("LEFT JOIN tool_catalog ON tool_catalog.id = tool_invocations.tool_catalog_id").
				Where(
					"tool_invocations.user_id = ? AND tool_invocations.origin_type = ? AND tool_invocations.origin_id IN ? AND tool_invocations.tool_call_id <> '' AND (tool_invocations.completed_at IS NOT NULL OR tool_invocations.status IN (?, ?, ?, ?))",
					userID,
					OriginChat,
					batch,
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
				turnID := strings.TrimSpace(row.OriginID)
				callID := strings.TrimSpace(row.ToolCallID)
				if turnID == "" || callID == "" {
					continue
				}
				indexByCall := indexByTurnCall[turnID]
				if indexByCall == nil {
					indexByCall = map[string]int{}
					indexByTurnCall[turnID] = indexByCall
				}
				display := toolInvocationRowToDisplay(row.ToolInvocation, row.ToolName, row.ToolDisplayName, row.ToolOrigin)
				if idx, ok := indexByCall[callID]; ok {
					// A consulta vem em ordem cronológica; retries com o mesmo tool_call_id
					// substituem a tentativa anterior para expor o resultado mais recente.
					results[turnID][idx] = display
					continue
				}
				indexByCall[callID] = len(results[turnID])
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

	return ChatToolInvocationDisplay{
		ID:          strings.TrimSpace(row.ToolCallID),
		Type:        tipo,
		Name:        name,
		Arguments:   meta.Display.Arguments,
		Result:      ExtractToolInvocationContent(row.Output),
		Origin:      origin,
		ServerLabel: meta.Display.ServerLabel,
		Iteration:   meta.Display.Iteration,
		DurationMs:  durationMs,
	}
}

func ExtractToolInvocationContent(raw string) string {
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
