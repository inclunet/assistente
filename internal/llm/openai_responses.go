package llm

import (
	"assistente/internal/logging"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/openai/openai-go/packages/param"
	"github.com/openai/openai-go/responses"
	"github.com/openai/openai-go/shared"
)

// sendChatResponses envia uma mensagem (não-streaming) via Responses API.
func (p *OpenAIProvider) sendChatResponses(ctx context.Context, model string, messages []Message, params ChatParams) (string, error) {
	if !params.AllowAssistantPrefill {
		messages = removeTrailingAssistantPrefill(messages)
	}
	respParams := responses.ResponseNewParams{
		Model: shared.ResponsesModel(model),
		Input: responses.ResponseNewParamsInputUnion{
			OfInputItemList: convertToResponsesInput(messages),
		},
	}
	if params.Temperature > 0 {
		respParams.Temperature = param.NewOpt(params.Temperature)
	}
	if params.MaxTokens > 0 {
		respParams.MaxOutputTokens = param.NewOpt(int64(params.MaxTokens))
	}
	applyPromptCacheKeyToResponses(&respParams, params)
	dumpHandle := dumpLLMRequest(p.provider, model, params, respParams)
	defer pruneDebugDumpHandle(dumpHandle)

	resp, err := p.client.Responses.New(ctx, respParams)
	if err != nil {
		if effectivePromptCacheKey(params) != "" && looksLikePromptCacheHintUnsupported(err.Error()) {
			params.PromptCacheHintFallback.Disable()
			if params.OnPromptCacheHintUnsupported != nil {
				params.OnPromptCacheHintUnsupported()
			}
			params.PromptCacheKey = ""
			return p.sendChatResponses(ctx, model, messages, params)
		}
		return "", fmt.Errorf("erro ao enviar mensagem: %w", err)
	}
	dumpLLMResponse(dumpHandle, params, map[string]any{
		"content":   resp.OutputText(),
		"model":     string(resp.Model),
		"usage":     resp.Usage,
		"usage_raw": rawJSONDump(resp.Usage.RawJSON()),
	})
	return resp.OutputText(), nil
}

func applyPromptCacheKeyToResponses(respParams *responses.ResponseNewParams, params ChatParams) {
	key := effectivePromptCacheKey(params)
	if respParams == nil || key == "" {
		return
	}
	respParams.PromptCacheKey = param.NewOpt(key)
}

