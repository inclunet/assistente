package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"assistente/internal/credentials"

	"github.com/anthropics/anthropic-sdk-go"
	anthropicoption "github.com/anthropics/anthropic-sdk-go/option"
	anthropicparam "github.com/anthropics/anthropic-sdk-go/packages/param"
)

// AnthropicProvider implementa ChatProvider usando a SDK anthropic-sdk-go.
type AnthropicProvider struct {
	client        *anthropic.Client
	provider      *ProviderConfig
	mcpServers    []MCPServerConfig // MCP servers HTTP para native connector
	betaAttemptFn func(context.Context, anthropic.BetaMessageNewParams, StreamHandler, []MCPServerConfig) mcpStreamAttemptResult
}

// NewAnthropicProvider cria um provider Anthropic com a SDK oficial.
func NewAnthropicProvider(provider *ProviderConfig, credMgr *credentials.Manager) *AnthropicProvider {
	httpClient := newHTTPClientForProvider(provider, credMgr)

	opts := []anthropicoption.RequestOption{
		anthropicoption.WithHTTPClient(httpClient),
	}
	if providerUsesPlaceholderAPIKey(provider) {
		opts = append(opts, anthropicoption.WithAPIKey("managed-by-credential-transport"))
	} else {
		opts = append(opts, anthropicoption.WithAPIKey(""))
	}

	if provider.BaseURL != "" {
		baseURL := strings.TrimSuffix(provider.BaseURL, "/") + "/"
		opts = append(opts, anthropicoption.WithBaseURL(baseURL))
	}

	client := anthropic.NewClient(opts...)

	return &AnthropicProvider{
		client:   &client,
		provider: provider,
	}
}

// NativeMCPCapable: a Anthropic suporta MCP nativo via Beta Messages API.
func (p *AnthropicProvider) NativeMCPCapable() bool {
	return true
}

func (p *AnthropicProvider) WithMCPServers(servers []MCPServerConfig) ChatProvider {
	if len(servers) == 0 {
		return p
	}
	return &AnthropicProvider{
		client:        p.client,
		provider:      p.provider,
		mcpServers:    servers,
		betaAttemptFn: p.betaAttemptFn,
	}
}

func (p *AnthropicProvider) SendChat(ctx context.Context, messages []Message, params ChatParams) (string, error) {
	model := resolveModel(p.provider, params.Model)
	if model == "" {
		return "", fmt.Errorf("nenhum modelo especificado e nenhum modelo padrão configurado")
	}

	maxTokens := int64(params.MaxTokens)
	if maxTokens <= 0 {
		maxTokens = 4096
	}

	system, anthropicMsgs := convertToAnthropicMessages(messages, params.ExplicitCacheControl)

	sdkParams := anthropic.MessageNewParams{
		Model:     anthropic.Model(model),
		Messages:  anthropicMsgs,
		MaxTokens: maxTokens,
	}
	if len(system) > 0 {
		sdkParams.System = system
	}
	if params.Temperature > 0 {
		sdkParams.Temperature = anthropicparam.NewOpt(params.Temperature)
	}

	msg, err := p.client.Messages.New(ctx, sdkParams)
	if err != nil {
		return "", fmt.Errorf("erro ao enviar mensagem: %w", err)
	}

	var sb strings.Builder
	for _, block := range msg.Content {
		if block.Type == "text" {
			sb.WriteString(block.Text)
		}
	}
	return sb.String(), nil
}

func (p *AnthropicProvider) GetModels(ctx context.Context) (models []string, retErr error) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[AnthropicProvider] PANIC no SDK Models.List: %v", r)
			retErr = fmt.Errorf("panic no SDK: %v", r)
		}
	}()

	page, err := p.client.Models.List(ctx, anthropic.ModelListParams{})
	if err != nil {
		return nil, fmt.Errorf("erro ao listar modelos: %w", err)
	}
	if page == nil {
		return nil, fmt.Errorf("resposta vazia do servidor ao listar modelos")
	}

	for _, m := range page.Data {
		models = append(models, m.ID)
	}

	sort.Strings(models)
	return models, nil
}

