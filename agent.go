package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	FullResponse string
	Reasoning    string
	ToolCalls    []llm.ToolCall
	Usage        llm.Usage
	Model        string
	Error        string
	IsDone       bool // true = finish_reason:"stop", false = finish_reason:"tool_calls"
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
	h.mu.Unlock()

	finalContent := fullResponse
	if finalContent == "" {
		finalContent = content
	}

	h.result = agenticResult{
		FullResponse: finalContent,
		Reasoning:    reasoning,
		ToolCalls:    calls,
		Usage:        usage,
		Model:        model,
		IsDone:       false,
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
	h.mu.Unlock()

	finalContent := fullResponse
	if finalContent == "" {
		finalContent = content
	}

	h.result = agenticResult{
		FullResponse: finalContent,
		Reasoning:    reasoning,
		Usage:        usage,
		Model:        model,
		IsDone:       true,
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
) {
	maxIterations := a.toolExecutor.Config().MaxIterations

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
		llm.StreamChat(ctx, cfg, messages, params, handler, toolDefs...)

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

		// 6a. Salva mensagem do assistant com tool_calls no banco
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

		// 6b. Adiciona mensagem do assistant ao histórico de mensagens (para próxima iteração)
		assistantMessage := llm.Message{
			Role:      "assistant",
			Content:   result.FullResponse,
			ToolCalls: result.ToolCalls,
		}
		messages = append(messages, assistantMessage)

		// 6c. Executa ferramentas em paralelo
		toolCalls := convertToolCalls(result.ToolCalls)
		a.emitToolStarts(conversationID, result.ToolCalls)

		execResults := a.toolExecutor.ExecuteAll(ctx, toolCalls)

		// 6d. Salva resultados e adiciona ao histórico
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

		// 6e. Continua o loop → próxima iteração chama o LLM com os resultados
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
}

// saveAndFinish salva a resposta final do assistente e emite os eventos de conclusão.
func (a *App) saveAndFinish(conversationID, turnID uint, result agenticResult) {
	var savedMsgID uint
	if conversationID > 0 && result.FullResponse != "" {
		// Se houve tool calls no turno, salva com TurnID
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
		// Só associa ao turno se houve iterações (tool calls) — ou seja, turnID > 0
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

// truncateString trunca uma string ao tamanho máximo
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
