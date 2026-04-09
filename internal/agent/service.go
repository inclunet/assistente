package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"

	"assistente/internal/chat"
	"assistente/internal/events"
	"assistente/internal/llm"
	"assistente/internal/messaging"
	"assistente/internal/tools"
	"assistente/internal/tools/invocationctx"
)

// AgenticResult captura o resultado de uma iteração do streaming LLM.
// Preenchido pelos callbacks OnDone/OnToolCalls/OnError do IterationHandler.
type AgenticResult struct {
	FullResponse    string
	Reasoning       string
	ToolCalls       []llm.ToolCall
	NativeMCPEvents []llm.MCPToolEvent
	Usage           llm.Usage
	Model           string
	Error           string
	IsDone          bool
}

// IterationHandler é implementado pelo agenticStreamHandler (package main) para cada iteração.
type IterationHandler interface {
	llm.StreamHandler
	Result() AgenticResult
}

// ServiceConfig contém todas as dependências injetadas do AgentService.
type ServiceConfig struct {
	Emitter          events.Emitter
	MsgRepo          chat.MessageRepository
	ToolExecutor     *tools.Executor
	ResponseNotifier *messaging.ResponseNotifier
	GetTokenStats    func(uint) (*chat.TokenStats, error)
	TriggerSummarize func(uint)
}

// Service encapsula a lógica do agentic loop sem dependências do Wails.
type Service struct {
	emitter          events.Emitter
	msgRepo          chat.MessageRepository
	toolExecutor     *tools.Executor
	responseNotifier *messaging.ResponseNotifier
	getTokenStats    func(uint) (*chat.TokenStats, error)
	triggerSummarize func(uint)
}

// NewService cria um novo Service com as dependências injetadas.
func NewService(cfg ServiceConfig) *Service {
	return &Service{
		emitter:          cfg.Emitter,
		msgRepo:          cfg.MsgRepo,
		toolExecutor:     cfg.ToolExecutor,
		responseNotifier: cfg.ResponseNotifier,
		getTokenStats:    cfg.GetTokenStats,
		triggerSummarize: cfg.TriggerSummarize,
	}
}