// streamChatResponses usa a Responses API como caminho padrão.
// Se há MCP servers configurados, eles são incluídos como tools type:mcp.
// Function tools locais coexistem normalmente.
//
// Limitações conhecidas vs Chat Completions:
//   - Multimodalidade (imagens): user messages com image_url são convertidas como texto puro.
//     A Responses API suporta imagens mas com formato diferente (não image_url parts).
//     TODO: implementar conversão de imagens para o formato Responses quando necessário.
func (p *OpenAIProvider) streamChatResponses(
	ctx context.Context,
	model string,
	messages []Message,
	params ChatParams,
	handler StreamHandler,
	tools ...ToolDefinition,
) {
	params.Model = model
	currentServers := cloneMCPServers(p.mcpServers)
	logging.Infof(ctx, "llm.openai-responses", "[OpenAIProvider] Responses API: %d MCP servers, %d tools locais", len(currentServers), len(tools))

	const maxAttempts = 10
	bk := 500 * time.Millisecond
	maxBk := 8 * time.Second
	degradeRetries := 0
	maxDegradeRetries := maxMCPDegradationRetries(len(currentServers))

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		select {
		case <-ctx.Done():
			handler.OnError("Streaming cancelado: " + ctx.Err().Error())
			return
		default:
		}

		respParams := p.buildResponsesParams(ctx, model, messages, params, currentServers, tools...)
		dumpHandle := dumpLLMRequest(p.provider, model, params, respParams)
		attemptFn := p.responsesAttemptFn
		if attemptFn == nil {
			attemptFn = p.doStreamResponses
		}
		result := attemptFn(ctx, respParams, handler, currentServers, params, dumpHandle)
		pruneDebugDumpHandle(dumpHandle)
		if result.done {
			return
		}
		if result.nativeMCPUnsupported {
			// O modelo/endpoint rejeitou type:"mcp". Dispara o auto-ajuste persistido
			// do perfil (nil→false) e degrada nativo→adapter.
			logging.Infof(ctx, "llm.openai-responses", "[MCP-DEGRADE] attempt=%d provider=openai action=native_to_adapter reason=model_rejects_type_mcp servers=%d", attempt, len(currentServers))
			if params.OnNativeMCPUnsupported != nil {
				params.OnNativeMCPUnsupported()
			}
			if params.NativeMCPFallback != nil {
				// O caller (loop agêntico) re-tenta o MESMO turno em modo adapter, com
				// as bridge tools presentes. Aborta sem emitir done/erro.
				resetStreamAttempt(handler)
				params.NativeMCPFallback.Trigger()
				return
			}
			// Sem fallback configurado (ex.: caminho simples sem tools): degrada
			// dropando os servers nativos e re-tenta "pelado" (sem type:"mcp").
			currentServers = nil
			resetStreamAttempt(handler)
			continue
		}
		if result.promptCacheHintUnsupported {
			if effectivePromptCacheKey(params) != "" {
				logging.Infof(ctx, "llm.openai-responses", "[PromptCache] provider=openai action=disable_provider_hint reason=prompt_cache_key_rejected")
				params.PromptCacheHintFallback.Disable()
				if params.OnPromptCacheHintUnsupported != nil {
					params.OnPromptCacheHintUnsupported()
				}
				params.PromptCacheKey = ""
				resetStreamAttempt(handler)
				continue
			}
			handler.OnError("provider rejeitou prompt_cache_key, mas o hint já estava desativado neste turno; verifique se o gateway/proxy está injetando esse parâmetro ou desative chat.prompt_cache.provider_hints no perfil")
			return
		}
		if result.mcpFailure != nil {
			if degradeRetries < maxDegradeRetries {
				if remaining, ok := planMCPDegradationRetry(ctx, "openai", attempt, currentServers, result.mcpFailure); ok {
					currentServers = remaining
					degradeRetries++
					resetStreamAttempt(handler)
					continue
				}
			}
			handler.OnError(strings.TrimSpace(result.mcpFailure.Message))
			return
		}
		if result.retry {
			if attempt < maxAttempts {
				// Visibilidade: nunca deixar a pessoa no silêncio do backoff.
				notifyTurnNotice(handler, TurnNotice{Kind: TurnNoticeStreamRetry, Count: attempt})
				resetStreamAttempt(handler)
				sleepWithJitter(ctx, bk)
				bk = nextBackoff(bk, maxBk)
				continue
			}
			discardStreamReasoning(handler)
			handler.OnError(streamRetriesExhaustedError)
			return
		}
		return

	}
}

func (p *OpenAIProvider) buildResponsesParams(
	ctx context.Context,
	model string,
	messages []Message,
	params ChatParams,
	mcpServers []MCPServerConfig,
	tools ...ToolDefinition,
) responses.ResponseNewParams {
	if !params.AllowAssistantPrefill {
		messages = removeTrailingAssistantPrefill(messages)
	}
	respParams := responses.ResponseNewParams{
		Model: shared.ResponsesModel(model),
		Input: responses.ResponseNewParamsInputUnion{
			OfInputItemList: convertToResponsesInput(messages),
		},
	}

	if params.Temperature > 0 {
		respParams.Temperature = param.NewOpt(params.Temperature)
	}
	if params.MaxTokens > 0 {
		respParams.MaxOutputTokens = param.NewOpt(int64(params.MaxTokens))
	}
	if params.TopP > 0 && params.TopP != 1.0 {
		respParams.TopP = param.NewOpt(params.TopP)
	}
	applyPromptCacheKeyToResponses(&respParams, params)

	switch params.ReasoningEffort {
	case "low", "medium", "high":
		respParams.Reasoning = shared.ReasoningParam{
			Effort:  shared.ReasoningEffort(params.ReasoningEffort),
			Summary: shared.ReasoningSummaryAuto,
		}
	}

	var respTools []responses.ToolUnionParam
	respTools = append(respTools, buildNativeMCPTools(mcpServers)...)

	for _, tool := range tools {
		var fnParams map[string]any
		if len(tool.Function.Parameters) > 0 {
			if err := json.Unmarshal(tool.Function.Parameters, &fnParams); err != nil {
				logging.Errorf(ctx, "llm.openai-responses", "[OpenAIProvider] Erro ao parsear parameters de %s: %v", tool.Function.Name, err)
				continue
			}
		}
		ft := responses.ToolParamOfFunction(tool.Function.Name, fnParams, false)
		if ft.OfFunction != nil {
			ft.OfFunction.Description = param.NewOpt(tool.Function.Description)
		}
		respTools = append(respTools, ft)
	}

	if len(respTools) > 0 {
		respParams.Tools = respTools
		toolChoice := responses.ToolChoiceOptionsAuto
		if choice, ok := toolChoiceFromContext(ctx); ok {
			if s, ok := choice.(string); ok {
				switch s {
				case "required":
					toolChoice = responses.ToolChoiceOptionsRequired
				case "none":
					toolChoice = responses.ToolChoiceOptionsNone
				default:
					toolChoice = responses.ToolChoiceOptionsAuto
				}
			}
		}
		respParams.ToolChoice = responses.ResponseNewParamsToolChoiceUnion{
			OfToolChoiceMode: param.NewOpt(toolChoice),
		}
	}

	return respParams
}

