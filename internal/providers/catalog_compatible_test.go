package providers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"assistente/internal/credentials"
	"assistente/internal/llm"
	"assistente/internal/llmcatalog"
)

type catalogCompatibleHandler struct {
	calls   []llm.ToolCall
	content string
	err     string
	done    bool
}

func (h *catalogCompatibleHandler) OnChunk(s string)    { h.content += s }
func (*catalogCompatibleHandler) OnThinking(string)     {}
func (*catalogCompatibleHandler) OnThinkingDone(string) {}
func (h *catalogCompatibleHandler) OnToolCalls(calls []llm.ToolCall, _ string, _ llm.Usage, _ string) {
	h.calls = calls
}
func (h *catalogCompatibleHandler) OnError(s string)                       { h.err = s }
func (h *catalogCompatibleHandler) OnDone(_ string, _ llm.Usage, _ string) { h.done = true }
func (*catalogCompatibleHandler) OnMCPToolEvent(llm.MCPToolEvent)          {}

func TestCatalogCompatibleProvidersUsePublishedRoutesAndCredentials(t *testing.T) {
	for _, tc := range []struct{ profile, base, path, auth string }{
		{"ollama", "http://localhost:11434/v1", "/v1", ""},
		{"cohere", "https://api.cohere.ai/compatibility/v1", "/compatibility/v1", "Bearer test-cohere"},
	} {
		t.Run(tc.profile, func(t *testing.T) {
			config, err := catalogTestTemplate(t, tc.profile)
			if err != nil {
				t.Fatal(err)
			}
			if config.BaseURL != tc.base || config.GetAPIFormat() != llm.APIFormatOpenAI {
				t.Fatalf("catalog configuration: %+v", config)
			}
			manager := credentials.NewManager(nil)
			if tc.auth != "" {
				if err := manager.RegisterPattern(config.CredentialPattern, &credentials.AuthConfig{Source: "static", Type: "bearer", Token: "test-cohere"}); err != nil {
					t.Fatal(err)
				}
			}
			const model = " exact-model "
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				if r.Header.Get("Authorization") != tc.auth {
					t.Errorf("Authorization=%q want=%q", r.Header.Get("Authorization"), tc.auth)
				}
				if r.URL.Path == tc.path+"/models" {
					w.Header().Set("Content-Type", "application/json")
					if tc.profile == "cohere" {
						http.Error(w, "not found", http.StatusNotFound)
						return
					}
					_, _ = fmt.Fprintf(w, `{"object":"list","data":[{"id":%q,"object":"model"}]}`, model)
					return
				}
				if r.URL.Path != tc.path+"/chat/completions" {
					t.Errorf("unexpected route %s", r.URL.Path)
					http.NotFound(w, r)
					return
				}
				var body struct {
					Model       string               `json:"model"`
					Stream      bool                 `json:"stream"`
					Temperature *float64             `json:"temperature"`
					MaxTokens   *int                 `json:"max_tokens"`
					Tools       []llm.ToolDefinition `json:"tools"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
					return
				}
				if body.Model != model || body.Temperature == nil || *body.Temperature != 0 || body.MaxTokens == nil || *body.MaxTokens != 128 {
					t.Errorf("parameters not preserved: %+v", body)
				}
				if !body.Stream {
					w.Header().Set("Content-Type", "application/json")
					_, _ = fmt.Fprint(w, `{"id":"chat-1","object":"chat.completion","created":1,"model":"exact-model","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`)
					return
				}
				if len(body.Tools) != 1 || body.Tools[0].Function.Name != "get_weather" {
					t.Errorf("tools=%+v", body.Tools)
				}
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = fmt.Fprint(w, "data: "+`{"id":"chat-2","object":"chat.completion.chunk","created":1,"model":"exact-model","choices":[{"index":0,"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"call-1","type":"function","function":{"name":"get_weather","arguments":"{\"city\":\"Recife\"}"}}]},"finish_reason":null}]}`+"\n\n"+"data: "+`{"id":"chat-2","object":"chat.completion.chunk","created":1,"model":"exact-model","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`+"\n\n"+"data: [DONE]\n\n")
			}))
			defer server.Close()
			// Preserve the catalog path; only replace origin with this isolated fixture.
			parsed, _ := url.Parse(config.BaseURL)
			config.BaseURL = server.URL + parsed.Path
			cp := llm.NewChatProvider(config, manager, nil)
			if _, ok := cp.(*llm.OpenAIProvider); !ok {
				t.Fatalf("unexpected adapter %T", cp)
			}
			if cp.NativeMCPCapable() {
				t.Fatal("compatible protocol must not claim native MCP")
			}
			models, err := cp.GetModels(t.Context())
			if tc.profile == "cohere" {
				if err == nil || !strings.Contains(err.Error(), "models_endpoint_not_supported") {
					t.Fatalf("missing models endpoint: %v", err)
				}
			} else if err != nil || len(models) != 1 || models[0] != model {
				t.Fatalf("models=%q err=%v", models, err)
			}
			temperature := 0.0
			tokens := 128
			params := llm.ChatParams{Model: model, Temperature: &temperature, MaxTokens: &tokens}
			answer, err := cp.SendChat(t.Context(), []llm.Message{{Role: "user", Content: "hello"}}, params)
			if err != nil || answer != "ok" {
				t.Fatalf("answer=%q err=%v", answer, err)
			}
			h := &catalogCompatibleHandler{}
			cp.StreamChat(t.Context(), []llm.Message{{Role: "user", Content: "weather"}}, params, h, llm.ToolDefinition{Type: "function", Function: llm.FunctionDefinition{Name: "get_weather", Parameters: json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"}},"required":["city"]}`)}})
			if h.err != "" || h.done || len(h.calls) != 1 || h.calls[0].ID != "call-1" || h.calls[0].Function.Arguments != `{"city":"Recife"}` {
				t.Fatalf("stream=%+v", h)
			}
			if requests < 3 {
				t.Fatalf("expected list, chat and stream; got %d", requests)
			}
		})
	}
}

func TestCatalogCompatibleModelErrorsAreNotUnsupportedEndpoint(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusBadRequest} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "access denied", status) }))
			defer srv.Close()
			config := &llm.ProviderConfig{ID: "test", Name: "Test", Type: llm.ProviderCohere, BaseURL: srv.URL + "/compatibility/v1", APIFormat: llm.APIFormatOpenAI, AuthMode: llm.AuthModeNone}
			_, err := llm.NewChatProvider(config, nil, nil).GetModels(t.Context())
			if err == nil || strings.Contains(err.Error(), "models_endpoint_not_supported") {
				t.Fatalf("status=%d err=%v", status, err)
			}
		})
	}
}

func TestCatalogCompatibleRejectsCachedCohereNativeRoute(t *testing.T) {
	_, err := TemplateFromCatalogServer(llmcatalog.Server{
		ID: "cohere", Name: "Cohere", ProviderFamily: "cohere", Status: "active",
		APIFormat: "openai", BaseURL: "https://api.cohere.ai",
		Auth:      llmcatalog.Auth{Mode: "bearer_api_key", Required: true},
		Endpoints: map[string]llmcatalog.Endpoint{"chat": {Path: "/compatibility/v1/chat/completions"}},
	})
	if err == nil {
		t.Fatal("cached profile with unsupported relative route must be rejected")
	}
}