// RunAgenticLoop executa o loop de tool calling.
// Em cada iteração:
//  1. Chama o LLM com streaming (texto aparece em tempo real via newHandler)
//  2. Se finish_reason="stop" → salva resposta final, emite chat:done
//  3. Se finish_reason="tool_calls" → emite segment_done, executa tools, salva no banco, repete
func (s *Service) RunAgenticLoop(
	ctx context.Context,
	messages []llm.Message,
	params llm.ChatParams,
	conversationID uint,
	turnID uint,
	toolDefs []llm.ToolDefinition,
	streamer llm.Streamer,
	newHandler func(conversationID uint, iteration int) IterationHandler,
) {
	if streamer == nil {
		errMsg := "Cliente LLM não disponível para o agentic loop. Verifique a configuração do provedor."
		log.Printf("🔴 [AGENT] streamer nil na conversa %d", conversationID)
		s.emitter.Emit("chat:stream", events.StreamEvent{
			Content:        "",
			Done:           true,
			Error:          errMsg,
			ConversationId: conversationID,
		})
		return
	}

	// Resolver maxIterations usando valor do perfil (params) ou fallback ao config do executor
	maxIterations := params.MaxAgenticIterations
	if maxIterations <= 0 {
		maxIterations = s.toolExecutor.Config().MaxIterations
	}

	// Propaga contexto de invocação (tab type + arquivo ativo) para as tools
	if params.TabType != "" {
		ctx = invocationctx.With(ctx, invocationctx.InvocationContext{
			TabType:        params.TabType,
			ActiveFilePath: params.ActiveFilePath,
		})
	}

	for iteration := 0; iteration < maxIterations; iteration++ {
		// Verifica cancelamento
		if ctx.Err() != nil {
			log.Printf("[Agent] loop cancelado na iteração %d", iteration)
			s.emitter.Emit("chat:stream", events.StreamEvent{
				Content:        "",
				Done:           true,
				Error:          "Operação cancelada",
				ConversationId: conversationID,
			})
			return
		}

		// 1. Cria handler para esta iteração e chama o LLM (bloqueante)
		handler := newHandler(conversationID, iteration)
		streamer.StreamChat(ctx, messages, params, handler, toolDefs...)

		result := handler.Result()

		// 2. Erro?
		if result.Error != "" {
			log.Printf("[Agent] erro na iteração %d: %s", iteration, result.Error)
			s.emitter.Emit("chat:stream", events.StreamEvent{
				Content:        result.FullResponse,
				Done:           true,
				Error:          result.Error,
				ConversationId: conversationID,
			})
			return
		}

		// 3. Emite segment_done para verbalização e acumulação de segmentos no frontend
		if result.FullResponse != "" || !result.IsDone {
			s.emitter.Emit("chat:segment_done", map[string]interface{}{
				"content":        result.FullResponse,
				"iteration":      iteration,
				"hasMore":        !result.IsDone,
				"conversationId": conversationID,
			})
		}

		// 4. finish_reason="stop" → resposta final
		if result.IsDone {
			s.SaveAndFinish(conversationID, turnID, result)
			return
		}

		// 5. finish_reason="tool_calls" → executar ferramentas
		// 5a. Persiste MCP calls nativas desta iteração antes das bridge calls
		if len(result.NativeMCPEvents) > 0 {
			s.persistNativeMCPCalls(conversationID, turnID, result.NativeMCPEvents)
		}

		// 5b. Salva mensagem do assistant com bridge tool_calls no banco
		toolCallsJSON, _ := json.Marshal(result.ToolCalls)
		_, err := s.msgRepo.AddAssistantToolMessage(
			conversationID,
			turnID,
			result.FullResponse,
			string(toolCallsJSON),
			result.Reasoning,
			result.Model,
		)
		if err != nil {
			if errors.Is(err, chat.ErrConversationDeleted) {
				log.Printf("[Agent] conversa %d deletada — abortando", conversationID)
				return
			}
			log.Printf("[Agent] erro ao salvar assistant com tool_calls: %v", err)
		}

		// 5c. Adiciona mensagem do assistant ao histórico para próxima iteração
		messages = append(messages, llm.Message{
			Role:      "assistant",
			Content:   result.FullResponse,
			ToolCalls: result.ToolCalls,
		})

		// 5d. Executa ferramentas em paralelo
		toolCalls := convertToolCalls(result.ToolCalls)
		s.emitToolStarts(conversationID, result.ToolCalls)
		execResults := s.toolExecutor.ExecuteAll(ctx, toolCalls)

		// 5e. Salva resultados e adiciona ao histórico
		for _, execResult := range execResults {
			status := "ok"
			if execResult.Result.IsError {
				status = "error"
			}
			s.emitter.Emit("chat:tool_end", map[string]interface{}{
				"name":           execResult.ToolName,
				"callId":         execResult.CallID,
				"status":         status,
				"summary":        truncateString(execResult.Result.Content, 200),
				"conversationId": conversationID,
			})

			_, err := s.msgRepo.AddToolResultMessage(
				conversationID,
				turnID,
				execResult.Result.Content,
				execResult.CallID,
			)
			if err != nil {
				if errors.Is(err, chat.ErrConversationDeleted) {
					log.Printf("[Agent] conversa %d deletada — abortando", conversationID)
					return
				}
				log.Printf("[Agent] erro ao salvar resultado de tool %s: %v", execResult.ToolName, err)
			}

			messages = append(messages, llm.Message{
				Role:       "tool",
				Content:    execResult.Result.Content,
				ToolCallID: execResult.CallID,
			})
		}

		// 5f. Emite token stats atualizadas em tempo real
		if s.getTokenStats != nil {
			if stats, err := s.getTokenStats(conversationID); err == nil && stats != nil {
				s.emitter.Emit("chat:token_stats_update", map[string]interface{}{
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
				})
			}
		}
	}

	// Atingiu limite de iterações
	log.Printf("[Agent] limite de %d iterações atingido para conversa %d", maxIterations, conversationID)
	s.emitter.Emit("chat:stream", events.StreamEvent{
		Content:        "Limite de iterações do agente atingido. A resposta pode estar incompleta.",
		Done:           true,
		ConversationId: conversationID,
	})
	s.emitter.Emit("chat:done", map[string]interface{}{
		"conversationId": conversationID,
	})

	if s.triggerSummarize != nil {
		go func() {
			defer s.recoverFromPanic(conversationID, "triggerSummarize")
			s.triggerSummarize(conversationID)
		}()
	}
}

