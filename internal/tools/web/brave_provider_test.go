package web

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"assistente/internal/credentials"
	httpclient "assistente/internal/tools/http"
)

func TestBraveToken_Bearer(t *testing.T) {
	auth := &credentials.AuthConfig{Type: "bearer", Token: "minha-chave"}
	if got := braveToken(auth); got != "minha-chave" {
		t.Errorf("esperava 'minha-chave', got %q", got)
	}
}

func TestBraveToken_BearerComPrefixo(t *testing.T) {
	auth := &credentials.AuthConfig{Type: "bearer", Token: "Bearer minha-chave"}
	if got := braveToken(auth); got != "minha-chave" {
		t.Errorf("esperava prefixo removido, got %q", got)
	}
}

func TestBraveToken_Custom(t *testing.T) {
	auth := &credentials.AuthConfig{Type: "custom", Headers: map[string]string{"X-Subscription-Token": "chave-custom"}}
	if got := braveToken(auth); got != "chave-custom" {
		t.Errorf("esperava 'chave-custom', got %q", got)
	}
}

func TestBraveToken_TiposInvalidos(t *testing.T) {
	for _, auth := range []*credentials.AuthConfig{
		nil,
		{Type: "basic", Username: "u", Password: "p"},
		{Type: "custom", Headers: map[string]string{"Outro": "x"}},
		{Type: "none"},
	} {
		if got := braveToken(auth); got != "" {
			t.Errorf("esperava vazio, got %q", got)
		}
	}
}

func TestParseBraveResponse(t *testing.T) {
	body := `{"web": {"results": [
		{"title": "Go", "url": "https://go.dev", "description": "Linguagem Go"},
		{"title": "", "url": "https://sem-titulo.dev", "description": "ignorado"},
		{"title": "Wiki", "url": "https://pt.wikipedia.org/wiki/Go", "description": "Verbete"}
	]}}`
	results, err := parseBraveResponse([]byte(body), 10)
	if err != nil {
		t.Fatalf("parse falhou: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("esperava 2 resultados válidos, got %d", len(results))
	}
	if results[0].Title != "Go" || results[0].URL != "https://go.dev" || results[0].Snippet != "Linguagem Go" {
		t.Errorf("mapeamento incorreto: %+v", results[0])
	}
}

func TestParseBraveResponse_RespeitaMaxResults(t *testing.T) {
	body := `{"web": {"results": [
		{"title": "A", "url": "https://a.dev", "description": ""},
		{"title": "B", "url": "https://b.dev", "description": ""}
	]}}`
	results, err := parseBraveResponse([]byte(body), 1)
	if err != nil {
		t.Fatalf("parse falhou: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("esperava 1 resultado, got %d", len(results))
	}
}

func TestParseBraveResponse_JSONInvalido(t *testing.T) {
	if _, err := parseBraveResponse([]byte("não é json"), 10); err == nil {
		t.Error("esperava erro para JSON inválido")
	}
}

func TestBraveFallbackable(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusTooManyRequests} {
		if !braveFallbackable(status) {
			t.Errorf("status %d deveria permitir fallback", status)
		}
	}
	for _, status := range []int{http.StatusOK, http.StatusBadRequest, http.StatusInternalServerError, http.StatusServiceUnavailable} {
		if braveFallbackable(status) {
			t.Errorf("status %d não deveria permitir fallback", status)
		}
	}
}

func TestBraveProvider_SemCredencial(t *testing.T) {
	credMgr := credentials.NewManager(nil)
	client := httpclient.New(&httpclient.Config{CredentialManager: credMgr}, map[string]string{})
	provider := &braveProvider{credMgr: credMgr}
	_, err := provider.Search(context.Background(), client, "go", 0, 5)
	if err != errNoBraveCredential {
		t.Errorf("esperava errNoBraveCredential, got %v", err)
	}
}

func TestBraveProvider_SemCredMgr(t *testing.T) {
	provider := &braveProvider{}
	client := httpclient.New(&httpclient.Config{CredentialManager: credentials.NewManager(nil)}, map[string]string{})
	_, err := provider.Search(context.Background(), client, "go", 0, 5)
	if err != errNoBraveCredential {
		t.Errorf("esperava errNoBraveCredential, got %v", err)
	}
}

// trustedTestContext libera o host do servidor fake no guard anti-SSRF.
func trustedTestContext(t *testing.T, ctx context.Context, srv *httptest.Server) context.Context {
	t.Helper()
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse da URL do servidor: %v", err)
	}
	host := u.Hostname()
	port := u.Port()
	if port == "" {
		port = "80"
	}
	ip := net.ParseIP(host)
	if ip == nil {
		addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil || len(addrs) == 0 {
			t.Fatalf("resolução do host de teste: %v", err)
		}
		ip = addrs[0].IP
	}
	return httpclient.WithTrustedIPs(ctx, []net.IP{ip}, port, true)
}

