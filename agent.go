package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"assistente/internal/config"
	"assistente/internal/database"
	"assistente/internal/llm"
	"assistente/internal/tools"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// ==================== Agentic Stream Handler ====================

// agenticResult captura o resultado de uma iteração do streaming LLM.
// Preenchido pelos callbacks OnDone/OnToolCalls/OnError.
type agenticResult struct {
	FullResponse    string
	Reasoning       string
	ToolCalls       []llm.ToolCall
	NativeMCPEvents []llm.MCPToolEvent // MCP tool calls executadas server-side (para persistência)
	Usage           llm.Usage
	Model           string
	Error           string
	IsDone          bool // true = finish_reason:"stop", false = finish_reason:"tool_calls"
}

// agenticStreamHandler implementa llm.StreamHandler para uso no agentic loop.
// Emite eventos de streaming para o frontend em tempo real (chat:stream, chat:thinking),
// mas NÃO salva no banco e NÃO emite chat:done — o loop controla isso.
type agenticStreamHandler struct {
	app            *App
	conversationID uint
	iteration      int // Número da iteração atual do loop

	// Acumuladores de conteúdo
	accumulatedContent   string
	accumulatedReasoning string
	isThinking           bool

	// Throttling para eventos de streaming
	mu            sync.Mutex
	lastEmitTime  time.Time
	throttleTimer *time.Timer
	pendingEmit   bool

	// Throttling para eventos de thinking
	lastThinkingEmitTime time.Time
	thinkingTimer        *time.Timer
	pendingThinkingEmit  bool

	// Resultado da iteração (preenchido por OnDone/OnToolCalls/OnError)
	result agenticResult

	// MCP tool events acumulados durante o streaming (para persistência)
	nativeMCPEvents []llm.MCPToolEvent
}

func (h *agenticStreamHandler) OnChunk(content string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.accumulatedContent += content

	const throttleInterval = 50 * time.Millisecond
	now := time.Now()

	if now.Sub(h.lastEmitTime) >= throttleInterval {
		h.emitStreamEvent()
		h.lastEmitTime = now
		h.pendingEmit = false
		if h.throttleTimer != nil {
			h.throttleTimer.Stop()
			h.throttleTimer = nil
		}
		return
	}

	if !h.pendingEmit {
		h.pendingEmit = true
		remainingTime := throttleInterval - now.Sub(h.lastEmitTime)
		h.throttleTimer = time.AfterFunc(remainingTime, func() {
			h.mu.Lock()
			defer h.mu.Unlock()
			if h.pendingEmit {
				h.emitStreamEvent()
				h.lastEmitTime = time.Now()
				h.pendingEmit = false
			}
		})
	}
}

func (h *agenticStreamHandler) emitStreamEvent() {
	runtime.EventsEmit(h.app.ctx, "chat:stream", StreamEvent{
		Content:        h.accumulatedContent,
		Done:           false,
		ConversationId: h.conversationID,
	})
}

func (h *agenticStreamHandler) OnThinking(content string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if !h.isThinking {
		h.isThinking = true
		runtime.EventsEmit(h.app.ctx, "chat:thinking", map[string]interface{}{
			"content":        content,
			"done":           false,
			"conversationId": h.conversationID,
			"started":        true,
		})
	}

	h.accumulatedReasoning += content

	const throttleInterval = 50 * time.Millisecond
	now := time.Now()

	if now.Sub(h.lastThinkingEmitTime) >= throttleInterval {
		h.emitThinkingEvent()
		h.lastThinkingEmitTime = now
		h.pendingThinkingEmit = false
		if h.thinkingTimer != nil {
			h.thinkingTimer.Stop()
			h.thinkingTimer = nil
		}
		return
	}

	if !h.pendingThinkingEmit {
		h.pendingThinkingEmit = true
		remainingTime := throttleInterval - now.Sub(h.lastThinkingEmitTime)
		h.thinkingTimer = time.AfterFunc(remainingTime, func() {
			h.mu.Lock()
			defer h.mu.Unlock()
			if h.pendingThinkingEmit {
				h.emitThinkingEvent()
				h.lastThinkingEmitTime = time.Now()
				h.pendingThinkingEmit = false
			}
		})
	}
}

