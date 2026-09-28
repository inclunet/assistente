package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"assistente/internal/credentials"
)

func TestWebSearch_ProviderInvalido(t *testing.T) {
	tool := NewWebSearch(credentials.NewManager(nil))
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"query":"go","provider":"google"}`))
	if err != nil {
		t.Fatalf("Execute retornou erro: %v", err)
	}
	if !result.IsError {
		t.Fatal("provedor desconhecido deveria ser erro")
	}
	for _, want := range []string{"auto", "brave", "tavily", "bing", "duckduckgo"} {
		if !strings.Contains(result.Content, want) {
			t.Errorf("erro deveria listar %q: %s", want, result.Content)
		}
	}
}

func TestWebSearch_ProviderSchemaEnum(t *testing.T) {
	tool := NewWebSearch(nil)
	var schema map[string]any
	if err := json.Unmarshal(tool.Parameters(), &schema); err != nil {
		t.Fatalf("Parameters() deve retornar JSON válido: %v", err)
	}
	props := schema["properties"].(map[string]any)
	provider, ok := props["provider"].(map[string]any)
	if !ok {
		t.Fatal("schema deve ter propriedade 'provider'")
	}
	var got []string
	for _, v := range provider["enum"].([]any) {
		got = append(got, v.(string))
	}
	want := []string{"auto", "brave", "tavily", "bing", "duckduckgo"}
	if len(got) != len(want) {
		t.Fatalf("enum incorreto: %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("enum incorreto: %v", got)
		}
	}
	if provider["default"] != "auto" {
		t.Errorf("default deveria ser auto, got %v", provider["default"])
	}
}

func TestWebSearch_ProviderExplicitoPulaCadeia(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results": [{"title": "Go", "url": "https://go.dev", "content": "x"}]}`))
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	credMgr := credentials.NewManager(nil)
	if err := credMgr.RegisterPattern(u.Hostname(), &credentials.AuthConfig{Source: "static", Type: "bearer", Token: "x"}); err != nil {
		t.Fatalf("registro de credencial: %v", err)
	}
	tool := NewWebSearch(credMgr)
	tool.tavily = &tavilyProvider{credMgr: credMgr, endpointOverride: srv.URL}
	tool.fallback = &mockSearchProvider{results: []SearchResult{{Title: "Nunca", URL: "https://nunca.dev"}}}

	// Brave não tem credencial, mas o pedido explícito começa na Tavily.
	result, err := tool.Execute(trustedBingTestContext(t, context.Background(), srv), json.RawMessage(`{"query":"go","provider":"tavily"}`))
	if err != nil {
		t.Fatalf("Execute retornou erro: %v", err)
	}
	if result.IsError {
		t.Fatalf("Execute marcou erro: %s", result.Content)
	}
	var out webSearchJSONOutput
	if err := json.Unmarshal([]byte(result.Content), &out); err != nil {
		t.Fatalf("saída não é JSON canônico: %v", err)
	}
	if out.Provider != "Tavily" {
		t.Errorf("esperava provider Tavily, got %q", out.Provider)
	}
	if out.Notice != "" {
		t.Errorf("pedido atendido não deve ter aviso: %q", out.Notice)
	}
}

func TestWebSearch_NoticeNoFallbackAuto(t *testing.T) {
	credMgr := credentials.NewManager(nil)
	tool := NewWebSearch(credMgr)
	tool.fallback = &mockSearchProvider{results: []SearchResult{{Title: "DDG", URL: "https://exemplo.dev"}}}

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"query":"go"}`))
	if err != nil {
		t.Fatalf("Execute retornou erro: %v", err)
	}
	var out webSearchJSONOutput
	if err := json.Unmarshal([]byte(result.Content), &out); err != nil {
		t.Fatalf("saída não é JSON canônico: %v", err)
	}
	if out.Notice == "" {
		t.Error("fallback gratuito deveria trazer aviso")
	}
	for _, want := range []string{"DuckDuckGo", "api.search.brave.com", "api.tavily.com", "api.bing.microsoft.com"} {
		if !strings.Contains(out.Notice, want) {
			t.Errorf("aviso deveria mencionar %q: %q", want, out.Notice)
		}
	}
}

func TestWebSearch_ProviderDuckDuckGoExplicitoSemAviso(t *testing.T) {
	credMgr := credentials.NewManager(nil)
	tool := NewWebSearch(credMgr)
	tool.fallback = &mockSearchProvider{results: []SearchResult{{Title: "DDG", URL: "https://exemplo.dev"}}}

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"query":"go","provider":"duckduckgo"}`))
	if err != nil {
		t.Fatalf("Execute retornou erro: %v", err)
	}
	var out webSearchJSONOutput
	if err := json.Unmarshal([]byte(result.Content), &out); err != nil {
		t.Fatalf("saída não é JSON canônico: %v", err)
	}
	if out.Count != 1 {
		t.Fatalf("fallback deveria atender: %+v", out)
	}
	if out.Notice != "" {
		t.Errorf("pedido explícito de DDG não deve ter aviso: %q", out.Notice)
	}
}

