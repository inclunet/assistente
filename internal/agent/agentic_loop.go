package agent

import (
	"assistente/internal/logging"
	"context"
	"errors"
	"sort"
	"strings"

	"assistente/internal/chat"
	"assistente/internal/core/ports"
	"assistente/internal/events"
	"assistente/internal/llm"
	"assistente/internal/toolinvocations"
	"assistente/internal/tools"
	"assistente/internal/tools/invocationctx"
)

// agenticLoopRunner mantém o estado de uma execução de RunAgenticLoop.
//
// A decomposição (AEP-0077 Fase 0, #245) extrai as responsabilidades do loop
// agêntico em métodos coesos e testáveis isoladamente, sem alterar comportamento:
//   - streamIteration: streaming do LLM com auto-retry e fallback MCP nativo→adapter.
//   - finishFinalResult: caminho finish_reason="stop".
//   - executeToolIteration: caminho finish_reason="tool_calls".
//   - finishLimitReached: encerramento ao atingir o teto de iterações.
//
// O estado mutável compartilhado entre iterações (mensagens, streamer/tools
// ativos e acumuladores de estatísticas) fica nos campos do runner.
type agenticLoopRunner struct {
	svc *Service

	// Configuração imutável do turno.
	conversationID     string
	turnID             string
	assistantMessageID string
	params             llm.ChatParams
	surfaceOrigin      *ports.ChatSurfaceOrigin
	newHandler         func(conversationID string, iteration int) IterationHandler

	maxIterations            int
	streamingRecoveryEnabled bool
	maxRecoveryAttempts      int

	// Estado ativo: pode ser trocado em runtime pelo fallback nativo→adapter
	// (AEP-0021). Quando o modelo rejeita MCP nativo, re-tentamos o MESMO turno
	// com as bridge tools (modo adapter), preservando as tools.
	messages       []llm.Message
	activeStreamer llm.Streamer
	activeToolDefs []llm.ToolDefinition
	activeResolve  func(active []llm.ToolDefinition, names []string) []llm.ToolDefinition

	// Acumuladores de estatísticas do loop (AEP-0039 Fase 2).
	totalToolCallCount int
	toolsUsedSet       map[string]struct{}
	lastUsage          llm.Usage
}

// run orquestra o loop de iterações preservando a ordem/semântica original:
//  1. Chama o LLM com streaming (texto aparece em tempo real via newHandler)
//  2. Se finish_reason="stop" → salva resposta final, emite chat:done
//  3. Se finish_reason="tool_calls" → emite segment_done, executa tools, salva, repete
func (r *agenticLoopRunner) run(ctx context.Context) {
	for iteration := 0; iteration < r.maxIterations; iteration++ {
		// Verifica cancelamento
		if ctx.Err() != nil {
			logging.Infof(ctx, "agent.agentic-loop", "[Agent] loop cancelado na iteração %d", iteration)
			r.svc.emitAgenticContextDone(ctx, r.conversationID, r.turnID, r.assistantMessageID, r.surfaceOrigin, iteration-1, r.totalToolCallCount, r.toolsUsedSet)
			return
		}

		// 1. Streaming do LLM (com auto-retry e fallback MCP nativo→adapter).
		result, streamErr, stop := r.streamIteration(ctx, iteration)
		if stop {
			return
		}

		// Acumula usage da última iteração (AEP-0039)
		if result.Usage.PromptTokens > 0 || result.Usage.CompletionTokens > 0 {
			r.lastUsage = result.Usage
		}

		// 2. Erro?
		if streamErr != "" {
			logging.Errorf(ctx, "agent.agentic-loop", "[Agent] erro na iteração %d: %s", iteration, result.Error)
			// chat:done é o evento terminal canônico — inclui ErrorMessage para que
			// adapters (CLI, frontend) exibam o erro sem depender de chat:stream terminal.
			r.svc.emitter.Emit("chat:done", r.buildErrorDoneEvent(streamErr, iteration))
			return
		}

		// 3. finish_reason="stop" → resposta final
		if result.IsDone {
			r.finishFinalResult(ctx, result, iteration)
			return
		}

		// TTS proativo: verbaliza segmentos intermediários (não interrompe áudio anterior).
		if r.svc.onSpeechRequest != nil && result.FullResponse != "" {
			r.svc.onSpeechRequest(r.conversationID, "", "assistant", result.FullResponse, "segment", r.params.ProfileSlug, false)
		}

		// 4. finish_reason="tool_calls" → executar ferramentas
		nextCtx, stopTools := r.executeToolIteration(ctx, result, iteration)
		if stopTools {
			return
		}
		ctx = nextCtx
	}

	// Atingiu limite de iterações
	r.finishLimitReached(ctx)
}

