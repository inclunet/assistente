package toolinvocations

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"assistente/internal/database"
)

// LoadChatToolInvocationResultsForTurnIDsWithUser carrega outputs de tool_invocations
// para turns de chat, organizados como turnID -> callID -> content.
//
// Observação: retornos são best-effort; o chamador decide log/propagação.
func LoadChatToolInvocationResultsForTurnIDsWithUser(ctx context.Context, userID string, turnIDs []string) (map[string]map[string]string, error) {
	if len(turnIDs) == 0 {
		return map[string]map[string]string{}, nil
	}

	db := database.DB()
	if db == nil {
		// Best-effort: em alguns cenários (ex.: testes com repos mockados) o DB pode não estar inicializado.
		return map[string]map[string]string{}, nil
	}
	if !db.Migrator().HasTable(&database.ToolInvocation{}) {
		return map[string]map[string]string{}, nil
	}

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
			q := db.WithContext(ctx).
				Where(
					"user_id = ? AND origin_type = ? AND origin_id IN ? AND tool_call_id <> '' AND (completed_at IS NOT NULL OR status IN (?, ?, ?, ?))",
					userID,
					OriginChat,
					batch,
					StatusSucceeded,
					StatusFailed,
					StatusCancelled,
					StatusTimedOut,
				)
			if cursorQueuedAt != nil {
				q = q.Where("(queued_at < ?) OR (queued_at = ? AND id < ?)", *cursorQueuedAt, *cursorQueuedAt, cursorID)
			}

			var rows []database.ToolInvocation
			err := q.Order("queued_at DESC, id DESC").Limit(pageSize).Find(&rows).Error
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
				byCall := results[turnID]
				if byCall == nil {
					byCall = make(map[string]string)
					results[turnID] = byCall
				}
				// Mantém o primeiro (mais recente pela order) por turn/call.
				if _, ok := byCall[callID]; ok {
					continue
				}
				byCall[callID] = ExtractToolInvocationContent(row.Output)
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
