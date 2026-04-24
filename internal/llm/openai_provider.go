package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"

	"assistente/internal/credentials"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/packages/param"
	"github.com/openai/openai-go/responses"
	"github.com/openai/openai-go/shared"
)

// OpenAIProvider implementa ChatProvider usando a SDK openai-go.
//
// Dois modos de operação, determinados pelo APIFormat do ProviderConfig:
//
//   - useResponses=false (APIFormatOpenAI / APIFormatOpenAICompatible):
//     Chat Completions API only (/v1/chat/completions).
//     Para provedores OpenAI-compatible: OpenRouter, Ollama, Groq, Together, etc.
//     Não suporta MCP nativo. SupportsNativeMCP() retorna false.
//     WithMCPServers() é no-op (retorna o provider inalterado).
//
//   - useResponses=true (APIFormatOpenAIResponses):
//     Responses API first (/v1/responses).
//     Para OpenAI real (api.openai.com). Suporta MCP nativo (type:mcp),
//     reasoning summaries (via Reasoning param), tool_choice, e features modernas.
//     SupportsNativeMCP() retorna true.
//     WithMCPServers() cria uma cópia com MCP servers configurados.
//
// Limitações conhecidas do path Responses vs Chat Completions:
//   - Multimodalidade: imagens em user messages são convertidas como texto.
//     A Responses API suporta imagens mas com formato diferente (input_image).
type OpenAIProvider struct {
	client       *openai.Client
	provider     *ProviderConfig
	credMgr      *credentials.Manager
	useResponses bool              // true = Responses API first; false = Chat Completions only
	mcpServers   []MCPServerConfig // MCP servers HTTP (só efetivo quando useResponses=true)
}

// NewOpenAIProvider cria um provider Chat Completions-only (OpenAI-compatible).
// Usado para OpenRouter, Ollama, Groq, Together, e qualquer endpoint /v1/chat/completions.
func NewOpenAIProvider(provider *ProviderConfig, credMgr *credentials.Manager) *OpenAIProvider {
	return newOpenAIProviderBase(provider, credMgr, false)
}

// NewOpenAIResponsesProvider cria um provider Responses API-first (OpenAI real).
// Usa /v1/responses como caminho padrão para streaming/chat.
// Suporta MCP nativo, reasoning summaries, e features modernas da OpenAI.
func NewOpenAIResponsesProvider(provider *ProviderConfig, credMgr *credentials.Manager) *OpenAIProvider {
	return newOpenAIProviderBase(provider, credMgr, true)
}

func newOpenAIProviderBase(provider *ProviderConfig, credMgr *credentials.Manager, useResponses bool) *OpenAIProvider {
	httpClient := newHTTPClientForProvider(provider, credMgr)

	opts := []option.RequestOption{
		option.WithHTTPClient(httpClient),
		option.WithAPIKey("managed-by-credential-transport"),
	}

	baseURL := strings.TrimSuffix(provider.BaseURL, "/")
	if !strings.HasSuffix(baseURL, "/v1") && !strings.HasSuffix(baseURL, "/v1beta") {
		baseURL += "/"
	} else {
		baseURL += "/"
	}
	opts = append(opts, option.WithBaseURL(baseURL))

	client := openai.NewClient(opts...)

	return &OpenAIProvider{
		client:       &client,
		provider:     provider,
		credMgr:      credMgr,
		useResponses: useResponses,
	}
}

func (p *OpenAIProvider) SupportsNativeMCP() bool {
	return p.useResponses
}

func (p *OpenAIProvider) WithMCPServers(servers []MCPServerConfig) ChatProvider {
	if !p.useResponses || len(servers) == 0 {
		return p
	}
	return &OpenAIProvider{
		client:       p.client,
		provider:     p.provider,
		credMgr:      p.credMgr,
		useResponses: p.useResponses,
		mcpServers:   servers,
	}
}

