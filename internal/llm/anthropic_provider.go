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
	client   *anthropic.Client
	provider *ProviderConfig
}

// NewAnthropicProvider cria um provider Anthropic com a SDK oficial.
func NewAnthropicProvider(provider *ProviderConfig, credMgr *credentials.Manager) *AnthropicProvider {
	httpClient := newHTTPClientForProvider(provider, credMgr)

	opts := []anthropicoption.RequestOption{
		anthropicoption.WithHTTPClient(httpClient),
		anthropicoption.WithAPIKey("managed-by-credential-transport"),
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

func (p *AnthropicProvider) SupportsNativeMCP() bool {
	return true
}

func (p *AnthropicProvider) WithMCPServers(_ []MCPServerConfig) ChatProvider {
	// TODO: MCP Connector (mcp_servers[] + beta header mcp-client-2025-11-20)
	return p
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

	system, anthropicMsgs := convertToAnthropicMessages(messages)

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

func (p *AnthropicProvider) GetModels(ctx context.Context) ([]string, error) {
	page, err := p.client.Models.List(ctx, anthropic.ModelListParams{})
	if err != nil {
		return nil, fmt.Errorf("erro ao listar modelos: %w", err)
	}

	var models []string
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

	system, anthropicMsgs := convertToAnthropicMessages(messages)

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

func (p *AnthropicProvider) doStream(ctx context.Context, params anthropic.MessageNewParams, handler StreamHandler) bool {
	stream := p.client.Messages.NewStreaming(ctx, params)

	var fullResponse strings.Builder
	var fullReasoning strings.Builder
	var emittedAnything bool
	var lastUsage Usage
	var lastModel string

	// Acumula tool calls por index (content_block_start → content_block_delta → content_block_stop)
	type pendingToolCall struct {
		ID        string
		Name      string
		ArgsJSON  strings.Builder
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
			if event.Message.Usage.InputTokens > 0 {
				lastUsage.PromptTokens = int(event.Message.Usage.InputTokens)
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
				lastUsage.CompletionTokens = int(event.Usage.OutputTokens)
				lastUsage.TotalTokens = lastUsage.PromptTokens + lastUsage.CompletionTokens
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
func convertToAnthropicMessages(msgs []Message) ([]anthropic.TextBlockParam, []anthropic.MessageParam) {
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
			system = append(system, anthropic.TextBlockParam{Text: content})

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