func (h *agenticStreamHandler) emitThinkingEvent() {
	runtime.EventsEmit(h.app.ctx, "chat:thinking", map[string]interface{}{
		"content":        h.accumulatedReasoning,
		"done":           false,
		"conversationId": h.conversationID,
	})
}

func (h *agenticStreamHandler) OnThinkingDone(fullReasoning string) {
	h.mu.Lock()
	if h.thinkingTimer != nil {
		h.thinkingTimer.Stop()
		h.thinkingTimer = nil
	}
	h.pendingThinkingEmit = false
	h.isThinking = false
	h.mu.Unlock()

	runtime.EventsEmit(h.app.ctx, "chat:thinking", map[string]interface{}{
		"content":        fullReasoning,
		"done":           true,
		"conversationId": h.conversationID,
	})
}

func (h *agenticStreamHandler) OnToolCalls(calls []llm.ToolCall, fullResponse string, usage llm.Usage, model string) {
	h.mu.Lock()
	if h.throttleTimer != nil {
		h.throttleTimer.Stop()
		h.throttleTimer = nil
	}
	h.pendingEmit = false
	content := h.accumulatedContent
	reasoning := h.accumulatedReasoning
	mcpEvents := h.nativeMCPEvents
	h.nativeMCPEvents = nil
	h.mu.Unlock()

	finalContent := fullResponse
	if finalContent == "" {
		finalContent = content
	}

	h.result = agenticResult{
		FullResponse:    finalContent,
		Reasoning:       reasoning,
		ToolCalls:       calls,
		NativeMCPEvents: mcpEvents,
		Usage:           usage,
		Model:           model,
		IsDone:          false,
	}
}

func (h *agenticStreamHandler) OnMCPToolEvent(event llm.MCPToolEvent) {
	if event.IsCompleted {
		h.mu.Lock()
		h.nativeMCPEvents = append(h.nativeMCPEvents, event)
		h.mu.Unlock()

		status := "ok"
		errSummary := ""
		if event.Error != "" {
			status = "error"
			errSummary = truncateString(event.Error, 200)
		}
		outputSummary := truncateString(event.Output, 200)

		runtime.EventsEmit(h.app.ctx, "chat:tool_end", map[string]interface{}{
			"name":           event.Name,
			"callId":         event.ID,
			"status":         status,
			"summary":        outputSummary,
			"error":          errSummary,
			"serverLabel":    event.ServerLabel,
			"native":         true,
			"conversationId": h.conversationID,
		})

		log.Printf("[MCP Native] ✅ %s (server=%s, id=%s): %d bytes output",
			event.Name, event.ServerLabel, event.ID, len(event.Output))
	} else {
		runtime.EventsEmit(h.app.ctx, "chat:tool_start", map[string]interface{}{
			"name":           event.Name,
			"callId":         event.ID,
			"args":           event.Arguments,
			"serverLabel":    event.ServerLabel,
			"native":         true,
			"conversationId": h.conversationID,
		})

		log.Printf("[MCP Native] 🔧 %s (server=%s, id=%s)",
			event.Name, event.ServerLabel, event.ID)
	}
}

func (h *agenticStreamHandler) OnError(err string) {
	h.mu.Lock()
	if h.throttleTimer != nil {
		h.throttleTimer.Stop()
		h.throttleTimer = nil
	}
	h.pendingEmit = false
	h.mu.Unlock()

	h.result = agenticResult{
		Error: err,
	}
}