func TestBraveProvider_BuscaComCredencial(t *testing.T) {
	var gotToken, gotQuery, gotCount string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotToken = r.Header.Get("X-Subscription-Token")
		gotQuery = r.URL.Query().Get("q")
		gotCount = r.URL.Query().Get("count")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"web": {"results": [{"title": "Go", "url": "https://go.dev", "description": "Linguagem Go"}]}}`))
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	credMgr := credentials.NewManager(nil)
	if err := credMgr.RegisterPattern(u.Hostname(), &credentials.AuthConfig{Source: "static", Type: "bearer", Token: "token-teste"}); err != nil {
		t.Fatalf("registro de credencial: %v", err)
	}
	client := httpclient.New(&httpclient.Config{CredentialManager: credMgr}, map[string]string{})
	provider := &braveProvider{credMgr: credMgr, endpointOverride: srv.URL}

	ctx := trustedTestContext(t, context.Background(), srv)
	results, err := provider.Search(ctx, client, "linguagem go", 0, 5)
	if err != nil {
		t.Fatalf("Search falhou: %v", err)
	}
	if gotToken != "token-teste" {
		t.Errorf("header X-Subscription-Token ausente/incorreto: %q", gotToken)
	}
	if gotQuery != "linguagem go" || gotCount != "5" {
		t.Errorf("query params incorretos: q=%q count=%q", gotQuery, gotCount)
	}
	if len(results) != 1 || results[0].Title != "Go" || results[0].URL != "https://go.dev" {
		t.Errorf("resultado incorreto: %+v", results)
	}
}

func TestBraveProvider_StatusAuthViraFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	credMgr := credentials.NewManager(nil)
	if err := credMgr.RegisterPattern(u.Hostname(), &credentials.AuthConfig{Source: "static", Type: "bearer", Token: "token-invalido"}); err != nil {
		t.Fatalf("registro de credencial: %v", err)
	}
	tool := NewWebSearch(credMgr)
	tool.brave = &braveProvider{credMgr: credMgr, endpointOverride: srv.URL}
	tool.fallback = &mockSearchProvider{results: []SearchResult{
		{Title: "Fallback", URL: "https://exemplo.dev", Snippet: "via fallback"},
	}}

	ctx := trustedTestContext(t, context.Background(), srv)
	results, providerName, err := tool.searchWithFallback(ctx, "go", 0, 5)
	if err != nil {
		t.Fatalf("fallback falhou: %v", err)
	}
	if providerName != "MockSearch" {
		t.Errorf("esperava provider do fallback, got %q", providerName)
	}
	if len(results) != 1 || results[0].Title != "Fallback" {
		t.Errorf("resultado do fallback não chegou: %+v", results)
	}
}

func TestWebSearch_FallbackSemCredencialUsaDuckDuckGo(t *testing.T) {
	// Sem credencial Brave, a cadeia padrão deve cair no fallback (mockado).
	credMgr := credentials.NewManager(nil)
	tool := NewWebSearch(credMgr)
	tool.fallback = &mockSearchProvider{results: []SearchResult{
		{Title: "Fallback", URL: "https://exemplo.dev", Snippet: "via fallback"},
	}}

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"query":"go"}`))
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
	if out.Provider != "MockSearch" {
		t.Errorf("esperava provider do fallback, got %q", out.Provider)
	}
	if out.Count != 1 || len(out.Results) == 0 || out.Results[0].Title != "Fallback" {
		t.Errorf("resultado do fallback não chegou: %+v", out)
	}
	if !strings.Contains(result.Content, "Fallback") {
		t.Errorf("conteúdo não traz o resultado: %s", result.Content)
	}
}