// streamIteration executa o streaming do LLM para uma iteração, com auto-retry
// opcional e o fallback MCP nativo→adapter no mesmo turno (AEP-0021, AEP-0064).
//
// Retorna o resultado da iteração, a mensagem de erro terminal de streaming (vazia
// em sucesso) e stop=true quando o cancelamento já foi tratado (chat:done emitido)
// e o loop deve retornar imediatamente.
func (r *agenticLoopRunner) streamIteration(ctx context.Context, iteration int) (AgenticResult, string, bool) {
	var result AgenticResult
	var lastStreamErr string
	attempts := 1
	if r.streamingRecoveryEnabled {
		attempts = r.maxRecoveryAttempts
	}
	for attempt := 1; attempt <= attempts; attempt++ {
		if ctx.Err() != nil {
			r.svc.emitAgenticContextDone(ctx, r.conversationID, r.turnID, r.assistantMessageID, r.surfaceOrigin, iteration-1, r.totalToolCallCount, r.toolsUsedSet)
			return result, "", true
		}
		handler := r.newHandler(r.conversationID, iteration)
		if setter, ok := handler.(interface{ SetAssistantMessageID(string) }); ok {
			setter.SetAssistantMessageID(r.assistantMessageID)
		}
		var setInitialContent func(string)
		if prefillSetter, ok := handler.(interface{ SetInitialContent(string) }); ok {
			setInitialContent = prefillSetter.SetInitialContent
		}
		r.messages = r.svc.applyContinuationPrefill(ctx, r.messages, r.params, r.assistantMessageID, setInitialContent)
		r.activeStreamer.StreamChat(ctx, r.messages, r.params, handler, r.activeToolDefs...)
		// Fallback nativo→adapter no MESMO turno: se o provider sinalizou que o
		// modelo não suporta MCP nativo, troca para o streamer/tools em modo
		// adapter (bridges presentes) e re-tenta esta iteração SEM consumir uma
		// tentativa de recovery. Consume() garante que isso ocorre só uma vez.
		if fb := r.params.NativeMCPFallback; fb != nil && fb.Consume() {
			logging.Infof(ctx, "agent.agentic-loop", "[Agent] MCP nativo não suportado (iteração %d): re-tentando o mesmo turno em modo adapter com bridge tools", iteration)
			if fb.Streamer != nil {
				r.activeStreamer = fb.Streamer
			}
			r.activeToolDefs = fb.ToolDefs
			if fb.ResolveToolDefs != nil {
				r.activeResolve = fb.ResolveToolDefs
			}
			attempt--
			continue
		}
		result = handler.Result()
		if ctx.Err() != nil {
			if partialHandler, ok := handler.(interface{ Finalize() (string, string) }); ok {
				partialContent, partialReasoning := partialHandler.Finalize()
				r.svc.persistAssistantPartialBestEffort(ctx, r.assistantMessageID, partialContent, partialReasoning)
			}
			r.svc.emitAgenticContextDone(ctx, r.conversationID, r.turnID, r.assistantMessageID, r.surfaceOrigin, iteration, r.totalToolCallCount, r.toolsUsedSet)
			return result, "", true
		}
		if result.Error == "" {
			lastStreamErr = ""
			break
		}
		lastStreamErr = result.Error
		if attempt == attempts {
			if partialHandler, ok := handler.(interface{ Finalize() (string, string) }); ok {
				partialContent, partialReasoning := partialHandler.Finalize()
				r.svc.persistAssistantPartialBestEffort(ctx, r.assistantMessageID, partialContent, partialReasoning)
			}
		}
		if attempt < attempts {
			logging.Errorf(ctx, "agent.agentic-loop", "[Agent] streaming interrompido (iteração %d, tentativa %d/%d): %s", iteration, attempt, attempts, result.Error)
			continue
		}
	}
	return result, lastStreamErr, false
}