// SaveAndFinish salva a resposta final do assistente e emite os eventos de conclusão.
// Se houve MCP tool calls nativas, persiste no banco antes da mensagem final.
func (s *Service) SaveAndFinish(conversationID, turnID uint, result AgenticResult) {
	var savedMsgID uint
	if conversationID > 0 && result.FullResponse != "" {
		if len(result.NativeMCPEvents) > 0 && turnID > 0 {
			s.persistNativeMCPCalls(conversationID, turnID, result.NativeMCPEvents)
		}

		opts := chat.MessageOptions{
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

		var err error
		savedMsgID, err = chat.SaveAssistantMessage(s.msgRepo, opts)
		if errors.Is(err, chat.ErrConversationGone) {
			return
		}
		if err != nil {
			log.Printf("[Agent] erro ao salvar resposta final: %v", err)
		}
	}

	if s.responseNotifier != nil {
		s.responseNotifier.Notify(conversationID, result.FullResponse, savedMsgID)
	}

	s.emitter.Emit("chat:stream", events.StreamEvent{
		Content:        result.FullResponse,
		Done:           true,
		ConversationId: conversationID,
		FullResponse:   result.FullResponse,
	})

	s.emitter.Emit("chat:done", map[string]interface{}{
		"conversationId": conversationID,
	})

	if s.triggerSummarize != nil {
		go func() {
			defer s.recoverFromPanic(conversationID, "triggerSummarize")
			s.triggerSummarize(conversationID)
		}()
	}
}

func (s *Service) emitToolStarts(conversationID uint, calls []llm.ToolCall) {
	for _, call := range calls {
		s.emitter.Emit("chat:tool_start", map[string]interface{}{
			"name":           call.Function.Name,
			"callId":         call.ID,
			"args":           call.Function.Arguments,
			"conversationId": conversationID,
		})
	}
}

// persistNativeMCPCalls salva MCP tool calls nativas no banco no mesmo formato que bridge calls:
// uma mensagem assistant com tool_calls JSON + mensagens tool separadas com resultados.
func (s *Service) persistNativeMCPCalls(conversationID, turnID uint, mcpEvents []llm.MCPToolEvent) {
	var toolCalls []llm.ToolCall
	for _, ev := range mcpEvents {
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

	_, err = s.msgRepo.AddAssistantToolMessage(conversationID, turnID, "", string(toolCallsJSON), "", "")
	if err != nil {
		log.Printf("[MCP Native] Erro ao salvar assistant tool_calls: %v", err)
		return
	}

	for _, ev := range mcpEvents {
		if !ev.IsCompleted {
			continue
		}
		content := ev.Output
		if ev.Error != "" {
			content = "ERROR: " + ev.Error
		}
		_, err := s.msgRepo.AddToolResultMessage(conversationID, turnID, content, ev.ID)
		if err != nil {
			log.Printf("[MCP Native] Erro ao salvar tool result (id=%s): %v", ev.ID, err)
		}
	}
}

func (s *Service) recoverFromPanic(conversationID uint, source string) {
	if r := recover(); r != nil {
		errMsg := fmt.Sprintf("Erro interno inesperado em %s: %v", source, r)
		log.Printf("🔴 [PANIC RECOVERED] %s (conversa %d): %v", source, conversationID, r)
		func() {
			defer func() { recover() }()
			if s != nil && s.emitter != nil {
				s.emitter.Emit("chat:stream", events.StreamEvent{
					Content:        "",
					Done:           true,
					Error:          errMsg,
					ConversationId: conversationID,
				})
			}
		}()
	}
}

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

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
