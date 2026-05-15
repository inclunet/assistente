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
	"assistente/internal/database"
	"assistente/internal/events"
	"assistente/internal/llm"
	"assistente/internal/mcp"
	"assistente/internal/messaging"
	"assistente/internal/toolinvocations"
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
	ToolInvocations  *toolinvocations.Service
	ResponseNotifier *messaging.ResponseNotifier
	GetTokenStats    func(string) (*chat.TokenStats, error)
	TriggerSummarize func(context.Context, string)
	// OnSpeechRequest é chamado após chat:done e chat:segment_done para disparar TTS proativo.
	// Parâmetros: conversationID, messageID, role, text, origin, profileSlug, interrupt.
	OnSpeechRequest func(conversationID string, messageID string, role, text, origin, profileSlug string, interrupt bool)
}

// Service encapsula a lógica do agentic loop sem dependências do Wails.
type Service struct {
	emitter          events.Emitter
	msgRepo          chat.MessageRepository
	toolExecutor     *tools.Executor
	toolInvocations  *toolinvocations.Service
	responseNotifier *messaging.ResponseNotifier
	getTokenStats    func(string) (*chat.TokenStats, error)
	triggerSummarize func(context.Context, string)
	onSpeechRequest  func(conversationID string, messageID string, role, text, origin, profileSlug string, interrupt bool)
}

