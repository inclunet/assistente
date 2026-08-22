package chat

import (
	"assistente/internal/logging"
	"context"
	"encoding/json"
	"strings"
)

// HistoryLoader carrega e filtra o histórico de mensagens de uma conversa.
// Responsabilidades: buscar resumo, buscar mensagens, filtrar por resumo, truncar por
// limite de contexto, limpar tool calls órfãos e garantir que a primeira mensagem é do usuário.
// A conversão para formato LLM (mídia, Whisper) fica a cargo do chamador.
type HistoryLoader struct {
	Repo    MessageRepository
	MaxMsgs int
}

type historyToolCall struct {
	ID     string  `json:"id"`
	Result *string `json:"result,omitempty"`
}

func parseHistoryToolCalls(raw string) (calls []historyToolCall, raws []json.RawMessage, wasArray bool, ok bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil, false, false
	}

	// Primeiro tenta como array.
	var arrRaw []json.RawMessage
	var arrCalls []historyToolCall
	if json.Unmarshal([]byte(raw), &arrRaw) == nil && json.Unmarshal([]byte(raw), &arrCalls) == nil {
		if len(arrRaw) == len(arrCalls) && len(arrCalls) > 0 {
			return arrCalls, arrRaw, true, true
		}
	}

	// Fallback: objeto único.
	var singleRaw json.RawMessage
	var singleCall historyToolCall
	if json.Unmarshal([]byte(raw), &singleRaw) == nil && json.Unmarshal([]byte(raw), &singleCall) == nil {
		if strings.TrimSpace(singleCall.ID) == "" {
			return nil, nil, false, false
		}
		return []historyToolCall{singleCall}, []json.RawMessage{singleRaw}, false, true
	}

	return nil, nil, false, false
}

// Load retorna as mensagens filtradas e o resumo da conversa.
// Os mensagens retornadas estão prontas para conversão ao formato LLM.
func (h *HistoryLoader) Load(ctx context.Context, conversationID string) ([]Message, string, error) {
	existingSummary, summaryUpToID, err := h.Repo.GetConversationSummary(ctx, conversationID)
	if err != nil {
		logging.Errorf(ctx, "chat.history", "[HISTORY] Erro ao buscar resumo da conversa %s: %v", conversationID, err)
		existingSummary = ""
		summaryUpToID = ""
	}

	allRootMessages, err := h.Repo.GetMessages(ctx, conversationID, nil)
	if err != nil {
		return nil, "", err
	}

	// Filtra mensagens para o contexto: apenas as que vêm depois do resumo.
	// Usa índice na lista (já ordenada por created_at ASC) em vez de comparação
	// lexicográfica de IDs, evitando problemas com UUIDs gerados no mesmo ms.
	var dbMessages []Message
	if summaryUpToID != "" {
		cutIdx := -1
		for i, m := range allRootMessages {
			if m.ID == summaryUpToID {
				cutIdx = i
				break
			}
		}
		if cutIdx >= 0 && cutIdx+1 < len(allRootMessages) {
			dbMessages = allRootMessages[cutIdx+1:]
		} else if cutIdx < 0 {
			// summaryUpToID não encontrado (mensagem deletada?): descartar resumo
			// para evitar duplicação (resumo + mensagens já resumidas no prompt).
			existingSummary = ""
			dbMessages = allRootMessages
		}
		// cutIdx == last index → nenhuma mensagem depois do resumo
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
			if tcs, _, _, ok := parseHistoryToolCalls(m.ToolCalls); ok {
				for _, tc := range tcs {
					id := strings.TrimSpace(tc.ID)
					if id == "" {
						continue
					}
					offeredIDs[id] = true
					if tc.Result != nil && strings.TrimSpace(*tc.Result) != "" {
						answeredIDs[id] = true
					}
				}
			}
		}
		if m.Role == "tool" && m.ToolCallID != "" {
			answeredIDs[m.ToolCallID] = true
		}
	}

	// Pass 2: remove tool_results e tool_calls órfãos.
	cleaned := make([]Message, 0, len(dbMessages))
	for _, m := range dbMessages {
		if m.Role == "tool" && m.ToolCallID != "" && !offeredIDs[m.ToolCallID] {
			logging.Infof(ctx, "chat.history", "[History] removendo tool_result órfão: %s (conversa %s)", m.ToolCallID, conversationID)
			continue
		}
		if m.ToolCalls != "" {
			if tcsParsed, tcsRaw, wasArray, ok := parseHistoryToolCalls(m.ToolCalls); ok {
				var kept []json.RawMessage
				for i, tc := range tcsParsed {
					id := strings.TrimSpace(tc.ID)
					if id == "" {
						continue
					}
					if answeredIDs[id] {
						kept = append(kept, tcsRaw[i])
					} else {
						logging.Infof(ctx, "chat.history", "[History] removendo tool_use órfão: %s (conversa %s)", id, conversationID)
					}
				}
				if len(kept) == 0 {
					m.ToolCalls = ""
				} else if wasArray {
					if len(kept) < len(tcsRaw) {
						if j, err := json.Marshal(kept); err == nil {
							m.ToolCalls = string(j)
						}
					}
				} else {
					// Objeto único: mantém como objeto.
					m.ToolCalls = string(kept[0])
				}
			}
		}
		if m.Role == "assistant" && strings.TrimSpace(m.Content) == "" && strings.TrimSpace(m.ToolCalls) == "" {
			// Evita manter placeholders de tool calling sem conteúdo após limpeza,
			// que seriam enviados ao LLM como mensagens vazias.
			// Reasoning, porém, não é vazio para o protocolo: o DeepSeek exige
			// reasoning_content no replay de requests que carregam tools.
			if strings.TrimSpace(m.Reasoning) == "" && strings.TrimSpace(m.Media) == "" && strings.TrimSpace(m.Audio) == "" {
				continue
			}
		}
		cleaned = append(cleaned, m)
	}

	return cleaned, existingSummary, nil
}