func (p *AnthropicProvider) SimpleChat(ctx context.Context, model, systemPrompt, userMessage string) (string, error) {
	msgs := []Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userMessage},
	}
	return p.SendChat(ctx, msgs, ChatParams{Model: model})
}

func (p *AnthropicProvider) StreamChat(ctx context.Context, messages []Message, params ChatParams, handler StreamHandler, tools ...ToolDefinition) {
	model := resolveModel(p.provider, params.Model)
	if model == "" {
		handler.OnError("Nenhum modelo especificado e nenhum modelo padrão configurado")
		return
	}

	maxTokens := int64(params.MaxTokens)
	if maxTokens <= 0 {
		maxTokens = 4096
	}

	system, anthropicMsgs := convertToAnthropicMessages(messages, params.ExplicitCacheControl)

	// Se há MCP servers configurados, usa Beta Messages API com MCP connector
	if len(p.mcpServers) > 0 {
		p.streamChatWithMCP(ctx, model, maxTokens, system, anthropicMsgs, params, handler, tools...)
		return
	}

	sdkParams := anthropic.MessageNewParams{
		Model:     anthropic.Model(model),
		Messages:  anthropicMsgs,
		MaxTokens: maxTokens,
	}

	if len(system) > 0 {
		sdkParams.System = system
	}

	if params.Temperature > 0 {
		sdkParams.Temperature = anthropicparam.NewOpt(params.Temperature)
	}

	if params.TopP > 0 && params.TopP != 1.0 {
		sdkParams.TopP = anthropicparam.NewOpt(params.TopP)
	}

	if len(tools) > 0 {
		sdkParams.Tools = convertAnthropicTools(tools)
		toolChoice := "auto"
		if choice, ok := toolChoiceFromContext(ctx); ok {
			if s, ok := choice.(string); ok {
				toolChoice = s
			}
		}
		sdkParams.ToolChoice = makeAnthropicToolChoice(toolChoice)
	}

	const maxAttempts = 10
	bk := 500 * time.Millisecond
	maxBk := 8 * time.Second

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		select {
		case <-ctx.Done():
			handler.OnError("Streaming cancelado: " + ctx.Err().Error())
			return
		default:
		}

		done := p.doStream(ctx, sdkParams, handler)
		if done {
			return
		}

		if attempt < maxAttempts {
			sleepWithJitter(ctx, bk)
			bk = nextBackoff(bk, maxBk)
			continue
		}

		handler.OnError("Máximo de tentativas de streaming excedido")
	}
}

// streamChatWithMCP usa a Beta Messages API com MCP connector nativo.
// MCP servers HTTP são passados diretamente ao Anthropic, que faz a comunicação server-side.
// Tools locais (function calling) continuam funcionando normalmente junto com MCP.
func (p *AnthropicProvider) streamChatWithMCP(
	ctx context.Context,
	model string,
	maxTokens int64,
	system []anthropic.TextBlockParam,
	msgs []anthropic.MessageParam,
	params ChatParams,
	handler StreamHandler,
	tools ...ToolDefinition,
) {
	currentServers := cloneMCPServers(p.mcpServers)
	log.Printf("[AnthropicProvider] MCP nativo: %d servers, %d tools locais", len(currentServers), len(tools))

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

		betaParams := p.buildBetaMCPParams(ctx, model, maxTokens, system, msgs, params, currentServers, tools...)
		attemptFn := p.betaAttemptFn
		if attemptFn == nil {
			attemptFn = p.doStreamBeta
		}
		result := attemptFn(ctx, betaParams, handler, currentServers)
		if result.done {
			return
		}
		if result.nativeMCPUnsupported {
			// Modelo/endpoint rejeitou MCP nativo: dispara o auto-ajuste persistido do
			// perfil (nil→false) e degrada nativo→adapter.
			log.Printf("[MCP-DEGRADE] attempt=%d provider=anthropic action=native_to_adapter reason=model_rejects_native_mcp servers=%d", attempt, len(currentServers))
			if params.OnNativeMCPUnsupported != nil {
				params.OnNativeMCPUnsupported()
			}
			if params.NativeMCPFallback != nil {
				// O caller (loop agêntico) re-tenta o MESMO turno em modo adapter, com
				// as bridge tools presentes. Aborta sem emitir done/erro.
				params.NativeMCPFallback.Trigger()
				return
			}
			currentServers = nil
			continue
		}
		if result.mcpFailure != nil {
			if degradeRetries < maxDegradeRetries {
				if remaining, ok := planMCPDegradationRetry(ctx, "anthropic", attempt, currentServers, result.mcpFailure); ok {
					currentServers = remaining
					degradeRetries++
					continue
				}
			}
			handler.OnError(strings.TrimSpace(result.mcpFailure.Message))
			return
		}
		if result.retry {
			if attempt < maxAttempts {
				sleepWithJitter(ctx, bk)
				bk = nextBackoff(bk, maxBk)
				continue
			}
			handler.OnError("Máximo de tentativas de streaming excedido")
			return
		}
		return

	}
}