func (h *agenticStreamHandler) OnDone(fullResponse string, usage llm.Usage, model string) {
	h.mu.Lock()
	if h.throttleTimer != nil {
		h.throttleTimer.Stop()
		h.throttleTimer = nil
	}
	h.pendingEmit = false
	content := h.accumulatedContent
	reasoning := h.accumulatedReasoning
	mcpEvents := h.nativeMCPEvents
	h.nativeMCPEvents = nil
	h.mu.Unlock()

	finalContent := fullResponse
	if finalContent == "" {
		finalContent = content
	}

	h.result = agenticResult{
		FullResponse:    finalContent,
		Reasoning:       reasoning,
		NativeMCPEvents: mcpEvents,
		Usage:           usage,
		Model:           model,
		IsDone:          true,
	}
}

// ==================== Agentic Loop ====================

// runAgenticLoop executa o loop de tool calling.
// Em cada iteração:
//  1. Chama o LLM com streaming (texto aparece em tempo real)
//  2. Se finish_reason="stop" → salva resposta final, emite chat:done
//  3. Se finish_reason="tool_calls" → emite segment_done, executa tools, salva no banco, repete
func (a *App) runAgenticLoop(
	ctx context.Context,
	cfg *config.Config,
	messages []llm.Message,
	params llm.ChatParams,
	conversationID uint,
	turnID uint, // ID da mensagem do usuário (agrupa o turno)
	toolDefs []llm.ToolDefinition,
	streamer llm.Streamer, // ChatProvider (SDK) ou *Client (legado) — ambos satisfazem Streamer
) {
	if streamer == nil {
		errMsg := "Cliente LLM não disponível para o agentic loop. Verifique a configuração do provedor."
		log.Printf("🔴 [AGENT] streamer nil na conversa %d", conversationID)
		runtime.EventsEmit(a.ctx, "chat:stream", StreamEvent{
			Content:        "",
			Done:           true,
			Error:          errMsg,
			ConversationId: conversationID,
		})
		return
	}

	// Resolver maxIterations: usar valor do perfil (params.MaxAgenticIterations) ou fallback ao config
	maxIterations := params.MaxAgenticIterations
	if maxIterations <= 0 {
		maxIterations = a.toolExecutor.Config().MaxIterations // Fallback ao default (25)
	}

	// Caso especial: perfil do editor (somente text_edit).
	// Aqui queremos: 1) forçar que o modelo chame a tool na 1ª iteração, e
	// 2) terminar o loop logo após a execução do text_edit (não precisamos de "resposta final" do LLM).
	editorToolOnly := strings.TrimSpace(params.ProfileSlug) == "editor-texto" &&
		len(toolDefs) == 1 && strings.TrimSpace(toolDefs[0].Function.Name) == "text_edit"

	for iteration := 0; iteration < maxIterations; iteration++ {
		// Verifica cancelamento
		if ctx.Err() != nil {
			fmt.Printf("🛑 [AGENT] Loop cancelado na iteração %d\n", iteration)
			runtime.EventsEmit(a.ctx, "chat:stream", StreamEvent{
				Content:        "",
				Done:           true,
				Error:          "Operação cancelada",
				ConversationId: conversationID,
			})
			return
		}

		fmt.Printf("🔄 [AGENT] Iteração %d (conversa %d, turno %d)\n", iteration, conversationID, turnID)

		// 1. Cria handler para esta iteração
		handler := &agenticStreamHandler{
			app:            a,
			conversationID: conversationID,
			iteration:      iteration,
		}

		// 2. Chama o LLM (bloqueante — streaming emite eventos em tempo real)
		iterCtx := ctx
		if editorToolOnly && iteration == 0 {
			iterCtx = llm.WithToolChoice(iterCtx, "required")
		}
		streamer.StreamChat(iterCtx, messages, params, handler, toolDefs...)

		result := handler.result

		// 3. Erro?
		if result.Error != "" {
			fmt.Printf("❌ [AGENT] Erro na iteração %d: %s\n", iteration, result.Error)
			runtime.EventsEmit(a.ctx, "chat:stream", StreamEvent{
				Content:        result.FullResponse,
				Done:           true,
				Error:          result.Error,
				ConversationId: conversationID,
			})
			return
		}

		// 4. Emite segment_done para verbalização e acumulação de segmentos.
		// Emite sempre que houver mais iterações (!IsDone) para que o frontend
		// capture as tool calls completadas como segmentos — mesmo quando o LLM
		// chama ferramentas sem produzir texto intermediário.
		if result.FullResponse != "" || !result.IsDone {
			runtime.EventsEmit(a.ctx, "chat:segment_done", map[string]interface{}{
				"content":        result.FullResponse,
				"iteration":      iteration,
				"hasMore":        !result.IsDone,
				"conversationId": conversationID,
			})
		}

		// 5. Se finish_reason="stop" → resposta final
		if result.IsDone {
			a.saveAndFinish(conversationID, turnID, result)
			return
		}

		// 6. finish_reason="tool_calls" → executar ferramentas
		fmt.Printf("🔧 [AGENT] LLM pediu %d ferramentas na iteração %d\n", len(result.ToolCalls), iteration)

		// 6a. Se houve MCP calls nativas nesta iteração, persiste antes das bridge calls
		if len(result.NativeMCPEvents) > 0 {
			a.persistNativeMCPCalls(conversationID, turnID, result.NativeMCPEvents)
		}

		// 6b. Salva mensagem do assistant com bridge tool_calls no banco
		toolCallsJSON, _ := json.Marshal(result.ToolCalls)
		assistantMsg, err := database.AddAssistantToolMessage(
			conversationID,
			turnID,
			result.FullResponse,
			string(toolCallsJSON),
			result.Reasoning,
			result.Model,
		)
		if err != nil {
			if errors.Is(err, database.ErrConversationDeleted) {
				fmt.Printf("🛑 [AGENT] Conversa %d deletada — abortando\n", conversationID)
				return
			}
			fmt.Printf("❌ [AGENT] Erro ao salvar assistant com tool_calls: %v\n", err)
		} else {
			fmt.Printf("✅ [AGENT] Assistant com tool_calls salvo: ID=%d\n", assistantMsg.ID)
		}

		// 6c. Adiciona mensagem do assistant ao histórico de mensagens (para próxima iteração)
		assistantMessage := llm.Message{
			Role:      "assistant",
			Content:   result.FullResponse,
			ToolCalls: result.ToolCalls,
		}
		messages = append(messages, assistantMessage)

		// 6d. Executa ferramentas em paralelo
		toolCalls := convertToolCalls(result.ToolCalls)
		a.emitToolStarts(conversationID, result.ToolCalls)

		execResults := a.toolExecutor.ExecuteAll(ctx, toolCalls)

		// 6e. Salva resultados e adiciona ao histórico
		for _, execResult := range execResults {
			// Emite evento de conclusão da tool
			status := "ok"
			if execResult.Result.IsError {
				status = "error"
			}
			runtime.EventsEmit(a.ctx, "chat:tool_end", map[string]interface{}{
				"name":           execResult.ToolName,
				"callId":         execResult.CallID,
				"status":         status,
				"summary":        truncateString(execResult.Result.Content, 200),
				"conversationId": conversationID,
			})

			// Salva resultado no banco
			_, err := database.AddToolResultMessage(
				conversationID,
				turnID,
				execResult.Result.Content,
				execResult.CallID,
			)
			if err != nil {
				if errors.Is(err, database.ErrConversationDeleted) {
					fmt.Printf("🛑 [AGENT] Conversa %d deletada — abortando\n", conversationID)
					return
				}
				fmt.Printf("❌ [AGENT] Erro ao salvar resultado de tool %s: %v\n", execResult.ToolName, err)
			}

			// Adiciona ao histórico de mensagens para próxima iteração
			toolMessage := llm.Message{
				Role:       "tool",
				Content:    execResult.Result.Content,
				ToolCallID: execResult.CallID,
			}
			messages = append(messages, toolMessage)

			fmt.Printf("   ✅ %s (call_id=%s): %d bytes\n",
				execResult.ToolName, execResult.CallID, len(execResult.Result.Content))
		}

		// 6e. Emite evento de atualização de tokens em tempo real
		// Após a execução de cada tool, envia stats atualizadas para o frontend
		if stats, err := a.GetConversationTokenStats(conversationID); err == nil && stats != nil {
			// Converte TokenStatsResult para map[string]interface{} para emissão de eventos
			statsData := map[string]interface{}{
				"conversationId":              conversationID,
				"promptTokens":                stats.PromptTokens,
				"completionTokens":            stats.CompletionTokens,
				"totalTokens":                 stats.TotalTokens,
				"contextUsage":                stats.ContextUsage,
				"contextLimit":                stats.ContextLimit,
				"isNearLimit":                 stats.IsNearLimit,
				"isCritical":                  stats.IsCritical,
				"messageCount":                stats.MessageCount,
				"systemPromptEstimatedTokens": stats.SystemPromptEstimatedTokens,
				"summaryTokens":               stats.SummaryTokens,
				"messagesInContextCount":      stats.MessagesInContextCount,
				"messagesInContextTokens":     stats.MessagesInContextTokens,
				"toolsUsedCount":              stats.ToolsUsedCount,
				"toolBreakdown":               stats.ToolBreakdown,
			}
			runtime.EventsEmit(a.ctx, "chat:token_stats_update", statsData)
		}

		// 6f. Continua o loop → próxima iteração chama o LLM com os resultados
		if editorToolOnly {
			// Para o editor, o tool_result já é o resultado final do turno.
			// Emite eventos de conclusão e encerra sem rodada extra do LLM.
			runtime.EventsEmit(a.ctx, "chat:stream", StreamEvent{
				Content:        "",
				Done:           true,
				ConversationId: conversationID,
			})
			runtime.EventsEmit(a.ctx, "chat:done", map[string]interface{}{
				"conversationId": conversationID,
			})
			go func() {
				defer a.recoverFromPanic(conversationID, "checkAndTriggerSummarization")
				a.checkAndTriggerSummarization(conversationID)
			}()
			return
		}
	}

	// Atingiu limite de iterações
	fmt.Printf("⚠️ [AGENT] Limite de %d iterações atingido para conversa %d\n", maxIterations, conversationID)
	runtime.EventsEmit(a.ctx, "chat:stream", StreamEvent{
		Content:        "Limite de iterações do agente atingido. A resposta pode estar incompleta.",
		Done:           true,
		ConversationId: conversationID,
	})
	runtime.EventsEmit(a.ctx, "chat:done", map[string]interface{}{
		"conversationId": conversationID,
	})

	// Verifica se precisa sumarizar (após resposta concluída)
	go func() {
		defer a.recoverFromPanic(conversationID, "checkAndTriggerSummarization")
		a.checkAndTriggerSummarization(conversationID)
	}()
}

