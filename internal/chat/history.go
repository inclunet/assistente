package chat

import (
	"encoding/json"
	"log"

	"assistente/internal/database"
)

// HistoryLoader carrega e filtra o histórico de mensagens de uma conversa.
// Responsabilidades: buscar resumo, buscar mensagens, filtrar por resumo, truncar por
// limite de contexto, limpar tool calls órfãos e garantir que a primeira mensagem é do usuário.
// A conversão para formato LLM (mídia, Whisper) fica a cargo do chamador.
type HistoryLoader struct {
	Repo    MessageRepository
	MaxMsgs int
}

// Load retorna as mensagens filtradas e o resumo da conversa.
// Os mensagens retornadas estão prontas para conversão ao formato LLM.
func (h *HistoryLoader) Load(conversationID uint) ([]database.ChatMessage, string, error) {
	existingSummary, summaryUpToID, err := h.Repo.GetConversationSummary(conversationID)
	if err != nil {
		log.Printf("[HISTORY] Erro ao buscar resumo da conversa %d: %v", conversationID, err)
		existingSummary = ""
		summaryUpToID = 0
	}

	allRootMessages, err := h.Repo.GetMessages(conversationID, nil)
	if err != nil {
		return nil, "", err
	}

	// Filtra mensagens para o contexto: apenas as que vêm depois do resumo
	var dbMessages []database.ChatMessage
	if summaryUpToID > 0 {
		for _, m := range allRootMessages {
			if m.ID > summaryUpToID {
				dbMessages = append(dbMessages, m)
			}
		}
	} else {
		dbMessages = allRootMessages
	}

	total := len(dbMessages)

	// Truncação por limite de mensagens no contexto (MaxMsgs).
	// Corta no limite de uma mensagem role="user", preservando turns completos.
	if total > h.MaxMsgs {
		cutIndex := -1
		for i := total - 1; i >= 2; i-- {
			if dbMessages[i].Role == "user" {
				msgCount := 2 + (total - i)
				if msgCount > h.MaxMsgs {
					break
				}
				cutIndex = i
			}
		}

		if cutIndex > 2 {
			dbMessages = append(dbMessages[:2], dbMessages[cutIndex:]...)
		} else {
			kept := h.MaxMsgs - 2
			if kept > total {
				kept = total
			}
			dbMessages = append(dbMessages[:2], dbMessages[total-kept:]...)
		}
	} else {
		// total <= MaxMsgs — use all in context
	}

	// Garante que a primeira mensagem no contexto é uma user message
	for len(dbMessages) > 0 && dbMessages[0].Role != "user" {
		dbMessages = dbMessages[1:]
	}

	// Safety net: verifica que todo tool_use tem seu tool_result e vice-versa.
	offeredIDs := make(map[string]bool)
	answeredIDs := make(map[string]bool)
	for _, m := range dbMessages {
		if m.ToolCalls != "" {
			var tcs []struct {
				ID string `json:"id"`
			}
			if json.Unmarshal([]byte(m.ToolCalls), &tcs) == nil {
				for _, tc := range tcs {
					offeredIDs[tc.ID] = true
				}
			}
		}
		if m.Role == "tool" && m.ToolCallID != "" {
			answeredIDs[m.ToolCallID] = true
		}
	}

	// Pass 2: remove tool_results e tool_calls órfãos.
	cleaned := make([]database.ChatMessage, 0, len(dbMessages))
	for _, m := range dbMessages {
		if m.Role == "tool" && m.ToolCallID != "" && !offeredIDs[m.ToolCallID] {
			log.Printf("[History] removendo tool_result órfão: %s (conversa %d)", m.ToolCallID, conversationID)
			continue
		}
		if m.ToolCalls != "" {
			var tcs []json.RawMessage
			var tcsParsed []struct {
				ID string `json:"id"`
			}
			if json.Unmarshal([]byte(m.ToolCalls), &tcs) == nil && json.Unmarshal([]byte(m.ToolCalls), &tcsParsed) == nil {
				var kept []json.RawMessage
				for i, tc := range tcsParsed {
					if answeredIDs[tc.ID] {
						kept = append(kept, tcs[i])
					} else {
						log.Printf("[History] removendo tool_use órfão: %s (conversa %d)", tc.ID, conversationID)
					}
				}
				if len(kept) == 0 {
					m.ToolCalls = ""
				} else if len(kept) < len(tcs) {
					if j, err := json.Marshal(kept); err == nil {
						m.ToolCalls = string(j)
					}
				}
			}
		}
		cleaned = append(cleaned, m)
	}

	return cleaned, existingSummary, nil
}
