package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"assistente/internal/credentials"
)

// newTestWebFetch cria um WebFetch que permite hosts privados (para httptest)
func newTestWebFetch() *WebFetch {
	credMgr := credentials.NewManager(nil)
	wf := NewWebFetch(credMgr)
	wf.allowPrivateHosts = true
	return wf
}

func TestWebFetch_Name(t *testing.T) {
	credMgr := credentials.NewManager(nil)
	tool := NewWebFetch(credMgr)
	if tool.Name() != "web_fetch" {
		t.Errorf("expected 'web_fetch', got '%s'", tool.Name())
	}
}

func TestWebFetch_Parameters(t *testing.T) {
	credMgr := credentials.NewManager(nil)
	tool := NewWebFetch(credMgr)
	var schema map[string]interface{}
	if err := json.Unmarshal(tool.Parameters(), &schema); err != nil {
		t.Fatalf("Parameters() deve retornar JSON válido: %v", err)
	}
	if schema["type"] != "object" {
		t.Error("schema deve ter type=object")
	}
}

func TestWebFetch_HTMLToText(t *testing.T) {
	// Servidor de teste com HTML
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<html>
		<head><title>Test Page</title><script>var x = 1;</script></head>
		<body>
			<h1>Hello World</h1>
			<p>This is a <strong>test</strong> paragraph.</p>
			<style>.hidden{display:none}</style>
			<div>Another section</div>
		</body>
		</html>`)
	}))
	defer server.Close()

	tool := newTestWebFetch()
	args, _ := json.Marshal(map[string]string{"url": server.URL})
	result, err := tool.Execute(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("Execute retornou erro: %v", err)
	}
	if result.IsError {
		t.Fatalf("resultado é erro: %s", result.Content)
	}

	// Deve conter o texto mas não o script/style
	if !strings.Contains(result.Content, "Hello World") {
		t.Error("deve conter 'Hello World'")
	}
	if !strings.Contains(result.Content, "test") {
		t.Error("deve conter 'test'")
	}
	if strings.Contains(result.Content, "var x = 1") {
		t.Error("não deve conter conteúdo de script")
	}
	if strings.Contains(result.Content, ".hidden") {
		t.Error("não deve conter conteúdo de style")
	}
}

func TestWebFetch_PlainText(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprint(w, "Simple plain text response")
	}))
	defer server.Close()

	tool := newTestWebFetch()
	args, _ := json.Marshal(map[string]string{"url": server.URL})
	result, err := tool.Execute(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("Execute retornou erro: %v", err)
	}
	if result.IsError {
		t.Fatalf("resultado é erro: %s", result.Content)
	}
	if !strings.Contains(result.Content, "Simple plain text response") {
		t.Error("deve conter o texto plain")
	}
}

func TestWebFetch_MarkdownMode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<html><body>
			<h1>Title</h1>
			<p>A paragraph with <strong>bold</strong> and <em>italic</em>.</p>
			<ul><li>Item 1</li><li>Item 2</li></ul>
		</body></html>`)
	}))
	defer server.Close()

	tool := newTestWebFetch()
	args, _ := json.Marshal(map[string]interface{}{"url": server.URL, "extract_mode": "markdown"})
	result, err := tool.Execute(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("Execute retornou erro: %v", err)
	}
	if result.IsError {
		t.Fatalf("resultado é erro: %s", result.Content)
	}
	if !strings.Contains(result.Content, "# Title") {
		t.Errorf("deve conter markdown heading: %s", result.Content)
	}
	// O extractor pode adicionar espaço antes de fechar **: "**bold **" — ambos são válidos
	if !strings.Contains(result.Content, "**bold") {
		t.Errorf("deve conter bold markdown: %s", result.Content)
	}
	if !strings.Contains(result.Content, "Item 1") {
		t.Errorf("deve conter item de lista: %s", result.Content)
	}
}

