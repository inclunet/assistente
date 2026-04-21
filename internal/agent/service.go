package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"unicode/utf8"

	"assistente/internal/chat"
	"assistente/internal/core/ports"
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
	// OnSpeechRequest é chamado após chat:done e chat:segment_done para disparar TTS proativo.
	// Parâmetros: conversationID, messageID, role, text, origin, profileSlug, interrupt.
	OnSpeechRequest func(conversationID uint, messageID uint, role, text, origin, profileSlug string, interrupt bool)
}

// Service encapsula a lógica do agentic loop sem dependências do Wails.
type Service struct {
	emitter          events.Emitter
	msgRepo          chat.MessageRepository
	toolExecutor     *tools.Executor
	responseNotifier *messaging.ResponseNotifier
	getTokenStats    func(uint) (*chat.TokenStats, error)
	triggerSummarize func(uint)
	onSpeechRequest  func(conversationID uint, messageID uint, role, text, origin, profileSlug string, interrupt bool)
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
		onSpeechRequest:  cfg.OnSpeechRequest,
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
	if params.TabType != "" || params.ActiveFilePath != "" || params.SurfaceStateJSON != "" || params.SurfaceContextJSON != "" {
		ctx = invocationctx.With(ctx, invocationctx.InvocationContext{
			TabType:        params.TabType,
			ActiveFilePath: params.ActiveFilePath,
			SurfaceState:   chat.DecodeSurfaceJSONMap(params.SurfaceStateJSON, "[agent] surface state payload"),
			SurfaceContext: chat.DecodeSurfaceJSONMap(params.SurfaceContextJSON, "[agent] surface context payload"),
		})
	}

	// AEP-0039 Fase 2: acumula estatísticas de tool calling ao longo do loop
	var (
		totalToolCallCount int
		toolsUsedSet       = map[string]struct{}{}
		lastUsage          llm.Usage
	)

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

		// Acumula usage da última iteração (AEP-0039)
		if result.Usage.PromptTokens > 0 || result.Usage.CompletionTokens > 0 {
			lastUsage = result.Usage
		}

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
		//    Para iterações finais (IsDone), emite imediatamente.
		//    Para iterações com tool calls, emite após execução com ToolsInIteration (AEP-0039).
		if result.IsDone {
			if result.FullResponse != "" {
				s.emitter.Emit("chat:segment_done", ports.SegmentDoneEvent{
					ConversationID: conversationID,
					Content:        result.FullResponse,
					Iteration:      iteration,
					HasMore:        false,
				})
			}

			// 4. finish_reason="stop" → resposta final
			s.SaveAndFinish(conversationID, turnID, result, params.ProfileSlug, &LoopStats{
				IterationCount: iteration + 1,
				ToolCallCount:  totalToolCallCount,
				ToolsUsed:      toolsUsedSet,
				LastUsage:      lastUsage,
			})
			return
		}

		// TTS proativo: verbaliza segmentos intermediários (não interrompe áudio anterior).
		if s.onSpeechRequest != nil && result.FullResponse != "" {
			s.onSpeechRequest(conversationID, 0, "assistant", result.FullResponse, "segment", params.ProfileSlug, false)
		}

		// 5. finish_reason="tool_calls" → executar ferramentas
		var iterationNativeTools []ports.ToolSummary

		// 5a. Persiste MCP calls nativas desta iteração antes das bridge calls
		if len(result.NativeMCPEvents) > 0 {
			s.persistNativeMCPCalls(conversationID, turnID, result.NativeMCPEvents, iteration)
			// AEP-0039: contabiliza MCP native tools
			for _, ev := range result.NativeMCPEvents {
				if ev.IsCompleted {
					name := ev.Name
					if ev.ServerLabel != "" {
						name = ev.ServerLabel + "/" + ev.Name
					}
					status := "ok"
					if ev.Error != "" {
						status = "error"
					}
					iterationNativeTools = append(iterationNativeTools, ports.ToolSummary{
						Name:   name,
						Status: status,
					})
					totalToolCallCount++
					toolsUsedSet[name] = struct{}{}
				}
			}
		}

		// 5b. Adiciona mensagem do assistant ao histórico para próxima iteração
		// (Persistência no DB movida para após execução — AEP-0039 Fase 5)
		messages = append(messages, llm.Message{
			Role:      "assistant",
			Content:   result.FullResponse,
			ToolCalls: result.ToolCalls,
		})

		// 5d. Executa ferramentas em paralelo
		toolCalls := convertToolCalls(result.ToolCalls)
		s.emitToolStarts(conversationID, result.ToolCalls)
		execResults := s.toolExecutor.ExecuteAll(ctx, toolCalls)

		// 5e. Retry automático para erros retryable (AEP-0039 Fase 3)
		for i, execResult := range execResults {
			if execResult.Result.IsError && execResult.Retryable && iteration < maxIterations-1 {
				retryOrigin, _ := detectToolOrigin(execResult.ToolName)
				// Emite tool_failure com willRetry=true
				EmitToolFailure(s.emitter, ports.ToolFailureEvent{
					ConversationID: conversationID,
					Name:           execResult.ToolName,
					CallID:         execResult.CallID,
					ErrorKind:      execResult.ErrorKind,
					Retryable:      true,
					Message:        truncateString(execResult.Result.Content, 200),
					DurationMs:     execResult.DurationMs,
					Origin:         retryOrigin,
					WillRetry:      true,
				})
				log.Printf("[Agent] tool %s falhou (kind=%s), tentando retry...", execResult.ToolName, execResult.ErrorKind)
				retried := s.toolExecutor.ExecuteOne(ctx, toolCalls[i])
				execResults[i] = retried
			}
		}

		// 5f. Emit tool_end/failure events e acumula stats
		var iterationTools []ports.ToolSummary
		for _, execResult := range execResults {
			origin, serverLabel := detectToolOrigin(execResult.ToolName)
			status := "ok"
			if execResult.Result.IsError {
				status = "error"
			}
			EmitToolEnd(s.emitter, ports.ToolEndEvent{
				ConversationID: conversationID,
				Name:           execResult.ToolName,
				CallID:         execResult.CallID,
				Status:         status,
				Summary:        truncateString(execResult.Result.Content, MaxResultDisplaySize),
				Origin:         origin,
				ServerLabel:    serverLabel,
				DurationMs:     execResult.DurationMs,
			})

			// AEP-0039 Fase 3: emite tool_failure para erros classificados (sem retry)
			if execResult.Result.IsError && execResult.ErrorKind != "" {
				EmitToolFailure(s.emitter, ports.ToolFailureEvent{
					ConversationID: conversationID,
					Name:           execResult.ToolName,
					CallID:         execResult.CallID,
					ErrorKind:      execResult.ErrorKind,
					Retryable:      execResult.Retryable,
					Message:        truncateString(execResult.Result.Content, MaxResultDisplaySize),
					DurationMs:     execResult.DurationMs,
					Origin:         origin,
				})
			}

			// AEP-0039: acumula stats
			iterationTools = append(iterationTools, ports.ToolSummary{
				Name:       execResult.ToolName,
				Status:     status,
				ErrorKind:  execResult.ErrorKind,
				DurationMs: execResult.DurationMs,
			})
			totalToolCallCount++
			toolsUsedSet[execResult.ToolName] = struct{}{}
		}

		// 5f-ii. AEP-0039 Fase 4: pre-check de context window — trunca resultados se necessário.
		// Usa cópia para truncamento; o conteúdo original é preservado para persistência no DB.
		toolContents := make([]string, len(execResults))
		for i, r := range execResults {
			toolContents[i] = r.Result.Content
		}
		preCheck := PreCheckContextWindow(params.ContextWindow, params.MaxTokens, messages, toolContents)

		// 5f-iii. AEP-0039 Fase 5: persiste assistant tool_calls com metadata enriquecida
		enrichedCalls := make([]llm.EnrichedToolCall, len(result.ToolCalls))
		for i, tc := range result.ToolCalls {
			tcOrigin, tcServerLabel := detectToolOrigin(tc.Function.Name)
			enrichedCalls[i] = llm.EnrichedToolCall{
				ID:          tc.ID,
				Type:        tc.Type,
				Function:    tc.Function,
				Origin:      tcOrigin,
				ServerLabel: tcServerLabel,
				Iteration:   iteration,
			}
			if i < len(execResults) {
				enrichedCalls[i].DurationMs = execResults[i].DurationMs
			}
		}
		toolCallsJSON, _ := json.Marshal(enrichedCalls)
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

		// 5f-iv. Persiste resultados originais no DB e adiciona conteúdo (possivelmente
		// truncado) ao histórico de mensagens enviado ao LLM.
		for i, execResult := range execResults {
			// Persiste conteúdo original completo no banco
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

			// Para o histórico LLM, usa versão truncada se pre-check aplicou truncamento
			content := execResult.Result.Content
			if preCheck.Truncated {
				content = toolContents[i]
			}
			messages = append(messages, llm.Message{
				Role:       "tool",
				Content:    content,
				ToolCallID: execResult.CallID,
			})
		}

		// 5g. Emite token stats atualizadas em tempo real
		if s.getTokenStats != nil {
			if stats, err := s.getTokenStats(conversationID); err == nil && stats != nil {
				s.emitter.Emit("chat:token_stats_update", ports.TokenStatsUpdateEvent{
					ConversationID:              conversationID,
					PromptTokens:                stats.PromptTokens,
					CompletionTokens:            stats.CompletionTokens,
					TotalTokens:                 stats.TotalTokens,
					ContextUsage:                stats.ContextUsage,
					ContextLimit:                stats.ContextLimit,
					IsNearLimit:                 stats.IsNearLimit,
					IsCritical:                  stats.IsCritical,
					MessageCount:                stats.MessageCount,
					SystemPromptEstimatedTokens: stats.SystemPromptEstimatedTokens,
					SummaryTokens:               stats.SummaryTokens,
					MessagesInContextCount:      stats.MessagesInContextCount,
					MessagesInContextTokens:     stats.MessagesInContextTokens,
					ToolsUsedCount:              stats.ToolsUsedCount,
					ToolBreakdown:               stats.ToolBreakdown,
				})
			}
		}

		// 5g. Emite segment_done com resumo de tools da iteração (AEP-0039)
		allIterTools := append(iterationNativeTools, iterationTools...)
		s.emitter.Emit("chat:segment_done", ports.SegmentDoneEvent{
			ConversationID:   conversationID,
			Content:          result.FullResponse,
			Iteration:        iteration,
			HasMore:          true,
			ToolsInIteration: allIterTools,
		})
	}

	// Atingiu limite de iterações
	log.Printf("[Agent] limite de %d iterações atingido para conversa %d", maxIterations, conversationID)
	s.emitter.Emit("chat:stream", events.StreamEvent{
		Content:        "Limite de iterações do agente atingido. A resposta pode estar incompleta.",
		Done:           true,
		ConversationId: conversationID,
	})
	toolsUsedList := make([]string, 0, len(toolsUsedSet))
	for name := range toolsUsedSet {
		toolsUsedList = append(toolsUsedList, name)
	}
	sort.Strings(toolsUsedList)
	s.emitter.Emit("chat:done", ports.DoneEvent{
		ConversationID:   conversationID,
		HadToolCalls:     totalToolCallCount > 0,
		Reason:           "limit_reached",
		IterationCount:   maxIterations,
		ToolCallCount:    totalToolCallCount,
		ToolsUsed:        toolsUsedList,
		PromptTokens:     lastUsage.PromptTokens,
		CompletionTokens: lastUsage.CompletionTokens,
	})

	if s.triggerSummarize != nil {
		go func() {
			defer s.recoverFromPanic(conversationID, "triggerSummarize")
			s.triggerSummarize(conversationID)
		}()
	}
}

