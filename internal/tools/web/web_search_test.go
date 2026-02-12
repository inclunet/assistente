package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestWebSearch_Name(t *testing.T) {
	tool := NewWebSearch()
	if tool.Name() != "web_search" {
		t.Errorf("expected 'web_search', got '%s'", tool.Name())
	}
}

func TestWebSearch_Parameters(t *testing.T) {
	tool := NewWebSearch()
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
	tool := NewWebSearch()
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

func (m *mockSearchProvider) Search(ctx context.Context, client *http.Client, query string, maxResults int) ([]SearchResult, error) {
	if m.err != nil {
		return nil, m.err
	}
	if maxResults < len(m.results) {
		return m.results[:maxResults], nil
	}
	return m.results, nil
}

func TestWebSearch_WithMockProvider(t *testing.T) {
	provider := &mockSearchProvider{
		results: []SearchResult{
			{Title: "Go Programming", URL: "https://go.dev", Snippet: "The Go programming language"},
			{Title: "Go Tutorial", URL: "https://go.dev/tour", Snippet: "A tour of Go"},
		},
	}

	tool := NewWebSearchWithProvider(provider)
	args := `{"query": "golang"}`
	result, err := tool.Execute(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("Execute retornou erro: %v", err)
	}
	if result.IsError {
		t.Fatalf("resultado é erro: %s", result.Content)
	}

	if !strings.Contains(result.Content, "Go Programming") {
		t.Error("deve conter primeiro resultado")
	}
	if !strings.Contains(result.Content, "Go Tutorial") {
		t.Error("deve conter segundo resultado")
	}
	if !strings.Contains(result.Content, "https://go.dev") {
		t.Error("deve conter URL")
	}
	if !strings.Contains(result.Content, "2 resultados") {
		t.Error("deve indicar número de resultados")
	}
}

func TestWebSearch_NoResults(t *testing.T) {
	provider := &mockSearchProvider{results: nil}
	tool := NewWebSearchWithProvider(provider)

	args := `{"query": "xyznonexistent123"}`
	result, err := tool.Execute(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("Execute retornou erro: %v", err)
	}
	if result.IsError {
		t.Error("sem resultados não é erro, apenas informa")
	}
	if !strings.Contains(result.Content, "Nenhum resultado") {
		t.Error("deve indicar que não encontrou")
	}
}

func TestWebSearch_ProviderError(t *testing.T) {
	provider := &mockSearchProvider{err: fmt.Errorf("network timeout")}
	tool := NewWebSearchWithProvider(provider)

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
	results := make([]SearchResult, 10)
	for i := range results {
		results[i] = SearchResult{
			Title:   fmt.Sprintf("Result %d", i),
			URL:     fmt.Sprintf("https://example.com/%d", i),
			Snippet: fmt.Sprintf("Snippet %d", i),
		}
	}

	provider := &mockSearchProvider{results: results}
	tool := NewWebSearchWithProvider(provider)

	args := `{"query": "test", "max_results": 3}`
	result, err := tool.Execute(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("Execute retornou erro: %v", err)
	}
	if result.IsError {
		t.Fatalf("resultado é erro: %s", result.Content)
	}
	if !strings.Contains(result.Content, "3 resultados") {
		t.Errorf("deve limitar a 3 resultados: %s", result.Content)
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