// saveAndFinish salva a resposta final do assistente e emite os eventos de conclusão.
// Se houve MCP tool calls nativas, salva no mesmo formato que bridge tool calls:
// assistant message com tool_calls + mensagens tool separadas com resultados.
func (a *App) saveAndFinish(conversationID, turnID uint, result agenticResult) {
	var savedMsgID uint
	if conversationID > 0 && result.FullResponse != "" {
		if len(result.NativeMCPEvents) > 0 && turnID > 0 {
			a.persistNativeMCPCalls(conversationID, turnID, result.NativeMCPEvents)
		}

		opts := database.MessageOptions{
			ConversationID:   conversationID,
			Role:             "assistant",
			Content:          result.FullResponse,
			Reasoning:        result.Reasoning,
			PromptTokens:     result.Usage.PromptTokens,
			CompletionTokens: result.Usage.CompletionTokens,
			TotalTokens:      result.Usage.TotalTokens,
			Model:            result.Model,
		}
		if turnID > 0 {
			opts.TurnID = &turnID
		}

		msg, err := database.CreateMessage(opts)
		if err != nil {
			if errors.Is(err, database.ErrConversationDeleted) || errors.Is(err, database.ErrParentMessageDeleted) {
				fmt.Printf("🛑 Conversa %d foi deletada/limpa — abortando\n", conversationID)
				return
			}
			fmt.Printf("❌ Erro ao salvar resposta final do assistant: %v\n", err)
		} else {
			fmt.Printf("✅ [AGENT] Resposta final salva: ID=%d\n", msg.ID)
			savedMsgID = msg.ID
		}
	}

	// Notifica o gateway de mensageria (se há callbacks pendentes para esta conversa)
	if a.responseNotifier != nil {
		a.responseNotifier.Notify(conversationID, result.FullResponse, savedMsgID)
	}

	// Emite evento final de streaming
	runtime.EventsEmit(a.ctx, "chat:stream", StreamEvent{
		Content:        result.FullResponse,
		Done:           true,
		ConversationId: conversationID,
		FullResponse:   result.FullResponse,
	})

	// Emite evento para frontend recarregar a conversa
	runtime.EventsEmit(a.ctx, "chat:done", map[string]interface{}{
		"conversationId": conversationID,
	})

	// Verifica se precisa sumarizar (após resposta concluída, não bloqueia nada)
	go func() {
		defer a.recoverFromPanic(conversationID, "checkAndTriggerSummarization")
		a.checkAndTriggerSummarization(conversationID)
	}()
}

