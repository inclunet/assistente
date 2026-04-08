package llm

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"assistente/internal/credentials"
)

// TestGetModelsNoAPIKey reproduz o cenário: LocalAI sem API key, HTTP OK com lista de modelos.
func TestGetModelsNoAPIKey(t *testing.T) {
	// Mock server que retorna lista de modelos no formato OpenAI
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Logf("Request: %s %s", r.Method, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"object":"list","data":[{"id":"model-a","object":"model"},{"id":"model-b","object":"model"}]}`)
	}))
	defer srv.Close()

	credMgr := credentials.NewManager(nil)

	provider := &ProviderConfig{
		ID:                "temp-form",
		Name:              "temp",
		Type:              "localai",
		BaseURL:           srv.URL + "/v1",
		CredentialPattern: "localhost",
		Timeout:           15,
	}

	cp := NewChatProvider(provider, credMgr)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	models, err := cp.GetModels(ctx)
	if err != nil {
		t.Fatalf("GetModels falhou: %v", err)
	}
	if len(models) != 2 {
		t.Fatalf("esperava 2 modelos, obteve %d: %v", len(models), models)
	}
	t.Logf("Modelos: %v", models)
}

// TestGetModelsNilCredManager testa com credMgr nil.
func TestGetModelsNilCredManager(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"object":"list","data":[{"id":"test-model","object":"model"}]}`)
	}))
	defer srv.Close()

	provider := &ProviderConfig{
		ID:                "temp-form",
		Name:              "temp",
		Type:              "localai",
		BaseURL:           srv.URL + "/v1",
		CredentialPattern: "localhost",
		Timeout:           15,
	}

	// credMgr nil - deve funcionar sem panic
	cp := NewChatProvider(provider, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	models, err := cp.GetModels(ctx)
	if err != nil {
		t.Fatalf("GetModels com credMgr nil falhou: %v", err)
	}
	if len(models) == 0 {
		t.Fatal("esperava pelo menos 1 modelo")
	}
	t.Logf("Modelos: %v", models)
}

// TestGetModelsServerUnavailable testa com servidor indisponível.
func TestGetModelsServerUnavailable(t *testing.T) {
	credMgr := credentials.NewManager(nil)

	provider := &ProviderConfig{
		ID:                "temp-form",
		Name:              "temp",
		Type:              "localai",
		BaseURL:           "http://127.0.0.1:1/v1",
		CredentialPattern: "localhost",
		Timeout:           2,
	}

	cp := NewChatProvider(provider, credMgr)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := cp.GetModels(ctx)
	if err == nil {
		t.Fatal("esperava erro com servidor indisponível")
	}
	t.Logf("Erro esperado: %v", err)
}

// TestGetModelsNullBody testa quando servidor retorna body "null".
func TestGetModelsNullBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `null`)
	}))
	defer srv.Close()

	credMgr := credentials.NewManager(nil)
	provider := &ProviderConfig{
		ID: "temp", Name: "temp", Type: "localai",
		BaseURL: srv.URL + "/v1", CredentialPattern: "localhost", Timeout: 5,
	}

	cp := NewChatProvider(provider, credMgr)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("PANIC com body null: %v", r)
		}
	}()

	models, err := cp.GetModels(ctx)
	t.Logf("models=%v err=%v", models, err)
}

// TestGetModelsEmptyObject testa quando servidor retorna body "{}".
func TestGetModelsEmptyObject(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{}`)
	}))
	defer srv.Close()

	credMgr := credentials.NewManager(nil)
	provider := &ProviderConfig{
		ID: "temp", Name: "temp", Type: "localai",
		BaseURL: srv.URL + "/v1", CredentialPattern: "localhost", Timeout: 5,
	}

	cp := NewChatProvider(provider, credMgr)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("PANIC com body {}: %v", r)
		}
	}()

	models, err := cp.GetModels(ctx)
	t.Logf("models=%v err=%v", models, err)
}

// TestGetModelsPlainText testa quando servidor retorna text/plain.
func TestGetModelsPlainText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprint(w, "OK")
	}))
	defer srv.Close()

	credMgr := credentials.NewManager(nil)
	provider := &ProviderConfig{
		ID: "temp", Name: "temp", Type: "localai",
		BaseURL: srv.URL + "/v1", CredentialPattern: "localhost", Timeout: 5,
	}

	cp := NewChatProvider(provider, credMgr)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("PANIC com text/plain: %v", r)
		}
	}()

	models, err := cp.GetModels(ctx)
	t.Logf("models=%v err=%v", models, err)
}

// TestGetModelsHTML testa quando servidor retorna HTML (como página de erro).
func TestGetModelsHTML(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, "<html><body>Error</body></html>")
	}))
	defer srv.Close()

	credMgr := credentials.NewManager(nil)
	provider := &ProviderConfig{
		ID: "temp", Name: "temp", Type: "localai",
		BaseURL: srv.URL + "/v1", CredentialPattern: "localhost", Timeout: 5,
	}

	cp := NewChatProvider(provider, credMgr)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("PANIC com HTML: %v", r)
		}
	}()

	models, err := cp.GetModels(ctx)
	t.Logf("models=%v err=%v", models, err)
}

// TestGetModelsNoContentType testa quando servidor não retorna Content-Type.
func TestGetModelsNoContentType(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Sem Content-Type
		fmt.Fprint(w, `{"object":"list","data":[{"id":"m1"}]}`)
	}))
	defer srv.Close()

	credMgr := credentials.NewManager(nil)
	provider := &ProviderConfig{
		ID: "temp", Name: "temp", Type: "localai",
		BaseURL: srv.URL + "/v1", CredentialPattern: "localhost", Timeout: 5,
	}

	cp := NewChatProvider(provider, credMgr)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("PANIC sem content-type: %v", r)
		}
	}()

	models, err := cp.GetModels(ctx)
	t.Logf("models=%v err=%v", models, err)
}
