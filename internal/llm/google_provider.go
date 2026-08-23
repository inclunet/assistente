package llm

import (
	"assistente/internal/logging"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"assistente/internal/credentials"

	"google.golang.org/genai"
)

// GoogleProvider implementa ChatProvider usando a SDK google.golang.org/genai.
type GoogleProvider struct {
	provider *ProviderConfig
	credMgr  *credentials.Manager
}

// NewGoogleProvider cria um provider Google Gemini com a SDK oficial.
// O client é criado sob demanda em cada chamada de StreamChat porque
// genai.NewClient requer context e pode falhar.
func NewGoogleProvider(provider *ProviderConfig, credMgr *credentials.Manager) *GoogleProvider {
	return &GoogleProvider{
		provider: provider,
		credMgr:  credMgr,
	}
}

// NativeMCPCapable: o SDK Gemini não implementa passthrough de MCP nativo, então
// não é fisicamente capaz de emitir type:"mcp" — um override de perfil "true"
// não tem como ser honrado e os MCP servers continuam via modo adapter.
func (p *GoogleProvider) NativeMCPCapable() bool {
	return false
}

func (p *GoogleProvider) WithMCPServers(_ []MCPServerConfig) ChatProvider {
	return p
}

func (p *GoogleProvider) newClient(ctx context.Context) (*genai.Client, error) {
	apiKey := ""
	if p.credMgr != nil && p.provider.CredentialPattern != "" {
		if auth, err := p.credMgr.GetByPatternWithContext(ctx, p.provider.CredentialPattern); err == nil && auth != nil && auth.Token != "" {
			apiKey = auth.Token
		}
	}

	cc := &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
		HTTPClient: &http.Client{
			Timeout: providerTimeout(p.provider),
		},
	}

	return genai.NewClient(ctx, cc)
}

func (p *GoogleProvider) SendChat(ctx context.Context, messages []Message, params ChatParams) (string, error) {
	model := resolveModel(p.provider, params.Model)
	if model == "" {
		return "", fmt.Errorf("nenhum modelo especificado e nenhum modelo padrão configurado")
	}

	client, err := p.newClient(ctx)
	if err != nil {
		return "", fmt.Errorf("erro ao criar cliente Google: %w", err)
	}

	system, contents := convertToGoogleContents(messages)

	config := &genai.GenerateContentConfig{}
	if system != nil {
		config.SystemInstruction = system
	}
	if params.Temperature > 0 {
		t := float32(params.Temperature)
		config.Temperature = &t
	}
	if params.MaxTokens > 0 {
		config.MaxOutputTokens = int32(params.MaxTokens)
	}

	resp, err := client.Models.GenerateContent(ctx, model, contents, config)
	if err != nil {
		return "", fmt.Errorf("erro ao enviar mensagem: %w", err)
	}
	if len(resp.Candidates) == 0 || resp.Candidates[0].Content == nil {
		return "", fmt.Errorf("nenhuma resposta recebida")
	}

	var sb strings.Builder
	for _, part := range resp.Candidates[0].Content.Parts {
		if part.Text != "" {
			sb.WriteString(part.Text)
		}
	}
	return sb.String(), nil
}

func (p *GoogleProvider) GetModels(ctx context.Context) (models []string, retErr error) {
	defer func() {
		if r := recover(); r != nil {
			logging.Errorf(ctx, "llm.google-provider", "[GoogleProvider] PANIC no SDK Models.List: %v", r)
			retErr = fmt.Errorf("panic no SDK: %v", r)
		}
	}()

	client, err := p.newClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("erro ao criar cliente Google: %w", err)
	}

	page, err := client.Models.List(ctx, &genai.ListModelsConfig{})
	if err != nil {
		return nil, fmt.Errorf("erro ao listar modelos: %w", err)
	}

	for _, m := range page.Items {
		if m.Name != "" {
			models = append(models, strings.TrimPrefix(m.Name, "models/"))
		}
	}

	sort.Strings(models)
	return models, nil
}

func (p *GoogleProvider) SimpleChat(ctx context.Context, model, systemPrompt, userMessage string) (string, error) {
	msgs := []Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userMessage},
	}
	return p.SendChat(ctx, msgs, ChatParams{Model: model})
}