// emitToolStarts emite eventos chat:tool_start para cada tool call.
func (a *App) emitToolStarts(conversationID uint, calls []llm.ToolCall) {
	for _, call := range calls {
		runtime.EventsEmit(a.ctx, "chat:tool_start", map[string]interface{}{
			"name":           call.Function.Name,
			"callId":         call.ID,
			"args":           call.Function.Arguments,
			"conversationId": conversationID,
		})
	}
}

// convertToolCalls converte llm.ToolCall para tools.ToolCall
func convertToolCalls(llmCalls []llm.ToolCall) []tools.ToolCall {
	result := make([]tools.ToolCall, len(llmCalls))
	for i, c := range llmCalls {
		result[i] = tools.ToolCall{
			ID:   c.ID,
			Type: c.Type,
			Function: tools.FunctionCall{
				Name:      c.Function.Name,
				Arguments: c.Function.Arguments,
			},
		}
	}
	return result
}

// persistNativeMCPCalls salva MCP tool calls nativas no banco no mesmo formato que bridge:
// 1. Uma mensagem assistant com tool_calls JSON (as chamadas feitas)
// 2. Mensagens tool separadas com resultados (uma por chamada)
// Isso permite que o MessageList.tsx consolide tudo automaticamente via turnID.
func (a *App) persistNativeMCPCalls(conversationID, turnID uint, events []llm.MCPToolEvent) {
	var toolCalls []llm.ToolCall
	for _, ev := range events {
		if !ev.IsCompleted {
			continue
		}
		name := ev.Name
		if ev.ServerLabel != "" {
			name = ev.ServerLabel + "/" + ev.Name
		}
		toolCalls = append(toolCalls, llm.ToolCall{
			ID:   ev.ID,
			Type: "function",
			Function: llm.FunctionCall{
				Name:      name,
				Arguments: ev.Arguments,
			},
		})
	}
	if len(toolCalls) == 0 {
		return
	}

	toolCallsJSON, err := json.Marshal(toolCalls)
	if err != nil {
		log.Printf("[MCP Native] Erro ao serializar tool calls: %v", err)
		return
	}

	_, err = database.AddAssistantToolMessage(conversationID, turnID, "", string(toolCallsJSON), "", "")
	if err != nil {
		log.Printf("[MCP Native] Erro ao salvar assistant tool_calls: %v", err)
		return
	}

	for _, ev := range events {
		if !ev.IsCompleted {
			continue
		}
		content := ev.Output
		if ev.Error != "" {
			content = "ERROR: " + ev.Error
		}
		_, err := database.AddToolResultMessage(conversationID, turnID, content, ev.ID)
		if err != nil {
			log.Printf("[MCP Native] Erro ao salvar tool result (id=%s): %v", ev.ID, err)
		}
	}
}

// truncateString trunca uma string ao tamanho máximo
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
