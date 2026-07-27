package agent

import (
	"assistente/internal/logging"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	// TriggerSummarize dispara a verificação/sumarização da conversa. Recebe o
	// conversationID e o profileSlug DA CONVERSA (o mesmo resolvido no envio),
	// para que o resumo use o provider/modelo do perfil da conversa e não o do
	// perfil ativo global (Issue #203).
	TriggerSummarize func(context.Context, string, string)
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
	triggerSummarize func(context.Context, string, string)
	onSpeechRequest  func(conversationID string, messageID string, role, text, origin, profileSlug string, interrupt bool)
}

// StreamSimpleWithRecovery executa um streaming simples (sem tool calling) com auto-retry opcional.
// Em tentativas intermediárias, suprime o erro terminal para não finalizar o streaming no frontend.
// Se o contexto for cancelado, emite chat:done como evento terminal canônico.
func (s *Service) StreamSimpleWithRecovery(
	ctx context.Context,
	streamer llm.Streamer,
	messages []llm.Message,
	params llm.ChatParams,
	conversationID string,
	turnID string,
	profileSlug string,
	surfaceOrigin *ports.ChatSurfaceOrigin,
	streamingRecoveryEnabled bool,
	streamingRecoveryMaxAttempts int,
) {
	if streamer == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	maxAttempts := normalizeRecoveryMaxAttempts(streamingRecoveryMaxAttempts)
	attempts := 1
	if streamingRecoveryEnabled {
		attempts = maxAttempts
	}

	for attempt := 1; attempt <= attempts; attempt++ {
		if ctx.Err() != nil {
			s.emitSimpleContextDone(ctx, conversationID, turnID, "", surfaceOrigin)
			return
		}
		h, err := s.NewSimpleStreamHandler(ctx, conversationID, turnID, profileSlug, surfaceOrigin)
		if errors.Is(err, chat.ErrConversationGone) {
			return
		}
		if err != nil {
			logging.Errorf(ctx, "agent.service", "[Chat] falha ao criar/reusar placeholder assistant (conversa %s, turno %s): %v", conversationID, turnID, err)
			s.emitPlaceholderErrorDone(conversationID, turnID, surfaceOrigin)
			return
		}
		messages = s.applyContinuationPrefill(ctx, messages, params, h.AssistantMessageID, h.SetInitialContent)
		// Só a última tentativa deve finalizar o streaming com erro.
		h.SuppressTerminalError(attempt < attempts)
		streamer.StreamChat(ctx, messages, params, h)
		if ctx.Err() != nil {
			partialContent, partialReasoning := h.Finalize()
			s.persistAssistantPartialBestEffort(ctx, h.AssistantMessageID, partialContent, partialReasoning)
			s.emitSimpleContextDone(ctx, conversationID, turnID, h.AssistantMessageID, surfaceOrigin)
			return
		}
		if h.LastError() == "" {
			return
		}
		if attempt == attempts {
			partialContent, partialReasoning := h.Finalize()
			s.persistAssistantPartialBestEffort(ctx, h.AssistantMessageID, partialContent, partialReasoning)
		}
		if attempt < attempts {
			logging.Errorf(context.Background(), "agent.service", "[Chat] streaming interrompido (conversa %s, tentativa %d/%d): %s", conversationID, attempt, attempts, h.LastError())
		}
	}
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
	resolveToolDefs func(active []llm.ToolDefinition, names []string) []llm.ToolDefinition,
	streamingRecoveryEnabled bool,
	streamingRecoveryMaxAttempts int,
) {
	if streamer == nil {
		errMsg := "Cliente LLM não disponível para o agentic loop. Verifique a configuração do provedor."
		logging.Errorf(ctx, "agent.service", "🔴 [AGENT] streamer nil na conversa %s", conversationID)
		s.emitter.Emit("chat:done", ports.DoneEvent{
			ConversationID: conversationID,
			TurnID:         turnID,
			SurfaceOrigin:  surfaceOrigin,
			Reason:         "error",
			ErrorMessage:   errMsg,
		})
		return
	}

	assistantMessageID, err := chat.EnsureAssistantPlaceholder(ctx, s.msgRepo, conversationID, turnID)
	if errors.Is(err, chat.ErrConversationGone) {
		return
	}
	if err != nil {
		logging.Errorf(ctx, "agent.service", "[Agent] falha ao criar/reusar placeholder assistant (conversa %s, turno %s): %v", conversationID, turnID, err)
		s.emitPlaceholderErrorDone(conversationID, turnID, surfaceOrigin)
		return
	}

	// Propaga contexto de invocação para as tools (AEP-0068).
	ctx = buildAgenticInvocationContext(ctx, params, conversationID, turnID)

	runner := &agenticLoopRunner{
		svc:                      s,
		conversationID:           conversationID,
		turnID:                   turnID,
		assistantMessageID:       assistantMessageID,
		params:                   params,
		surfaceOrigin:            surfaceOrigin,
		newHandler:               newHandler,
		maxIterations:            resolveAgenticMaxIterations(params, s.toolExecutor),
		streamingRecoveryEnabled: streamingRecoveryEnabled,
		maxRecoveryAttempts:      normalizeRecoveryMaxAttempts(streamingRecoveryMaxAttempts),
		messages:                 messages,
		activeStreamer:           streamer,
		activeToolDefs:           toolDefs,
		activeResolve:            resolveToolDefs,
		toolsUsedSet:             map[string]struct{}{},
	}
	runner.run(ctx)
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
	assistantMessageID string,
	result AgenticResult,
	profileSlug string,
	loopStats *LoopStats,
	surfaceOrigin *ports.ChatSurfaceOrigin,
) {
	var savedMsgID string
	if conversationID != "" && turnID != "" && len(result.NativeMCPEvents) > 0 {
		finalIteration := 0
		if loopStats != nil && loopStats.IterationCount > 0 {
			finalIteration = loopStats.IterationCount - 1
		}
		s.persistNativeMCPCalls(ctx, conversationID, turnID, result.NativeMCPEvents, finalIteration)
	}
	if conversationID != "" && result.FullResponse != "" {

		opts := chat.MessageOptions{
			ConversationID:   conversationID,
			Role:             "assistant",
			Content:          result.FullResponse,
			Reasoning:        result.Reasoning,
			PromptTokens:     result.Usage.PromptTokens,
			CompletionTokens: result.Usage.CompletionTokens,
			TotalTokens:      result.Usage.TotalTokens,
			CacheReadTokens:  result.Usage.CacheReadTokens,
			CacheWriteTokens: result.Usage.CacheWriteTokens,
			CacheMissTokens:  result.Usage.CacheMissTokens,
			Model:            result.Model,
		}
		if turnID != "" {
			opts.TurnID = &turnID
		}

		var err error
		savedMsgID, err = chat.FinalizeAssistantMessage(ctx, s.msgRepo, assistantMessageID, opts)
		if errors.Is(err, chat.ErrConversationGone) {
			return
		}
		if err != nil {
			logging.Errorf(ctx, "agent.service", "[Agent] erro ao salvar resposta final: %v", err)
		}
	}
	if savedMsgID == "" {
		savedMsgID = assistantMessageID
	}

	if s.responseNotifier != nil {
		s.responseNotifier.NotifyContext(ctx, conversationID, result.FullResponse, savedMsgID)
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
		doneEvent.CacheReadTokens = loopStats.LastUsage.CacheReadTokens
		doneEvent.CacheWriteTokens = loopStats.LastUsage.CacheWriteTokens
		doneEvent.CacheMissTokens = loopStats.LastUsage.CacheMissTokens
	}
	if doneEvent.PromptTokens == 0 && result.Usage.PromptTokens > 0 {
		doneEvent.PromptTokens = result.Usage.PromptTokens
	}
	if doneEvent.CompletionTokens == 0 && result.Usage.CompletionTokens > 0 {
		doneEvent.CompletionTokens = result.Usage.CompletionTokens
	}
	if doneEvent.CacheReadTokens == 0 {
		doneEvent.CacheReadTokens = result.Usage.CacheReadTokens
	}
	if doneEvent.CacheWriteTokens == 0 {
		doneEvent.CacheWriteTokens = result.Usage.CacheWriteTokens
	}
	if doneEvent.CacheMissTokens == 0 {
		doneEvent.CacheMissTokens = result.Usage.CacheMissTokens
	}
	s.emitter.Emit("chat:done", doneEvent)

	if s.triggerSummarize != nil {
		go func() {
			defer s.recoverFromPanic(conversationID, "triggerSummarize")
			s.triggerSummarize(ctx, conversationID, profileSlug)
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
		ConversationID:      conversationID,
		TotalTokens:         stats.TotalTokens,
		ContextTokens:       stats.ContextTokens,
		ContextLimit:        stats.ContextLimit,
		ContextUsage:        stats.ContextUsage,
		IsNearLimit:         stats.IsNearLimit,
		IsCritical:          stats.IsCritical,
		PromptTokens:        stats.PromptTokens,
		CompletionTokens:    stats.CompletionTokens,
		CacheReadTokens:     stats.CacheReadTokens,
		CacheWriteTokens:    stats.CacheWriteTokens,
		CacheMissTokens:     stats.CacheMissTokens,
		CacheHitRate:        stats.CacheHitRate,
		CacheTokensReported: stats.CacheTokensReported,
		PromptCacheEnabled:  stats.PromptCacheEnabled,
		MessageCount:        stats.MessageCount,
		ModelCallCount:      stats.ModelCallCount,
	})
	if stats.IsCritical {
		logging.Warnf(context.Background(), "agent.service", "[Context] conversa %s em CRÍTICO: %0.1f%% (%d/%d tokens)",
			conversationID, stats.ContextUsage, stats.ContextTokens, stats.ContextLimit)
		s.emitter.Emit("chat:context_warning", ports.ContextWarningEvent{
			ConversationID: conversationID,
			Level:          "critical",
			Message: fmt.Sprintf("Atenção: Contexto em %0.1f%% (%d/%d tokens). Considere limpar a conversa ou resumir o histórico.",
				stats.ContextUsage, stats.ContextTokens, stats.ContextLimit),
			Percentage:    stats.ContextUsage,
			ContextTokens: stats.ContextTokens,
			ContextLimit:  stats.ContextLimit,
		})
	} else if stats.IsNearLimit {
		logging.Warnf(context.Background(), "agent.service", "[Context] conversa %s próxima do limite: %0.1f%% (%d/%d tokens)",
			conversationID, stats.ContextUsage, stats.ContextTokens, stats.ContextLimit)
		s.emitter.Emit("chat:context_warning", ports.ContextWarningEvent{
			ConversationID: conversationID,
			Level:          "warning",
			Message: fmt.Sprintf("Contexto em %0.1f%% (%d/%d tokens). Considere limpar a conversa em breve.",
				stats.ContextUsage, stats.ContextTokens, stats.ContextLimit),
			Percentage:    stats.ContextUsage,
			ContextTokens: stats.ContextTokens,
			ContextLimit:  stats.ContextLimit,
		})
	}
}