// finishFinalResult trata o caminho finish_reason="stop": contabiliza eventuais
// MCP calls nativas, emite chat:segment_done final e delega a SaveAndFinish.
func (r *agenticLoopRunner) finishFinalResult(ctx context.Context, result AgenticResult, iteration int) {
	// MCP nativo pode aparecer em uma resposta final (finish_reason="stop").
	// A persistência ocorre em SaveAndFinish; aqui só atualizamos stats.
	if len(result.NativeMCPEvents) > 0 {
		for _, ev := range result.NativeMCPEvents {
			if !ev.IsCompleted {
				continue
			}
			r.totalToolCallCount++
			r.toolsUsedSet[ev.Name] = struct{}{}
		}
	}

	if result.FullResponse != "" {
		r.svc.emitter.Emit("chat:segment_done", ports.SegmentDoneEvent{
			ConversationID:     r.conversationID,
			TurnID:             r.turnID,
			AssistantMessageID: r.assistantMessageID,
			Content:            result.FullResponse,
			Iteration:          iteration,
			HasMore:            false,
			SurfaceOrigin:      r.surfaceOrigin,
		})
	}

	r.svc.SaveAndFinish(ctx, r.conversationID, r.turnID, r.assistantMessageID, result, r.params.ProfileSlug, &LoopStats{
		IterationCount: iteration + 1,
		ToolCallCount:  r.totalToolCallCount,
		ToolsUsed:      r.toolsUsedSet,
		LastUsage:      r.lastUsage,
	}, r.surfaceOrigin)
}

