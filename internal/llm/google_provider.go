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
// O client ÃƒÆ’Ã‚Â© criado sob demanda em cada chamada de StreamChat porque
// genai.NewClient requer context e pode falhar.
func NewGoogleProvider(provider *ProviderConfig, credMgr *credentials.Manager) *GoogleProvider {
	return &GoogleProvider{
		provider: provider,
		credMgr:  credMgr,
	}
}

// NativeMCPCapable: o SDK Gemini nÃƒÆ’Ã‚Â£o implementa passthrough de MCP nativo, entÃƒÆ’Ã‚Â£o
// nÃƒÆ’Ã‚Â£o ÃƒÆ’Ã‚Â© fisicamente capaz de emitir type:"mcp" ÃƒÂ¢Ã¢â€šÂ¬Ã¢â‚¬Â um override de perfil "true"
// nÃƒÆ’Ã‚Â£o tem como ser honrado e os MCP servers continuam via modo adapter.
func (p *GoogleProvider) NativeMCPCapable() bool {
	return false
}

func (p *GoogleProvider) WithMCPServers(_ []MCPServerConfig) ChatProvider {
	return p
}

// newStreamingClient cria o client Gemini para streaming: http.Client sem
// Timeout global (que cortava streams longos no meio), com timeouts
// granulares de conexÃƒÂ£o/cabeÃƒÂ§alho. O teto ÃƒÂ© o contexto da request.
func (p *GoogleProvider) newStreamingClient(ctx context.Context) (*genai.Client, error) {
	apiKey := ""
	if p.credMgr != nil && p.provider.CredentialPattern != "" {
		if auth, err := p.credMgr.GetByPatternWithContext(ctx, p.provider.CredentialPattern); err == nil && auth != nil && auth.Token != "" {
			apiKey = auth.Token
		}
	}

	cc := &genai.ClientConfig{
		APIKey:     apiKey,
		Backend:    genai.BackendGeminiAPI,
		HTTPClient: newStreamingHTTPClientForProvider(p.provider, p.credMgr),
	}
	if u := strings.TrimSpace(p.provider.BaseURL); u != "" {
		cc.HTTPOptions.BaseURL = strings.TrimSuffix(u, "/")
	}

	return genai.NewClient(ctx, cc)
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
	if u := strings.TrimSpace(p.provider.BaseURL); u != "" {
		cc.HTTPOptions.BaseURL = strings.TrimSuffix(u, "/")
	}

	return genai.NewClient(ctx, cc)
}

func (p *GoogleProvider) SendChat(ctx context.Context, messages []Message, params ChatParams) (string, error) {
	model := resolveModel(p.provider, params.Model)
	if model == "" {
		return "", fmt.Errorf("nenhum modelo especificado e nenhum modelo padrÃƒÆ’Ã‚Â£o configurado")
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
		handler.OnError("Nenhum modelo especificado e nenhum modelo padrÃƒÆ’Ã‚Â£o configurado")
		return
	}

	client, err := p.newStreamingClient(ctx)
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
			// Visibilidade: nunca deixar a pessoa no silÃƒÂªncio do backoff.
			notifyTurnNotice(handler, TurnNotice{Kind: TurnNoticeStreamRetry, Count: attempt})
			sleepWithJitter(ctx, bk)
			bk = nextBackoff(bk, maxBk)
			continue
		}

		handler.OnError("MÃƒÆ’Ã‚Â¡ximo de tentativas de streaming excedido")
	}
}

func (p *GoogleProvider) doStream(ctx context.Context, client *genai.Client, model string, contents []*genai.Content, config *genai.GenerateContentConfig, handler StreamHandler) bool {
	// Watchdog de ociosidade (ver stream_watchdog.go): servidor que para de
	// enviar sem fechar a conexÃƒÂ£o nÃƒÂ£o pode prender a leitura atÃƒÂ© o timeout.
	watchCtx, wd := startStreamWatchdog(ctx, streamIdleTimeoutForProvider(p.provider), nil)
	defer wd.Stop()

	var fullResponse strings.Builder
	var fullReasoning strings.Builder
	var emittedAnything bool
	var lastUsage Usage
	var functionCalls []ToolCall
	var finish FinishInfo

	for resp, err := range client.Models.GenerateContentStream(watchCtx, model, contents, config) {
		wd.Kick()
		if err != nil {
			errStr := err.Error()
			logging.Errorf(ctx, "llm.google-provider", "[GoogleProvider] Stream error: %s", errStr)

			// Cancelamento do usuÃƒÂ¡rio (contexto pai): nunca retentar.
			if ctx.Err() != nil {
				handler.OnError("Streaming cancelado: " + ctx.Err().Error())
				return true
			}

			// Watchdog de ociosidade estourou. Sem conteÃƒÂºdo emitido, a tentativa
			// ÃƒÂ© descartÃƒÂ¡vel; com conteÃƒÂºdo jÃƒÂ¡ entregue, repetir duplicaria a resposta.
			if wd.TimedOut() {
				if !emittedAnything {
					return false
				}
				handler.OnError(streamIdleErrorMessage)
				return true
			}

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

	// Guarda de corrida: o watchdog pode estourar exatamente quando o
	// servidor fecha a conexão, deixando o iterador terminar sem erro com
	// resposta truncada. Nesse caso não há conclusão válida a entregar.
	if wd.TimedOut() {
		logging.Errorf(ctx, "llm.google-provider", "[GoogleProvider] Stream encerrou junto com timeout de inatividade: %d bytes parciais", fullResponse.Len())
		if !emittedAnything {
			return false
		}
		handler.OnError(streamIdleErrorMessage)
		return true
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
			// Google usa FunctionResponse com o nome da funÃƒÆ’Ã‚Â§ÃƒÆ’Ã‚Â£o.
			// Precisamos extrair o nome do tool call correspondente.
			// O ToolCallID contÃƒÆ’Ã‚Â©m o ID, mas precisamos do nome.
			// ConvenÃƒÆ’Ã‚Â§ÃƒÆ’Ã‚Â£o: usar ToolCallID como nome se nÃƒÆ’Ã‚Â£o tivermos melhor info.
			name := msg.ToolCallID
			contents = append(contents, genai.NewContentFromFunctionResponse(name, resp, "user"))
		}
	}

	return system, contents
}

// convertGoogleTools converte definiÃƒÆ’Ã‚Â§ÃƒÆ’Ã‚Âµes de ferramentas para o formato Google GenAI.
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