func (p *OpenAIProvider) SendChat(ctx context.Context, messages []Message, params ChatParams) (string, error) {
	model := resolveModel(p.provider, params.Model)
	if model == "" {
		return "", fmt.Errorf("nenhum modelo especificado e nenhum modelo padrão configurado")
	}

	if p.useResponses {
		return p.sendChatResponses(ctx, model, messages, params)
	}
	return p.sendChatCompletions(ctx, model, messages, params)
}

func (p *OpenAIProvider) sendChatCompletions(ctx context.Context, model string, messages []Message, params ChatParams) (string, error) {
	sdkParams := openai.ChatCompletionNewParams{
		Model:    shared.ChatModel(model),
		Messages: convertMessages(messages),
	}
	if params.Temperature > 0 {
		sdkParams.Temperature = param.NewOpt(params.Temperature)
	}
	if params.MaxTokensMode == "completion_tokens" {
		sdkParams.MaxCompletionTokens = param.NewOpt(int64(params.MaxTokens))
	} else if params.MaxTokens > 0 {
		sdkParams.MaxTokens = param.NewOpt(int64(params.MaxTokens))
	}

	completion, err := p.client.Chat.Completions.New(ctx, sdkParams)
	if err != nil {
		return "", fmt.Errorf("erro ao enviar mensagem: %w", err)
	}
	if len(completion.Choices) == 0 {
		return "", fmt.Errorf("nenhuma resposta recebida")
	}
	return completion.Choices[0].Message.Content, nil
}

func (p *OpenAIProvider) sendChatResponses(ctx context.Context, model string, messages []Message, params ChatParams) (string, error) {
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

	resp, err := p.client.Responses.New(ctx, respParams)
	if err != nil {
		return "", fmt.Errorf("erro ao enviar mensagem: %w", err)
	}
	return resp.OutputText(), nil
}

func (p *OpenAIProvider) GetModels(ctx context.Context) ([]string, error) {
	// Tenta via SDK primeiro
	models, err := p.getModelsSDK(ctx)
	if err == nil {
		return models, nil
	}

	// Se o SDK falhou (inclusive por panic capturado), tenta fallback HTTP direto
	log.Printf("[OpenAIProvider] SDK falhou ao listar modelos: %v — tentando fallback HTTP", err)
	return p.getModelsHTTP(ctx)
}

// getModelsSDK lista modelos usando a SDK openai-go.
func (p *OpenAIProvider) getModelsSDK(ctx context.Context) (models []string, retErr error) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[OpenAIProvider] PANIC no SDK Models.List: %v", r)
			retErr = fmt.Errorf("panic no SDK: %v", r)
		}
	}()

	page, err := p.client.Models.List(ctx)
	if err != nil {
		if strings.Contains(err.Error(), "404") {
			return nil, fmt.Errorf("models_endpoint_not_supported")
		}
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

// getModelsHTTP lista modelos via HTTP direto (fallback quando o SDK falha).
func (p *OpenAIProvider) getModelsHTTP(ctx context.Context) ([]string, error) {
	baseURL := strings.TrimSuffix(p.provider.BaseURL, "/")
	modelsURL := baseURL + "/models"
	if !strings.Contains(baseURL, "/v1") {
		modelsURL = baseURL + "/v1/models"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, modelsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("erro ao criar request: %w", err)
	}

	client := newHTTPClientForProvider(p.provider, p.credMgr)
	client.Timeout = 15 * time.Second

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("erro ao conectar ao provedor: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("models_endpoint_not_supported")
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("API Key inválida ou não autorizada")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("provedor retornou status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("erro ao ler resposta: %w", err)
	}

	type modelEntry struct {
		ID string `json:"id"`
	}
	type modelsResponse struct {
		Data []modelEntry `json:"data"`
	}

	var parsed modelsResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		var arr []modelEntry
		if err2 := json.Unmarshal(body, &arr); err2 != nil {
			return nil, fmt.Errorf("resposta inválida do servidor")
		}
		parsed.Data = arr
	}

	var models []string
	for _, m := range parsed.Data {
		if m.ID != "" {
			models = append(models, m.ID)
		}
	}
	sort.Strings(models)
	return models, nil
}

func (p *OpenAIProvider) SimpleChat(ctx context.Context, model, systemPrompt, userMessage string) (string, error) {
	msgs := []Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userMessage},
	}
	return p.SendChat(ctx, msgs, ChatParams{Model: model})
}