func (p *AnthropicProvider) buildBetaMCPParams(
	ctx context.Context,
	model string,
	maxTokens int64,
	system []anthropic.TextBlockParam,
	msgs []anthropic.MessageParam,
	params ChatParams,
	mcpServers []MCPServerConfig,
	tools ...ToolDefinition,
) anthropic.BetaMessageNewParams {
	betaParams := anthropic.BetaMessageNewParams{
		Model:     anthropic.Model(model),
		Messages:  convertToBetaMessages(msgs),
		MaxTokens: maxTokens,
		Betas:     []anthropic.AnthropicBeta{"mcp-client-2025-11-20"},
	}

	if len(system) > 0 {
		betaSystem := make([]anthropic.BetaTextBlockParam, len(system))
		for i, s := range system {
			betaSystem[i] = anthropic.BetaTextBlockParam{Text: s.Text}
			if s.CacheControl.Type != "" {
				betaSystem[i].CacheControl = anthropic.BetaCacheControlEphemeralParam{
					Type: "ephemeral",
					TTL:  anthropic.BetaCacheControlEphemeralTTL(s.CacheControl.TTL),
				}
			}
		}
		betaParams.System = betaSystem
	}
	if params.Temperature > 0 {
		betaParams.Temperature = anthropicparam.NewOpt(params.Temperature)
	}
	if params.TopP > 0 && params.TopP != 1.0 {
		betaParams.TopP = anthropicparam.NewOpt(params.TopP)
	}

	for _, srv := range mcpServers {
		mcpDef := anthropic.BetaRequestMCPServerURLDefinitionParam{
			Name: srv.Name,
			URL:  srv.URL,
		}
		if srv.AuthToken != "" {
			mcpDef.AuthorizationToken = anthropicparam.NewOpt(srv.AuthToken)
		}
		betaParams.MCPServers = append(betaParams.MCPServers, mcpDef)
		log.Printf("[AnthropicProvider] MCP native server: name=%q url=%q hasAuth=%v allowedTools=%d",
			srv.Name, srv.URL, srv.AuthToken != "", len(srv.AllowedTools))
	}

	var betaTools []anthropic.BetaToolUnionParam
	for _, srv := range mcpServers {
		toolset := anthropic.BetaMCPToolsetParam{
			MCPServerName: srv.Name,
		}
		if len(srv.AllowedTools) > 0 {
			toolset.DefaultConfig = anthropic.BetaMCPToolDefaultConfigParam{
				Enabled: anthropicparam.NewOpt(false),
			}
			toolset.Configs = make(map[string]anthropic.BetaMCPToolConfigParam, len(srv.AllowedTools))
			for _, name := range srv.AllowedTools {
				toolset.Configs[name] = anthropic.BetaMCPToolConfigParam{
					Enabled: anthropicparam.NewOpt(true),
				}
			}
		}
		betaTools = append(betaTools, anthropic.BetaToolUnionParam{OfMCPToolset: &toolset})
	}

	if len(tools) > 0 {
		for _, tool := range tools {
			var schema anthropic.BetaToolInputSchemaParam
			if len(tool.Function.Parameters) > 0 {
				if err := json.Unmarshal(tool.Function.Parameters, &schema); err != nil {
					log.Printf("[AnthropicProvider] Erro ao parsear parameters de %s: %v", tool.Function.Name, err)
					continue
				}
			}
			betaTools = append(betaTools, anthropic.BetaToolUnionParam{
				OfTool: &anthropic.BetaToolParam{
					Name:        tool.Function.Name,
					Description: anthropicparam.NewOpt(tool.Function.Description),
					InputSchema: schema,
				},
			})
		}

		toolChoice := "auto"
		if choice, ok := toolChoiceFromContext(ctx); ok {
			if s, ok := choice.(string); ok {
				toolChoice = s
			}
		}
		betaParams.ToolChoice = makeBetaAnthropicToolChoice(toolChoice)
	}

	betaParams.Tools = betaTools
	return betaParams
}

