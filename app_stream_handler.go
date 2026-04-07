package main

import (
	"errors"
	"fmt"
	"log"

	"assistente/internal/chat"
	"assistente/internal/llm"
)

// appStreamHandler implementa llm.StreamHandler usando *App
// NOVA ARQUITETURA v2: Hierarquia baseada na mensagem do usuário
// - n0: user/assistant (parentID=null)
// - n1: interações com agentes (parentID=userMessageID)
// - n2: interações do agente com tools (parentID=agentMessageID)
type appStreamHandler struct {
	baseStreamHandler
	app           *App
	userMessageID uint // ID da mensagem do usuário (raiz da thread)
}

func (h *appStreamHandler) OnError(err string) {
	h.mu.Lock()
	h.cancelPendingChunkTimer()
	content := h.accumulatedContent
	h.mu.Unlock()

	h.emitter.Emit("chat:stream", StreamEvent{
		Content:        content,
		Done:           true,
		Error:          err,
		ConversationId: h.conversationID,
	})
}

// OnToolCalls é chamado quando o LLM solicita execução de ferramentas.
// Por enquanto (antes do agentic loop), loga e emite como done.
// Será substituído pelo agentic loop na Fase 5.
func (h *appStreamHandler) OnToolCalls(calls []llm.ToolCall, fullResponse string, usage llm.Usage, model string) {
	fmt.Printf("🔧 [TOOL_CALLS] LLM solicitou %d ferramentas (ainda não implementado)\n", len(calls))
	for _, call := range calls {
		fmt.Printf("   - %s(%s)\n", call.Function.Name, call.Function.Arguments)
	}
	// Delega para OnDone até o agentic loop ser implementado
	h.OnDone(fullResponse, usage, model)
}

func (h *appStreamHandler) OnMCPToolEvent(event llm.MCPToolEvent) {
	if event.IsCompleted {
		log.Printf("[MCP Native] ✅ %s (server=%s, id=%s): %d bytes output",
			event.Name, event.ServerLabel, event.ID, len(event.Output))
	} else {
		log.Printf("[MCP Native] 🔧 %s (server=%s, id=%s)",
			event.Name, event.ServerLabel, event.ID)
	}
}

func (h *appStreamHandler) OnDone(fullResponse string, usage llm.Usage, model string) {
	h.mu.Lock()
	h.cancelPendingChunkTimer()
	accumulatedContent := h.accumulatedContent
	accumulatedReasoning := h.accumulatedReasoning
	h.mu.Unlock()

	finalContent := fullResponse
	if finalContent == "" {
		finalContent = accumulatedContent
	}

	// Salva resposta final do assistant no nível 0 (sem parentID)
	// Inclui reasoning se houver
	savedMsgID, err := chat.SaveAssistantMessage(h.app.msgRepo, chat.MessageOptions{
		ConversationID:   h.conversationID,
		Role:             "assistant",
		Content:          finalContent,
		Reasoning:        accumulatedReasoning,
		PromptTokens:     usage.PromptTokens,
		CompletionTokens: usage.CompletionTokens,
		TotalTokens:      usage.TotalTokens,
		Model:            model,
	})
	if errors.Is(err, chat.ErrConversationGone) {
		return
	}
	if err != nil {
		fmt.Printf("❌ Erro ao salvar resposta do assistant: %v\n", err)
	} else if savedMsgID > 0 {
		if accumulatedReasoning != "" {
			fmt.Printf("✅ Resposta do assistant salva: ID=%d (nível 0) com %d chars de reasoning\n", savedMsgID, len(accumulatedReasoning))
		} else {
			fmt.Printf("✅ Resposta do assistant salva: ID=%d (nível 0)\n", savedMsgID)
		}
	}

	// Notifica o gateway de mensageria (se há callbacks pendentes para esta conversa)
	if h.app.responseNotifier != nil {
		h.app.responseNotifier.Notify(h.conversationID, finalContent, savedMsgID)
	}

	// Emite evento final de streaming
	h.emitter.Emit("chat:stream", StreamEvent{
		Content:        finalContent,
		Done:           true,
		ConversationId: h.conversationID,
		FullResponse:   finalContent,
	})

	// Emite evento para frontend recarregar a conversa
	h.emitter.Emit("chat:done", map[string]interface{}{
		"conversationId": h.conversationID,
	})

	// Verifica uso do contexto e emite aviso se necessário
	h.checkAndEmitContextWarning()

	// Verifica se precisa sumarizar (após resposta concluída, não bloqueia nada)
	go func() {
		defer h.app.recoverFromPanic(h.conversationID, "checkAndTriggerSummarization")
		h.app.checkAndTriggerSummarization(h.conversationID)
	}()
}

// checkAndEmitContextWarning verifica se a conversa está próxima do limite de contexto
// e emite um evento de aviso para o frontend
func (h *appStreamHandler) checkAndEmitContextWarning() {
	if h.conversationID == 0 {
		return
	}

	stats, err := h.app.GetConversationTokenStats(h.conversationID)
	if err != nil {
		return
	}

	if stats.ContextLimit == 0 {
		return
	}

	h.emitter.Emit("chat:token_stats", map[string]interface{}{
		"conversationId":   h.conversationID,
		"totalTokens":      stats.TotalTokens,
		"contextLimit":     stats.ContextLimit,
		"contextUsage":     stats.ContextUsage,
		"isNearLimit":      stats.IsNearLimit,
		"isCritical":       stats.IsCritical,
		"promptTokens":     stats.PromptTokens,
		"completionTokens": stats.CompletionTokens,
		"messageCount":     stats.MessageCount,
	})

	if stats.IsCritical {
		h.emitter.Emit("chat:context_warning", map[string]interface{}{
			"conversationId": h.conversationID,
			"level":          "critical",
			"message": fmt.Sprintf("Atenção: Contexto em %0.1f%% (%d/%d tokens). Considere limpar a conversa ou resumir o histórico.",
				stats.ContextUsage, stats.TotalTokens, stats.ContextLimit),
			"percentage":   stats.ContextUsage,
			"totalTokens":  stats.TotalTokens,
			"contextLimit": stats.ContextLimit,
		})
		fmt.Printf("⚠️  [CONTEXT] Conversa %d em nível CRÍTICO: %0.1f%% (%d/%d tokens)\n",
			h.conversationID, stats.ContextUsage, stats.TotalTokens, stats.ContextLimit)
	} else if stats.IsNearLimit {
		h.emitter.Emit("chat:context_warning", map[string]interface{}{
			"conversationId": h.conversationID,
			"level":          "warning",
			"message": fmt.Sprintf("Contexto em %0.1f%% (%d/%d tokens). Considere limpar a conversa em breve.",
				stats.ContextUsage, stats.TotalTokens, stats.ContextLimit),
			"percentage":   stats.ContextUsage,
			"totalTokens":  stats.TotalTokens,
			"contextLimit": stats.ContextLimit,
		})
		fmt.Printf("⚠️  [CONTEXT] Conversa %d próxima do limite: %0.1f%% (%d/%d tokens)\n",
			h.conversationID, stats.ContextUsage, stats.TotalTokens, stats.ContextLimit)
	}
}
