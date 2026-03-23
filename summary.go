package main

import (
	"context"
	"fmt"
	"log"
	"strings"

	"assistente/internal/config"
	"assistente/internal/database"
	"assistente/internal/llm"
	"assistente/internal/profiles"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

const (
	// Heurística: ~4 caracteres por token (aproximação para estimativa rápida)
	charsPerToken = 4

	// Fração da janela de contexto reservada como margem de segurança
	contextWindowSafetyMargin = 0.25
)

// SummaryPrompt é o prompt usado para gerar resumos de conversas
const SummaryPrompt = `You are a conversation summarizer. Your task is to create a concise but comprehensive summary of the conversation provided.

Rules:
- Preserve all key information: decisions, facts, user preferences, technical details, and action items
- If a previous summary is provided, integrate and extend it with the new messages (do not repeat what's already in the summary)
- Write in the same language as the conversation
- Be concise but don't lose important context
- Use bullet points for clarity
- Include any code snippets, file paths, or technical references that are relevant
- DO NOT add commentary or meta-text — output ONLY the summary`

// estimateTokens estimates the token count of a text using a chars/token heuristic
func estimateTokens(text string) int {
	if len(text) == 0 {
		return 0
	}
	return (len(text) + charsPerToken - 1) / charsPerToken
}

// estimateMessagesTokens estimates total tokens for a slice of database messages
func estimateMessagesTokens(messages []database.ChatMessage) int {
	total := 0
	for _, m := range messages {
		total += estimateTokens(m.Content)
		if m.ToolCalls != "" {
			total += estimateTokens(m.ToolCalls)
		}
	}
	return total
}

// shouldTriggerSummarization checks if the conversation needs summarization.
// Returns true if the estimated context usage exceeds the safe budget.
func shouldTriggerSummarization(
	profile *profiles.Profile,
	contextMessages []database.ChatMessage,
	existingSummary string,
) bool {
	if profile == nil || profile.Chat.ContextWindow <= 0 {
		return false
	}

	contextWindow := profile.Chat.ContextWindow
	maxTokens := profile.Chat.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 4096
	}

	// Budget = context_window - max_tokens (resposta) - 25% margem de segurança
	safetyMargin := int(float64(contextWindow) * contextWindowSafetyMargin)
	budget := contextWindow - maxTokens - safetyMargin
	if budget <= 0 {
		return false
	}

	estimated := estimateMessagesTokens(contextMessages)
	if existingSummary != "" {
		estimated += estimateTokens(existingSummary)
	}

	if estimated > budget {
		log.Printf("[Summary] Trigger: estimated %d tokens > budget %d (window=%d, maxTokens=%d, margin=%d)",
			estimated, budget, contextWindow, maxTokens, safetyMargin)
		return true
	}
	return false
}

// triggerSummarizationInBackground launches a goroutine to summarize old messages.
// It respects MinContextMessages: only messages beyond the min threshold are summarized.
func (a *App) triggerSummarizationInBackground(
	conversationID uint,
	profile *profiles.Profile,
	allRootMessages []database.ChatMessage,
) {
	inProgress, err := database.IsSummarizingInProgress(conversationID)
	if err != nil {
		log.Printf("[Summary] Erro ao verificar status: %v", err)
		return
	}
	if inProgress {
		log.Printf("[Summary] Sumarização já em andamento para conversa %d", conversationID)
		return
	}

	minKeep := profile.GetMinContextMessages()
	totalMessages := len(allRootMessages)

	if totalMessages <= minKeep {
		log.Printf("[Summary] Apenas %d mensagens, mínimo é %d — nada a sumarizar", totalMessages, minKeep)
		return
	}

	// Mensagens a sumarizar: tudo exceto as últimas minKeep
	// O ponto de corte deve cair em uma mensagem "user" para manter turnos completos
	cutIndex := totalMessages - minKeep
	for cutIndex > 0 && allRootMessages[cutIndex].Role != "user" {
		cutIndex--
	}
	if cutIndex <= 0 {
		log.Printf("[Summary] Não encontrou ponto de corte válido (user message) — abortando")
		return
	}

	messagesToSummarize := allRootMessages[:cutIndex]
	lastSummarizedMsgID := messagesToSummarize[len(messagesToSummarize)-1].ID

	// Busca resumo anterior
	existingSummary, currentUpToID, err := database.GetConversationSummary(conversationID)
	if err != nil {
		log.Printf("[Summary] Erro ao buscar resumo existente: %v", err)
		return
	}

	// Filtra apenas mensagens novas (que ainda não foram resumidas)
	var newMessages []database.ChatMessage
	for _, m := range messagesToSummarize {
		if m.ID > currentUpToID {
			newMessages = append(newMessages, m)
		}
	}
	if len(newMessages) == 0 {
		log.Printf("[Summary] Nenhuma mensagem nova para resumir (já resumido até ID %d)", currentUpToID)
		return
	}

	// Marca como em andamento
	if err := database.SetSummarizingInProgress(conversationID, true); err != nil {
		log.Printf("[Summary] Erro ao marcar summarizing_in_progress: %v", err)
		return
	}

	go func() {
		defer a.recoverFromPanic(conversationID, "executeSummarization")
		a.executeSummarization(conversationID, profile, existingSummary, newMessages, lastSummarizedMsgID)
	}()
}

