package chat

import (
	"assistente/internal/logging"
	"context"
	"errors"
	"fmt"
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

func (h *HistoryLoader) maxMessages() int {
	if h.MaxMsgs > 0 {
		return max(h.MaxMsgs, 2)
	}
	return DefaultMaxContextMessages
}

// Load retorna as mensagens filtradas e o resumo da conversa.
// Os mensagens retornadas estão prontas para conversão ao formato LLM.
func (h *HistoryLoader) Load(ctx context.Context, conversationID string) ([]Message, string, error) {
	if windowRepo, ok := h.Repo.(HistoryWindowRepository); ok {
		window, err := windowRepo.LoadHistoryWindow(ctx, conversationID, h.maxMessages())
		if err != nil {
			return nil, "", err
		}
		if window == nil {
			return nil, "", errors.New("janela de histórico indisponível")
		}
		summary := window.Summary
		if window.SummaryUpToMessageID != "" && !window.SummaryBoundaryAvailable {
			summary = ""
		}
		return h.filter(ctx, conversationID, window.Messages, summary)
	}

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

	return h.filter(ctx, conversationID, dbMessages, existingSummary)
}

// LoadThroughMessage carrega somente a janela até a mensagem raiz selecionada.
// É usado por retry para que mensagens e resumo posteriores à pergunta escolhida
// não contaminem o novo turno.
func (h *HistoryLoader) LoadThroughMessage(ctx context.Context, conversationID, messageID string) ([]Message, string, error) {
	if h.Repo == nil {
		return nil, "", errors.New("repositório de mensagens indisponível")
	}
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

	selectedIndex := -1
	for index, message := range allRootMessages {
		if message.ID == messageID {
			selectedIndex = index
			if message.ParentID != nil {
				return nil, "", fmt.Errorf("mensagem %s não é uma pergunta raiz", messageID)
			}
			break
		}
	}
	if selectedIndex < 0 {
		return nil, "", fmt.Errorf("mensagem %s não encontrada no histórico da conversa", messageID)
	}

	// Um resumo que alcança a pergunta selecionada já pode conter mensagens
	// posteriores ao ponto de retry. Descartá-lo evita duplicação e vazamento de
	// contexto posterior; o próprio histórico ancorado é a fonte de verdade.
	if summaryUpToID != "" {
		summaryIndex := -1
		for index, message := range allRootMessages {
			if message.ID == summaryUpToID {
				summaryIndex = index
				break
			}
		}
		switch {
		case summaryIndex < 0:
			existingSummary = ""
		case summaryIndex < selectedIndex:
			allRootMessages = allRootMessages[summaryIndex+1 : selectedIndex+1]
			return h.filter(ctx, conversationID, allRootMessages, existingSummary)
		default:
			existingSummary = ""
		}
	} else {
		existingSummary = ""
	}

	return h.filter(ctx, conversationID, allRootMessages[:selectedIndex+1], existingSummary)
}

// filter preserva a semântica histórica de truncamento e limpeza. Tanto o
// caminho legado quanto a janela batch passam por esta única implementação.
func (h *HistoryLoader) filter(ctx context.Context, conversationID string, dbMessages []Message, existingSummary string) ([]Message, string, error) {
	total := len(dbMessages)
	maxMessages := h.maxMessages()

	// Truncação por limite de mensagens no contexto (MaxMsgs).
	// Corta no limite de uma mensagem role="user", preservando turns completos.
	if total > maxMessages {
		cutIndex := -1
		for i := total - 1; i >= 2; i-- {
			if dbMessages[i].Role == "user" {
				msgCount := 2 + (total - i)
				if msgCount > maxMessages {
					break
				}
				cutIndex = i
			}
		}

		if cutIndex > 2 {
			dbMessages = append(dbMessages[:2], dbMessages[cutIndex:]...)
		} else {
			kept := maxMessages - 2
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

	cleaned := make([]Message, 0, len(dbMessages))
	for _, m := range dbMessages {
		if m.Role == "assistant" && strings.TrimSpace(m.Content) == "" {
			// Evita manter placeholders de tool calling sem conteúdo após limpeza,
			// que seriam enviados ao LLM como mensagens vazias. Reasoning
			// persistido serve à UI/exportação; a extensão de protocolo só vive
			// no agentic loop corrente (AEP-0097).
			if strings.TrimSpace(m.Media) == "" && strings.TrimSpace(m.Audio) == "" {
				continue
			}
		}
		cleaned = append(cleaned, m)
	}

	return cleaned, existingSummary, nil
}