// LoopStats acumula estatísticas do agentic loop para inclusão no chat:done (AEP-0039 Fase 2).
type LoopStats struct {
	IterationCount   int
	ToolCallCount    int
	ToolsUsed        map[string]struct{}
	LastUsage        llm.Usage
}

// SaveAndFinish salva a resposta final do assistente e emite os eventos de conclusão.
// Se houve MCP tool calls nativas, persiste no banco antes da mensagem final.
// loopStats é opcional — se nil, apenas os campos enriquecidos derivados das estatísticas do loop ficam vazios.
func (s *Service) SaveAndFinish(conversationID, turnID uint, result AgenticResult, profileSlug string, loopStats *LoopStats) {
	var savedMsgID uint
	if conversationID > 0 && result.FullResponse != "" {
		if len(result.NativeMCPEvents) > 0 && turnID > 0 {
			finalIteration := 0
			if loopStats != nil && loopStats.IterationCount > 0 {
				finalIteration = loopStats.IterationCount - 1
			}
			s.persistNativeMCPCalls(conversationID, turnID, result.NativeMCPEvents, finalIteration)
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
		MessageID:      savedMsgID,
		Content:        result.FullResponse,
		Done:           true,
		ConversationId: conversationID,
		FullResponse:   result.FullResponse,
	})

	// TTS proativo: dispara ANTES de chat:done pois chat:done causa cleanup dos listeners no frontend
	if s.onSpeechRequest != nil && result.FullResponse != "" {
		s.onSpeechRequest(conversationID, savedMsgID, "assistant", result.FullResponse, "assistant_message", profileSlug, true)
	}

	hadTools := turnID > 0
	if loopStats != nil {
		hadTools = loopStats.ToolCallCount > 0 || len(result.NativeMCPEvents) > 0
	}
	doneEvent := ports.DoneEvent{
		ConversationID:     conversationID,
		AssistantMessageID: savedMsgID,
		HadToolCalls:       hadTools,
		Reason:             "completed",
	}
	if loopStats != nil {
		doneEvent.IterationCount = loopStats.IterationCount
		doneEvent.ToolCallCount = loopStats.ToolCallCount
		if len(loopStats.ToolsUsed) > 0 {
			list := make([]string, 0, len(loopStats.ToolsUsed))
			for name := range loopStats.ToolsUsed {
				list = append(list, name)
			}
			sort.Strings(list)
			doneEvent.ToolsUsed = list
		}
		if loopStats.LastUsage.PromptTokens > 0 {
			doneEvent.PromptTokens = loopStats.LastUsage.PromptTokens
		}
		if loopStats.LastUsage.CompletionTokens > 0 {
			doneEvent.CompletionTokens = loopStats.LastUsage.CompletionTokens
		}
	}
	if doneEvent.PromptTokens == 0 && result.Usage.PromptTokens > 0 {
		doneEvent.PromptTokens = result.Usage.PromptTokens
	}
	if doneEvent.CompletionTokens == 0 && result.Usage.CompletionTokens > 0 {
		doneEvent.CompletionTokens = result.Usage.CompletionTokens
	}
	s.emitter.Emit("chat:done", doneEvent)

	if s.triggerSummarize != nil {
		go func() {
			defer s.recoverFromPanic(conversationID, "triggerSummarize")
			s.triggerSummarize(conversationID)
		}()
	}

	s.emitTokenStats(conversationID)
}

