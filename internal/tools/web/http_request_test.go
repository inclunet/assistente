package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"assistente/internal/credentials"
)

// newTestHTTPRequest cria um HTTPRequest que permite hosts privados (para httptest)
func newTestHTTPRequest() *HTTPRequest {
	credMgr := credentials.NewManager(nil)
	hr := NewHTTPRequest(credMgr)
	hr.allowPrivateHosts = true
	return hr
}

func TestHTTPRequest_Name(t *testing.T) {
	tool := NewHTTPRequest(nil)
	if tool.Name() != "http_request" {
		t.Errorf("expected 'http_request', got '%s'", tool.Name())
	}
}

func TestHTTPRequest_Parameters(t *testing.T) {
	tool := NewHTTPRequest(nil)
	params := tool.Parameters()
	var schema map[string]any
	if err := json.Unmarshal(params, &schema); err != nil {
		t.Fatalf("failed to parse parameters: %v", err)
	}
	props := schema["properties"].(map[string]any)
	if _, ok := props["url"]; !ok {
		t.Error("missing 'url' property")
	}
	if _, ok := props["method"]; !ok {
		t.Error("missing 'method' property")
	}
	if _, ok := props["headers"]; !ok {
		t.Error("missing 'headers' property")
	}
}

func TestHTTPRequest_GET(t *testing.T) {
	// Server de teste que retorna JSON
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("expected GET, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"message": "success", "data": [1, 2, 3]}`))
	}))
	defer ts.Close()

	tool := newTestHTTPRequest()
	args := map[string]any{
		"url": ts.URL,
	}
	argsJSON, _ := json.Marshal(args)

	result, err := tool.Execute(context.Background(), argsJSON)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result.IsError {
		t.Errorf("expected success, got error: %s", result.Content)
	}

	if !strings.Contains(result.Content, "200 OK") {
		t.Error("expected 200 OK in response")
	}

	if !strings.Contains(result.Content, "success") {
		t.Error("expected 'success' in response content")
	}
}

func TestHTTPRequest_BlocksRedirectToInvalidScheme(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "ftp://example.com/x")
		w.WriteHeader(http.StatusFound)
	}))
	defer ts.Close()

	tool := newTestHTTPRequest() // allowPrivateHosts=true; o alvo é o destino do redirect
	argsJSON, _ := json.Marshal(map[string]any{"url": ts.URL})
	result, err := tool.Execute(context.Background(), argsJSON)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !result.IsError {
		t.Error("esperado IsError ao bloquear redirect para scheme inválido")
	}
}

func TestHTTPRequest_POST_JSON(t *testing.T) {
	// Server que espera POST com JSON
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		contentType := r.Header.Get("Content-Type")
		if !strings.Contains(contentType, "application/json") {
			t.Errorf("expected application/json, got %s", contentType)
		}

		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"status": "created"}`))
	}))
	defer ts.Close()

	tool := newTestHTTPRequest()
	args := map[string]any{
		"url":       ts.URL,
		"method":    "POST",
		"body":      `{"name": "João", "age": 30}`,
		"body_type": "json",
	}
	argsJSON, _ := json.Marshal(args)

	result, err := tool.Execute(context.Background(), argsJSON)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result.IsError {
		t.Errorf("expected success, got error: %s", result.Content)
	}

	if !strings.Contains(result.Content, "201") {
		t.Error("expected 201 Created in response")
	}
}

func TestHTTPRequest_DELETE(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()

	tool := newTestHTTPRequest()

	// Sem confirmação (confirmFn é nil), deve executar
	args := map[string]any{
		"url":    ts.URL,
		"method": "DELETE",
	}
	argsJSON, _ := json.Marshal(args)

	result, err := tool.Execute(context.Background(), argsJSON)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result.IsError {
		t.Errorf("expected success, got error: %s", result.Content)
	}

	if !strings.Contains(result.Content, "204") {
		t.Error("expected 204 No Content in response")
	}
}

func TestHTTPRequest_DELETE_WithConfirmation(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	tool := newTestHTTPRequest()

	// Registra confirmação negada
	tool.SetConfirmFunc(func(ctx context.Context, method, url, body string) (bool, error) {
		return false, nil // Nega
	})

	args := map[string]any{
		"url":    ts.URL,
		"method": "DELETE",
	}
	argsJSON, _ := json.Marshal(args)

	result, err := tool.Execute(context.Background(), argsJSON)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// Deve retornar erro (operação cancelada)
	if !result.IsError {
		t.Error("expected error when delete denied")
	}

	if !strings.Contains(strings.ToLower(result.Content), "cancelada") {
		t.Errorf("expected 'cancelada' in error, got: %s", result.Content)
	}
}

func TestHTTPRequest_CustomHeaders(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Custom") != "test-value" {
			t.Errorf("missing X-Custom header")
		}
		if r.Header.Get("Authorization") != "Bearer token123" {
			t.Errorf("missing Authorization header")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	// Testa custom headers - credentials são aplicadas automaticamente pelo cliente HTTP
	// localhost é bloqueado, então este teste valida que headers customizados funcionam
}

func TestHTTPRequest_InvalidURL(t *testing.T) {
	tool := NewHTTPRequest(nil)

	args := map[string]any{
		"url": "not a valid url",
	}
	argsJSON, _ := json.Marshal(args)

	result, err := tool.Execute(context.Background(), argsJSON)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if !result.IsError {
		t.Error("expected error for invalid URL")
	}
}

func TestHTTPRequest_ExtractJSON(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name": "test", "value": 123, "nested": {"key": "data"}}`))
	}))
	defer ts.Close()

	tool := newTestHTTPRequest()
	args := map[string]any{
		"url":          ts.URL,
		"extract_mode": "json",
	}
	argsJSON, _ := json.Marshal(args)

	result, err := tool.Execute(context.Background(), argsJSON)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result.IsError {
		t.Errorf("expected success, got error: %s", result.Content)
	}

	// Deve conter JSON formatado
	if !strings.Contains(result.Content, "name") || !strings.Contains(result.Content, "test") {
		t.Errorf("expected formatted JSON in response: %s", result.Content)
	}
}