func (p *GoogleProvider) StreamChat(ctx context.Context, messages []Message, params ChatParams, handler StreamHandler, tools ...ToolDefinition) {
	model := resolveModel(p.provider, params.Model)
	if model == "" {
		handler.OnError("Nenhum modelo especificado e nenhum modelo padrão configurado")
		return
	}

	client, err := p.newClient(ctx)
	if err != nil {
		handler.OnError("Erro ao criar cliente Google: " + err.Error())
		return
	}

	system, contents := convertToGoogleContents(messages)

	config := &genai.GenerateContentConfig{}

	if system != nil {
		config.SystemInstruction = system
	}

	if params.Temperature > 0 {
		t := float32(params.Temperature)
		config.Temperature = &t
	}

	if params.MaxTokens > 0 {
		config.MaxOutputTokens = int32(params.MaxTokens)
	}

	if params.TopP > 0 && params.TopP != 1.0 {
		tp := float32(params.TopP)
		config.TopP = &tp
	}

	if len(tools) > 0 {
		config.Tools = []*genai.Tool{convertGoogleTools(tools)}
		toolChoice := "auto"
		if choice, ok := toolChoiceFromContext(ctx); ok {
			if s, ok := choice.(string); ok {
				toolChoice = s
			}
		}
		config.ToolConfig = makeGoogleToolConfig(toolChoice)
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

		done := p.doStream(ctx, client, model, contents, config, handler)
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

func (p *GoogleProvider) doStream(ctx context.Context, client *genai.Client, model string, contents []*genai.Content, config *genai.GenerateContentConfig, handler StreamHandler) bool {
	var fullResponse strings.Builder
	var fullReasoning strings.Builder
	var emittedAnything bool
	var lastUsage Usage
	var functionCalls []ToolCall
	var finish FinishInfo

	for resp, err := range client.Models.GenerateContentStream(ctx, model, contents, config) {
		if err != nil {
			errStr := err.Error()
			logging.Errorf(ctx, "llm.google-provider", "[GoogleProvider] Stream error: %s", errStr)

			if !emittedAnything && isRetryableError(errStr) {
				return false
			}

			handler.OnError(errStr)
			return true
		}

		if resp.UsageMetadata != nil {
			lastUsage = UsageFromGemini(
				int(resp.UsageMetadata.PromptTokenCount),
				int(resp.UsageMetadata.CandidatesTokenCount),
				int(resp.UsageMetadata.TotalTokenCount),
				int(resp.UsageMetadata.CachedContentTokenCount),
			)
		}

		if len(resp.Candidates) == 0 {
			continue
		}

		candidate := resp.Candidates[0]
		if normalized := normalizeGoogleFinishReason(string(candidate.FinishReason)); normalized.Reason != "" {
			finish = normalized
		}
		if candidate.Content == nil {
			continue
		}

		for _, part := range candidate.Content.Parts {
			if part.Thought && part.Text != "" {
				fullReasoning.WriteString(part.Text)
				emittedAnything = true
				handler.OnThinking(part.Text)
				continue
			}

			if part.Text != "" {
				fullResponse.WriteString(part.Text)
				emittedAnything = true
				handler.OnChunk(part.Text)
			}

			if part.FunctionCall != nil {
				argsJSON, _ := json.Marshal(part.FunctionCall.Args)
				callID := part.FunctionCall.ID
				if callID == "" {
					callID = fmt.Sprintf("call_%s_%d", part.FunctionCall.Name, len(functionCalls))
				}
				functionCalls = append(functionCalls, ToolCall{
					ID:   callID,
					Type: "function",
					Function: FunctionCall{
						Name:      part.FunctionCall.Name,
						Arguments: string(argsJSON),
					},
				})
			}
		}
	}

	if fullReasoning.Len() > 0 {
		handler.OnThinkingDone(fullReasoning.String())
	}
	finish = finishInfoWithToolCalls(finish, len(functionCalls))
	ReportFinishReason(handler, finish)

	if len(functionCalls) > 0 {
		handler.OnToolCalls(functionCalls, fullResponse.String(), lastUsage, model)
		return true
	}

	handler.OnDone(fullResponse.String(), lastUsage, model)
	return true
}

// convertToGoogleContents converte mensagens internas para o formato Google GenAI.
// Retorna system instruction separada e lista de contents.
func convertToGoogleContents(msgs []Message) (*genai.Content, []*genai.Content) {
	var system *genai.Content
	var contents []*genai.Content

	for _, msg := range msgs {
		content := msg.GetContentAsString()

		switch msg.Role {
		case "system":
			if system == nil {
				system = &genai.Content{Parts: []*genai.Part{}}
			}
			system.Parts = append(system.Parts, genai.NewPartFromText(content))

		case "user":
			contents = append(contents, genai.NewContentFromText(content, "user"))

		case "assistant":
			c := &genai.Content{Role: "model", Parts: []*genai.Part{}}
			if content != "" {
				c.Parts = append(c.Parts, genai.NewPartFromText(content))
			}
			for _, tc := range msg.ToolCalls {
				var args map[string]any
				if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
					args = map[string]any{}
				}
				c.Parts = append(c.Parts, genai.NewPartFromFunctionCall(tc.Function.Name, args))
			}
			if len(c.Parts) > 0 {
				contents = append(contents, c)
			}

		case "tool":
			var resp map[string]any
			if err := json.Unmarshal([]byte(content), &resp); err != nil {
				resp = map[string]any{"result": content}
			}
			// Google usa FunctionResponse com o nome da função.
			// Precisamos extrair o nome do tool call correspondente.
			// O ToolCallID contém o ID, mas precisamos do nome.
			// Convenção: usar ToolCallID como nome se não tivermos melhor info.
			name := msg.ToolCallID
			contents = append(contents, genai.NewContentFromFunctionResponse(name, resp, "user"))
		}
	}

	return system, contents
}

// convertGoogleTools converte definições de ferramentas para o formato Google GenAI.
func convertGoogleTools(tools []ToolDefinition) *genai.Tool {
	decls := make([]*genai.FunctionDeclaration, 0, len(tools))

	for _, tool := range tools {
		decl := &genai.FunctionDeclaration{
			Name:        tool.Function.Name,
			Description: tool.Function.Description,
		}

		if len(tool.Function.Parameters) > 0 {
			var schema any
			if err := json.Unmarshal(tool.Function.Parameters, &schema); err == nil {
				decl.ParametersJsonSchema = schema
			}
		}

		decls = append(decls, decl)
	}

	return &genai.Tool{FunctionDeclarations: decls}
}

func makeGoogleToolConfig(choice string) *genai.ToolConfig {
	switch choice {
	case "required":
		return &genai.ToolConfig{
			FunctionCallingConfig: &genai.FunctionCallingConfig{
				Mode: genai.FunctionCallingConfigModeAny,
			},
		}
	case "none":
		return &genai.ToolConfig{
			FunctionCallingConfig: &genai.FunctionCallingConfig{
				Mode: genai.FunctionCallingConfigModeNone,
			},
		}
	default:
		return &genai.ToolConfig{
			FunctionCallingConfig: &genai.FunctionCallingConfig{
				Mode: genai.FunctionCallingConfigModeAuto,
			},
		}
	}
}

var _ ChatProvider = (*GoogleProvider)(nil)