// emitTokenStats queries token usage for a conversation and emits chat:token_stats
// plus a chat:context_warning if the context window is near or at capacity.
func (s *Service) emitTokenStats(conversationID uint) {
	if conversationID == 0 || s.getTokenStats == nil {
		return
	}
	stats, err := s.getTokenStats(conversationID)
	if err != nil || stats == nil || stats.ContextLimit == 0 {
		return
	}
	s.emitter.Emit("chat:token_stats", ports.TokenStatsEvent{
		ConversationID:   conversationID,
		TotalTokens:      stats.TotalTokens,
		ContextLimit:     stats.ContextLimit,
		ContextUsage:     stats.ContextUsage,
		IsNearLimit:      stats.IsNearLimit,
		IsCritical:       stats.IsCritical,
		PromptTokens:     stats.PromptTokens,
		CompletionTokens: stats.CompletionTokens,
		MessageCount:     stats.MessageCount,
	})
	if stats.IsCritical {
		log.Printf("[Context] conversa %d em CRÍTICO: %0.1f%% (%d/%d tokens)",
			conversationID, stats.ContextUsage, stats.TotalTokens, stats.ContextLimit)
		s.emitter.Emit("chat:context_warning", ports.ContextWarningEvent{
			ConversationID: conversationID,
			Level:          "critical",
			Message: fmt.Sprintf("Atenção: Contexto em %0.1f%% (%d/%d tokens). Considere limpar a conversa ou resumir o histórico.",
				stats.ContextUsage, stats.TotalTokens, stats.ContextLimit),
			Percentage:   stats.ContextUsage,
			TotalTokens:  stats.TotalTokens,
			ContextLimit: stats.ContextLimit,
		})
	} else if stats.IsNearLimit {
		log.Printf("[Context] conversa %d próxima do limite: %0.1f%% (%d/%d tokens)",
			conversationID, stats.ContextUsage, stats.TotalTokens, stats.ContextLimit)
		s.emitter.Emit("chat:context_warning", ports.ContextWarningEvent{
			ConversationID: conversationID,
			Level:          "warning",
			Message: fmt.Sprintf("Contexto em %0.1f%% (%d/%d tokens). Considere limpar a conversa em breve.",
				stats.ContextUsage, stats.TotalTokens, stats.ContextLimit),
			Percentage:   stats.ContextUsage,
			TotalTokens:  stats.TotalTokens,
			ContextLimit: stats.ContextLimit,
		})
	}
}

