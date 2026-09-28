package web

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"assistente/internal/credentials"
	httpclient "assistente/internal/tools/http"
)

func TestBingKey_Bearer(t *testing.T) {
	auth := &credentials.AuthConfig{Type: "bearer", Token: "bing-chave"}
	if got := bingKey(auth); got != "bing-chave" {
		t.Errorf("esperava 'bing-chave', got %q", got)
	}
}

func TestBingKey_CustomSubscriptionKey(t *testing.T) {
	auth := &credentials.AuthConfig{Type: "custom", Headers: map[string]string{"ocp-apim-subscription-key": "bing-custom"}}
	if got := bingKey(auth); got != "bing-custom" {
		t.Errorf("esperava 'bing-custom', got %q", got)
	}
}

func TestBingKey_TiposInvalidos(t *testing.T) {
	for _, auth := range []*credentials.AuthConfig{
		nil,
		{Type: "basic", Username: "u", Password: "p"},
		{Type: "custom", Headers: map[string]string{"Authorization": "Bearer x"}},
		{Type: "none"},
	} {
		if got := bingKey(auth); got != "" {
			t.Errorf("esperava vazio, got %q", got)
		}
	}
}

func TestParseBingResponse(t *testing.T) {
	body := `{"_type": "SearchResponse", "webPages": {"webSearchUrl": "https://www.bing.com/search?q=go",
		"value": [
			{"name": "Go", "url": "https://go.dev", "snippet": "Linguagem Go"},
			{"name": "", "url": "https://sem-nome.dev", "snippet": "ignorado"},
			{"name": "Wiki", "url": "https://pt.wikipedia.org/wiki/Go", "snippet": "Verbete"}
		], "totalEstimatedMatches": 1200}}`
	results, err := parseBingResponse([]byte(body), 10)
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

func TestParseBingResponse_RespeitaMaxResults(t *testing.T) {
	body := `{"webPages": {"value": [
		{"name": "A", "url": "https://a.dev", "snippet": ""},
		{"name": "B", "url": "https://b.dev", "snippet": ""}
	]}}`
	results, err := parseBingResponse([]byte(body), 1)
	if err != nil {
		t.Fatalf("parse falhou: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("esperava 1 resultado, got %d", len(results))
	}
}

func TestParseBingResponse_SemWebPagesEValido(t *testing.T) {
	// Semântica documentada da API: bloco ausente = zero matches.
	for _, body := range []string{`{}`, `{"_type": "SearchResponse"}`} {
		results, err := parseBingResponse([]byte(body), 10)
		if err != nil {
			t.Errorf("bloco ausente deveria ser lista vazia válida em %s: %v", body, err)
		}
		if len(results) != 0 {
			t.Errorf("esperava 0 resultados em %s, got %d", body, len(results))
		}
	}
}

func TestParseBingResponse_DescartaBrancos(t *testing.T) {
	body := `{"webPages": {"value": [
		{"name": "   ", "url": "https://a.dev", "snippet": ""},
		{"name": "B", "url": "   ", "snippet": ""},
		{"name": " OK ", "url": " https://ok.dev ", "snippet": ""}
	]}}`
	results, err := parseBingResponse([]byte(body), 10)
	if err != nil {
		t.Fatalf("parse falhou: %v", err)
	}
	if len(results) != 1 || results[0].Title != "OK" || results[0].URL != "https://ok.dev" {
		t.Errorf("deveria restar só o resultado válido aparado: %+v", results)
	}
}

func TestParseBingResponse_JSONInvalido(t *testing.T) {
	if _, err := parseBingResponse([]byte("não é json"), 10); err == nil {
		t.Error("esperava erro para JSON inválido")
	}
}

func TestBingFallbackable(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusTooManyRequests} {
		if !bingFallbackable(status) {
			t.Errorf("status %d deveria avançar na cadeia", status)
		}
	}
	for _, status := range []int{http.StatusOK, http.StatusBadRequest, http.StatusInternalServerError, http.StatusServiceUnavailable} {
		if bingFallbackable(status) {
			t.Errorf("status %d não deveria avançar na cadeia", status)
		}
	}
}

func TestBingProvider_SemCredencial(t *testing.T) {
	credMgr := credentials.NewManager(nil)
	client := httpclient.New(&httpclient.Config{CredentialManager: credMgr}, map[string]string{})
	provider := &bingProvider{credMgr: credMgr}
	_, err := provider.Search(context.Background(), client, "go", 0, 5)
	if err != errNoBingCredential {
		t.Errorf("esperava errNoBingCredential, got %v", err)
	}
}

func TestBingProvider_SemCredMgr(t *testing.T) {
	provider := &bingProvider{}
	client := httpclient.New(&httpclient.Config{CredentialManager: credentials.NewManager(nil)}, map[string]string{})
	_, err := provider.Search(context.Background(), client, "go", 0, 5)
	if err != errNoBingCredential {
		t.Errorf("esperava errNoBingCredential, got %v", err)
	}
}

func TestBingProvider_ErroResolucaoPropaga(t *testing.T) {
	credMgr := credentials.NewManager(nil)
	if err := credMgr.RegisterPattern("127.0.0.1", &credentials.AuthConfig{
		Source:       "command",
		Type:         "bearer",
		SourceConfig: &credentials.SourceConfig{Command: "comando-bing-inexistente-xyz"},
	}); err != nil {
		t.Fatalf("registro de credencial: %v", err)
	}
	provider := &bingProvider{credMgr: credMgr, endpointOverride: "http://127.0.0.1/"}
	client := httpclient.New(&httpclient.Config{CredentialManager: credMgr}, map[string]string{})
	_, err := provider.Search(context.Background(), client, "go", 0, 5)
	if err == nil {
		t.Fatal("esperava erro na resolução")
	}
	if isSearchFallbackable(err) {
		t.Errorf("erro operacional não pode avançar na cadeia: %v", err)
	}
}

// trustedBingTestContext libera o host do servidor fake no guard anti-SSRF.
func trustedBingTestContext(t *testing.T, ctx context.Context, srv *httptest.Server) context.Context {
	t.Helper()
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse da URL do servidor: %v", err)
	}
	port := u.Port()
	if port == "" {
		port = "80"
	}
	ip := net.ParseIP(u.Hostname())
	if ip == nil {
		t.Fatalf("host do httptest não é IP: %q", u.Hostname())
	}
	return httpclient.WithTrustedIPs(ctx, []net.IP{ip}, port, true)
}

