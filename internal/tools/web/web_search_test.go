package web

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"assistente/internal/credentials"
	httpclient "assistente/internal/tools/http"
)

func TestWebSearch_Name(t *testing.T) {
	credMgr := credentials.NewManager(nil)
	tool := NewWebSearch(credMgr)
	if tool.Name() != "web_search" {
		t.Errorf("expected 'web_search', got '%s'", tool.Name())
	}
}

func TestWebSearch_Parameters(t *testing.T) {
	credMgr := credentials.NewManager(nil)
	tool := NewWebSearch(credMgr)
	var schema map[string]interface{}
	if err := json.Unmarshal(tool.Parameters(), &schema); err != nil {
		t.Fatalf("Parameters() deve retornar JSON válido: %v", err)
	}
	props := schema["properties"].(map[string]interface{})
	if _, ok := props["query"]; !ok {
		t.Error("schema deve ter propriedade 'query'")
	}
}

func TestWebSearch_MissingQuery(t *testing.T) {
	credMgr := credentials.NewManager(nil)
	tool := NewWebSearch(credMgr)
	args := `{}`
	result, err := tool.Execute(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("Execute retornou erro: %v", err)
	}
	if !result.IsError {
		t.Error("deve ser erro sem query")
	}
}

// mockSearchProvider é um provider de busca para testes
type mockSearchProvider struct {
	results []SearchResult
	err     error
}

func (m *mockSearchProvider) Name() string { return "MockSearch" }

func (m *mockSearchProvider) Search(ctx context.Context, client *httpclient.Client, query string, offset, maxResults int) ([]SearchResult, error) {
	if m.err != nil {
		return nil, m.err
	}
	// Simula paginação por offset sobre o conjunto fixo de resultados.
	if offset >= len(m.results) {
		return nil, nil
	}
	page := m.results[offset:]
	if maxResults < len(page) {
		return page[:maxResults], nil
	}
	return page, nil
}

func TestWebSearch_WithMockProvider(t *testing.T) {
	credMgr := credentials.NewManager(nil)
	provider := &mockSearchProvider{
		results: []SearchResult{
			{Title: "Go Programming", URL: "https://go.dev", Snippet: "The Go programming language"},
			{Title: "Go Tutorial", URL: "https://go.dev/tour", Snippet: "A tour of Go"},
		},
	}

	tool := NewWebSearchWithProvider(credMgr, provider)
	args := `{"query": "golang"}`
	result, err := tool.Execute(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("Execute retornou erro: %v", err)
	}
	if result.IsError {
		t.Fatalf("resultado é erro: %s", result.Content)
	}

	// Saída canônica é JSON parseável.
	var out webSearchJSONOutput
	if err := json.Unmarshal([]byte(result.Content), &out); err != nil {
		t.Fatalf("Content deve ser JSON válido: %v\ncontent=%s", err, result.Content)
	}
	if out.Count != 2 || len(out.Results) != 2 {
		t.Fatalf("esperado 2 resultados, got count=%d len=%d", out.Count, len(out.Results))
	}
	if out.Results[0].Title != "Go Programming" || out.Results[0].URL != "https://go.dev" {
		t.Errorf("primeiro resultado inesperado: %+v", out.Results[0])
	}
	if out.Results[1].Title != "Go Tutorial" {
		t.Errorf("segundo resultado inesperado: %+v", out.Results[1])
	}
}