// doStreamResponses executa streaming via Responses API.
// Trata eventos de texto, function calls locais e MCP (transparente/server-side).
func (p *OpenAIProvider) doStreamResponses(ctx context.Context, params responses.ResponseNewParams, handler StreamHandler, mcpServers []MCPServerConfig, chatParams ChatParams, dumpHandle *DebugDumpHandle) mcpStreamAttemptResult {
	// Watchdog de ociosidade: se o servidor parar de enviar sem fechar a
	// conexão, cancela a leitura e transforma em erro retryable (quando nada
	// foi emitido). Cada evento recebido reinicia a contagem.
	watchCtx, wd := startStreamWatchdog(ctx, streamIdleTimeoutForProvider(p.provider), nil)
	defer wd.Stop()

	stream := p.streamClient.Responses.NewStreaming(watchCtx, params)

	var fullResponse strings.Builder
	var fullReasoning strings.Builder
	// Texto visível e eventos MCP impedem retry automático porque seriam
	// duplicados. Thinking é descartável e não entra nesta barreira.
	var emittedNonRetryableEffect bool
	var lastUsage Usage
	var lastUsageRaw any
	var lastModel string
	var finish FinishInfo
	var isThinking bool
	var thinkingBuffer strings.Builder
	thinkingFinished := false
	finishThinking := func() {
		if thinkingFinished || fullReasoning.Len() == 0 {
			return
		}
		handler.OnThinkingDone(fullReasoning.String())
		thinkingFinished = true
		isThinking = false
		thinkingBuffer.Reset()
	}
	discardThinking := func() {
		if thinkingFinished || fullReasoning.Len() == 0 {
			return
		}
		discardStreamReasoning(handler)
		thinkingFinished = true
		isThinking = false
		thinkingBuffer.Reset()
	}
	reportCurrentDiagnostics := func() {
		model := lastModel
		if model == "" {
			model = chatParams.Model
		}
		currentFinish := finishInfoWithDiagnostics(
			finish, p.provider, model, chatParams.MaxTokens, fullResponse.Len(),
		)
		reportUsage(handler, lastUsage)
		ReportFinishReason(handler, currentFinish)
	}

	type pendingFuncCall struct {
		ID   string
		Name string
		Args strings.Builder
	}
	activeFuncCalls := make(map[string]*pendingFuncCall) // keyed by item_id
	var finishedToolCalls []ToolCall

	activeMCPCalls := make(map[string]*pendingMCPCall) // keyed by item_id

	var eventCount int

	for stream.Next() {
		wd.Kick()
		if ctx.Err() != nil {
			finishThinking()
			return mcpStreamAttemptResult{done: true}
		}
		if wd.TimedOut() {
			if !emittedNonRetryableEffect {
				reportCurrentDiagnostics()
				return mcpStreamAttemptResult{retry: true}
			}
			finishThinking()
			reportCurrentDiagnostics()
			markErrorNotRetryable(handler)
			handler.OnError(streamIdleErrorMessage)
			return mcpStreamAttemptResult{done: true}
		}
		event := stream.Current()
		eventCount++

		switch event.Type {
		case "response.created":
			ev := event.AsResponseCreated()
			if string(ev.Response.Model) != "" {
				lastModel = string(ev.Response.Model)
			}

		case "response.in_progress":
			// Response is being processed, nothing to do

		case "response.output_text.delta":
			ev := event.AsResponseOutputTextDelta()
			if ev.Delta != "" {
				content := processThinkingTags(ev.Delta, &isThinking, &thinkingBuffer, &fullReasoning, handler)
				if content != "" {
					select {
					case <-ctx.Done():
						finishThinking()
						return mcpStreamAttemptResult{done: true}
					default:
					}
					if wd.TimedOut() {
						if !emittedNonRetryableEffect {
							reportCurrentDiagnostics()
							return mcpStreamAttemptResult{retry: true}
						}
						finishThinking()
						reportCurrentDiagnostics()
						markErrorNotRetryable(handler)
						handler.OnError(streamIdleErrorMessage)
						return mcpStreamAttemptResult{done: true}
					}
					fullResponse.WriteString(content)
					emittedNonRetryableEffect = true
					handler.OnChunk(content)
				}
			}

		case "response.output_text.done":
			// Final text for an output item, already accumulated via deltas

		case "response.reasoning_summary_text.delta":
			ev := event.AsResponseReasoningSummaryTextDelta()
			if ev.Delta != "" {
				if ctx.Err() != nil {
					finishThinking()
					return mcpStreamAttemptResult{done: true}
				}
				if wd.TimedOut() {
					if !emittedNonRetryableEffect {
						reportCurrentDiagnostics()
						return mcpStreamAttemptResult{retry: true}
					}
					finishThinking()
					reportCurrentDiagnostics()
					markErrorNotRetryable(handler)
					handler.OnError(streamIdleErrorMessage)
					return mcpStreamAttemptResult{done: true}
				}
				fullReasoning.WriteString(ev.Delta)
				handler.OnThinking(ev.Delta)
				if ctx.Err() != nil {
					finishThinking()
					return mcpStreamAttemptResult{done: true}
				}
			}

		case "response.reasoning_summary_text.done",
			"response.reasoning_summary_part.added",
			"response.reasoning_summary_part.done":
			// Reasoning summary lifecycle events, content already handled via deltas

		case "response.output_item.added":
			ev := event.AsResponseOutputItemAdded()
			switch ev.Item.Type {
			case "function_call":
				activeFuncCalls[ev.Item.ID] = &pendingFuncCall{
					ID:   ev.Item.CallID,
					Name: ev.Item.Name,
				}
			case "mcp_call":
				mc := &pendingMCPCall{
					ID:          ev.Item.ID,
					Name:        ev.Item.Name,
					ServerLabel: ev.Item.ServerLabel,
				}
				activeMCPCalls[ev.Item.ID] = mc
				emittedNonRetryableEffect = true
				handler.OnMCPToolEvent(MCPToolEvent{
					ID:          mc.ID,
					Name:        mc.Name,
					ServerLabel: mc.ServerLabel,
					IsCompleted: false,
				})
			}

		case "response.output_item.done":
			ev := event.AsResponseOutputItemDone()
			if ev.Item.Type == "mcp_call" {
				mc, ok := activeMCPCalls[ev.Item.ID]
				args := ""
				if ok {
					args = mc.Args.String()
				}
				if args == "" {
					args = ev.Item.Arguments
				}
				emittedNonRetryableEffect = true
				handler.OnMCPToolEvent(MCPToolEvent{
					ID:          ev.Item.ID,
					Name:        ev.Item.Name,
					ServerLabel: ev.Item.ServerLabel,
					Arguments:   args,
					Output:      ev.Item.Output,
					Error:       ev.Item.Error,
					IsCompleted: true,
				})
				delete(activeMCPCalls, ev.Item.ID)
			}

		case "response.content_part.added",
			"response.content_part.done":
			// Content part lifecycle events

		case "response.function_call_arguments.delta":
			ev := event.AsResponseFunctionCallArgumentsDelta()
			if fc, ok := activeFuncCalls[ev.ItemID]; ok {
				fc.Args.WriteString(ev.Delta)
			}

		case "response.function_call_arguments.done":
			ev := event.AsResponseFunctionCallArgumentsDone()
			if fc, ok := activeFuncCalls[ev.ItemID]; ok {
				finishedToolCalls = append(finishedToolCalls, ToolCall{
					ID:   fc.ID,
					Type: "function",
					Function: FunctionCall{
						Name:      fc.Name,
						Arguments: fc.Args.String(),
					},
				})
				delete(activeFuncCalls, ev.ItemID)
			}

		case "response.mcp_call_arguments.delta":
			ev := event.AsResponseMcpCallArgumentsDelta()
			if mc, ok := activeMCPCalls[ev.ItemID]; ok {
				mc.Args.WriteString(ev.Delta)
			}

		case "response.mcp_call_arguments.done":
			ev := event.AsResponseMcpCallArgumentsDone()
			if mc, ok := activeMCPCalls[ev.ItemID]; ok {
				mc.Args.Reset()
				mc.Args.WriteString(ev.Arguments)
			}

		case "response.mcp_call.in_progress":
			// Server-side execution in progress, tracking handled via output_item events

		case "response.mcp_call.completed":
			// A finalização canônica (com output/args) ocorre em
			// response.output_item.done. Porém alguns endpoints/proxies não emitem
			// esse output_item.done para itens mcp_call — só este evento, que NÃO
			// carrega output nem arguments. Marcamos o item como concluído e, ao
			// fim do stream, um fallback emite a conclusão para itens que nunca
			// receberam output_item.done (senão a tool sumiria do histórico).
			if ev := event.AsResponseMcpCallCompleted(); ev.ItemID != "" {
				if mc, ok := activeMCPCalls[ev.ItemID]; ok {
					mc.Completed = true
				}
			}

		case "response.mcp_call.failed":
			ev := event.AsResponseMcpCallFailed()
			logging.Errorf(ctx, "llm.openai-responses", "[OpenAIProvider] MCP call FAILED: itemID=%s", ev.ItemID)
			fallbackServer := ""
			if mc, ok := activeMCPCalls[ev.ItemID]; ok {
				fallbackServer = mc.ServerLabel
			}
			if failure := inferMCPFailure(MCPFailureStageCall, "", ev.RawJSON(), fallbackServer, mcpServers); failure != nil && !emittedNonRetryableEffect {
				return mcpStreamAttemptResult{mcpFailure: failure}
			}

		case "response.mcp_list_tools.in_progress":
			logging.Debugf(ctx, "llm.openai-responses", "[OpenAIProvider] MCP listing tools (server-side)")
		case "response.mcp_list_tools.completed":
			logging.Debugf(ctx, "llm.openai-responses", "[OpenAIProvider] MCP tool listing done (server-side)")
		case "response.mcp_list_tools.failed":
			logging.Errorf(ctx, "llm.openai-responses", "[OpenAIProvider] MCP tool listing FAILED (server-side)")
			ev := event.AsResponseMcpListToolsFailed()
			if failure := inferMCPFailure(MCPFailureStageListTools, "", ev.RawJSON(), "", mcpServers); failure != nil && !emittedNonRetryableEffect {
				return mcpStreamAttemptResult{mcpFailure: failure}
			}

		case "response.completed":
			ev := event.AsResponseCompleted()
			// "response.completed" é o tipo do evento, não um finish_reason
			// informado pelo provider. Preserve a ausência no motivo bruto.
			finish = FinishInfo{Reason: FinishReasonStop}
			usageRaw := ev.Response.Usage.RawJSON()
			if openAIUsageReported(usageRaw,
				int(ev.Response.Usage.InputTokens), int(ev.Response.Usage.OutputTokens), int(ev.Response.Usage.TotalTokens)) {
				lastUsageRaw = rawJSONDump(usageRaw)
				lastUsage = UsageFromOpenAIResponses(
					int(ev.Response.Usage.InputTokens),
					int(ev.Response.Usage.OutputTokens),
					int(ev.Response.Usage.TotalTokens),
					int(ev.Response.Usage.InputTokensDetails.CachedTokens),
					ev.Response.Usage.RawJSON(),
				)
			}
			if string(ev.Response.Model) != "" {
				lastModel = string(ev.Response.Model)
			}
			logging.Debugf(ctx, "llm.openai-responses", "[OpenAIProvider] Stream completed: %d events, response=%d bytes, toolCalls=%d, model=%s",
				eventCount, fullResponse.Len(), len(finishedToolCalls), lastModel)

		case "response.incomplete":
			ev := event.AsResponseIncomplete()
			finish = normalizeOpenAIResponsesFinishReason(ev.Response.IncompleteDetails.Reason)
			usageRaw := ev.Response.Usage.RawJSON()
			if openAIUsageReported(usageRaw,
				int(ev.Response.Usage.InputTokens), int(ev.Response.Usage.OutputTokens), int(ev.Response.Usage.TotalTokens)) {
				lastUsageRaw = rawJSONDump(usageRaw)
				lastUsage = UsageFromOpenAIResponses(
					int(ev.Response.Usage.InputTokens),
					int(ev.Response.Usage.OutputTokens),
					int(ev.Response.Usage.TotalTokens),
					int(ev.Response.Usage.InputTokensDetails.CachedTokens),
					ev.Response.Usage.RawJSON(),
				)
			}
			if string(ev.Response.Model) != "" {
				lastModel = string(ev.Response.Model)
			}
			logging.Infof(ctx, "llm.openai-responses", "[OpenAIProvider] Stream incompleto: reason=%s, events=%d, toolCalls=%d",
				ev.Response.IncompleteDetails.Reason, eventCount, len(finishedToolCalls))

		case "response.failed":
			ev := event.AsResponseFailed()
			errMsg := "erro na Responses API"
			if ev.Response.Error.Message != "" {
				errMsg = ev.Response.Error.Message
			}
			logging.Errorf(ctx, "llm.openai-responses", "[OpenAIProvider] Response FAILED: %s", errMsg)
			if len(mcpServers) > 0 && !emittedNonRetryableEffect && looksLikeNativeMCPUnsupported(errMsg) {
				return mcpStreamAttemptResult{nativeMCPUnsupported: true}
			}
			if !emittedNonRetryableEffect && looksLikePromptCacheHintUnsupported(errMsg) {
				return mcpStreamAttemptResult{promptCacheHintUnsupported: true}
			}
			if failure := inferMCPFailure(MCPFailureStageHandshake, errMsg, ev.RawJSON(), "", mcpServers); failure != nil && !emittedNonRetryableEffect {
				return mcpStreamAttemptResult{mcpFailure: failure}
			}
			if !emittedNonRetryableEffect && isRetryableError(errMsg) {
				reportCurrentDiagnostics()
				return mcpStreamAttemptResult{retry: true}
			}
			finishThinking()
			reportCurrentDiagnostics()
			if emittedNonRetryableEffect {
				markErrorNotRetryable(handler)
			}
			handler.OnError(errMsg)
			return mcpStreamAttemptResult{done: true}

		default:
			logging.Errorf(ctx, "llm.openai-responses", "[OpenAIProvider] Unhandled event type: %s", event.Type)
		}
	}

	wd.Stop()
	if err := stream.Err(); err != nil {
		errStr := err.Error()
		logging.Errorf(ctx, "llm.openai-responses", "[OpenAIProvider] Responses stream error: %s", errStr)

		// Cancelamento do usuário (contexto pai): nunca retentar.
		if ctx.Err() != nil {
			finishThinking()
			reportCurrentDiagnostics()
			handler.OnError("Streaming cancelado: " + ctx.Err().Error())
			return mcpStreamAttemptResult{done: true}
		}

		// Watchdog de ociosidade estourou. Sem conteúdo emitido, a tentativa
		// é descartável; com conteúdo já entregue, repetir duplicaria a resposta.
		if wd.TimedOut() {
			if !emittedNonRetryableEffect {
				reportCurrentDiagnostics()
				return mcpStreamAttemptResult{retry: true}
			}
			finishThinking()
			reportCurrentDiagnostics()
			markErrorNotRetryable(handler)
			handler.OnError(streamIdleErrorMessage)
			return mcpStreamAttemptResult{done: true}
		}

		if len(mcpServers) > 0 && !emittedNonRetryableEffect && looksLikeNativeMCPUnsupported(errStr) {
			return mcpStreamAttemptResult{nativeMCPUnsupported: true}
		}
		if !emittedNonRetryableEffect && looksLikePromptCacheHintUnsupported(errStr) {
			return mcpStreamAttemptResult{promptCacheHintUnsupported: true}
		}
		if failure := inferMCPFailure(MCPFailureStageHandshake, errStr, "", "", mcpServers); failure != nil && !emittedNonRetryableEffect {
			return mcpStreamAttemptResult{mcpFailure: failure}
		}
		if !emittedNonRetryableEffect && isRetryableError(errStr) {
			reportCurrentDiagnostics()
			return mcpStreamAttemptResult{retry: true}
		}
		finishThinking()
		reportCurrentDiagnostics()
		if emittedNonRetryableEffect {
			markErrorNotRetryable(handler)
		}
		handler.OnError(errStr)
		return mcpStreamAttemptResult{done: true}
	}

	// Guarda de corrida: o watchdog pode estourar exatamente quando o
	// servidor fecha a conexão, deixando stream.Err() == nil com resposta
	// truncada. Parar e aguardar o watchdog fecha a janela entre consultar
	// TimedOut e entregar OnDone.
	if wd.TimedOut() {
		logging.Logger(ctx, "llm.openai-responses").ErrorContext(
			ctx,
			"stream encerrou junto com timeout de inatividade",
			"partial_bytes", fullResponse.Len(),
		)
		if !emittedNonRetryableEffect {
			reportCurrentDiagnostics()
			return mcpStreamAttemptResult{retry: true}
		}
		finishThinking()
		reportCurrentDiagnostics()
		markErrorNotRetryable(handler)
		handler.OnError(streamIdleErrorMessage)
		return mcpStreamAttemptResult{done: true}
	}

	logging.Infof(ctx, "llm.openai-responses", "[OpenAIProvider] Stream loop ended: %d events, response=%d bytes, reasoning=%d bytes, toolCalls=%d",
		eventCount, fullResponse.Len(), fullReasoning.Len(), len(finishedToolCalls))

	// Fallback de conclusão de MCP nativo para itens que receberam
	// response.mcp_call.completed mas não response.output_item.done. Após o loop,
	// emittedNonRetryableEffect já não é mais lido (servia para gatear
	// fallback/falha durante o stream), então não o reatribuímos aqui.
	emittedNonRetryableEffect = flushPendingCompletedMCPCalls(activeMCPCalls, handler) ||
		emittedNonRetryableEffect

	if isThinking && thinkingBuffer.Len() > 0 {
		select {
		case <-ctx.Done():
			finishThinking()
			return mcpStreamAttemptResult{done: true}
		default:
		}
		thinkingBuffer.Reset()
		isThinking = false
	}
	if finish.Reason == FinishReasonMaxTokens && len(activeFuncCalls) > 0 {
		itemIDs := make([]string, 0, len(activeFuncCalls))
		for itemID := range activeFuncCalls {
			itemIDs = append(itemIDs, itemID)
		}
		sort.Strings(itemIDs)
		for _, itemID := range itemIDs {
			call := activeFuncCalls[itemID]
			finishedToolCalls = append(finishedToolCalls, ToolCall{
				ID:   call.ID,
				Type: "function",
				Function: FunctionCall{
					Name:      call.Name,
					Arguments: call.Args.String(),
				},
			})
		}
	}
	finish = finishInfoWithToolCalls(finish, len(finishedToolCalls))
	diagnosticModel := lastModel
	if diagnosticModel == "" {
		diagnosticModel = chatParams.Model
	}
	lastModel = diagnosticModel
	finish = finishInfoWithDiagnostics(finish, p.provider, diagnosticModel, chatParams.MaxTokens, fullResponse.Len())
	reportUsage(handler, lastUsage)
	if finish.Reason == "" && len(finishedToolCalls) == 0 && !emittedNonRetryableEffect {
		discardThinking()
	} else {
		finishThinking()
	}
	select {
	case <-ctx.Done():
		finishThinking()
		return mcpStreamAttemptResult{done: true}
	default:
	}
	ReportFinishReason(handler, finish)

	if finish.Reason == "" && len(finishedToolCalls) == 0 {
		if emittedNonRetryableEffect {
			markErrorNotRetryable(handler)
		}
		handler.OnError("streaming_interrupted")
		return mcpStreamAttemptResult{done: true}
	}

	if len(finishedToolCalls) > 0 {
		dumpLLMResponse(dumpHandle, chatParams, map[string]any{
			"content":    fullResponse.String(),
			"reasoning":  fullReasoning.String(),
			"tool_calls": finishedToolCalls,
			"usage":      lastUsage,
			"usage_raw":  lastUsageRaw,
			"model":      lastModel,
		})
		handler.OnToolCalls(finishedToolCalls, fullResponse.String(), lastUsage, lastModel)
		return mcpStreamAttemptResult{done: true}
	}

	dumpLLMResponse(dumpHandle, chatParams, map[string]any{
		"content":   fullResponse.String(),
		"reasoning": fullReasoning.String(),
		"usage":     lastUsage,
		"usage_raw": lastUsageRaw,
		"model":     lastModel,
	})
	handler.OnDone(fullResponse.String(), lastUsage, lastModel)
	return mcpStreamAttemptResult{done: true}
}

func rawJSONDump(raw string) any {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	if !json.Valid([]byte(raw)) {
		return raw
	}
	return json.RawMessage(raw)
}