// executeToolIteration trata o caminho finish_reason="tool_calls": persiste MCP
// nativo, executa as bridge tools (com retry), emite eventos, persiste resultados
// e emite o segment_done da iteração. Retorna o contexto atualizado (pode ser
// trocado por controles de runtime/skill) e stop=true quando o turno/conversa
// deixou de existir e o loop deve abortar.
func (r *agenticLoopRunner) executeToolIteration(ctx context.Context, result AgenticResult, iteration int) (context.Context, bool) {
	// 5a. Persiste MCP calls nativas desta iteração antes das bridge calls
	iterationNativeTools := r.persistAndAccountNativeMCP(ctx, result, iteration)

	// 5b. Adiciona mensagem do assistant ao histórico para próxima iteração
	// (Persistência no DB movida para após execução — AEP-0039 Fase 5)
	r.messages = append(r.messages, llm.Message{
		Role:      "assistant",
		Content:   result.FullResponse,
		ToolCalls: result.ToolCalls,
	})

	// 5d. Executa ferramentas em paralelo
	toolCalls := convertToolCalls(result.ToolCalls)
	// Defesa (repo-driven): evita iniciar execução/persistência de tools se o
	// turno já não existe (ou pertence a outra conversa).
	if !r.turnStillValid(ctx) {
		return ctx, true
	}
	r.svc.emitToolStarts(r.conversationID, r.turnID, r.assistantMessageID, result.ToolCalls, r.surfaceOrigin)
	execBatch := r.svc.executeToolCallsWithRuntimeControls(ctx, toolCalls, toolinvocations.Origin{Type: toolinvocations.OriginChat, ID: r.turnID}, r.conversationID, r.turnID, iteration, r.surfaceOrigin)
	ctx = execBatch.Context
	execResults := execBatch.Executions

	// 5e. Retry automático para erros retryable (AEP-0039 Fase 3)
	retriedCallIDs := r.retryRetryableTools(ctx, toolCalls, execResults, execBatch.PersistedByCallID, iteration)

	// 5f. Emit tool_end/failure events e acumula stats
	iterationTools := r.emitToolEndsAndAccount(execResults, retriedCallIDs)
	r.activeToolDefs = expandToolDefsFromCatalogResults(r.activeToolDefs, execResults, r.activeResolve)

	// 5f-ii. AEP-0039 Fase 4: pre-check de context window — trunca resultados se necessário.
	// Usa cópia para truncamento; o conteúdo original é preservado para persistência no DB.
	toolContents := make([]string, len(execResults))
	for i, res := range execResults {
		toolContents[i] = res.Result.Content
	}
	preCheck := PreCheckContextWindow(r.params.ContextWindow, r.params.MaxTokens, r.messages, toolContents)

	// 5f-iii. Persiste o texto intermediário do assistant. AEP-0078 depreca o L3:
	// novas mensagens não gravam mais o JSON tool_calls; o snapshot exibível fica
	// em tool_invocations.metadata, associado por tool_call_id.
	assistantToolMsg, err := r.svc.msgRepo.AddAssistantToolMessage(
		ctx,
		r.conversationID,
		r.turnID,
		result.FullResponse,
		"",
		result.Reasoning,
		result.Model,
	)
	if err != nil {
		if errors.Is(err, chat.ErrConversationDeleted) {
			logging.Errorf(ctx, "agent.agentic-loop", "[Agent] conversa %s deletada — abortando", r.conversationID)
			return ctx, true
		}
		logging.Errorf(ctx, "agent.agentic-loop", "[Agent] erro ao salvar assistant com tool_calls: %v", err)
	} else if assistantToolMsg != nil {
		r.svc.tagChatToolInvocationsWithAssistantMessage(ctx, r.turnID, execResults, assistantToolMsg.ID)
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
		// PersistedByCallID indica que a linha técnica foi escrita. A associação
		// call↔result agora vem de tool_invocations.tool_call_id, então a falha de
		// salvar a mensagem intermediária não exige fallback role=tool.
		persisted := execBatch.PersistedByCallID[execResult.CallID]
		if !persisted {
			persistedContent := execResult.Result.Content
			if _, err := r.svc.msgRepo.AddToolResultMessage(ctx, r.conversationID, r.turnID, persistedContent, execResult.CallID); err != nil {
				if errors.Is(err, chat.ErrConversationDeleted) {
					logging.Errorf(ctx, "agent.agentic-loop", "[Agent] conversa %s deletada durante tool execution — abortando", r.conversationID)
					return ctx, true
				}
				logging.Errorf(ctx, "agent.agentic-loop", "[Agent] erro ao salvar tool result message (fallback): %v", err)
			}
		}
		r.messages = append(r.messages, llm.Message{
			Role:       "tool",
			Content:    content,
			ToolCallID: execResult.CallID,
		})
	}

	// 5g. Emite token stats atualizadas em tempo real
	r.emitTokenStatsUpdate()

	// 5g. Emite segment_done com resumo de tools da iteração (AEP-0039)
	allIterTools := append(iterationNativeTools, iterationTools...)
	r.svc.emitter.Emit("chat:segment_done", ports.SegmentDoneEvent{
		ConversationID:     r.conversationID,
		TurnID:             r.turnID,
		AssistantMessageID: r.assistantMessageID,
		Content:            result.FullResponse,
		Iteration:          iteration,
		HasMore:            true,
		ToolsInIteration:   allIterTools,
		SurfaceOrigin:      r.surfaceOrigin,
	})
	return ctx, false
}

