package llm

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"assistente/internal/logging"
	"assistente/internal/oauthflow"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/packages/param"
	"github.com/openai/openai-go/responses"
)

// Apply route capabilities without mutating the user's profile or its history.
func applyChatGPTParams(p *responses.ResponseNewParams, messages []Message) {
	p.Temperature = param.Opt[float64]{}
	p.TopP = param.Opt[float64]{}
	p.MaxOutputTokens = param.Opt[int64]{}
	p.Store = param.NewOpt(false)
	var instructions []string
	filtered := make([]Message, 0, len(messages))
	for _, m := range messages {
		if m.Role == "system" {
			instructions = append(instructions, m.GetContentAsString())
		} else {
			filtered = append(filtered, m)
		}
	}
	p.Instructions = param.NewOpt(strings.Join(instructions, "\n\n"))
	p.Input = responses.ResponseNewParamsInputUnion{OfInputItemList: convertToResponsesInput(filtered)}
	for _, item := range p.Input.OfInputItemList {
		if item.OfFunctionCall != nil {
			item.OfFunctionCall.SetExtraFields(map[string]any{"namespace": "assistente"})
		}
	}
	if len(p.Tools) > 0 {
		functions := p.Tools
		p.Tools = nil
		p.SetExtraFields(map[string]any{"tools": []any{map[string]any{"type": "namespace", "name": "assistente", "description": "Local Assistente tools", "tools": functions}}})
	}
}
func (p *OpenAIProvider) ModelOptions(ctx context.Context) ([]ModelOption, error) {
	if p.provider.Type == ProviderChatGPT {
		return p.chatGPTModels(ctx)
	}
	ids, err := p.GetModels(ctx)
	return modelOptionsOf(ids), err
}
func (p *OpenAIProvider) RefreshModelOptions(ctx context.Context) ([]ModelOption, error) {
	return p.ModelOptions(ctx)
}
func (p *OpenAIProvider) chatGPTModels(ctx context.Context) ([]ModelOption, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(p.provider.BaseURL, "/")+"/models", nil)
	if err != nil {
		return nil, errors.New(chatGPTTransportFailure(ctx, err))
	}
	resp, err := newHTTPClientForProvider(p.provider, p.credMgr).Do(req)
	if err != nil {
		return nil, errors.New(chatGPTTransportFailure(ctx, err))
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New(chatGPTStatusFailure(ctx, resp.StatusCode))
	}
	var result struct {
		Models []struct {
			Slug       string `json:"slug"`
			Name       string `json:"display_name"`
			Visibility string `json:"visibility"`
		} `json:"models"`
	}
	if err = json.NewDecoder(io.LimitReader(resp.Body, 2*1024*1024)).Decode(&result); err != nil {
		return nil, errors.New(chatGPTFailure(ctx, ""))
	}
	options := make([]ModelOption, 0, len(result.Models))
	for _, m := range result.Models {
		if m.Visibility == "list" && m.Slug != "" {
			name := m.Name
			if name == "" {
				name = m.Slug
			}
			options = append(options, ModelOption{Value: m.Slug, Label: name})
		}
	}
	return options, nil
}

// Synchronous callers still use the same streaming parser and completion guard.
type chatGPTCollector struct {
	text string
	err  error
}

func (h *chatGPTCollector) OnChunk(string)              {}
func (h *chatGPTCollector) OnThinking(string)           {}
func (h *chatGPTCollector) OnThinkingDone(string)       {}
func (h *chatGPTCollector) OnMCPToolEvent(MCPToolEvent) {}
func (h *chatGPTCollector) OnToolCalls([]ToolCall, string, Usage, string) {
	h.err = errors.New("chatgpt_unexpected_tool_call")
}
func (h *chatGPTCollector) OnError(e string)                      { h.err = errors.New(e) }
func (h *chatGPTCollector) OnDone(text string, _ Usage, _ string) { h.text = text }

func chatGPTFailure(ctx context.Context, code string) string {
	result := "chatgpt_request_failed"
	switch code {
	case "subscription_sharing_usage_limit_exceeded":
		result = "chatgpt_plan_limit"
	case "model_not_found", "model_not_available":
		result = "chatgpt_model_unavailable"
	case "invalid_api_key", "authentication_error":
		result = "chatgpt_reauthorization_required"
	case "permission_denied", "insufficient_scope":
		result = "chatgpt_permission_required"
	case "temporarily_unavailable":
		result = "chatgpt_temporarily_unavailable"
	case "rate_limit_exceeded":
		result = "chatgpt_rate_limit"
	}
	// Only allowlisted diagnostics; the remote description can contain secrets.
	logging.Warnf(ctx, "llm.chatgpt", "chatgpt_failure code=%s", result)
	return result
}
func chatGPTTransportFailure(ctx context.Context, err error) string {
	if errors.Is(err, oauthflow.ErrReauthorize) || errors.Is(err, oauthflow.ErrNotFound) {
		return chatGPTFailure(ctx, "authentication_error")
	}
	if errors.Is(err, oauthflow.ErrPermission) {
		return chatGPTFailure(ctx, "insufficient_scope")
	}
	if errors.Is(err, oauthflow.ErrTransient) {
		return chatGPTFailure(ctx, "temporarily_unavailable")
	}
	if errors.Is(err, context.Canceled) {
		return "chatgpt_request_cancelled"
	}
	var apiError *openai.Error
	if errors.As(err, &apiError) {
		if apiError.Code != "" {
			return chatGPTFailure(ctx, apiError.Code)
		}
		return chatGPTStatusFailure(ctx, apiError.StatusCode)
	}
	return chatGPTFailure(ctx, "")
}

func chatGPTStatusFailure(ctx context.Context, status int) string {
	switch {
	case status == http.StatusUnauthorized:
		return chatGPTFailure(ctx, "authentication_error")
	case status == http.StatusForbidden:
		return chatGPTFailure(ctx, "permission_denied")
	case status == http.StatusTooManyRequests:
		return chatGPTFailure(ctx, "rate_limit_exceeded")
	case status >= 500:
		return chatGPTFailure(ctx, "temporarily_unavailable")
	default:
		return chatGPTFailure(ctx, "")
	}
}