func TestWebSearch_Pagination(t *testing.T) {
	credMgr := credentials.NewManager(nil)
	results := make([]SearchResult, 10)
	for i := range results {
		results[i] = SearchResult{Title: fmt.Sprintf("R%d", i), URL: fmt.Sprintf("https://e/%d", i)}
	}
	provider := &mockSearchProvider{results: results}
	tool := NewWebSearchWithProvider(credMgr, provider)

	// Página 1: offset 0, 4 por página → cheia, has_more true.
	res1, err := tool.Execute(context.Background(), json.RawMessage(`{"query":"x","max_results":4,"offset":0}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	var p1 webSearchJSONOutput
	if err := json.Unmarshal([]byte(res1.Content), &p1); err != nil {
		t.Fatalf("JSON inválido: %v", err)
	}
	if p1.Offset != 0 || p1.Count != 4 || !p1.HasMore {
		t.Fatalf("página 1 inesperada: %+v", p1)
	}
	if p1.Results[0].Title != "R0" {
		t.Errorf("página 1 deveria começar em R0: %+v", p1.Results[0])
	}

	// Página 2: offset = 0 + count (4).
	res2, _ := tool.Execute(context.Background(), json.RawMessage(`{"query":"x","max_results":4,"offset":4}`))
	var p2 webSearchJSONOutput
	if err := json.Unmarshal([]byte(res2.Content), &p2); err != nil {
		t.Fatalf("JSON inválido: %v", err)
	}
	if p2.Offset != 4 || p2.Count != 4 || !p2.HasMore {
		t.Fatalf("página 2 inesperada: %+v", p2)
	}
	if p2.Results[0].Title != "R4" {
		t.Errorf("página 2 deveria começar em R4: %+v", p2.Results[0])
	}

	// Última página: offset 8, restam 2 → não cheia, has_more false.
	res3, _ := tool.Execute(context.Background(), json.RawMessage(`{"query":"x","max_results":4,"offset":8}`))
	var p3 webSearchJSONOutput
	if err := json.Unmarshal([]byte(res3.Content), &p3); err != nil {
		t.Fatalf("JSON inválido: %v", err)
	}
	if p3.Offset != 8 || p3.Count != 2 || p3.HasMore {
		t.Fatalf("última página inesperada: %+v", p3)
	}

	// Além do fim: offset 20 → vazio, has_more false, results [].
	res4, _ := tool.Execute(context.Background(), json.RawMessage(`{"query":"x","max_results":4,"offset":20}`))
	if !strings.Contains(res4.Content, `"results":[]`) {
		t.Errorf("além do fim deveria ter results []: %s", res4.Content)
	}
}

func TestWebSearch_NoResults(t *testing.T) {
	credMgr := credentials.NewManager(nil)
	provider := &mockSearchProvider{results: nil}
	tool := NewWebSearchWithProvider(credMgr, provider)

	args := `{"query": "xyznonexistent123"}`
	result, err := tool.Execute(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("Execute retornou erro: %v", err)
	}
	if result.IsError {
		t.Error("sem resultados não é erro, apenas informa")
	}
	// Sem resultados ainda deve ser JSON válido com results: [] (não null), para
	// consumo programático estável.
	if !strings.Contains(result.Content, `"results":[]`) {
		t.Errorf("results vazio deveria serializar como [], got: %s", result.Content)
	}
	var out webSearchJSONOutput
	if err := json.Unmarshal([]byte(result.Content), &out); err != nil {
		t.Fatalf("Content deve ser JSON válido: %v", err)
	}
	if out.Count != 0 || out.Results == nil {
		t.Errorf("esperado count=0 e results não-nulo, got count=%d results=%v", out.Count, out.Results)
	}
}

func TestWebSearch_ProviderError(t *testing.T) {
	credMgr := credentials.NewManager(nil)
	provider := &mockSearchProvider{err: fmt.Errorf("network timeout")}
	tool := NewWebSearchWithProvider(credMgr, provider)

	args := `{"query": "test"}`
	result, err := tool.Execute(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("Execute retornou erro: %v", err)
	}
	if !result.IsError {
		t.Error("deve ser erro quando provider falha")
	}
	if !strings.Contains(result.Content, "network timeout") {
		t.Error("deve conter mensagem do erro")
	}
}

func TestWebSearch_MaxResults(t *testing.T) {
	credMgr := credentials.NewManager(nil)
	results := make([]SearchResult, 10)
	for i := range results {
		results[i] = SearchResult{
			Title:   fmt.Sprintf("Result %d", i),
			URL:     fmt.Sprintf("https://example.com/%d", i),
			Snippet: fmt.Sprintf("Snippet %d", i),
		}
	}

	provider := &mockSearchProvider{results: results}
	tool := NewWebSearchWithProvider(credMgr, provider)

	args := `{"query": "test", "max_results": 3}`
	result, err := tool.Execute(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("Execute retornou erro: %v", err)
	}
	if result.IsError {
		t.Fatalf("resultado é erro: %s", result.Content)
	}
	var out webSearchJSONOutput
	if err := json.Unmarshal([]byte(result.Content), &out); err != nil {
		t.Fatalf("Content deve ser JSON válido: %v", err)
	}
	if out.Count != 3 || len(out.Results) != 3 {
		t.Errorf("deve limitar a 3 resultados: count=%d len=%d", out.Count, len(out.Results))
	}
}

func TestParseDuckDuckGoHTML(t *testing.T) {
	// HTML simulado da estrutura DuckDuckGo HTML lite
	// NOTA: no HTML real do DDG, href vem ANTES de class="result__a"
	html := `<div class="results">
		<div class="result results_links results_links_deep web-result">
			<a rel="nofollow" href="//duckduckgo.com/l/?uddg=https%3A%2F%2Fgo.dev&amp;rut=abc" class="result__a">Go Programming Language</a>
			<a class="result__snippet">The Go programming language official site</a>
		</div>
		<div class="result results_links results_links_deep web-result">
			<a rel="nofollow" href="//duckduckgo.com/l/?uddg=https%3A%2F%2Fen.wikipedia.org%2Fwiki%2FGo&amp;rut=def" class="result__a">Go (programming language) - Wikipedia</a>
			<a class="result__snippet">Go is a statically typed compiled language</a>
		</div>
	</div>`

	results := parseDuckDuckGoHTML(html, 10)

	if len(results) < 2 {
		t.Fatalf("expected at least 2 results, got %d", len(results))
	}

	if results[0].Title != "Go Programming Language" {
		t.Errorf("primeiro título: %s", results[0].Title)
	}
	if results[0].URL != "https://go.dev" {
		t.Errorf("primeira URL: %s", results[0].URL)
	}
}

func TestExtractDDGURL(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{
			"//duckduckgo.com/l/?uddg=https%3A%2F%2Fgo.dev&rut=abc",
			"https://go.dev",
		},
		{
			"https://direct-url.com",
			"https://direct-url.com",
		},
		{
			"//example.com/page",
			"https://example.com/page",
		},
	}

	for _, tt := range tests {
		result := extractDDGURL(tt.input)
		if result != tt.expected {
			t.Errorf("extractDDGURL(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

// TestWebSearch_DuckDuckGoIntegration testa com um servidor HTTP fake
func TestWebSearch_DuckDuckGoHTTPParsing(t *testing.T) {
	// Este teste verifica que o parsing funciona com href antes de class
	body := `<div class="result">
		<a href="//duckduckgo.com/l/?uddg=https%3A%2F%2Fexample.com&rut=x" class="result__a">Example Site</a>
		<a class="result__snippet">An example website</a>
	</div>`

	results := parseDuckDuckGoHTML(body, 5)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Title != "Example Site" {
		t.Errorf("título: %s", results[0].Title)
	}
	if results[0].URL != "https://example.com" {
		t.Errorf("URL: %s", results[0].URL)
	}
}