// NewService cria um novo Service com as dependências injetadas.
func NewService(cfg ServiceConfig) *Service {
	return &Service{
		emitter:          cfg.Emitter,
		msgRepo:          cfg.MsgRepo,
		toolExecutor:     cfg.ToolExecutor,
		toolInvocations:  cfg.ToolInvocations,
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
	conversationID string,
	turnID string,
	toolDefs []llm.ToolDefinition,
	streamer llm.Streamer,
	surfaceOrigin *ports.ChatSurfaceOrigin,
	newHandler func(conversationID string, iteration int) IterationHandler,
	resolveToolDefs func([]string) []llm.ToolDefinition,
) {
	if streamer == nil {
		errMsg := "Cliente LLM não disponível para o agentic loop. Verifique a configuração do provedor."
		log.Printf("🔴 [AGENT] streamer nil na conversa %s", conversationID)
		s.emitter.Emit("chat:done", ports.DoneEvent{
			ConversationID: conversationID,
			TurnID:         turnID,
			SurfaceOrigin:  surfaceOrigin,
			Reason:         "error",
			ErrorMessage:   errMsg,
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
			cancelToolsUsed := make([]string, 0, len(toolsUsedSet))
			for name := range toolsUsedSet {
				cancelToolsUsed = append(cancelToolsUsed, name)
			}
			sort.Strings(cancelToolsUsed)
			s.emitter.Emit("chat:done", ports.DoneEvent{
				ConversationID:   conversationID,
				TurnID:           turnID,
				SurfaceOrigin:    surfaceOrigin,
				HadToolCalls:     totalToolCallCount > 0,
				Reason:           "error",
				ErrorMessage:     "Operação cancelada",
				IterationCount:   iteration,
				ToolCallCount:    totalToolCallCount,
				ToolsUsed:        cancelToolsUsed,
				PromptTokens:     lastUsage.PromptTokens,
				CompletionTokens: lastUsage.CompletionTokens,
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
			// chat:done é o evento terminal canônico — inclui ErrorMessage para que
			// adapters (CLI, frontend) exibam o erro sem depender de chat:stream terminal.
			errToolsUsed := make([]string, 0, len(toolsUsedSet))
			for name := range toolsUsedSet {
				errToolsUsed = append(errToolsUsed, name)
			}
			sort.Strings(errToolsUsed)
			s.emitter.Emit("chat:done", ports.DoneEvent{
				ConversationID:   conversationID,
				TurnID:           turnID,
				SurfaceOrigin:    surfaceOrigin,
				HadToolCalls:     totalToolCallCount > 0,
				Reason:           "error",
				ErrorMessage:     result.Error,
				IterationCount:   iteration + 1,
				ToolCallCount:    totalToolCallCount,
				ToolsUsed:        errToolsUsed,
				PromptTokens:     lastUsage.PromptTokens,
				CompletionTokens: lastUsage.CompletionTokens,
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
					TurnID:         turnID,
					Content:        result.FullResponse,
					Iteration:      iteration,
					HasMore:        false,
					SurfaceOrigin:  surfaceOrigin,
				})
			}

			// 4. finish_reason="stop" → resposta final
			s.SaveAndFinish(ctx, conversationID, turnID, result, params.ProfileSlug, &LoopStats{
				IterationCount: iteration + 1,
				ToolCallCount:  totalToolCallCount,
				ToolsUsed:      toolsUsedSet,
				LastUsage:      lastUsage,
			}, surfaceOrigin)
			return
		}

		// TTS proativo: verbaliza segmentos intermediários (não interrompe áudio anterior).
		if s.onSpeechRequest != nil && result.FullResponse != "" {
			s.onSpeechRequest(conversationID, "", "assistant", result.FullResponse, "segment", params.ProfileSlug, false)
		}

		// 5. finish_reason="tool_calls" → executar ferramentas
		var iterationNativeTools []ports.ToolSummary

		// 5a. Persiste MCP calls nativas desta iteração antes das bridge calls
		if len(result.NativeMCPEvents) > 0 {
			s.persistNativeMCPCalls(ctx, conversationID, turnID, result.NativeMCPEvents, iteration)
			// AEP-0039: contabiliza MCP native tools
			for _, ev := range result.NativeMCPEvents {
				if ev.IsCompleted {
					status := "ok"
					if ev.Error != "" {
						status = "error"
					}
					iterationNativeTools = append(iterationNativeTools, ports.ToolSummary{
						Name:        ev.Name,
						Status:      status,
						Origin:      OriginMCPNative,
						ServerLabel: ev.ServerLabel,
					})
					totalToolCallCount++
					toolsUsedSet[ev.Name] = struct{}{}
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
		s.emitToolStarts(conversationID, turnID, result.ToolCalls, surfaceOrigin)
		execResults := s.executeToolCalls(ctx, toolCalls, toolinvocations.Origin{Type: toolinvocations.OriginChat, ID: turnID})

		// 5e. Retry automático para erros retryable (AEP-0039 Fase 3)
		retriedCallIDs := make(map[string]struct{})
		for i, execResult := range execResults {
			if execResult.Result.IsError && execResult.Retryable && iteration < maxIterations-1 {
				retriedCallIDs[execResult.CallID] = struct{}{}
				retryOrigin, retryServerLabel := detectToolOrigin(execResult.ToolName)
				retryName := extractLogicalToolName(execResult.ToolName)
				// Emite tool_end para a tentativa que falhou (attempt=0)
				EmitToolEnd(s.emitter, ports.ToolEndEvent{
					ConversationID: conversationID,
					TurnID:         turnID,
					Name:           retryName,
					CallID:         execResult.CallID,
					Status:         "error",
					Summary:        truncateString(execResult.Result.Content, MaxResultDisplaySize),
					Origin:         retryOrigin,
					ServerLabel:    retryServerLabel,
					DurationMs:     execResult.DurationMs,
					Attempt:        0,
					SurfaceOrigin:  surfaceOrigin,
				})
				// Emite tool_failure com willRetry=true
				EmitToolFailure(s.emitter, ports.ToolFailureEvent{
					ConversationID: conversationID,
					TurnID:         turnID,
					Name:           retryName,
					CallID:         execResult.CallID,
					ErrorKind:      string(execResult.ErrorKind),
					Retryable:      true,
					Message:        truncateString(execResult.Result.Content, MaxResultDisplaySize),
					DurationMs:     execResult.DurationMs,
					Origin:         retryOrigin,
					WillRetry:      true,
					Attempt:        0,
					SurfaceOrigin:  surfaceOrigin,
				})
				log.Printf("[Agent] tool %s falhou (kind=%s), tentando retry...", retryName, execResult.ErrorKind)
				// Emite tool_start para a nova tentativa (attempt=1)
				EmitToolStart(s.emitter, ports.ToolStartEvent{
					ConversationID: conversationID,
					TurnID:         turnID,
					Name:           retryName,
					CallID:         execResult.CallID,
					Args:           toolCalls[i].Function.Arguments,
					Origin:         retryOrigin,
					ServerLabel:    retryServerLabel,
					Attempt:        1,
					SurfaceOrigin:  surfaceOrigin,
				})
				retried := s.executeToolCall(ctx, toolCalls[i], toolinvocations.Origin{Type: toolinvocations.OriginChat, ID: turnID})
				execResults[i] = retried
			}
		}

		// 5f. Emit tool_end/failure events e acumula stats
		var iterationTools []ports.ToolSummary
		for _, execResult := range execResults {
			origin, serverLabel := detectToolOrigin(execResult.ToolName)
			logicalName := extractLogicalToolName(execResult.ToolName)
			status := "ok"
			if execResult.Result.IsError {
				status = "error"
			}
			attempt := 0
			if _, wasRetried := retriedCallIDs[execResult.CallID]; wasRetried {
				attempt = 1
			}
			EmitToolEnd(s.emitter, ports.ToolEndEvent{
				ConversationID: conversationID,
				TurnID:         turnID,
				Name:           logicalName,
				CallID:         execResult.CallID,
				Status:         status,
				Summary:        truncateString(execResult.Result.Content, MaxResultDisplaySize),
				Origin:         origin,
				ServerLabel:    serverLabel,
				DurationMs:     execResult.DurationMs,
				Attempt:        attempt,
				SurfaceOrigin:  surfaceOrigin,
			})

			// AEP-0039 Fase 3: emite tool_failure para erros classificados (sem retry)
			if execResult.Result.IsError && execResult.ErrorKind != "" {
				EmitToolFailure(s.emitter, ports.ToolFailureEvent{
					ConversationID: conversationID,
					TurnID:         turnID,
					Name:           logicalName,
					CallID:         execResult.CallID,
					ErrorKind:      string(execResult.ErrorKind),
					Retryable:      execResult.Retryable,
					Message:        truncateString(execResult.Result.Content, MaxResultDisplaySize),
					DurationMs:     execResult.DurationMs,
					Origin:         origin,
					Attempt:        attempt,
					SurfaceOrigin:  surfaceOrigin,
				})
			}

			// AEP-0039: acumula stats
			iterationTools = append(iterationTools, ports.ToolSummary{
				Name:        logicalName,
				Status:      status,
				ErrorKind:   string(execResult.ErrorKind),
				DurationMs:  execResult.DurationMs,
				Origin:      origin,
				ServerLabel: serverLabel,
			})
			totalToolCallCount++
			toolsUsedSet[logicalName] = struct{}{}
		}
		toolDefs = expandToolDefsFromCatalogResults(toolDefs, execResults, resolveToolDefs)

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
				ID:   tc.ID,
				Type: tc.Type,
				Function: llm.FunctionCall{
					Name:      extractLogicalToolName(tc.Function.Name),
					Arguments: tc.Function.Arguments,
				},
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
			ctx,
			conversationID,
			turnID,
			result.FullResponse,
			string(toolCallsJSON),
			result.Reasoning,
			result.Model,
		)
		if err != nil {
			if errors.Is(err, chat.ErrConversationDeleted) {
				log.Printf("[Agent] conversa %s deletada — abortando", conversationID)
				return
			}
			log.Printf("[Agent] erro ao salvar assistant com tool_calls: %v", err)
		}

		// 5f-iv. Persiste resultados técnicos em tool_invocations e adiciona
		// conteúdo (possivelmente truncado) apenas ao histórico enviado ao LLM.
		// Fallback: se tool_invocations não estiver configurado, persiste como
		// mensagens role=tool para manter o histórico completo.
		for i, execResult := range execResults {
			// Para o histórico LLM, usa versão truncada se pre-check aplicou truncamento
			content := execResult.Result.Content
			if preCheck.Truncated {
				content = toolContents[i]
			}
			if s.toolInvocations == nil || !s.toolInvocations.CanPersist() {
				persistedContent := execResult.Result.Content
				if _, err := s.msgRepo.AddToolResultMessage(ctx, conversationID, turnID, persistedContent, execResult.CallID); err != nil {
					log.Printf("[Agent] erro ao salvar tool result message (fallback): %v", err)
				}
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
			TurnID:           turnID,
			Content:          result.FullResponse,
			Iteration:        iteration,
			HasMore:          true,
			ToolsInIteration: allIterTools,
			SurfaceOrigin:    surfaceOrigin,
		})
	}

	// Atingiu limite de iterações
	log.Printf("[Agent] limite de %d iterações atingido para conversa %s", maxIterations, conversationID)
	s.emitter.Emit("chat:stream", events.StreamEvent{
		Content:        "Limite de iterações do agente atingido. A resposta pode estar incompleta.",
		Done:           true,
		ConversationId: conversationID,
		TurnID:         turnID,
		SurfaceOrigin:  surfaceOrigin,
	})
	toolsUsedList := make([]string, 0, len(toolsUsedSet))
	for name := range toolsUsedSet {
		toolsUsedList = append(toolsUsedList, name)
	}
	sort.Strings(toolsUsedList)
	s.emitter.Emit("chat:done", ports.DoneEvent{
		ConversationID:   conversationID,
		TurnID:           turnID,
		HadToolCalls:     totalToolCallCount > 0,
		Reason:           "limit_reached",
		IterationCount:   maxIterations,
		ToolCallCount:    totalToolCallCount,
		ToolsUsed:        toolsUsedList,
		PromptTokens:     lastUsage.PromptTokens,
		CompletionTokens: lastUsage.CompletionTokens,
		SurfaceOrigin:    surfaceOrigin,
	})

	if s.triggerSummarize != nil {
		go func() {
			defer s.recoverFromPanic(conversationID, "triggerSummarize")
			s.triggerSummarize(ctx, conversationID)
		}()
	}
}

// LoopStats acumula estatísticas do agentic loop para inclusão no chat:done (AEP-0039 Fase 2).
type LoopStats struct {
	IterationCount int
	ToolCallCount  int
	ToolsUsed      map[string]struct{}
	LastUsage      llm.Usage
}

// SaveAndFinish salva a resposta final do assistente e emite os eventos de conclusão.
// Se houve MCP tool calls nativas, persiste no banco antes da mensagem final.
// loopStats é opcional — se nil, apenas os campos enriquecidos derivados das estatísticas do loop ficam vazios.
func (s *Service) SaveAndFinish(
	ctx context.Context,
	conversationID, turnID string,
	result AgenticResult,
	profileSlug string,
	loopStats *LoopStats,
	surfaceOrigin *ports.ChatSurfaceOrigin,
) {
	var savedMsgID string
	if conversationID != "" && result.FullResponse != "" {
		if len(result.NativeMCPEvents) > 0 && turnID != "" {
			finalIteration := 0
			if loopStats != nil && loopStats.IterationCount > 0 {
				finalIteration = loopStats.IterationCount - 1
			}
			s.persistNativeMCPCalls(ctx, conversationID, turnID, result.NativeMCPEvents, finalIteration)
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
		if turnID != "" {
			opts.TurnID = &turnID
		}

		var err error
		savedMsgID, err = chat.SaveAssistantMessage(ctx, s.msgRepo, opts)
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
		TurnID:         turnID,
		FullResponse:   result.FullResponse,
		SurfaceOrigin:  surfaceOrigin,
	})

	// TTS proativo: dispara ANTES de chat:done pois chat:done causa cleanup dos listeners no frontend
	if s.onSpeechRequest != nil && result.FullResponse != "" {
		s.onSpeechRequest(conversationID, savedMsgID, "assistant", result.FullResponse, "assistant_message", profileSlug, true)
	}

	hadTools := false
	if loopStats != nil {
		hadTools = loopStats.ToolCallCount > 0 || len(result.NativeMCPEvents) > 0
	}
	doneEvent := ports.DoneEvent{
		ConversationID:     conversationID,
		TurnID:             turnID,
		AssistantMessageID: savedMsgID,
		HadToolCalls:       hadTools,
		Reason:             "completed",
		SurfaceOrigin:      surfaceOrigin,
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
			s.triggerSummarize(ctx, conversationID)
		}()
	}

	s.emitTokenStats(conversationID)
}

// emitTokenStats queries token usage for a conversation and emits chat:token_stats
// plus a chat:context_warning if the context window is near or at capacity.
func (s *Service) emitTokenStats(conversationID string) {
	if conversationID == "" || s.getTokenStats == nil {
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
		log.Printf("[Context] conversa %s em CRÍTICO: %0.1f%% (%d/%d tokens)",
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
		log.Printf("[Context] conversa %s próxima do limite: %0.1f%% (%d/%d tokens)",
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

func (s *Service) emitToolStarts(conversationID string, turnID string, calls []llm.ToolCall, surfaceOrigin *ports.ChatSurfaceOrigin) {
	for _, call := range calls {
		origin, serverLabel := detectToolOrigin(call.Function.Name)
		name := extractLogicalToolName(call.Function.Name)
		EmitToolStart(s.emitter, ports.ToolStartEvent{
			ConversationID: conversationID,
			TurnID:         turnID,
			Name:           name,
			CallID:         call.ID,
			Args:           call.Function.Arguments,
			Origin:         origin,
			ServerLabel:    serverLabel,
			SurfaceOrigin:  surfaceOrigin,
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

// extractLogicalToolName retorna o nome lógico da tool, sem prefixo MCP bridge.
// Para "mcp_github__search_code" retorna "search_code".
// Para tools builtin/native retorna o nome inalterado.
func extractLogicalToolName(toolName string) string {
	if strings.HasPrefix(toolName, "mcp_") {
		if idx := strings.Index(toolName, "__"); idx > 4 {
			return toolName[idx+2:]
		}
	}
	return toolName
}

// persistNativeMCPCalls salva MCP tool calls nativas no banco no mesmo formato que bridge calls:
// uma mensagem assistant com tool_calls JSON e os resultados técnicos em tool_invocations.
// AEP-0039 Fase 5: serializa com EnrichedToolCall para incluir origin, server_label, iteration.
// Persistência: salva tool calls no assistant message (sem criar mensagens role=tool)
// e registra os resultados técnicos em tool_invocations.
func (s *Service) persistNativeMCPCalls(ctx context.Context, conversationID, turnID string, mcpEvents []llm.MCPToolEvent, iteration int) {
	if len(mcpEvents) == 0 {
		return
	}

	argsByID := map[string]string{}
	for _, ev := range mcpEvents {
		if strings.TrimSpace(ev.ID) == "" {
			continue
		}
		if strings.TrimSpace(ev.Arguments) == "" {
			continue
		}
		if _, ok := argsByID[ev.ID]; ok {
			continue
		}
		argsByID[ev.ID] = ev.Arguments
	}

	var toolCalls []llm.EnrichedToolCall
	for _, ev := range mcpEvents {
		if !ev.IsCompleted {
			continue
		}
		args := ev.Arguments
		if strings.TrimSpace(args) == "" {
			args = argsByID[ev.ID]
		}
		toolCalls = append(toolCalls, llm.EnrichedToolCall{
			ID:   ev.ID,
			Type: "function",
			Function: llm.FunctionCall{
				Name:      ev.Name,
				Arguments: args,
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

	_, err = s.msgRepo.AddAssistantToolMessage(ctx, conversationID, turnID, "", string(toolCallsJSON), "", "")
	if err != nil {
		log.Printf("[MCP Native] Erro ao salvar assistant tool_calls: %v", err)
		return
	}

	// Resultados técnicos: persistir em tool_invocations quando disponível.
	// Não criar novas mensagens role=tool.
	if s.toolInvocations == nil || !s.toolInvocations.CanPersist() {
		return
	}
	slugCache := map[string]string{}
	for _, ev := range mcpEvents {
		if !ev.IsCompleted {
			continue
		}
		label := strings.TrimSpace(ev.ServerLabel)
		slug := strings.TrimSpace(slugCache[label])
		if slug == "" {
			resolved, ok := resolveMCPServerSlug(ctx, label)
			if ok {
				slug = resolved
				slugCache[label] = resolved
			}
		}
		if strings.TrimSpace(slug) == "" {
			log.Printf("[MCP Native] não foi possível resolver server slug para %q; pulando persistência (id=%s)", ev.ServerLabel, ev.ID)
			continue
		}
		fullName := mcp.BuildToolName(slug, ev.Name)
		args := ev.Arguments
		if strings.TrimSpace(args) == "" {
			args = argsByID[ev.ID]
		}
		result := tools.ToolResult{Content: ev.Output}
		errKind := tools.ErrorKindNone
		errMsg := ""
		if ev.Error != "" {
			result = tools.ToolResult{Content: ev.Error, IsError: true}
			errKind = tools.ErrorKindUnknown
			errMsg = ev.Error
		}
		_, recErr := s.toolInvocations.Record(ctx, toolinvocations.RecordRequest{
			Call: tools.ToolCall{
				ID:   ev.ID,
				Type: "function",
				Function: tools.FunctionCall{
					Name:      fullName,
					Arguments: args,
				},
			},
			Origin: toolinvocations.Origin{Type: toolinvocations.OriginChat, ID: turnID},
			DryRun: false,
			Result: result,
			// Sem sinalização de timeout/cancel no contrato do MCP event hoje.
			ErrorKind:    errKind,
			ErrorMessage: errMsg,
			Retryable:    false,
			DurationMs:   0,
		})
		if recErr != nil {
			log.Printf("[MCP Native] Erro ao registrar tool invocation (id=%s): %v", ev.ID, recErr)
		}
	}
}

func resolveMCPServerSlug(ctx context.Context, serverLabel string) (string, bool) {
	userID, err := database.RequireUserID(ctx)
	if err != nil {
		return "", false
	}
	label := strings.TrimSpace(serverLabel)
	if label == "" {
		return "", false
	}
	normalized := strings.ToLower(label)
	var server database.MCPServer

	// Caminho rápido: slug é único e indexado.
	err = database.DB().WithContext(ctx).
		Where("user_id = ? AND slug = ?", userID, normalized).
		First(&server).Error
	if err == nil {
		return server.Slug, true
	}

	// Segundo caminho: match exato por name (normalmente igual ao label emitido).
	err = database.DB().WithContext(ctx).
		Where("user_id = ? AND name = ?", userID, label).
		First(&server).Error
	if err == nil {
		return server.Slug, true
	}

	// Fallback: busca case-insensitive por name.
	err = database.DB().WithContext(ctx).
		Where("user_id = ? AND LOWER(name) = ?", userID, normalized).
		First(&server).Error
	if err != nil {
		return "", false
	}
	return server.Slug, true
}

// recoverFromPanic captura panic e delega o tratamento para events.HandlePanic.
// recover() deve ser chamado diretamente no corpo da função adiada — não pode ser delegado.
func (s *Service) recoverFromPanic(conversationID string, source string) {
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

func selectedToolsFromCatalog(results []tools.ToolExecutionResult) []string {
	var selected []string
	seen := map[string]struct{}{}
	for _, result := range results {
		if result.ToolName != tools.ToolCatalogName || result.Result.IsError {
			continue
		}
		var payload struct {
			SelectedTools []string `json:"selected_tools"`
		}
		if err := json.Unmarshal([]byte(result.Result.Content), &payload); err != nil {
			log.Printf("[Agent] resposta inválida de %s: %v", tools.ToolCatalogName, err)
			continue
		}
		for _, name := range payload.SelectedTools {
			name = strings.TrimSpace(name)
			if name == "" || name == tools.ToolCatalogName {
				continue
			}
			if _, ok := seen[name]; ok {
				continue
			}
			seen[name] = struct{}{}
			selected = append(selected, name)
		}
	}
	return selected
}

func appendUniqueToolDefs(existing []llm.ToolDefinition, additions ...llm.ToolDefinition) []llm.ToolDefinition {
	if len(additions) == 0 {
		return existing
	}
	seen := make(map[string]struct{}, len(existing)+len(additions))
	for _, def := range existing {
		seen[def.Function.Name] = struct{}{}
	}
	for _, def := range additions {
		if def.Function.Name == "" {
			continue
		}
		if _, ok := seen[def.Function.Name]; ok {
			continue
		}
		existing = append(existing, def)
		seen[def.Function.Name] = struct{}{}
	}
	return existing
}

func expandToolDefsFromCatalogResults(
	existing []llm.ToolDefinition,
	results []tools.ToolExecutionResult,
	resolveToolDefs func([]string) []llm.ToolDefinition,
) []llm.ToolDefinition {
	if resolveToolDefs == nil {
		return existing
	}
	return appendUniqueToolDefs(existing, resolveToolDefs(selectedToolsFromCatalog(results))...)
}

func (s *Service) executeToolCalls(ctx context.Context, calls []tools.ToolCall, origin toolinvocations.Origin) []tools.ToolExecutionResult {
	if s.toolInvocations == nil {
		return s.toolExecutor.ExecuteAll(ctx, calls)
	}
	results := s.toolInvocations.ExecuteAll(ctx, calls, origin)
	out := make([]tools.ToolExecutionResult, len(results))
	for i, result := range results {
		out[i] = result.Execution
	}
	return out
}

func (s *Service) executeToolCall(ctx context.Context, call tools.ToolCall, origin toolinvocations.Origin) tools.ToolExecutionResult {
	if s.toolInvocations == nil {
		return s.toolExecutor.ExecuteOne(ctx, call)
	}
	return s.toolInvocations.Execute(ctx, toolinvocations.ExecuteRequest{Call: call, Origin: origin}).Execution
}

func truncateString(s string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	if len(s) <= maxLen {
		return s
	}

	const suffix = "..."
	if maxLen <= len(suffix) {
		return suffix[:maxLen]
	}

	cutoff := maxLen - len(suffix)
	// UTF-8 safe: recua até achar limite de rune válido
	for cutoff > 0 && !utf8.RuneStart(s[cutoff]) {
		cutoff--
	}
	return s[:cutoff] + suffix
}