func TestBingProvider_BuscaComCredencial(t *testing.T) {
	var gotKey, gotAuth, gotQuery, gotCount, gotOffset string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("Ocp-Apim-Subscription-Key")
		gotAuth = r.Header.Get("Authorization")
		gotQuery = r.URL.Query().Get("q")
		gotCount = r.URL.Query().Get("count")
		gotOffset = r.URL.Query().Get("offset")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"webPages": {"value": [{"name": "Go", "url": "https://go.dev", "snippet": "Linguagem Go"}]}}`))
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	credMgr := credentials.NewManager(nil)
	if err := credMgr.RegisterPattern(u.Hostname(), &credentials.AuthConfig{Source: "static", Type: "bearer", Token: "bing-teste"}); err != nil {
		t.Fatalf("registro de credencial: %v", err)
	}
	client := httpclient.New(&httpclient.Config{CredentialManager: credMgr}, map[string]string{})
	provider := &bingProvider{credMgr: credMgr, endpointOverride: srv.URL}

	ctx := trustedBingTestContext(t, context.Background(), srv)
	results, err := provider.Search(ctx, client, "linguagem go", 8, 5)
	if err != nil {
		t.Fatalf("Search falhou: %v", err)
	}
	if gotKey != "bing-teste" {
		t.Errorf("header Ocp-Apim-Subscription-Key ausente/incorreto: %q", gotKey)
	}
	if gotAuth != "" {
		t.Errorf("Authorization redundante não pode ser enviado ao Bing: %q", gotAuth)
	}
	if gotQuery != "linguagem go" || gotCount != "5" || gotOffset != "8" {
		t.Errorf("query params incorretos: q=%q count=%q offset=%q", gotQuery, gotCount, gotOffset)
	}
	if len(results) != 1 || results[0].Title != "Go" || results[0].URL != "https://go.dev" {
		t.Errorf("resultado incorreto: %+v", results)
	}
}

func TestBingProvider_CustomComAuthorizationNaoVaza(t *testing.T) {
	// Credencial custom com Authorization junto: só a subscription key pode
	// viajar (redundância = 401 no Bing).
	var gotKey, gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("Ocp-Apim-Subscription-Key")
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"webPages": {"value": [{"name": "Go", "url": "https://go.dev", "snippet": ""}]}}`))
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	credMgr := credentials.NewManager(nil)
	if err := credMgr.RegisterPattern(u.Hostname(), &credentials.AuthConfig{Source: "static",
		Type:    "custom",
		Headers: map[string]string{"Ocp-Apim-Subscription-Key": "bing-custom", "Authorization": "Bearer intruso"},
	}); err != nil {
		t.Fatalf("registro de credencial: %v", err)
	}
	client := httpclient.New(&httpclient.Config{CredentialManager: credMgr}, map[string]string{})
	provider := &bingProvider{credMgr: credMgr, endpointOverride: srv.URL}

	ctx := trustedBingTestContext(t, context.Background(), srv)
	results, err := provider.Search(ctx, client, "go", 0, 5)
	if err != nil {
		t.Fatalf("Search falhou: %v", err)
	}
	if gotKey != "bing-custom" {
		t.Errorf("subscription key incorreta: %q", gotKey)
	}
	if gotAuth != "" {
		t.Errorf("Authorization não pode viajar junto (redundância=401): %q", gotAuth)
	}
	if len(results) != 1 {
		t.Errorf("resultado não chegou: %+v", results)
	}
}