// doStreamBeta executa streaming via Beta Messages API (MCP connector).
// Eventos de MCP (mcp_tool_use, mcp_tool_result) são transparentes — o Anthropic
// executa as tool calls MCP server-side. Tool calls locais (tool_use) continuam
// sendo reportadas via OnToolCalls para execução local.
func (p *AnthropicProvider) doStreamBeta(ctx context.Context, params anthropic.BetaMessageNewParams, handler StreamHandler, mcpServers []MCPServerConfig) mcpStreamAttemptResult {
	stream := p.client.Beta.Messages.NewStreaming(ctx, params)

	var fullResponse strings.Builder
	var fullReasoning strings.Builder
	var emittedAnything bool
	var lastUsage Usage
	var lastModel string

	type pendingToolCall struct {
		ID       string
		Name     string
		ArgsJSON strings.Builder
	}
	activeToolCalls := make(map[int64]*pendingToolCall)
	var finishedToolCalls []ToolCall
	var stopReason string

	type mcpToolInfo struct {
		ID         string
		Name       string
		ServerName string
		ArgsJSON   string
	}
	activeMCPTools := make(map[string]*mcpToolInfo) // keyed by tool use ID

	for stream.Next() {
		event := stream.Current()

		switch event.Type {
		case "message_start":
			if event.Message.Model != "" {
				lastModel = string(event.Message.Model)
			}
			if event.Message.Usage.InputTokens > 0 || event.Message.Usage.CacheCreationInputTokens > 0 || event.Message.Usage.CacheReadInputTokens > 0 {
				lastUsage = mergeAnthropicStreamingUsage(
					lastUsage,
					int(event.Message.Usage.InputTokens),
					0,
					int(event.Message.Usage.CacheCreationInputTokens),
					int(event.Message.Usage.CacheReadInputTokens),
				)
			}

		case "content_block_start":
			cb := event.ContentBlock
			switch cb.Type {
			case "text":
				// streamed via deltas
			case "thinking":
				// streamed via deltas
			case "tool_use":
				activeToolCalls[event.Index] = &pendingToolCall{
					ID:   cb.ID,
					Name: cb.Name,
				}
			case "mcp_tool_use":
				mcpBlock := cb.AsMCPToolUse()
				argsStr := ""
				if mcpBlock.Input != nil {
					if b, err := json.Marshal(mcpBlock.Input); err == nil {
						argsStr = string(b)
					}
				}
				activeMCPTools[mcpBlock.ID] = &mcpToolInfo{
					ID:         mcpBlock.ID,
					Name:       mcpBlock.Name,
					ServerName: mcpBlock.ServerName,
					ArgsJSON:   argsStr,
				}
				emittedAnything = true
				handler.OnMCPToolEvent(MCPToolEvent{
					ID:          mcpBlock.ID,
					Name:        mcpBlock.Name,
					ServerLabel: mcpBlock.ServerName,
					Arguments:   argsStr,
					IsCompleted: false,
				})
			case "mcp_tool_result":
				mcpResult := cb.AsMCPToolResult()
				output := mcpResult.Content.RawJSON()
				toolName := ""
				serverName := ""
				if info, ok := activeMCPTools[mcpResult.ToolUseID]; ok {
					toolName = info.Name
					serverName = info.ServerName
					delete(activeMCPTools, mcpResult.ToolUseID)
				}
				errMsg := ""
				if mcpResult.IsError {
					errMsg = output
				}
				emittedAnything = true
				handler.OnMCPToolEvent(MCPToolEvent{
					ID:          mcpResult.ToolUseID,
					Name:        toolName,
					ServerLabel: serverName,
					Output:      output,
					Error:       errMsg,
					IsCompleted: true,
				})
			}

		case "content_block_delta":
			delta := event.Delta
			switch delta.Type {
			case "text_delta":
				if delta.Text != "" {
					fullResponse.WriteString(delta.Text)
					emittedAnything = true
					handler.OnChunk(delta.Text)
				}
			case "thinking_delta":
				if delta.Thinking != "" {
					fullReasoning.WriteString(delta.Thinking)
					emittedAnything = true
					handler.OnThinking(delta.Thinking)
				}
			case "input_json_delta":
				if tc, ok := activeToolCalls[event.Index]; ok {
					tc.ArgsJSON.WriteString(delta.PartialJSON)
				}
			}

		case "content_block_stop":
			if tc, ok := activeToolCalls[event.Index]; ok {
				finishedToolCalls = append(finishedToolCalls, ToolCall{
					ID:   tc.ID,
					Type: "function",
					Function: FunctionCall{
						Name:      tc.Name,
						Arguments: tc.ArgsJSON.String(),
					},
				})
				delete(activeToolCalls, event.Index)
			}

		case "message_delta":
			if string(event.Delta.StopReason) != "" {
				stopReason = string(event.Delta.StopReason)
			}
			if event.Usage.OutputTokens > 0 {
				lastUsage = mergeAnthropicStreamingUsage(
					lastUsage,
					int(event.Usage.InputTokens),
					int(event.Usage.OutputTokens),
					int(event.Usage.CacheCreationInputTokens),
					int(event.Usage.CacheReadInputTokens),
				)
			}
		}
	}

	if err := stream.Err(); err != nil {
		errStr := err.Error()
		log.Printf("[AnthropicProvider] Beta stream error: %s", errStr)
		if len(mcpServers) > 0 && !emittedAnything && looksLikeNativeMCPUnsupported(errStr) {
			return mcpStreamAttemptResult{nativeMCPUnsupported: true}
		}
		if failure := inferMCPFailure(MCPFailureStageHandshake, errStr, "", "", mcpServers); failure != nil && !emittedAnything {
			return mcpStreamAttemptResult{mcpFailure: failure}
		}
		if !emittedAnything && isRetryableError(errStr) {
			return mcpStreamAttemptResult{retry: true}
		}
		handler.OnError(errStr)
		return mcpStreamAttemptResult{done: true}
	}

	if fullReasoning.Len() > 0 {
		handler.OnThinkingDone(fullReasoning.String())
	}

	if stopReason == "tool_use" && len(finishedToolCalls) > 0 {
		handler.OnToolCalls(finishedToolCalls, fullResponse.String(), lastUsage, lastModel)
		return mcpStreamAttemptResult{done: true}
	}

	handler.OnDone(fullResponse.String(), lastUsage, lastModel)
	return mcpStreamAttemptResult{done: true}
}