// persistAndAccountNativeMCP persiste as MCP calls nativas da iteração e
// contabiliza as completas nas estatísticas do loop, retornando o resumo para o
// segment_done (AEP-0039 Fase 1).
func (r *agenticLoopRunner) persistAndAccountNativeMCP(ctx context.Context, result AgenticResult, iteration int) []ports.ToolSummary {
	if len(result.NativeMCPEvents) == 0 {
		return nil
	}
	r.svc.persistNativeMCPCalls(ctx, r.conversationID, r.turnID, result.NativeMCPEvents, iteration)
	var iterationNativeTools []ports.ToolSummary
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
			r.totalToolCallCount++
			r.toolsUsedSet[ev.Name] = struct{}{}
		}
	}
	return iterationNativeTools
}

// turnStillValid valida, de forma repo-driven, que a mensagem do turno ainda
// existe e pertence à conversa atual antes de executar/persistir tools. Não
// acessa o DB global diretamente para permitir repos alternativos/mocks.
func (r *agenticLoopRunner) turnStillValid(ctx context.Context) bool {
	if r.svc.msgRepo == nil {
		// Se o repo não está configurado, não temos como validar; segue o fluxo.
		return true
	}
	turnMsg, err := r.svc.msgRepo.GetMessage(ctx, r.turnID)
	if err != nil {
		logging.Errorf(ctx, "agent.agentic-loop", "[Agent] turn message %s não existe mais antes de executar tools: %v", r.turnID, err)
		return false
	}
	if turnMsg == nil {
		// Defesa: um repo/mock pode devolver (nil, nil). Tratamos como turno
		// inexistente e abortamos a execução de tools (mesmo efeito do erro),
		// evitando panic ao acessar turnMsg.ConversationID.
		logging.Errorf(ctx, "agent.agentic-loop", "[Agent] turn message %s retornou nil sem erro antes de executar tools; abortando", r.turnID)
		return false
	}
	turnConv := strings.TrimSpace(turnMsg.ConversationID)
	conv := strings.TrimSpace(r.conversationID)
	// Se o repo não popula ConversationID, não temos como validar; segue o fluxo.
	if turnConv != "" && conv != "" && turnConv != conv {
		logging.Errorf(ctx, "agent.agentic-loop", "[Agent] turn message %s pertence a outra conversa (%s); abortando execução de tools", r.turnID, turnMsg.ConversationID)
		return false
	}
	return true
}