func (s *Service) emitToolStarts(conversationID string, turnID string, assistantMessageID string, calls []llm.ToolCall, surfaceOrigin *ports.ChatSurfaceOrigin) {
	for _, call := range calls {
		origin, serverLabel := detectToolOrigin(call.Function.Name)
		name := extractLogicalToolName(call.Function.Name)
		EmitToolStart(s.emitter, ports.ToolStartEvent{
			ConversationID:     conversationID,
			TurnID:             turnID,
			AssistantMessageID: assistantMessageID,
			Name:               name,
			CallID:             call.ID,
			Args:               call.Function.Arguments,
			Origin:             origin,
			ServerLabel:        serverLabel,
			SurfaceOrigin:      surfaceOrigin,
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
	// Defesa (repo-driven): se o turno foi deletado enquanto o provider streamava,
	// não registrar tool_invocations para evitar registros órfãos.
	if s.msgRepo != nil {
		if turnMsg, err := s.msgRepo.GetMessage(ctx, turnID); err != nil {
			logging.Infof(ctx, "agent.service", "[MCP Native] turn message %s não existe mais; ignorando persistência de MCP events: %v", turnID, err)
			return
		} else {
			turnConv := strings.TrimSpace(turnMsg.ConversationID)
			conv := strings.TrimSpace(conversationID)
			if turnConv != "" && conv != "" && turnConv != conv {
				logging.Infof(ctx, "agent.service", "[MCP Native] turn message %s pertence a outra conversa (%s); ignorando persistência de MCP events", turnID, turnMsg.ConversationID)
				return
			}
		}
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

	// Persistência do output: AEP-0063 (D2) evita armazenar tool results como mensagens.
	// O output completo fica efêmero em tool_invocations; se a persistência estiver
	// indisponível, o fallback role=tool é usado para manter histórico/export legível.
	// IMPORTANTE: grava os fallbacks APÓS a mensagem assistant tool_calls para manter
	// a ordem tool-call -> tool-result no histórico/export.
	formatFallbackContent := func(output, errMsg string) string {
		if strings.TrimSpace(errMsg) == "" {
			return output
		}
		// Mantém um marcador explícito para consumidores de histórico/export.
		// Evita duplicar se o backend já prefixou.
		trimmed := strings.TrimSpace(errMsg)
		if strings.HasPrefix(trimmed, "Error:") || strings.HasPrefix(trimmed, "ERROR:") {
			return trimmed
		}
		return "Error: " + trimmed
	}
	type fallbackToolResult struct {
		CallID  string
		Content string
	}
	persistable := s.toolInvocations != nil && s.toolInvocations.CanPersist()
	fallbackResults := make([]fallbackToolResult, 0)
	if !persistable {
		for _, ev := range mcpEvents {
			if !ev.IsCompleted {
				continue
			}
			content := formatFallbackContent(ev.Output, ev.Error)
			fallbackResults = append(fallbackResults, fallbackToolResult{CallID: ev.ID, Content: content})
		}
	}

	if persistable {
		// Resultados técnicos: persistir em tool_invocations quando disponível.
		// Não criar novas mensagens role=tool em caso de sucesso (export/import lê tool_calls enriquecido).
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
				logging.Errorf(ctx, "agent.service", "[MCP Native] não foi possível resolver server slug para %q; usando fallback role=tool (id=%s)", ev.ServerLabel, ev.ID)
				content := formatFallbackContent(ev.Output, ev.Error)
				fallbackResults = append(fallbackResults, fallbackToolResult{CallID: ev.ID, Content: content})
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
				Origin:    toolinvocations.Origin{Type: toolinvocations.OriginChat, ID: turnID},
				DryRun:    false,
				Iteration: iteration,
				Result:    result,
				// Sem sinalização de timeout/cancel no contrato do MCP event hoje.
				ErrorKind:    errKind,
				ErrorMessage: errMsg,
				Retryable:    false,
				DurationMs:   0,
			})
			if recErr != nil {
				logging.Errorf(ctx, "agent.service", "[MCP Native] Erro ao registrar tool invocation (id=%s): %v", ev.ID, recErr)
				// Fallback: garante que exista ao menos um resultado persistido
				// para o tool_call_id no histórico da conversa.
				content := formatFallbackContent(ev.Output, ev.Error)
				fallbackResults = append(fallbackResults, fallbackToolResult{CallID: ev.ID, Content: content})
				continue
			}
		}
	}

	hasCompletedToolCall := false
	for _, ev := range mcpEvents {
		if !ev.IsCompleted {
			continue
		}
		hasCompletedToolCall = true
	}
	if !hasCompletedToolCall {
		return
	}
	assistantMarker, err := s.msgRepo.AddAssistantToolMessage(ctx, conversationID, turnID, "", "", "", "")
	if err != nil {
		logging.Errorf(ctx, "agent.service", "[MCP Native] Erro ao salvar marcador assistant de tools: %v", err)
		// Ainda assim, tenta persistir resultados como role=tool (melhor que perder output).
		for _, ev := range mcpEvents {
			if !ev.IsCompleted {
				continue
			}
			callID := strings.TrimSpace(ev.ID)
			if callID == "" {
				continue
			}
			content := strings.TrimSpace(formatFallbackContent(ev.Output, ev.Error))
			if content == "" {
				continue
			}
			if _, err2 := s.msgRepo.AddToolResultMessage(ctx, conversationID, turnID, content, callID); err2 != nil {
				logging.Errorf(ctx, "agent.service", "[MCP Native] Erro ao salvar tool result message (fallback, id=%s): %v", callID, err2)
			}
		}
		return
	}
	if assistantMarker != nil {
		execResults := make([]tools.ToolExecutionResult, 0, len(mcpEvents))
		for _, ev := range mcpEvents {
			if ev.IsCompleted {
				execResults = append(execResults, tools.ToolExecutionResult{CallID: ev.ID})
			}
		}
		s.tagChatToolInvocationsWithAssistantMessage(ctx, turnID, execResults, assistantMarker.ID)
	}
	for _, fb := range fallbackResults {
		if strings.TrimSpace(fb.CallID) == "" {
			continue
		}
		if _, err := s.msgRepo.AddToolResultMessage(ctx, conversationID, turnID, fb.Content, fb.CallID); err != nil {
			logging.Errorf(ctx, "agent.service", "[MCP Native] Erro ao salvar tool result message (fallback, id=%s): %v", fb.CallID, err)
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

	// Segundo caminho: match exato por name, mas com desambiguação (name não é único).
	var servers []database.MCPServer
	err = database.DB().WithContext(ctx).
		Where("user_id = ? AND name = ?", userID, label).
		Limit(2).
		Find(&servers).Error
	if err == nil {
		if len(servers) == 1 {
			return servers[0].Slug, true
		}
		if len(servers) > 1 {
			logging.Infof(ctx, "agent.service", "[MCP Native] server label %q é ambíguo (name duplicado); não persistindo por slug", serverLabel)
			return "", false
		}
	}

	// Terceiro caminho: busca case-insensitive por name, também exige unicidade.
	servers = nil
	err = database.DB().WithContext(ctx).
		Where("user_id = ? AND LOWER(name) = ?", userID, normalized).
		Limit(2).
		Find(&servers).Error
	if err != nil {
		return "", false
	}
	if len(servers) == 1 {
		return servers[0].Slug, true
	}
	if len(servers) > 1 {
		logging.Errorf(ctx, "agent.service", "[MCP Native] server label %q é ambíguo (LOWER(name) duplicado); não persistindo por slug", serverLabel)
		return "", false
	}
	return "", false
}

// recoverFromPanic captura panic e delega o tratamento para events.HandlePanic.
// recover() deve ser chamado diretamente no corpo da função adiada — não pode ser delegado.
func (s *Service) recoverFromPanic(conversationID string, source string) {
	r := recover()
	events.HandlePanic(s.emitter, conversationID, source, r)
}

func (s *Service) HandleRecoveredPanic(
	ctx context.Context,
	conversationID string,
	turnID string,
	source string,
	r any,
	surfaceOrigin *ports.ChatSurfaceOrigin,
) {
	if r == nil {
		return
	}
	logging.Errorf(ctx, "agent.service", "🔴 [PANIC RECOVERED] %s (conversa %s): %v", source, conversationID, r)

	assistantMessageID := ""
	if s.msgRepo != nil && turnID != "" {
		if ctx == nil {
			ctx = context.Background()
		}
		msgID, err := chat.EnsureAssistantPlaceholder(context.WithoutCancel(ctx), s.msgRepo, conversationID, turnID)
		if errors.Is(err, chat.ErrConversationGone) {
			return
		}
		if err != nil {
			logging.Errorf(ctx, "agent.service", "[Agent] falha ao criar/reusar placeholder assistant após panic (conversa %s, turno %s): %v", conversationID, turnID, err)
		} else {
			assistantMessageID = msgID
		}
	}

	if s.emitter != nil {
		s.emitter.Emit("chat:done", ports.DoneEvent{
			ConversationID:     conversationID,
			TurnID:             turnID,
			AssistantMessageID: assistantMessageID,
			SurfaceOrigin:      surfaceOrigin,
			Reason:             "error",
			ErrorMessage:       ports.ChatErrorInternal,
		})
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

func selectedToolsFromCatalog(results []tools.ToolExecutionResult) []string {
	var selected []string
	seen := map[string]struct{}{}
	for _, result := range results {
		if result.ToolName != tools.ToolCatalogName || result.Result.IsError {
			continue
		}
		var payload struct {
			LoadedTools []string `json:"loaded_tools"`
		}
		if err := json.Unmarshal([]byte(result.Result.Content), &payload); err != nil {
			logging.Infof(context.Background(), "agent.service", "[Agent] resposta inválida de %s: %v", tools.ToolCatalogName, err)
			continue
		}
		for _, name := range payload.LoadedTools {
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

func unloadedToolsFromCatalog(results []tools.ToolExecutionResult) []string {
	var unloaded []string
	seen := map[string]struct{}{}
	for _, result := range results {
		if result.ToolName != tools.ToolCatalogName || result.Result.IsError {
			continue
		}
		var payload struct {
			UnloadedTools []string `json:"unloaded_tools"`
		}
		if err := json.Unmarshal([]byte(result.Result.Content), &payload); err != nil {
			logging.Infof(context.Background(), "agent.service", "[Agent] resposta inválida de %s: %v", tools.ToolCatalogName, err)
			continue
		}
		for _, name := range payload.UnloadedTools {
			name = strings.TrimSpace(name)
			if name == "" || name == tools.ToolCatalogName {
				continue
			}
			if _, ok := seen[name]; ok {
				continue
			}
			seen[name] = struct{}{}
			unloaded = append(unloaded, name)
		}
	}
	return unloaded
}

func removeToolDefs(existing []llm.ToolDefinition, names []string) []llm.ToolDefinition {
	if len(existing) == 0 || len(names) == 0 {
		return existing
	}
	remove := make(map[string]struct{}, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name != "" && name != tools.ToolCatalogName {
			remove[name] = struct{}{}
		}
	}
	if len(remove) == 0 {
		return existing
	}
	filtered := existing[:0]
	for _, def := range existing {
		if _, ok := remove[def.Function.Name]; ok {
			continue
		}
		filtered = append(filtered, def)
	}
	return filtered
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
	resolveToolDefs func(active []llm.ToolDefinition, names []string) []llm.ToolDefinition,
) []llm.ToolDefinition {
	existing = removeToolDefs(existing, unloadedToolsFromCatalog(results))
	if resolveToolDefs == nil {
		return existing
	}
	// resolveToolDefs orça o conjunto ACUMULADO (ativas + novas) via ToolPlanner e
	// já devolve o resultado final do turno — sem novo append/dedup aqui.
	return resolveToolDefs(existing, selectedToolsFromCatalog(results))
}

func applyLoadedSkillExecutionContext(ctx context.Context, results []tools.ToolExecutionResult, emitter events.Emitter, conversationID, turnID string, surfaceOrigin *ports.ChatSurfaceOrigin) context.Context {
	for _, result := range results {
		if result.ToolName != tools.LoadSkillName || result.Result.IsError || result.Result.Metadata == nil {
			continue
		}
		emitLoadedSkillEvent(emitter, result.Result.Metadata, conversationID, turnID, surfaceOrigin)
		read := metadataStringSlice(result.Result.Metadata, "filesystem_read")
		write := metadataStringSlice(result.Result.Metadata, "filesystem_write")
		deny := metadataStringSlice(result.Result.Metadata, "filesystem_deny")
		allowedTools := metadataStringSlice(result.Result.Metadata, "tools_allowed")
		deniedTools := metadataStringSlice(result.Result.Metadata, "tools_denied")
		allowedBash := metadataStringSlice(result.Result.Metadata, "bash_commands_allowed")
		deniedBash := metadataStringSlice(result.Result.Metadata, "bash_commands_denied")
		networkAllowed := metadataStringSlice(result.Result.Metadata, "network_allowed_hosts")
		networkDenied := metadataStringSlice(result.Result.Metadata, "network_denied_hosts")
		ec, _ := tools.GetExecutionContext(ctx)
		if len(read) > 0 || len(write) > 0 || len(deny) > 0 {
			ec.Filesystem = mergeFilesystemScope(ec.Filesystem, &tools.FilesystemScope{
				Read:  read,
				Write: write,
				Deny:  deny,
			})
		}
		ec.AllowedTools = appendUniqueStrings(ec.AllowedTools, allowedTools...)
		ec.DeniedTools = appendUniqueStrings(ec.DeniedTools, deniedTools...)
		ec.AllowedBash = appendUniqueStrings(ec.AllowedBash, allowedBash...)
		ec.DeniedBash = appendUniqueStrings(ec.DeniedBash, deniedBash...)
		ec.NetworkAllowedHost = appendUniqueStrings(ec.NetworkAllowedHost, networkAllowed...)
		ec.NetworkDeniedHost = appendUniqueStrings(ec.NetworkDeniedHost, networkDenied...)
		if ec.InvokedSkillSlug == "" {
			if slug, _ := result.Result.Metadata["skill_slug"].(string); strings.TrimSpace(slug) != "" {
				ec.InvokedSkillSlug = strings.TrimSpace(slug)
			}
		}
		ctx = tools.WithExecutionContext(ctx, ec)
	}
	return ctx
}

func emitLoadedSkillEvent(emitter events.Emitter, metadata map[string]any, conversationID, turnID string, surfaceOrigin *ports.ChatSurfaceOrigin) {
	if emitter == nil {
		return
	}
	slug, _ := metadata["skill_slug"].(string)
	name, _ := metadata["skill_name"].(string)
	slug = strings.TrimSpace(slug)
	name = strings.TrimSpace(name)
	if slug == "" {
		return
	}
	emitter.Emit("chat:skill_loaded", ports.SkillLoadedEvent{
		ConversationID: conversationID,
		TurnID:         turnID,
		Slug:           slug,
		DisplayName:    name,
		Mode:           "on_demand",
		SurfaceOrigin:  surfaceOrigin,
	})
}

func mergeFilesystemScope(existing, next *tools.FilesystemScope) *tools.FilesystemScope {
	if existing == nil {
		return &tools.FilesystemScope{
			Read:  append([]string{}, next.Read...),
			Write: append([]string{}, next.Write...),
			Deny:  append([]string{}, next.Deny...),
		}
	}
	return &tools.FilesystemScope{
		Read:  appendUniqueStrings(existing.Read, next.Read...),
		Write: appendUniqueStrings(existing.Write, next.Write...),
		Deny:  appendUniqueStrings(existing.Deny, next.Deny...),
	}
}

func metadataStringSlice(metadata map[string]any, key string) []string {
	raw, ok := metadata[key]
	if !ok || raw == nil {
		return nil
	}
	switch value := raw.(type) {
	case []string:
		return append([]string{}, value...)
	case []any:
		out := make([]string, 0, len(value))
		for _, item := range value {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, strings.TrimSpace(s))
			}
		}
		return out
	default:
		return nil
	}
}

func appendUniqueStrings(existing []string, additions ...string) []string {
	out := append([]string{}, existing...)
	seen := make(map[string]struct{}, len(out)+len(additions))
	for _, item := range out {
		seen[item] = struct{}{}
	}
	for _, item := range additions {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		out = append(out, item)
		seen[item] = struct{}{}
	}
	return out
}

type toolExecutionBatch struct {
	Executions        []tools.ToolExecutionResult
	PersistedByCallID map[string]bool
	Context           context.Context
}

func (s *Service) executeToolCallsWithRuntimeControls(ctx context.Context, calls []tools.ToolCall, origin toolinvocations.Origin, conversationID, turnID string, iteration int, surfaceOrigin *ports.ChatSurfaceOrigin) toolExecutionBatch {
	// Runtime control tools can change the execution context for the rest of the
	// batch. Run load_skill first even when the model emitted it after regular
	// tools, then place results back in the original order for persistence/history.
	loadSkillCalls := make([]tools.ToolCall, 0)
	loadSkillIndexes := make([]int, 0)
	regularCalls := make([]tools.ToolCall, 0, len(calls))
	regularIndexes := make([]int, 0, len(calls))
	for i, call := range calls {
		if call.Function.Name == tools.LoadSkillName {
			loadSkillCalls = append(loadSkillCalls, call)
			loadSkillIndexes = append(loadSkillIndexes, i)
			continue
		}
		regularCalls = append(regularCalls, call)
		regularIndexes = append(regularIndexes, i)
	}
	if len(loadSkillCalls) == 0 {
		batch := s.executeToolCalls(ctx, calls, origin, iteration)
		batch.Context = ctx
		return batch
	}

	executions := make([]tools.ToolExecutionResult, len(calls))
	persisted := make(map[string]bool, len(calls))
	loadBatch := s.executeToolCalls(ctx, loadSkillCalls, origin, iteration)
	for i, result := range loadBatch.Executions {
		executions[loadSkillIndexes[i]] = result
		persisted[result.CallID] = loadBatch.PersistedByCallID[result.CallID]
	}
	ctx = applyLoadedSkillExecutionContext(ctx, loadBatch.Executions, s.emitter, conversationID, turnID, surfaceOrigin)

	if len(regularCalls) > 0 {
		regularBatch := s.executeToolCalls(ctx, regularCalls, origin, iteration)
		for i, result := range regularBatch.Executions {
			executions[regularIndexes[i]] = result
			persisted[result.CallID] = regularBatch.PersistedByCallID[result.CallID]
		}
	}
	return toolExecutionBatch{Executions: executions, PersistedByCallID: persisted, Context: ctx}
}

func (s *Service) executeToolCalls(ctx context.Context, calls []tools.ToolCall, origin toolinvocations.Origin, iteration int) toolExecutionBatch {
	if s.toolInvocations == nil {
		execs := s.toolExecutor.ExecuteAll(ctx, calls)
		persisted := make(map[string]bool, len(execs))
		for _, r := range execs {
			persisted[r.CallID] = false
		}
		return toolExecutionBatch{Executions: execs, PersistedByCallID: persisted, Context: ctx}
	}
	results := s.toolInvocations.ExecuteAll(ctx, calls, origin, iteration)
	out := make([]tools.ToolExecutionResult, len(results))
	persisted := make(map[string]bool, len(results))
	for i, result := range results {
		out[i] = result.Execution
		persisted[result.Execution.CallID] = result.Persisted
	}
	return toolExecutionBatch{Executions: out, PersistedByCallID: persisted, Context: ctx}
}

func (s *Service) executeToolCall(ctx context.Context, call tools.ToolCall, origin toolinvocations.Origin, iteration int) (tools.ToolExecutionResult, bool) {
	if s.toolInvocations == nil {
		return s.toolExecutor.ExecuteOne(ctx, call), false
	}
	res := s.toolInvocations.Execute(ctx, toolinvocations.ExecuteRequest{Call: call, Origin: origin, Iteration: iteration})
	return res.Execution, res.Persisted
}

func (s *Service) tagChatToolInvocationsWithAssistantMessage(ctx context.Context, turnID string, execResults []tools.ToolExecutionResult, assistantMessageID string) {
	turnID = strings.TrimSpace(turnID)
	assistantMessageID = strings.TrimSpace(assistantMessageID)
	if turnID == "" || assistantMessageID == "" || len(execResults) == 0 {
		return
	}
	db := database.DB()
	if db == nil || !db.Migrator().HasTable(&database.ToolInvocation{}) {
		return
	}
	callIDs := make([]string, 0, len(execResults))
	seen := map[string]struct{}{}
	for _, result := range execResults {
		callID := strings.TrimSpace(result.CallID)
		if callID == "" {
			continue
		}
		if _, ok := seen[callID]; ok {
			continue
		}
		seen[callID] = struct{}{}
		callIDs = append(callIDs, callID)
	}
	if len(callIDs) == 0 {
		return
	}
	var rows []database.ToolInvocation
	if err := database.ScopeByUser(ctx, db.WithContext(ctx).Model(&database.ToolInvocation{}), "user_id").
		Select("id", "metadata").
		Where("origin_type = ? AND origin_id = ? AND tool_call_id IN ?", toolinvocations.OriginChat, turnID, callIDs).
		Find(&rows).Error; err != nil {
		logging.Warnf(ctx, "agent.service", "[Agent] falha ao carregar tool_invocations para marcar assistant_message_id: %v", err)
		return
	}
	for _, row := range rows {
		var metadata map[string]any
		if strings.TrimSpace(row.Metadata) != "" {
			_ = json.Unmarshal([]byte(row.Metadata), &metadata)
		}
		if metadata == nil {
			metadata = map[string]any{}
		}
		display, _ := metadata["display"].(map[string]any)
		if display == nil {
			display = map[string]any{"version": 1}
		}
		if currentID, _ := display["assistant_message_id"].(string); strings.TrimSpace(currentID) == assistantMessageID {
			continue
		}
		display["assistant_message_id"] = assistantMessageID
		metadata["display"] = display
		encoded, err := json.Marshal(metadata)
		if err != nil {
			continue
		}
		if err := database.ScopeByUser(ctx, db.WithContext(ctx).Model(&database.ToolInvocation{}), "user_id").
			Where("id = ?", row.ID).
			Update("metadata", string(encoded)).Error; err != nil {
			logging.Warnf(ctx, "agent.service", "[Agent] falha ao marcar tool_invocation %s com assistant_message_id: %v", row.ID, err)
		}
	}
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

func (s *Service) loadAssistantPrefill(ctx context.Context, assistantMessageID string) string {
	assistantMessageID = strings.TrimSpace(assistantMessageID)
	if assistantMessageID == "" || s.msgRepo == nil {
		return ""
	}
	msg, err := s.msgRepo.GetMessage(ctx, assistantMessageID)
	if err != nil || msg == nil {
		return ""
	}
	return msg.Content
}

func (s *Service) persistAssistantPartialBestEffort(ctx context.Context, assistantMessageID, content, reasoning string) {
	assistantMessageID = strings.TrimSpace(assistantMessageID)
	if assistantMessageID == "" || strings.TrimSpace(content) == "" || s.msgRepo == nil {
		return
	}
	persistCtx := context.WithoutCancel(ctx)

	var (
		promptTokens     int
		completionTokens int
		totalTokens      int
		model            string
	)
	if msg, err := s.msgRepo.GetMessage(persistCtx, assistantMessageID); err == nil && msg != nil {
		promptTokens = msg.PromptTokens
		completionTokens = msg.CompletionTokens
		totalTokens = msg.TotalTokens
		model = msg.Model
		if strings.TrimSpace(reasoning) == "" {
			reasoning = msg.Reasoning
		}
	}

	if err := s.msgRepo.UpdateMessageContentAndReasoning(persistCtx, assistantMessageID, content, reasoning, promptTokens, completionTokens, totalTokens, model); err != nil {
		logging.Warnf(ctx, "agent.service", "[Agent] aviso: falha ao persistir conteúdo parcial da mensagem assistant %s: %v", assistantMessageID, err)
	}
}

func (s *Service) emitSimpleContextDone(
	ctx context.Context,
	conversationID string,
	turnID string,
	assistantMessageID string,
	surfaceOrigin *ports.ChatSurfaceOrigin,
) {
	err := ctx.Err()
	if err == nil || s.emitter == nil {
		return
	}
	errorMessage := "geração cancelada"
	if errors.Is(err, context.DeadlineExceeded) {
		errorMessage = "tempo limite da geração atingido"
	}
	s.emitter.Emit("chat:done", ports.DoneEvent{
		ConversationID:     conversationID,
		TurnID:             turnID,
		AssistantMessageID: assistantMessageID,
		SurfaceOrigin:      surfaceOrigin,
		Reason:             "error",
		ErrorMessage:       errorMessage,
	})
}

func (s *Service) emitPlaceholderErrorDone(
	conversationID string,
	turnID string,
	surfaceOrigin *ports.ChatSurfaceOrigin,
) {
	if s.emitter == nil {
		return
	}
	s.emitter.Emit("chat:done", ports.DoneEvent{
		ConversationID: conversationID,
		TurnID:         turnID,
		SurfaceOrigin:  surfaceOrigin,
		Reason:         "error",
		ErrorMessage:   ports.ChatErrorAssistantPlaceholder,
	})
}

func (s *Service) emitAgenticContextDone(
	ctx context.Context,
	conversationID string,
	turnID string,
	assistantMessageID string,
	surfaceOrigin *ports.ChatSurfaceOrigin,
	iteration int,
	toolCallCount int,
	toolsUsedSet map[string]struct{},
) {
	err := ctx.Err()
	if err == nil || s.emitter == nil {
		return
	}
	errorMessage := "geração cancelada"
	if errors.Is(err, context.DeadlineExceeded) {
		errorMessage = "tempo limite da geração atingido"
	}
	s.emitter.Emit("chat:done", ports.DoneEvent{
		ConversationID:     conversationID,
		TurnID:             turnID,
		AssistantMessageID: assistantMessageID,
		SurfaceOrigin:      surfaceOrigin,
		HadToolCalls:       toolCallCount > 0,
		Reason:             "error",
		ErrorMessage:       errorMessage,
		IterationCount:     iteration + 1,
		ToolCallCount:      toolCallCount,
		ToolsUsed:          sortedToolNames(toolsUsedSet),
	})
}

// patchTrailingAssistantPrefill substitui o conteúdo do trailing assistant no prompt.
// Intencionalmente NÃO adiciona uma nova mensagem assistant: isso preserva a regra
// padrão de que o prompt termina em user, exceto quando o histórico já carrega um
// trailing assistant (caso de continuação explícita).
func patchTrailingAssistantPrefill(messages []llm.Message, prefill string) []llm.Message {
	if strings.TrimSpace(prefill) == "" || len(messages) == 0 {
		return messages
	}
	lastIdx := len(messages) - 1
	if strings.TrimSpace(messages[lastIdx].Role) != "assistant" {
		return messages
	}
	messages[lastIdx].Content = prefill
	return messages
}

// buildUserContinuationPrompt monta o conteúdo da mensagem de usuário usada no
// fallback de continuação para providers/modelos sem suporte a assistant prefill
// (Issue #124). O texto parcial é embutido na instrução para que o modelo
// continue exatamente de onde parou, sem repetir o que já foi escrito.
func buildUserContinuationPrompt(prefill string) string {
	return "Continue a resposta a partir deste texto, sem repetir o que já foi escrito e sem reintroduções:\n\n" + prefill
}

// patchTrailingAssistantAsUserContinuation converte o trailing assistant (parcial)
// em uma mensagem de usuário "continue a partir deste texto: ...". É o fallback
// usado quando o provider/modelo não suporta assistant prefill: o prompt volta a
// terminar em user (compatível com qualquer provider, inclusive Qwen/LocalAI que
// rejeitam prefill com enable_thinking) e o texto parcial é preservado na instrução.
func patchTrailingAssistantAsUserContinuation(messages []llm.Message, prefill string) []llm.Message {
	if strings.TrimSpace(prefill) == "" || len(messages) == 0 {
		return messages
	}
	lastIdx := len(messages) - 1
	if strings.TrimSpace(messages[lastIdx].Role) != "assistant" {
		return messages
	}
	messages[lastIdx] = llm.Message{
		Role:    "user",
		Content: buildUserContinuationPrompt(prefill),
	}
	return messages
}

// applyContinuationPrefill prepara o prompt e o handler para uma continuação
// explícita. Centraliza a regra dos dois modos: assistant prefill (suportado)
// vs. fallback por mensagem de usuário (provider não suporta prefill).
// Retorna as mensagens (possivelmente alteradas) e o prefill carregado.
func (s *Service) applyContinuationPrefill(
	ctx context.Context,
	messages []llm.Message,
	params llm.ChatParams,
	assistantMessageID string,
	setInitialContent func(string),
) []llm.Message {
	if !params.AllowAssistantPrefill && !params.ContinueViaUserMessage {
		return messages
	}
	prefill := s.loadAssistantPrefill(ctx, assistantMessageID)
	if prefill == "" {
		return messages
	}
	if params.ContinueViaUserMessage {
		messages = patchTrailingAssistantAsUserContinuation(messages, prefill)
	} else {
		messages = patchTrailingAssistantPrefill(messages, prefill)
	}
	if setInitialContent != nil {
		setInitialContent(prefill)
	}
	return messages
}