func (s *Service) emitToolStarts(conversationID uint, calls []llm.ToolCall) {
	for _, call := range calls {
		origin, serverLabel := detectToolOrigin(call.Function.Name)
		EmitToolStart(s.emitter, ports.ToolStartEvent{
			ConversationID: conversationID,
			Name:           call.Function.Name,
			CallID:         call.ID,
			Args:           call.Function.Arguments,
			Origin:         origin,
			ServerLabel:    serverLabel,
		})
	}
}

// detectToolOrigin determina origin e serverLabel a partir do nome da tool.
// Tools MCP bridge seguem o formato "mcp_{serverSlug}__{toolName}".
func detectToolOrigin(toolName string) (origin, serverLabel string) {
	if strings.HasPrefix(toolName, "mcp_") {
		if idx := strings.Index(toolName, "__"); idx > 4 {
			return OriginMCPBridge, toolName[4:idx]
		}
	}
	return OriginBuiltin, ""
}

// persistNativeMCPCalls salva MCP tool calls nativas no banco no mesmo formato que bridge calls:
// uma mensagem assistant com tool_calls JSON + mensagens tool separadas com resultados.
// AEP-0039 Fase 5: serializa com EnrichedToolCall para incluir origin, server_label, iteration.
func (s *Service) persistNativeMCPCalls(conversationID, turnID uint, mcpEvents []llm.MCPToolEvent, iteration int) {
	var toolCalls []llm.EnrichedToolCall
	for _, ev := range mcpEvents {
		if !ev.IsCompleted {
			continue
		}
		name := ev.Name
		if ev.ServerLabel != "" {
			name = ev.ServerLabel + "/" + ev.Name
		}
		toolCalls = append(toolCalls, llm.EnrichedToolCall{
			ID:   ev.ID,
			Type: "function",
			Function: llm.FunctionCall{
				Name:      name,
				Arguments: ev.Arguments,
			},
			Origin:      OriginMCPNative,
			ServerLabel: ev.ServerLabel,
			Iteration:   iteration,
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

// recoverFromPanic captura panic e delega o tratamento para events.HandlePanic.
// recover() deve ser chamado diretamente no corpo da função adiada — não pode ser delegado.
func (s *Service) recoverFromPanic(conversationID uint, source string) {
	r := recover()
	events.HandlePanic(s.emitter, conversationID, source, r)
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
	// UTF-8 safe: recua até achar limite de rune válido
	for maxLen > 0 && !utf8.RuneStart(s[maxLen]) {
		maxLen--
	}
	return s[:maxLen] + "..."
}