// executeSummarization calls the LLM to generate a summary of the given messages
func (a *App) executeSummarization(
	conversationID uint,
	profile *profiles.Profile,
	existingSummary string,
	newMessages []database.ChatMessage,
	upToMessageID uint,
) {
	// Resolve sentinelas $default
	profile = a.resolveProfileDefaults(profile)

	// Notifica frontend: sumarização iniciou
	runtime.EventsEmit(a.ctx, "chat:summary_started", map[string]interface{}{
		"conversationId": conversationID,
		"messageCount":   len(newMessages),
	})

	defer func() {
		if err := database.SetSummarizingInProgress(conversationID, false); err != nil {
			log.Printf("[Summary] Erro ao desmarcar summarizing_in_progress: %v", err)
		}
	}()

	cfg, err := config.Load()
	if err != nil {
		log.Printf("[Summary] Erro ao carregar config: %v", err)
		runtime.EventsEmit(a.ctx, "chat:summary_error", map[string]interface{}{
			"conversationId": conversationID,
			"error":          "Erro ao carregar configuração",
		})
		return
	}

	model := profile.Chat.Model
	if model == "" {
		model = cfg.DefaultModel
	}

	userPrompt := buildSummarizationUserPrompt(existingSummary, newMessages)

	log.Printf("[Summary] Iniciando sumarização: conversa=%d, modelo=%s, %d mensagens novas, resumo anterior=%d chars",
		conversationID, model, len(newMessages), len(existingSummary))

	// Busca provider do perfil para criar client
	provider := a.llmRegistry.Get(profile.Chat.LLMProvider)
	if provider == nil {
		log.Printf("[Summary] Provider não encontrado: %s", profile.Chat.LLMProvider)
		runtime.EventsEmit(a.ctx, "chat:summary_error", map[string]interface{}{
			"conversationId": conversationID,
			"error":          "Provider não encontrado",
		})
		return
	}

	client := llm.NewSyncClient(provider, a.credMgr)
	summary, err := client.SimpleChat(context.Background(), model, SummaryPrompt, userPrompt)
	if err != nil {
		log.Printf("[Summary] Erro na chamada LLM: %v", err)
		runtime.EventsEmit(a.ctx, "chat:summary_error", map[string]interface{}{
			"conversationId": conversationID,
			"error":          fmt.Sprintf("Erro ao gerar resumo: %v", err),
		})
		return
	}

	summary = strings.TrimSpace(summary)
	if summary == "" {
		log.Printf("[Summary] LLM retornou resumo vazio — abortando")
		runtime.EventsEmit(a.ctx, "chat:summary_error", map[string]interface{}{
			"conversationId": conversationID,
			"error":          "Resumo gerado está vazio",
		})
		return
	}

	if err := database.UpdateConversationSummary(conversationID, summary, upToMessageID); err != nil {
		log.Printf("[Summary] Erro ao salvar resumo: %v", err)
		runtime.EventsEmit(a.ctx, "chat:summary_error", map[string]interface{}{
			"conversationId": conversationID,
			"error":          "Erro ao salvar resumo",
		})
		return
	}

	log.Printf("[Summary] Resumo salvo: conversa=%d, até msgID=%d, %d chars",
		conversationID, upToMessageID, len(summary))

	// Notifica frontend: sumarização concluída
	runtime.EventsEmit(a.ctx, "chat:summary_completed", map[string]interface{}{
		"conversationId":       conversationID,
		"summaryUpToMessageId": upToMessageID,
		"summaryLength":        len(summary),
		"messageCount":         len(newMessages),
	})
}

// buildSummarizationUserPrompt mounts the user message for the summarization LLM call
func buildSummarizationUserPrompt(existingSummary string, messages []database.ChatMessage) string {
	var sb strings.Builder

	if existingSummary != "" {
		sb.WriteString("## Previous Summary\n\n")
		sb.WriteString(existingSummary)
		sb.WriteString("\n\n---\n\n")
		sb.WriteString("## New Messages to Incorporate\n\n")
	} else {
		sb.WriteString("## Conversation to Summarize\n\n")
	}

	for _, m := range messages {
		sb.WriteString(fmt.Sprintf("**[%s]**: ", m.Role))
		content := m.Content
		if len(content) > 2000 {
			content = content[:2000] + "... [truncated]"
		}
		sb.WriteString(content)
		sb.WriteString("\n\n")
	}

	if existingSummary != "" {
		sb.WriteString("---\n\nPlease produce an updated summary that integrates the previous summary with the new messages above.")
	} else {
		sb.WriteString("---\n\nPlease produce a concise summary of the conversation above.")
	}

	return sb.String()
}

// checkAndTriggerSummarization verifica se a conversa precisa de sumarização e dispara em background.
// Deve ser chamado APÓS a resposta do LLM ser salva, para não atrasar a interação do usuário.
func (a *App) checkAndTriggerSummarization(conversationID uint) {
	if conversationID == 0 {
		return
	}

	profile, err := a.profileManager.GetActive()
	if err != nil || profile == nil {
		return
	}
	if profile.Chat.ContextWindow <= 0 {
		return
	}

	allRootMessages, err := database.GetMessages(conversationID, nil)
	if err != nil {
		log.Printf("[Summary] Erro ao carregar mensagens para check: %v", err)
		return
	}

	existingSummary, summaryUpToID, _ := database.GetConversationSummary(conversationID)

	var contextMessages []database.ChatMessage
	for _, m := range allRootMessages {
		if m.ID > summaryUpToID {
			contextMessages = append(contextMessages, m)
		}
	}

	if shouldTriggerSummarization(profile, contextMessages, existingSummary) {
		a.triggerSummarizationInBackground(conversationID, profile, allRootMessages)
	}
}