// convertToBetaMessages converte MessageParam regulares para BetaMessageParam.
func convertToBetaMessages(msgs []anthropic.MessageParam) []anthropic.BetaMessageParam {
	result := make([]anthropic.BetaMessageParam, len(msgs))
	for i, msg := range msgs {
		betaContent := make([]anthropic.BetaContentBlockParamUnion, len(msg.Content))
		for j, block := range msg.Content {
			data, _ := json.Marshal(block)
			var betaBlock anthropic.BetaContentBlockParamUnion
			_ = json.Unmarshal(data, &betaBlock)
			betaContent[j] = betaBlock
		}
		result[i] = anthropic.BetaMessageParam{
			Role:    anthropic.BetaMessageParamRole(msg.Role),
			Content: betaContent,
		}
	}
	return result
}

func makeBetaAnthropicToolChoice(choice string) anthropic.BetaToolChoiceUnionParam {
	switch choice {
	case "required":
		return anthropic.BetaToolChoiceUnionParam{
			OfAny: &anthropic.BetaToolChoiceAnyParam{},
		}
	case "none":
		return anthropic.BetaToolChoiceUnionParam{
			OfNone: &anthropic.BetaToolChoiceNoneParam{},
		}
	default:
		return anthropic.BetaToolChoiceUnionParam{
			OfAuto: &anthropic.BetaToolChoiceAutoParam{},
		}
	}
}