func TestWebSearch_CadeiaTavily429CaiNoBing(t *testing.T) {
	// Tavily 429 com Bing saudável: o Bing atende, sem alcançar o DDG.
	tavilySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer tavilySrv.Close()
	var bingChamadas int
	bingSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bingChamadas++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"webPages": {"value": [{"name": "Go", "url": "https://go.dev", "snippet": "Bing"}]}}`))
	}))
	defer bingSrv.Close()

	u, _ := url.Parse(tavilySrv.URL)
	credMgr := credentials.NewManager(nil)
	if err := credMgr.RegisterPattern(u.Hostname(), &credentials.AuthConfig{Source: "static", Type: "bearer", Token: "x"}); err != nil {
		t.Fatalf("registro de credencial: %v", err)
	}
	ub, _ := url.Parse(bingSrv.URL)
	_ = ub
	// Mesmo host (127.0.0.1) cobre os dois fakes; portas liberadas via trust.
	tool := NewWebSearch(credMgr)
	tool.tavily = &tavilyProvider{credMgr: credMgr, endpointOverride: tavilySrv.URL}
	tool.tavily = &tavilyProvider{credMgr: credMgr, endpointOverride: tavilySrv.URL}
	tool.bing = &bingProvider{credMgr: credMgr, endpointOverride: bingSrv.URL}
	tool.fallback = &mockSearchProvider{results: []SearchResult{
		{Title: "Nunca", URL: "https://nunca.dev", Snippet: "não deve ser alcançado"},
	}}

	ctx := trustedBingTestContext(t, context.Background(), tavilySrv)
	ctx = trustedBingTestContext(t, ctx, bingSrv)
	// Brave sem credencial => pula; Tavily 429 => avança; Bing atende.
	results, providerName, err := tool.searchWithFallback(ctx, "go", 0, 5)
	if err != nil {
		t.Fatalf("cadeia falhou: %v", err)
	}
	if bingChamadas != 1 {
		t.Errorf("Bing deveria ter sido chamado 1 vez, foi %d", bingChamadas)
	}
	if providerName != "Bing" {
		t.Errorf("esperava provider Bing, got %q", providerName)
	}
	if len(results) != 1 || results[0].Title != "Go" {
		t.Errorf("resultado do Bing não chegou: %+v", results)
	}
}

func TestWebSearch_CadeiaBing401CaiNoFallback(t *testing.T) {
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
	tool.fallback = &mockSearchProvider{results: []SearchResult{
		{Title: "Fallback", URL: "https://exemplo.dev", Snippet: "via 401"},
	}}

	ctx := trustedBingTestContext(t, context.Background(), srv)
	// Brave e Tavily sem credencial => pulam; Bing 401 => avança; mock responde.
	results, providerName, err := tool.searchWithFallback(ctx, "go", 0, 5)
	if err != nil {
		t.Fatalf("cadeia falhou: %v", err)
	}
	if providerName != "MockSearch" {
		t.Errorf("esperava provider do fallback, got %q", providerName)
	}
	if len(results) != 1 || results[0].Title != "Fallback" {
		t.Errorf("resultado do fallback não chegou: %+v", results)
	}
}