// retryRetryableTools re-executa tools com erro retryable (AEP-0039 Fase 3),
// emitindo a sequência de eventos tool_end(falha)→tool_failure→tool_start(retry).
// Substitui os resultados re-tentados in-place em execResults e persistedByCallID,
// retornando o conjunto de CallIDs que foram re-tentados.
func (r *agenticLoopRunner) retryRetryableTools(ctx context.Context, toolCalls []tools.ToolCall, execResults []tools.ToolExecutionResult, persistedByCallID map[string]bool, iteration int) map[string]struct{} {
	retriedCallIDs := make(map[string]struct{})
	for i, execResult := range execResults {
		if execResult.Result.IsError && execResult.Retryable && iteration < r.maxIterations-1 {
			retriedCallIDs[execResult.CallID] = struct{}{}
			retryOrigin, retryServerLabel := detectToolOrigin(execResult.ToolName)
			retryName := extractLogicalToolName(execResult.ToolName)
			// Emite tool_end para a tentativa que falhou (attempt=0)
			EmitToolEnd(r.svc.emitter, ports.ToolEndEvent{
				ConversationID:     r.conversationID,
				TurnID:             r.turnID,
				AssistantMessageID: r.assistantMessageID,
				Name:               retryName,
				CallID:             execResult.CallID,
				Status:             "error",
				Summary:            truncateString(execResult.Result.Content, MaxResultDisplaySize),
				Origin:             retryOrigin,
				ServerLabel:        retryServerLabel,
				DurationMs:         execResult.DurationMs,
				Attempt:            0,
				SurfaceOrigin:      r.surfaceOrigin,
			})
			// Emite tool_failure com willRetry=true
			EmitToolFailure(r.svc.emitter, ports.ToolFailureEvent{
				ConversationID:     r.conversationID,
				TurnID:             r.turnID,
				AssistantMessageID: r.assistantMessageID,
				Name:               retryName,
				CallID:             execResult.CallID,
				ErrorKind:          string(execResult.ErrorKind),
				Retryable:          true,
				Message:            truncateString(execResult.Result.Content, MaxResultDisplaySize),
				DurationMs:         execResult.DurationMs,
				Origin:             retryOrigin,
				WillRetry:          true,
				Attempt:            0,
				SurfaceOrigin:      r.surfaceOrigin,
			})
			logging.Errorf(ctx, "agent.agentic-loop", "[Agent] tool %s falhou (kind=%s), tentando retry...", retryName, execResult.ErrorKind)
			// Emite tool_start para a nova tentativa (attempt=1)
			EmitToolStart(r.svc.emitter, ports.ToolStartEvent{
				ConversationID:     r.conversationID,
				TurnID:             r.turnID,
				AssistantMessageID: r.assistantMessageID,
				Name:               retryName,
				CallID:             execResult.CallID,
				Args:               toolCalls[i].Function.Arguments,
				Origin:             retryOrigin,
				ServerLabel:        retryServerLabel,
				Attempt:            1,
				SurfaceOrigin:      r.surfaceOrigin,
			})
			retried, retriedPersisted := r.svc.executeToolCall(ctx, toolCalls[i], toolinvocations.Origin{Type: toolinvocations.OriginChat, ID: r.turnID}, iteration)
			execResults[i] = retried
			persistedByCallID[retried.CallID] = retriedPersisted
		}
	}
	return retriedCallIDs
}

