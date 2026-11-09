package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"assistente/internal/database"
)

func connectionTestCtx() context.Context {
	return database.WithUserID(context.Background(), "test-user")
}

// brokenConnectionURL aceita a conexão local e encerra sem resposta HTTP.
// Exercita a falha real de transporte sem depender de rede externa ou timeout.
func brokenConnectionURL(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Errorf("hijack da conexão de teste: %v", err)
			return
		}
		_ = conn.Close()
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

// --- validateWizardURL ---

func TestValidateWizardURL_ValidFormats(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	if err := ValidateURL(connectionTestCtx(), srv.URL); err != nil {
		t.Errorf("should accept valid reachable URL: %v", err)
	}
}

func TestValidateWizardURL_InvalidScheme(t *testing.T) {
	if err := ValidateURL(connectionTestCtx(), "ftp://example.com"); err == nil {
		t.Error("should reject ftp:// scheme")
	}
}

func TestValidateWizardURL_EmptyHost(t *testing.T) {
	if err := ValidateURL(connectionTestCtx(), "http://"); err == nil {
		t.Error("should reject URL with empty host")
	}
}

func TestValidateWizardURL_Unreachable(t *testing.T) {
	if err := ValidateURL(connectionTestCtx(), brokenConnectionURL(t)); err == nil {
		t.Error("should reject unreachable URL")
	}
}

func TestValidateWizardURL_Accepts401(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	if err := ValidateURL(connectionTestCtx(), srv.URL); err != nil {
		t.Errorf("should accept 401 (server is alive): %v", err)
	}
}

func TestValidateWizardURL_RejectsServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	if err := ValidateURL(connectionTestCtx(), srv.URL); err == nil {
		t.Error("should reject 500 server error")
	}
}

// --- validateWizardConnection ---

func modelsJSON(ids ...string) []byte {
	type model struct {
		ID string `json:"id"`
	}
	type resp struct {
		Data []model `json:"data"`
	}
	r := resp{}
	for _, id := range ids {
		r.Data = append(r.Data, model{ID: id})
	}
	b, _ := json.Marshal(r)
	return b
}

func TestValidateWizardConnection_SuccessWithModels(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(modelsJSON("gpt-4o", "gpt-4o-mini"))
	}))
	defer srv.Close()

	service := NewService(ServiceConfig{})
	result := service.ProbeConnection(connectionTestCtx(), srv.URL, "sk-test")

	if result.ErrorType != "" {
		t.Fatalf("unexpected error: %s - %s", result.ErrorType, result.ErrorDetail)
	}
	if !result.URLReachable {
		t.Error("URLReachable should be true")
	}
	if !result.AuthOK {
		t.Error("AuthOK should be true")
	}
	if !result.ModelsAvailable {
		t.Error("ModelsAvailable should be true")
	}
	if len(result.Models) != 2 {
		t.Errorf("expected 2 models, got %d", len(result.Models))
	}
}

func TestValidateWizardConnection_Unauthorized_WithKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = fmt.Fprint(w, `{"error": "invalid api key"}`)
	}))
	defer srv.Close()

	service := NewService(ServiceConfig{})
	result := service.ProbeConnection(connectionTestCtx(), srv.URL, "bad-key")

	if result.ErrorType != "auth_invalid" {
		t.Errorf("expected auth_invalid, got %s", result.ErrorType)
	}
	if !result.URLReachable {
		t.Error("URLReachable should be true (server responded)")
	}
}

func TestValidateWizardConnection_Unauthorized_NoKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	service := NewService(ServiceConfig{})
	result := service.ProbeConnection(connectionTestCtx(), srv.URL, "")

	if result.ErrorType != "auth_required" {
		t.Errorf("expected auth_required, got %s", result.ErrorType)
	}
}

func TestValidateWizardConnection_Forbidden(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	service := NewService(ServiceConfig{})
	result := service.ProbeConnection(connectionTestCtx(), srv.URL, "some-key")

	if result.ErrorType != "auth_invalid" {
		t.Errorf("expected auth_invalid, got %s", result.ErrorType)
	}
}

func TestValidateWizardConnection_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	service := NewService(ServiceConfig{})
	result := service.ProbeConnection(connectionTestCtx(), srv.URL, "")

	if result.ErrorType != "server_error" {
		t.Errorf("expected server_error, got %s", result.ErrorType)
	}
	if !result.URLReachable {
		t.Error("URLReachable should be true")
	}
}

func TestValidateWizardConnection_Unreachable(t *testing.T) {
	service := NewService(ServiceConfig{})
	result := service.ProbeConnection(connectionTestCtx(), brokenConnectionURL(t), "")

	if result.ErrorType != "url_unreachable" {
		t.Errorf("expected url_unreachable, got %s", result.ErrorType)
	}
	if result.URLReachable {
		t.Error("URLReachable should be false")
	}
}

func TestValidateWizardConnection_InvalidURL(t *testing.T) {
	service := NewService(ServiceConfig{})

	tests := []string{
		"",
		"not-a-url",
		"ftp://example.com",
		"://missing-scheme",
	}

	for _, u := range tests {
		t.Run(u, func(t *testing.T) {
			result := service.ProbeConnection(connectionTestCtx(), u, "")
			if result.ErrorType != "url_invalid" && result.ErrorType != "url_unreachable" {
				t.Errorf("expected url_invalid or url_unreachable for %q, got %s", u, result.ErrorType)
			}
		})
	}
}

func TestValidateWizardConnection_NotFound_ModelsEndpoint(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	service := NewService(ServiceConfig{})
	result := service.ProbeConnection(connectionTestCtx(), srv.URL, "sk-test")

	if result.ErrorType != "" {
		t.Errorf("unexpected error: %s", result.ErrorType)
	}
	if !result.URLReachable {
		t.Error("URLReachable should be true")
	}
	if !result.AuthOK {
		t.Error("AuthOK should be true for 404")
	}
	if result.ModelsAvailable {
		t.Error("ModelsAvailable should be false for 404")
	}
}

func TestValidateWizardConnection_EmptyModelsResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data": []}`))
	}))
	defer srv.Close()

	service := NewService(ServiceConfig{})
	result := service.ProbeConnection(connectionTestCtx(), srv.URL, "sk-test")

	if result.ErrorType != "" {
		t.Errorf("unexpected error: %s", result.ErrorType)
	}
	if !result.AuthOK {
		t.Error("AuthOK should be true")
	}
	if result.ModelsAvailable {
		t.Error("ModelsAvailable should be false with empty data")
	}
}

func TestValidateWizardConnection_AuthHeaderSent(t *testing.T) {
	var receivedAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(modelsJSON("model-1"))
	}))
	defer srv.Close()

	service := NewService(ServiceConfig{})
	service.ProbeConnection(connectionTestCtx(), srv.URL, "my-secret-key")

	if receivedAuth != "Bearer my-secret-key" {
		t.Errorf("expected Bearer header, got %q", receivedAuth)
	}
}

func TestValidateWizardConnection_NoAuthHeaderWhenEmpty(t *testing.T) {
	var receivedAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(modelsJSON("model-1"))
	}))
	defer srv.Close()

	service := NewService(ServiceConfig{})
	service.ProbeConnection(connectionTestCtx(), srv.URL, "")

	if receivedAuth != "" {
		t.Errorf("should not send auth header when key is empty, got %q", receivedAuth)
	}
}