func (p *OpenAIProvider) StreamChat(ctx context.Context, messages []Message, params ChatParams, handler StreamHandler, tools ...ToolDefinition) {
	model := resolveModel(p.provider, params.Model)
	if model == "" {
		handler.OnError("Nenhum modelo especificado e nenhum modelo padrão configurado")
		return
	}

	// Responses-first: SEMPRE usa Responses API (com ou sem MCP servers)
	if p.useResponses {
		p.streamChatResponses(ctx, model, messages, params, handler, tools...)
		return
	}

	// Chat Completions path (OpenAI-compatible legado)
	sdkParams := openai.ChatCompletionNewParams{
		Model:    shared.ChatModel(model),
		Messages: convertMessages(messages),
		StreamOptions: openai.ChatCompletionStreamOptionsParam{
			IncludeUsage: param.NewOpt(true),
		},
	}

	if params.Temperature > 0 {
		sdkParams.Temperature = param.NewOpt(params.Temperature)
	}

	if params.MaxTokensMode == "completion_tokens" {
		sdkParams.MaxCompletionTokens = param.NewOpt(int64(params.MaxTokens))
	} else if params.MaxTokens > 0 {
		sdkParams.MaxTokens = param.NewOpt(int64(params.MaxTokens))
	}

	if params.TopP > 0 && params.TopP != 1.0 {
		sdkParams.TopP = param.NewOpt(params.TopP)
	}

	switch params.ReasoningEffort {
	case "low", "medium", "high":
		sdkParams.ReasoningEffort = shared.ReasoningEffort(params.ReasoningEffort)
	}

	if len(tools) > 0 {
		sdkParams.Tools = convertTools(tools)
		toolChoice := "auto"
		if choice, ok := toolChoiceFromContext(ctx); ok {
			if s, ok := choice.(string); ok {
				toolChoice = s
			}
		}
		sdkParams.ToolChoice = makeToolChoice(toolChoice)
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

		done := p.doStream(ctx, sdkParams, handler, &sdkParams)
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

// doStream executa uma tentativa de streaming. Retorna true se concluiu (sucesso ou erro terminal).
func (p *OpenAIProvider) doStream(ctx context.Context, params openai.ChatCompletionNewParams, handler StreamHandler, origParams *openai.ChatCompletionNewParams) bool {
	stream := p.client.Chat.Completions.NewStreaming(ctx, params)
	acc := openai.ChatCompletionAccumulator{}

	var fullResponse strings.Builder
	var fullReasoning strings.Builder
	var isThinking bool
	var thinkingBuffer strings.Builder
	var emittedAnything bool

	// Coletar tool calls finalizadas durante streaming
	var finishedToolCalls []ToolCall

	for stream.Next() {
		chunk := stream.Current()
		acc.AddChunk(chunk)

		if tool, ok := acc.JustFinishedToolCall(); ok {
			finishedToolCalls = append(finishedToolCalls, ToolCall{
				ID:   tool.ID,
				Type: "function",
				Function: FunctionCall{
					Name:      tool.Name,
					Arguments: tool.Arguments,
				},
			})
		}

		if len(chunk.Choices) == 0 {
			continue
		}

		delta := chunk.Choices[0].Delta

		if delta.Content != "" {
			content := delta.Content

			content = processThinkingTags(content, &isThinking, &thinkingBuffer, &fullReasoning, handler)

			if content != "" {
				fullResponse.WriteString(content)
				emittedAnything = true
				handler.OnChunk(content)
			}
		}
	}

	if err := stream.Err(); err != nil {
		errStr := err.Error()
		log.Printf("[OpenAIProvider] Stream error: %s", errStr)

		if !emittedAnything {
			// tool_choice downgrade
			if origParams.ToolChoice.OfAuto.Valid() && origParams.ToolChoice.OfAuto.Value == "required" {
				if strings.Contains(strings.ToLower(errStr), "tool_choice") || strings.Contains(strings.ToLower(errStr), "tool choice") {
					origParams.ToolChoice = makeToolChoice("auto")
					return false
				}
			}

			if isRetryableError(errStr) {
				return false
			}
		}

		handler.OnError(errStr)
		return true
	}

	if fullReasoning.Len() > 0 {
		handler.OnThinkingDone(fullReasoning.String())
	}

	usage := Usage{}
	if acc.Usage.TotalTokens > 0 {
		usage = Usage{
			PromptTokens:     int(acc.Usage.PromptTokens),
			CompletionTokens: int(acc.Usage.CompletionTokens),
			TotalTokens:      int(acc.Usage.TotalTokens),
		}
	}

	model := acc.Model

	if len(finishedToolCalls) > 0 {
		handler.OnToolCalls(finishedToolCalls, fullResponse.String(), usage, model)
		return true
	}

	handler.OnDone(fullResponse.String(), usage, model)
	return true
}

func isRetryableError(errStr string) bool {
	lower := strings.ToLower(errStr)
	return strings.Contains(lower, "timeout") ||
		strings.Contains(lower, "connection refused") ||
		strings.Contains(lower, "connection reset") ||
		strings.Contains(lower, "524") ||
		strings.Contains(lower, "502") ||
		strings.Contains(lower, "503") ||
		strings.Contains(lower, "429")
}

// convertMessages converte nossas mensagens internas para o formato SDK.
func convertMessages(msgs []Message) []openai.ChatCompletionMessageParamUnion {
	result := make([]openai.ChatCompletionMessageParamUnion, 0, len(msgs))

	for _, msg := range msgs {
		content := msg.GetContentAsString()

		switch msg.Role {
		case "system":
			result = append(result, openai.SystemMessage(content))

		case "user":
			if parts := extractImageParts(msg); parts != nil {
				result = append(result, openai.UserMessage(parts))
			} else {
				result = append(result, openai.UserMessage(content))
			}

		case "assistant":
			m := openai.AssistantMessage(content)
			if len(msg.ToolCalls) > 0 {
				toolCalls := make([]openai.ChatCompletionMessageToolCallParam, 0, len(msg.ToolCalls))
				for _, tc := range msg.ToolCalls {
					toolCalls = append(toolCalls, openai.ChatCompletionMessageToolCallParam{
						ID: tc.ID,
						Function: openai.ChatCompletionMessageToolCallFunctionParam{
							Name:      tc.Function.Name,
							Arguments: tc.Function.Arguments,
						},
					})
				}
				m.OfAssistant.ToolCalls = toolCalls
			}
			result = append(result, m)

		case "tool":
			result = append(result, openai.ToolMessage(content, msg.ToolCallID))
		}
	}

	return result
}

// extractImageParts extrai partes multimodal (texto + imagens) de uma mensagem user.
// Retorna nil se não houver imagens.
func extractImageParts(msg Message) []openai.ChatCompletionContentPartUnionParam {
	rawParts, ok := msg.Content.([]interface{})
	if !ok {
		return nil
	}

	hasImage := false
	for _, part := range rawParts {
		if partMap, ok := part.(map[string]interface{}); ok {
			if partMap["type"] == "image_url" {
				hasImage = true
				break
			}
		}
	}
	if !hasImage {
		return nil
	}

	var parts []openai.ChatCompletionContentPartUnionParam
	for _, part := range rawParts {
		partMap, ok := part.(map[string]interface{})
		if !ok {
			continue
		}

		switch partMap["type"] {
		case "text":
			if text, ok := partMap["text"].(string); ok {
				parts = append(parts, openai.TextContentPart(text))
			}
		case "image_url":
			if imgURLObj, ok := partMap["image_url"].(map[string]interface{}); ok {
				if urlStr, ok := imgURLObj["url"].(string); ok {
					imgParam := openai.ChatCompletionContentPartImageImageURLParam{
						URL:    urlStr,
						Detail: "auto",
					}
					if d, ok := imgURLObj["detail"].(string); ok && d != "" {
						imgParam.Detail = d
					}
					parts = append(parts, openai.ImageContentPart(imgParam))
				}
			}
		}
	}

	return parts
}

// convertTools converte nossas definições de ferramentas para o formato SDK.
func convertTools(tools []ToolDefinition) []openai.ChatCompletionToolParam {
	result := make([]openai.ChatCompletionToolParam, 0, len(tools))

	for _, tool := range tools {
		var params shared.FunctionParameters
		if len(tool.Function.Parameters) > 0 {
			if err := json.Unmarshal(tool.Function.Parameters, &params); err != nil {
				log.Printf("[OpenAIProvider] Erro ao parsear parameters de %s: %v", tool.Function.Name, err)
				continue
			}
		}

		result = append(result, openai.ChatCompletionToolParam{
			Function: shared.FunctionDefinitionParam{
				Name:        tool.Function.Name,
				Description: param.NewOpt(tool.Function.Description),
				Parameters:  params,
			},
		})
	}

	return result
}

func makeToolChoice(choice string) openai.ChatCompletionToolChoiceOptionUnionParam {
	return openai.ChatCompletionToolChoiceOptionUnionParam{
		OfAuto: param.NewOpt(choice),
	}
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
	currentServers := cloneMCPServers(p.mcpServers)
	log.Printf("[OpenAIProvider] Responses API: %d MCP servers, %d tools locais", len(currentServers), len(tools))

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
		result := p.doStreamResponses(ctx, respParams, handler, currentServers)
		if result.done {
			return
		}
		if result.mcpFailure != nil {
			if degradeRetries < maxDegradeRetries {
				if remaining, ok := planMCPDegradationRetry(ctx, "openai", attempt, currentServers, result.mcpFailure); ok {
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

func (p *OpenAIProvider) buildResponsesParams(
	ctx context.Context,
	model string,
	messages []Message,
	params ChatParams,
	mcpServers []MCPServerConfig,
	tools ...ToolDefinition,
) responses.ResponseNewParams {
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

	switch params.ReasoningEffort {
	case "low", "medium", "high":
		respParams.Reasoning = shared.ReasoningParam{
			Effort:  shared.ReasoningEffort(params.ReasoningEffort),
			Summary: shared.ReasoningSummaryAuto,
		}
	}

	var respTools []responses.ToolUnionParam
	for _, srv := range mcpServers {
		mcpTool := responses.ToolParamOfMcp(srv.Name, srv.URL)
		mcpTool.OfMcp.RequireApproval = responses.ToolMcpRequireApprovalUnionParam{
			OfMcpToolApprovalSetting: param.NewOpt(string(responses.ToolMcpRequireApprovalMcpToolApprovalSettingNever)),
		}
		if srv.AuthToken != "" {
			mcpTool.OfMcp.Headers = map[string]string{
				"Authorization": "Bearer " + srv.AuthToken,
			}
		}
		for k, v := range srv.Headers {
			if mcpTool.OfMcp.Headers == nil {
				mcpTool.OfMcp.Headers = make(map[string]string)
			}
			mcpTool.OfMcp.Headers[k] = v
		}
		if len(srv.AllowedTools) > 0 {
			mcpTool.OfMcp.AllowedTools = responses.ToolMcpAllowedToolsUnionParam{
				OfMcpAllowedTools: srv.AllowedTools,
			}
		}
		respTools = append(respTools, mcpTool)
		log.Printf("[OpenAIProvider] MCP native tool: label=%q url=%q hasAuth=%v allowedTools=%d",
			srv.Name, srv.URL, srv.AuthToken != "", len(srv.AllowedTools))
	}

	for _, tool := range tools {
		var fnParams map[string]any
		if len(tool.Function.Parameters) > 0 {
			if err := json.Unmarshal(tool.Function.Parameters, &fnParams); err != nil {
				log.Printf("[OpenAIProvider] Erro ao parsear parameters de %s: %v", tool.Function.Name, err)
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
func (p *OpenAIProvider) doStreamResponses(ctx context.Context, params responses.ResponseNewParams, handler StreamHandler, mcpServers []MCPServerConfig) mcpStreamAttemptResult {
	stream := p.client.Responses.NewStreaming(ctx, params)

	var fullResponse strings.Builder
	var fullReasoning strings.Builder
	var emittedAnything bool
	var lastUsage Usage
	var lastModel string
	var isThinking bool
	var thinkingBuffer strings.Builder

	type pendingFuncCall struct {
		ID   string
		Name string
		Args strings.Builder
	}
	activeFuncCalls := make(map[string]*pendingFuncCall) // keyed by item_id
	var finishedToolCalls []ToolCall

	type pendingMCPCall struct {
		ID          string
		Name        string
		ServerLabel string
		Args        strings.Builder
	}
	activeMCPCalls := make(map[string]*pendingMCPCall) // keyed by item_id

	var eventCount int

	for stream.Next() {
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
					fullResponse.WriteString(content)
					emittedAnything = true
					handler.OnChunk(content)
				}
			}

		case "response.output_text.done":
			// Final text for an output item, already accumulated via deltas

		case "response.reasoning_summary_text.delta":
			ev := event.AsResponseReasoningSummaryTextDelta()
			if ev.Delta != "" {
				fullReasoning.WriteString(ev.Delta)
				emittedAnything = true
				handler.OnThinking(ev.Delta)
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
				emittedAnything = true
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
				emittedAnything = true
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
			// Completion tracked via output_item.done for full data

		case "response.mcp_call.failed":
			ev := event.AsResponseMcpCallFailed()
			log.Printf("[OpenAIProvider] MCP call FAILED: itemID=%s", ev.ItemID)
			fallbackServer := ""
			if mc, ok := activeMCPCalls[ev.ItemID]; ok {
				fallbackServer = mc.ServerLabel
			}
			if failure := inferMCPFailure(MCPFailureStageCall, "", ev.RawJSON(), fallbackServer, mcpServers); failure != nil && !emittedAnything {
				return mcpStreamAttemptResult{mcpFailure: failure}
			}

		case "response.mcp_list_tools.in_progress":
			log.Printf("[OpenAIProvider] MCP listing tools (server-side)")
		case "response.mcp_list_tools.completed":
			log.Printf("[OpenAIProvider] MCP tool listing done (server-side)")
		case "response.mcp_list_tools.failed":
			log.Printf("[OpenAIProvider] MCP tool listing FAILED (server-side)")
			ev := event.AsResponseMcpListToolsFailed()
			if failure := inferMCPFailure(MCPFailureStageListTools, "", ev.RawJSON(), "", mcpServers); failure != nil && !emittedAnything {
				return mcpStreamAttemptResult{mcpFailure: failure}
			}

		case "response.completed":
			ev := event.AsResponseCompleted()
			if ev.Response.Usage.TotalTokens > 0 {
				lastUsage = Usage{
					PromptTokens:     int(ev.Response.Usage.InputTokens),
					CompletionTokens: int(ev.Response.Usage.OutputTokens),
					TotalTokens:      int(ev.Response.Usage.TotalTokens),
				}
			}
			if string(ev.Response.Model) != "" {
				lastModel = string(ev.Response.Model)
			}
			log.Printf("[OpenAIProvider] Stream completed: %d events, response=%d bytes, toolCalls=%d, model=%s",
				eventCount, fullResponse.Len(), len(finishedToolCalls), lastModel)

		case "response.failed":
			ev := event.AsResponseFailed()
			errMsg := "Responses API error"
			if ev.Response.Error.Message != "" {
				errMsg = ev.Response.Error.Message
			}
			log.Printf("[OpenAIProvider] Response FAILED: %s", errMsg)
			if failure := inferMCPFailure(MCPFailureStageHandshake, errMsg, ev.RawJSON(), "", mcpServers); failure != nil && !emittedAnything {
				return mcpStreamAttemptResult{mcpFailure: failure}
			}
			handler.OnError(errMsg)
			return mcpStreamAttemptResult{done: true}

		default:
			log.Printf("[OpenAIProvider] Unhandled event type: %s", event.Type)
		}
	}

	if err := stream.Err(); err != nil {
		errStr := err.Error()
		log.Printf("[OpenAIProvider] Responses stream error: %s", errStr)
		if failure := inferMCPFailure(MCPFailureStageHandshake, errStr, "", "", mcpServers); failure != nil && !emittedAnything {
			return mcpStreamAttemptResult{mcpFailure: failure}
		}
		if !emittedAnything && isRetryableError(errStr) {
			return mcpStreamAttemptResult{retry: true}
		}
		handler.OnError(errStr)
		return mcpStreamAttemptResult{done: true}
	}

	log.Printf("[OpenAIProvider] Stream loop ended: %d events, response=%d bytes, reasoning=%d bytes, toolCalls=%d",
		eventCount, fullResponse.Len(), fullReasoning.Len(), len(finishedToolCalls))

	if fullReasoning.Len() > 0 {
		handler.OnThinkingDone(fullReasoning.String())
	}

	if len(finishedToolCalls) > 0 {
		handler.OnToolCalls(finishedToolCalls, fullResponse.String(), lastUsage, lastModel)
		return mcpStreamAttemptResult{done: true}
	}

	handler.OnDone(fullResponse.String(), lastUsage, lastModel)
	return mcpStreamAttemptResult{done: true}
}

// convertToResponsesInput converte mensagens internas para o formato Responses API.
//
// Diferenças conhecidas vs convertMessages (Chat Completions):
//   - Imagens: user messages com image_url são convertidas apenas como texto (GetContentAsString).
//     A Responses API suporta imagens via input_image, mas com formato diferente.
//   - Assistant com content + tool_calls: gera items separados (message + function_call),
//     que é o formato correto para a Responses API (items são independentes).
//   - Tool results: mapeados para function_call_output (equivalente funcional).
func convertToResponsesInput(msgs []Message) responses.ResponseInputParam {
	var items []responses.ResponseInputItemUnionParam

	for _, msg := range msgs {
		content := msg.GetContentAsString()

		switch msg.Role {
		case "system":
			items = append(items, responses.ResponseInputItemParamOfMessage(
				content, responses.EasyInputMessageRoleSystem,
			))

		case "user":
			items = append(items, responses.ResponseInputItemParamOfMessage(
				content, responses.EasyInputMessageRoleUser,
			))

		case "assistant":
			if content != "" {
				items = append(items, responses.ResponseInputItemParamOfMessage(
					content, responses.EasyInputMessageRoleAssistant,
				))
			}
			for _, tc := range msg.ToolCalls {
				items = append(items, responses.ResponseInputItemParamOfFunctionCall(
					tc.Function.Arguments, tc.ID, tc.Function.Name,
				))
			}

		case "tool":
			items = append(items, responses.ResponseInputItemParamOfFunctionCallOutput(
				msg.ToolCallID, content,
			))
		}
	}

	return items
}

var _ ChatProvider = (*OpenAIProvider)(nil)
