package providers

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"assistente/internal/credentials"
	"assistente/internal/llm"
)

// TestBuildTempProviderForListModels_HerdaAPIFormatDoExistente é a regressão
// arquitetural do incidente: ao testar a chave de um provider já persistido
// (ex.: LiteLLM com api_format="openai_responses"), o tempProvider DEVE
// herdar o api_format. Sem isso, o teste cai no client default (Chat
// Completions) e o usuário valida uma rota que não é a usada em produção,
// abrindo espaço para erros 400/404 inexplicados quando começa a chatear.
func TestBuildTempProviderForListModels_HerdaAPIFormatDoExistente(t *testing.T) {
	req := ListModelsRawRequest{
		Type:       "openai",
		BaseURL:    "https://ist-prod-litellm.example.com/v1",
		ProviderID: "litellm-prod",
	}
	existing := &llm.ProviderConfig{
		ID:        "litellm-prod",
		Name:      "LiteLLM Prod",
		Type:      llm.ProviderOpenAI,
		APIFormat: llm.APIFormatOpenAIResponses,
		BaseURL:   "https://ist-prod-litellm.example.com/v1",
	}

	temp := buildTempProviderForListModels(req, "ist-prod-litellm.example.com", existing)
	if temp.APIFormat != llm.APIFormatOpenAIResponses {
		t.Fatalf("tempProvider deveria herdar APIFormat=%q, got %q", llm.APIFormatOpenAIResponses, temp.APIFormat)
	}
	if temp.GetAPIFormat() != llm.APIFormatOpenAIResponses {
		t.Fatalf("GetAPIFormat() deveria devolver %q, got %q", llm.APIFormatOpenAIResponses, temp.GetAPIFormat())
	}
	if temp.CredentialPattern != "ist-prod-litellm.example.com" {
		t.Fatalf("CredentialPattern errado: %q", temp.CredentialPattern)
	}
}

// TestBuildTempProviderForListModels_SemExistenteCaiNoDefault garante que
// providers ad-hoc (ainda não persistidos, ex.: novo cadastro) não ganham
// APIFormat fantasma — a inferência por base_url em GetAPIFormat continua
// sendo o único caminho de auto-detecção.
func TestBuildTempProviderForListModels_SemExistenteCaiNoDefault(t *testing.T) {
	req := ListModelsRawRequest{
		Type:    "openai",
		BaseURL: "https://api.openai.com/v1",
	}
	temp := buildTempProviderForListModels(req, "api.openai.com", nil)
	if temp.APIFormat != "" {
		t.Fatalf("tempProvider sem provider existente deve nascer com APIFormat vazio, got %q", temp.APIFormat)
	}
	// Inferência por URL deve continuar funcionando para api.openai.com.
	if temp.GetAPIFormat() != llm.APIFormatOpenAIResponses {
		t.Fatalf("api.openai.com deveria inferir openai_responses, got %q", temp.GetAPIFormat())
	}
}

// TestListModelsRaw_TrimAPIKey valida o trim defensivo no backend: copy/paste
// de chaves frequentemente arrasta `\n`/espaços invisíveis. Sem o trim, o
// header Authorization vai com whitespace e o upstream responde 400.
func TestListModelsRaw_TrimAPIKey(t *testing.T) {
	var capturedAuth atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth.Store(r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"object":"list","data":[]}`)
	}))
	defer srv.Close()

	registry := llm.NewProviderRegistry()
	credMgr := credentials.NewManager(nil)

	svc := NewService(ServiceConfig{
		Registry: registry,
		CredMgr:  credMgr,
	})

	parsed, _ := url.Parse(srv.URL + "/v1")
	dirtyKey := "  sk-test-key-123\n\t"
	_, err := svc.ListModelsRaw(context.Background(), ListModelsRawRequest{
		Type:    "openai",
		BaseURL: parsed.String(),
		APIKey:  dirtyKey,
	})
	if err != nil {
		t.Fatalf("ListModelsRaw falhou: %v", err)
	}

	got, _ := capturedAuth.Load().(string)
	want := "Bearer sk-test-key-123"
	if got != want {
		t.Fatalf("Authorization recebido pelo upstream foi %q, esperava %q (trim deveria ter limpado whitespace)", got, want)
	}
	if strings.ContainsAny(got, "\n\t") || strings.HasPrefix(strings.TrimPrefix(got, "Bearer "), " ") {
		t.Fatalf("Authorization com whitespace residual: %q", got)
	}
}

// TestListModelsRaw_PreservaBodyDoUpstreamEm400 é a regressão direta do
// sintoma reportado pelo usuário: o upstream LiteLLM respondia 400 ao
// /models, mas a UI mostrava apenas "provedor retornou status 400" sem
// motivo. O fix preserva o body (≤512 bytes) na mensagem de erro.
func TestListModelsRaw_PreservaBodyDoUpstreamEm400(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = fmt.Fprint(w, `{"error":{"message":"team_id missing on virtual key","type":"litellm_error"}}`)
	}))
	defer srv.Close()

	registry := llm.NewProviderRegistry()
	credMgr := credentials.NewManager(nil)
	svc := NewService(ServiceConfig{Registry: registry, CredMgr: credMgr})

	_, err := svc.ListModelsRaw(context.Background(), ListModelsRawRequest{
		Type:    "openai",
		BaseURL: srv.URL + "/v1",
		APIKey:  "sk-fake",
	})
	if err == nil {
		t.Fatal("esperava erro com upstream 400")
	}
	msg := err.Error()
	if !strings.Contains(msg, "400") {
		t.Errorf("mensagem deveria conter status 400: %q", msg)
	}
	if !strings.Contains(msg, "team_id missing on virtual key") {
		t.Errorf("mensagem deveria conter o body do upstream para diagnóstico: %q", msg)
	}
}
