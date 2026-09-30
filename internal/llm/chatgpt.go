package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

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
		return nil, err
	}
	resp, err := newHTTPClientForProvider(p.provider, p.credMgr).Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("chatgpt_models_http_%d", resp.StatusCode)
	}
	var result struct {
		Models []struct {
			Slug       string `json:"slug"`
			Name       string `json:"display_name"`
			Visibility string `json:"visibility"`
		} `json:"models"`
	}
	if err = json.NewDecoder(io.LimitReader(resp.Body, 2*1024*1024)).Decode(&result); err != nil {
		return nil, errors.New("chatgpt_models_invalid")
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