func TestWebFetch_Truncation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		// Conteúdo grande
		fmt.Fprint(w, strings.Repeat("a", 10000))
	}))
	defer server.Close()

	tool := newTestWebFetch()
	maxLen := 100
	args, _ := json.Marshal(map[string]interface{}{"url": server.URL, "max_length": maxLen})
	result, err := tool.Execute(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("Execute retornou erro: %v", err)
	}
	if result.IsError {
		t.Fatalf("resultado é erro: %s", result.Content)
	}
	if !strings.Contains(result.Content, "TRUNCADO") {
		t.Error("deve indicar truncamento")
	}
}

func TestWebFetch_404(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
	}))
	defer server.Close()

	tool := newTestWebFetch()
	args, _ := json.Marshal(map[string]string{"url": server.URL})
	result, err := tool.Execute(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("Execute retornou erro: %v", err)
	}
	if !result.IsError {
		t.Error("deve ser erro para 404")
	}
	if !strings.Contains(result.Content, "404") {
		t.Error("deve mencionar status 404")
	}
}

func TestWebFetch_InvalidURL(t *testing.T) {
	credMgr := credentials.NewManager(nil)
	tool := NewWebFetch(credMgr)
	args := `{"url": "not-a-url"}`
	result, err := tool.Execute(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("Execute retornou erro: %v", err)
	}
	if !result.IsError {
		t.Error("deve ser erro para URL inválida")
	}
}

func TestWebFetch_MissingURL(t *testing.T) {
	credMgr := credentials.NewManager(nil)
	tool := NewWebFetch(credMgr)
	args := `{}`
	result, err := tool.Execute(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("Execute retornou erro: %v", err)
	}
	if !result.IsError {
		t.Error("deve ser erro sem URL")
	}
}

func TestWebFetch_BlocksLocalhost(t *testing.T) {
	credMgr := credentials.NewManager(nil)
	tool := NewWebFetch(credMgr)
	localURLs := []string{
		"http://localhost:8080/test",
		"http://127.0.0.1/test",
		"http://192.168.1.1/test",
		"http://10.0.0.1/test",
	}

	for _, u := range localURLs {
		args, _ := json.Marshal(map[string]string{"url": u})
		result, err := tool.Execute(context.Background(), json.RawMessage(args))
		if err != nil {
			t.Fatalf("Execute retornou erro para %s: %v", u, err)
		}
		if !result.IsError {
			t.Errorf("deve bloquear acesso a %s", u)
		}
	}
}

func TestIsPrivateHost(t *testing.T) {
	tests := []struct {
		host     string
		expected bool
	}{
		{"localhost", true},
		{"127.0.0.1", true},
		{"192.168.1.1", true},
		{"10.0.0.1", true},
		{"172.16.0.1", true},
		{"::1", true},
		{"google.com", false},
		{"example.org", false},
		{"8.8.8.8", false},
	}

	for _, tt := range tests {
		result := isPrivateHost(tt.host)
		if result != tt.expected {
			t.Errorf("isPrivateHost(%q) = %v, want %v", tt.host, result, tt.expected)
		}
	}
}

func TestHtmlToText(t *testing.T) {
	html := `<div><p>Hello <b>World</b></p><script>alert('x')</script><style>.x{}</style></div>`
	text := htmlToText(html)

	if !strings.Contains(text, "Hello") {
		t.Error("deve conter 'Hello'")
	}
	if !strings.Contains(text, "World") {
		t.Error("deve conter 'World'")
	}
	if strings.Contains(text, "alert") {
		t.Error("não deve conter script")
	}
	if strings.Contains(text, ".x{") {
		t.Error("não deve conter style")
	}
}

func TestCollapseWhitespace(t *testing.T) {
	input := "hello   world\n\n\n\n\ntest"
	result := collapseWhitespace(input)

	if strings.Contains(result, "\n\n\n") {
		t.Error("não deve ter 3+ newlines consecutivas")
	}
	if strings.Contains(result, "   ") {
		t.Error("não deve ter espaços triplos")
	}
}