func TestWebSearch_ProviderBing401AvancaComAviso(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	credMgr := credentials.NewManager(nil)
	if err := credMgr.RegisterPattern(u.Hostname(), &credentials.AuthConfig{Source: "static", Type: "bearer", Token: "x"}); err != nil {
		t.Fatalf("registro de credencial: %v", err)
	}
	tool := NewWebSearch(credMgr)
	tool.bing = &bingProvider{credMgr: credMgr, endpointOverride: srv.URL}
	tool.fallback = &mockSearchProvider{results: []SearchResult{{Title: "DDG", URL: "https://exemplo.dev"}}}

	result, err := tool.Execute(trustedBingTestContext(t, context.Background(), srv), json.RawMessage(`{"query":"go","provider":"bing"}`))
	if err != nil {
		t.Fatalf("Execute retornou erro: %v", err)
	}
	var out webSearchJSONOutput
	if err := json.Unmarshal([]byte(result.Content), &out); err != nil {
		t.Fatalf("saída não é JSON canônico: %v", err)
	}
	if out.Count != 1 || out.Results[0].Title != "DDG" {
		t.Fatalf("fallback deveria atender após 401: %+v", out)
	}
	if out.Notice == "" {
		t.Error("queda no fallback deveria trazer aviso")
	}
}

func TestWebSearch_ProviderCustomRejeitaSelecao(t *testing.T) {
	tool := NewWebSearchWithProvider(credentials.NewManager(nil), &mockSearchProvider{})
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"query":"go","provider":"bing"}`))
	if err != nil {
		t.Fatalf("Execute retornou erro: %v", err)
	}
	if !result.IsError {
		t.Error("backend customizado deveria rejeitar seleção explícita")
	}
}

func TestWebSearch_ProviderCaseInsensitive(t *testing.T) {
	credMgr := credentials.NewManager(nil)
	tool := NewWebSearch(credMgr)
	tool.fallback = &mockSearchProvider{results: []SearchResult{{Title: "DDG", URL: "https://exemplo.dev"}}}

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"query":"go","provider":"DuckDuckGo"}`))
	if err != nil {
		t.Fatalf("Execute retornou erro: %v", err)
	}
	if result.IsError {
		t.Fatalf("nome com outra caixa deveria ser aceito: %s", result.Content)
	}
	var out webSearchJSONOutput
	if err := json.Unmarshal([]byte(result.Content), &out); err != nil {
		t.Fatalf("saída não é JSON canônico: %v", err)
	}
	if out.Count != 1 {
		t.Fatalf("fallback deveria atender: %+v", out)
	}
}

func TestWebSearch_ProviderNoticeOmitidoQuandoVazio(t *testing.T) {
	// Compatibilidade com consumidores programáticos: sem aviso, o campo
	// não aparece no JSON.
	credMgr := credentials.NewManager(nil)
	tool := NewWebSearchWithProvider(credMgr, &mockSearchProvider{results: []SearchResult{{Title: "M", URL: "https://m.dev"}}})
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"query":"go"}`))
	if err != nil {
		t.Fatalf("Execute retornou erro: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(result.Content), &raw); err != nil {
		t.Fatalf("saída não é JSON: %v", err)
	}
	if _, ok := raw["notice"]; ok {
		t.Error("campo notice deveria ser omitido quando vazio")
	}
}