func (p *AnthropicProvider) doStream(ctx context.Context, params anthropic.MessageNewParams, handler StreamHandler) bool {
	stream := p.client.Messages.NewStreaming(ctx, params)

	var fullResponse strings.Builder
	var fullReasoning strings.Builder
	var emittedAnything bool
	var lastUsage Usage
	var lastModel string

	// Acumula tool calls por index (content_block_start → content_block_delta → content_block_stop)
	type pendingToolCall struct {
		ID       string
		Name     string
		ArgsJSON strings.Builder
	}
	activeToolCalls := make(map[int64]*pendingToolCall)
	var finishedToolCalls []ToolCall
	var stopReason string

	for stream.Next() {
		event := stream.Current()

		switch event.Type {
		case "message_start":
			if event.Message.Model != "" {
				lastModel = string(event.Message.Model)
			}
			if event.Message.Usage.InputTokens > 0 || event.Message.Usage.CacheCreationInputTokens > 0 || event.Message.Usage.CacheReadInputTokens > 0 {
				lastUsage = mergeAnthropicStreamingUsage(
					lastUsage,
					int(event.Message.Usage.InputTokens),
					0,
					int(event.Message.Usage.CacheCreationInputTokens),
					int(event.Message.Usage.CacheReadInputTokens),
				)
			}

		case "content_block_start":
			cb := event.ContentBlock
			switch cb.Type {
			case "text":
				// Texto vai ser streamed via deltas
			case "thinking":
				// Thinking vai ser streamed via deltas
			case "tool_use":
				activeToolCalls[event.Index] = &pendingToolCall{
					ID:   cb.ID,
					Name: cb.Name,
				}
			}

		case "content_block_delta":
			delta := event.Delta
			switch delta.Type {
			case "text_delta":
				if delta.Text != "" {
					fullResponse.WriteString(delta.Text)
					emittedAnything = true
					handler.OnChunk(delta.Text)
				}
			case "thinking_delta":
				if delta.Thinking != "" {
					fullReasoning.WriteString(delta.Thinking)
					emittedAnything = true
					handler.OnThinking(delta.Thinking)
				}
			case "input_json_delta":
				if tc, ok := activeToolCalls[event.Index]; ok {
					tc.ArgsJSON.WriteString(delta.PartialJSON)
				}
			}

		case "content_block_stop":
			if tc, ok := activeToolCalls[event.Index]; ok {
				finishedToolCalls = append(finishedToolCalls, ToolCall{
					ID:   tc.ID,
					Type: "function",
					Function: FunctionCall{
						Name:      tc.Name,
						Arguments: tc.ArgsJSON.String(),
					},
				})
				delete(activeToolCalls, event.Index)
			}

		case "message_delta":
			if string(event.Delta.StopReason) != "" {
				stopReason = string(event.Delta.StopReason)
			}
			if event.Usage.OutputTokens > 0 {
				lastUsage = mergeAnthropicStreamingUsage(
					lastUsage,
					int(event.Usage.InputTokens),
					int(event.Usage.OutputTokens),
					int(event.Usage.CacheCreationInputTokens),
					int(event.Usage.CacheReadInputTokens),
				)
			}
		}
	}

	if err := stream.Err(); err != nil {
		errStr := err.Error()
		log.Printf("[AnthropicProvider] Stream error: %s", errStr)

		if !emittedAnything && isRetryableError(errStr) {
			return false
		}

		handler.OnError(errStr)
		return true
	}

	if fullReasoning.Len() > 0 {
		handler.OnThinkingDone(fullReasoning.String())
	}

	if stopReason == "tool_use" && len(finishedToolCalls) > 0 {
		handler.OnToolCalls(finishedToolCalls, fullResponse.String(), lastUsage, lastModel)
		return true
	}

	handler.OnDone(fullResponse.String(), lastUsage, lastModel)
	return true
}