// emitToolEndsAndAccount emite tool_end (e tool_failure quando aplicável) para
// cada execução e acumula as estatísticas do loop, retornando o resumo das tools
// da iteração (AEP-0039).
func (r *agenticLoopRunner) emitToolEndsAndAccount(execResults []tools.ToolExecutionResult, retriedCallIDs map[string]struct{}) []ports.ToolSummary {
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
		EmitToolEnd(r.svc.emitter, ports.ToolEndEvent{
			ConversationID:     r.conversationID,
			TurnID:             r.turnID,
			AssistantMessageID: r.assistantMessageID,
			Name:               logicalName,
			CallID:             execResult.CallID,
			Status:             status,
			Summary:            truncateString(execResult.Result.Content, MaxResultDisplaySize),
			Origin:             origin,
			ServerLabel:        serverLabel,
			DurationMs:         execResult.DurationMs,
			Attempt:            attempt,
			SurfaceOrigin:      r.surfaceOrigin,
		})

		// AEP-0039 Fase 3: emite tool_failure para erros classificados (sem retry)
		if execResult.Result.IsError && execResult.ErrorKind != "" {
			EmitToolFailure(r.svc.emitter, ports.ToolFailureEvent{
				ConversationID:     r.conversationID,
				TurnID:             r.turnID,
				AssistantMessageID: r.assistantMessageID,
				Name:               logicalName,
				CallID:             execResult.CallID,
				ErrorKind:          string(execResult.ErrorKind),
				Retryable:          execResult.Retryable,
				Message:            truncateString(execResult.Result.Content, MaxResultDisplaySize),
				DurationMs:         execResult.DurationMs,
				Origin:             origin,
				Attempt:            attempt,
				SurfaceOrigin:      r.surfaceOrigin,
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
		r.totalToolCallCount++
		r.toolsUsedSet[logicalName] = struct{}{}
	}
	return iterationTools
}

// emitTokenStatsUpdate consulta as estatísticas de tokens da conversa e emite
// chat:token_stats_update em tempo real (no-op se getTokenStats não estiver configurado).
func (r *agenticLoopRunner) emitTokenStatsUpdate() {
	if r.svc.getTokenStats == nil {
		return
	}
	stats, err := r.svc.getTokenStats(r.conversationID)
	if err != nil || stats == nil {
		return
	}
	r.svc.emitter.Emit("chat:token_stats_update", ports.TokenStatsUpdateEvent{
		ConversationID:              r.conversationID,
		PromptTokens:                stats.PromptTokens,
		CompletionTokens:            stats.CompletionTokens,
		TotalTokens:                 stats.TotalTokens,
		CacheReadTokens:             stats.CacheReadTokens,
		CacheWriteTokens:            stats.CacheWriteTokens,
		CacheMissTokens:             stats.CacheMissTokens,
		CacheHitRate:                stats.CacheHitRate,
		CacheTokensReported:         stats.CacheTokensReported,
		PromptCacheEnabled:          stats.PromptCacheEnabled,
		ContextTokens:               stats.ContextTokens,
		ContextUsage:                stats.ContextUsage,
		ContextLimit:                stats.ContextLimit,
		IsNearLimit:                 stats.IsNearLimit,
		IsCritical:                  stats.IsCritical,
		MessageCount:                stats.MessageCount,
		ModelCallCount:              stats.ModelCallCount,
		SystemPromptEstimatedTokens: stats.SystemPromptEstimatedTokens,
		SummaryTokens:               stats.SummaryTokens,
		MessagesInContextCount:      stats.MessagesInContextCount,
		MessagesInContextTokens:     stats.MessagesInContextTokens,
		MessagesOutOfContextCount:   stats.MessagesOutOfContextCount,
		MessagesOutOfContextTokens:  stats.MessagesOutOfContextTokens,
		ToolsUsedCount:              stats.ToolsUsedCount,
		ToolBreakdown:               stats.ToolBreakdown,
	})
}

// finishLimitReached emite os eventos terminais quando o loop atinge o teto de
// iterações (chat:stream informativo + chat:done com Reason="limit_reached").
func (r *agenticLoopRunner) finishLimitReached(ctx context.Context) {
	logging.Infof(ctx, "agent.agentic-loop", "[Agent] limite de %d iterações atingido para conversa %s", r.maxIterations, r.conversationID)
	r.svc.emitter.Emit("chat:stream", events.StreamEvent{
		Content:        "Limite de iterações do agente atingido. A resposta pode estar incompleta.",
		Done:           true,
		MessageID:      r.assistantMessageID,
		ConversationId: r.conversationID,
		TurnID:         r.turnID,
		SurfaceOrigin:  r.surfaceOrigin,
	})
	r.svc.emitter.Emit("chat:done", ports.DoneEvent{
		ConversationID:     r.conversationID,
		TurnID:             r.turnID,
		AssistantMessageID: r.assistantMessageID,
		HadToolCalls:       r.totalToolCallCount > 0,
		Reason:             "limit_reached",
		IterationCount:     r.maxIterations,
		ToolCallCount:      r.totalToolCallCount,
		ToolsUsed:          sortedToolNames(r.toolsUsedSet),
		PromptTokens:       r.lastUsage.PromptTokens,
		CompletionTokens:   r.lastUsage.CompletionTokens,
		CacheReadTokens:    r.lastUsage.CacheReadTokens,
		CacheWriteTokens:   r.lastUsage.CacheWriteTokens,
		CacheMissTokens:    r.lastUsage.CacheMissTokens,
		SurfaceOrigin:      r.surfaceOrigin,
	})

	if r.svc.triggerSummarize != nil {
		go func() {
			defer r.svc.recoverFromPanic(r.conversationID, "triggerSummarize")
			r.svc.triggerSummarize(ctx, r.conversationID, r.params.ProfileSlug)
		}()
	}
}

// buildErrorDoneEvent monta o chat:done terminal de erro de streaming, incluindo
// as estatísticas acumuladas do loop até a iteração corrente.
func (r *agenticLoopRunner) buildErrorDoneEvent(errMessage string, iteration int) ports.DoneEvent {
	return ports.DoneEvent{
		ConversationID:     r.conversationID,
		TurnID:             r.turnID,
		AssistantMessageID: r.assistantMessageID,
		SurfaceOrigin:      r.surfaceOrigin,
		HadToolCalls:       r.totalToolCallCount > 0,
		Reason:             "error",
		ErrorMessage:       errMessage,
		IterationCount:     iteration + 1,
		ToolCallCount:      r.totalToolCallCount,
		ToolsUsed:          sortedToolNames(r.toolsUsedSet),
		PromptTokens:       r.lastUsage.PromptTokens,
		CompletionTokens:   r.lastUsage.CompletionTokens,
		CacheReadTokens:    r.lastUsage.CacheReadTokens,
		CacheWriteTokens:   r.lastUsage.CacheWriteTokens,
		CacheMissTokens:    r.lastUsage.CacheMissTokens,
	}
}

// resolveAgenticMaxIterations resolve o teto de iterações do loop: usa o valor do
// perfil (params) quando positivo, caindo no config do executor caso contrário.
// O executor só é consultado quando params não define o valor (lazy), preservando
// o comportamento de chamadores que rodam sem executor configurado.
func resolveAgenticMaxIterations(params llm.ChatParams, executor *tools.Executor) int {
	if params.MaxAgenticIterations > 0 {
		return params.MaxAgenticIterations
	}
	return executor.Config().MaxIterations
}

// normalizeRecoveryMaxAttempts normaliza o número máximo de tentativas de recovery
// de streaming: default 3 quando não informado, com piso de 1.
func normalizeRecoveryMaxAttempts(maxAttempts int) int {
	if maxAttempts <= 0 {
		maxAttempts = 3
	}
	if maxAttempts < 1 {
		maxAttempts = 1
	}
	return maxAttempts
}

// buildAgenticInvocationContext propaga o contexto de invocação para as tools.
// Sempre carrega a identidade da conversa/turno/profile (AEP-0068, usado pela
// tool `subagent`); os campos de superfície (tab/arquivo) só quando presentes.
func buildAgenticInvocationContext(ctx context.Context, params llm.ChatParams, conversationID, turnID string) context.Context {
	return invocationctx.With(ctx, invocationctx.InvocationContext{
		TabType:        params.TabType,
		ActiveFilePath: params.ActiveFilePath,
		SurfaceState:   chat.DecodeSurfaceJSONMap(params.SurfaceStateJSON, "[agent] surface state payload"),
		SurfaceContext: chat.DecodeSurfaceJSONMap(params.SurfaceContextJSON, "[agent] surface context payload"),
		ConversationID: conversationID,
		TurnID:         turnID,
		ProfileSlug:    params.ProfileSlug,
	})
}

// buildEnrichedToolCalls monta os EnrichedToolCall para persistência da mensagem
// assistant (AEP-0039 Fase 5 / AEP-0063), incluindo origin, server_label, iteração
// e a duração da execução correspondente quando disponível.
func buildEnrichedToolCalls(toolCalls []llm.ToolCall, execResults []tools.ToolExecutionResult, iteration int) []llm.EnrichedToolCall {
	enrichedCalls := make([]llm.EnrichedToolCall, len(toolCalls))
	// AEP-0063 (D2): não persistir tool results como parte das mensagens.
	// O resultado é efêmero em tool_invocations e hidratado sob demanda.
	for i, tc := range toolCalls {
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
	return enrichedCalls
}

// sortedToolNames retorna os nomes de tools de um set ordenados alfabeticamente.
// Sempre retorna uma slice não-nil (vazia quando o set é vazio) para manter a
// serialização de eventos estável.
func sortedToolNames(set map[string]struct{}) []string {
	names := make([]string, 0, len(set))
	for name := range set {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