// convertToAnthropicMessages converte mensagens internas para o formato Anthropic.
// Retorna o system prompt separado (Anthropic não usa role "system" nas mensagens)
// e a lista de mensagens user/assistant com content blocks.
func convertToAnthropicMessages(msgs []Message, explicitCacheControl bool) ([]anthropic.TextBlockParam, []anthropic.MessageParam) {
	var system []anthropic.TextBlockParam
	var result []anthropic.MessageParam

	// Buffer para agrupar tool results consecutivos em uma única mensagem user
	var pendingToolResults []anthropic.ContentBlockParamUnion

	flushToolResults := func() {
		if len(pendingToolResults) == 0 {
			return
		}
		result = append(result, anthropic.MessageParam{
			Role:    anthropic.MessageParamRoleUser,
			Content: pendingToolResults,
		})
		pendingToolResults = nil
	}

	for _, msg := range msgs {
		content := msg.GetContentAsString()

		switch msg.Role {
		case "system":
			system = append(system, anthropicSystemBlocks(msg, content, explicitCacheControl)...)

		case "user":
			flushToolResults()
			result = append(result, anthropic.NewUserMessage(
				anthropic.NewTextBlock(content),
			))

		case "assistant":
			flushToolResults()
			var blocks []anthropic.ContentBlockParamUnion
			if content != "" {
				blocks = append(blocks, anthropic.NewTextBlock(content))
			}
			for _, tc := range msg.ToolCalls {
				var input any
				if err := json.Unmarshal([]byte(tc.Function.Arguments), &input); err != nil {
					input = map[string]any{}
				}
				blocks = append(blocks, anthropic.NewToolUseBlock(tc.ID, input, tc.Function.Name))
			}
			if len(blocks) > 0 {
				result = append(result, anthropic.MessageParam{
					Role:    anthropic.MessageParamRoleAssistant,
					Content: blocks,
				})
			}

		case "tool":
			pendingToolResults = append(pendingToolResults,
				anthropic.NewToolResultBlock(msg.ToolCallID, content, false),
			)
		}
	}

	flushToolResults()

	return system, result
}

func anthropicSystemBlocks(msg Message, content string, explicitCacheControl bool) []anthropic.TextBlockParam {
	if content == "" {
		return nil
	}
	prefixLen := msg.SystemCacheControlPrefixLen
	if !explicitCacheControl || prefixLen <= 0 {
		return []anthropic.TextBlockParam{{Text: content}}
	}
	if prefixLen > len(content) {
		prefixLen = len(content)
	}
	prefix := content[:prefixLen]
	suffix := content[prefixLen:]
	if strings.TrimSpace(prefix) == "" {
		return []anthropic.TextBlockParam{{Text: content}}
	}
	block := anthropic.TextBlockParam{
		Text:         prefix,
		CacheControl: anthropic.NewCacheControlEphemeralParam(),
	}
	if suffix == "" {
		return []anthropic.TextBlockParam{block}
	}
	return []anthropic.TextBlockParam{block, anthropic.TextBlockParam{Text: suffix}}
}

// convertAnthropicTools converte definições de ferramentas para o formato Anthropic.
func convertAnthropicTools(tools []ToolDefinition) []anthropic.ToolUnionParam {
	result := make([]anthropic.ToolUnionParam, 0, len(tools))

	for _, tool := range tools {
		var schema anthropic.ToolInputSchemaParam
		if len(tool.Function.Parameters) > 0 {
			if err := json.Unmarshal(tool.Function.Parameters, &schema); err != nil {
				log.Printf("[AnthropicProvider] Erro ao parsear parameters de %s: %v", tool.Function.Name, err)
				continue
			}
		}

		result = append(result, anthropic.ToolUnionParam{
			OfTool: &anthropic.ToolParam{
				Name:        tool.Function.Name,
				Description: anthropicparam.NewOpt(tool.Function.Description),
				InputSchema: schema,
			},
		})
	}

	return result
}

func makeAnthropicToolChoice(choice string) anthropic.ToolChoiceUnionParam {
	switch choice {
	case "required":
		return anthropic.ToolChoiceUnionParam{
			OfAny: &anthropic.ToolChoiceAnyParam{},
		}
	case "none":
		return anthropic.ToolChoiceUnionParam{
			OfNone: &anthropic.ToolChoiceNoneParam{},
		}
	default:
		return anthropic.ToolChoiceUnionParam{
			OfAuto: &anthropic.ToolChoiceAutoParam{},
		}
	}
}

var _ ChatProvider = (*AnthropicProvider)(nil)
