package web

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"assistente/internal/credentials"
	httpclient "assistente/internal/tools/http"
)

func TestTavilyToken_Bearer(t *testing.T) {
	auth := &credentials.AuthConfig{Type: "bearer", Token: "tvly-chave"}
	if got := tavilyToken(auth); got != "tvly-chave" {
		t.Errorf("esperava 'tvly-chave', got %q", got)
	}
}

func TestTavilyToken_BearerComPrefixo(t *testing.T) {
	for _, raw := range []string{"Bearer tvly-chave", "bearer tvly-chave", "BEARER tvly-chave"} {
		auth := &credentials.AuthConfig{Type: "bearer", Token: raw}
		if got := tavilyToken(auth); got != "tvly-chave" {
			t.Errorf("prefixo %q: esperava remoção, got %q", raw, got)
		}
	}
}

func TestTavilyToken_CustomAuthorization(t *testing.T) {
	auth := &credentials.AuthConfig{Type: "custom", Headers: map[string]string{"authorization": "Bearer tvly-custom"}}
	if got := tavilyToken(auth); got != "tvly-custom" {
		t.Errorf("esperava 'tvly-custom', got %q", got)
	}
}

func TestTavilyToken_TiposInvalidos(t *testing.T) {
	for _, auth := range []*credentials.AuthConfig{
		nil,
		{Type: "basic", Username: "u", Password: "p"},
		{Type: "custom", Headers: map[string]string{"X-Subscription-Token": "x"}},
		{Type: "none"},
	} {
		if got := tavilyToken(auth); got != "" {
			t.Errorf("esperava vazio, got %q", got)
		}
	}
}

func TestParseTavilyResponse(t *testing.T) {
	body := `{"query": "go", "results": [
		{"title": "Go", "url": "https://go.dev", "content": "Linguagem Go", "score": 0.9},
		{"title": "", "url": "https://sem-titulo.dev", "content": "ignorado"},
		{"title": "Wiki", "url": "https://pt.wikipedia.org/wiki/Go", "content": "Verbete"}
	], "answer": "Go é uma linguagem"}`
	results, err := parseTavilyResponse([]byte(body), 0, 10)
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

func TestParseTavilyResponse_OffsetLocal(t *testing.T) {
	body := `{"results": [
		{"title": "A", "url": "https://a.dev", "content": ""},
		{"title": "B", "url": "https://b.dev", "content": ""},
		{"title": "C", "url": "https://c.dev", "content": ""}
	]}`
	results, err := parseTavilyResponse([]byte(body), 1, 1)
	if err != nil {
		t.Fatalf("parse falhou: %v", err)
	}
	if len(results) != 1 || results[0].Title != "B" {
		t.Errorf("esperava só o segundo resultado, got %+v", results)
	}
}

func TestParseTavilyResponse_BlocoAusente(t *testing.T) {
	for _, body := range []string{`{}`, `{"query": "go"}`, `{"results": null}`} {
		if _, err := parseTavilyResponse([]byte(body), 0, 10); err == nil {
			t.Errorf("esperava erro para results ausente em %s", body)
		}
	}
}

func TestParseTavilyResponse_ListaVaziaEValida(t *testing.T) {
	results, err := parseTavilyResponse([]byte(`{"results": []}`), 0, 10)
	if err != nil {
		t.Fatalf("lista vazia deveria ser válida: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("esperava 0 resultados, got %d", len(results))
	}
}

func TestParseTavilyResponse_DescartaBrancos(t *testing.T) {
	body := `{"results": [
		{"title": "   ", "url": "https://a.dev", "content": ""},
		{"title": "B", "url": "   ", "content": ""},
		{"title": " OK ", "url": " https://ok.dev ", "content": ""}
	]}`
	results, err := parseTavilyResponse([]byte(body), 0, 10)
	if err != nil {
		t.Fatalf("parse falhou: %v", err)
	}
	if len(results) != 1 || results[0].Title != "OK" || results[0].URL != "https://ok.dev" {
		t.Errorf("deveria restar só o resultado válido aparado: %+v", results)
	}
}

func TestParseTavilyResponse_JSONInvalido(t *testing.T) {
	if _, err := parseTavilyResponse([]byte("não é json"), 0, 10); err == nil {
		t.Error("esperava erro para JSON inválido")
	}
}

func TestTavilyFallbackable(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusTooManyRequests, 432, 433} {
		if !tavilyFallbackable(status) {
			t.Errorf("status %d deveria avançar na cadeia", status)
		}
	}
	for _, status := range []int{http.StatusOK, http.StatusBadRequest, http.StatusUnprocessableEntity, http.StatusInternalServerError} {
		if tavilyFallbackable(status) {
			t.Errorf("status %d não deveria avançar na cadeia", status)
		}
	}
}

func TestTavilyProvider_SemCredencial(t *testing.T) {
	credMgr := credentials.NewManager(nil)
	client := httpclient.New(&httpclient.Config{CredentialManager: credMgr}, map[string]string{})
	provider := &tavilyProvider{credMgr: credMgr}
	_, err := provider.Search(context.Background(), client, "go", 0, 5)
	if err != errNoTavilyCredential {
		t.Errorf("esperava errNoTavilyCredential, got %v", err)
	}
}

func TestTavilyProvider_SemCredMgr(t *testing.T) {
	provider := &tavilyProvider{}
	client := httpclient.New(&httpclient.Config{CredentialManager: credentials.NewManager(nil)}, map[string]string{})
	_, err := provider.Search(context.Background(), client, "go", 0, 5)
	if err != errNoTavilyCredential {
		t.Errorf("esperava errNoTavilyCredential, got %v", err)
	}
}

func TestTavilyProvider_ErroResolucaoPropaga(t *testing.T) {
	credMgr := credentials.NewManager(nil)
	if err := credMgr.RegisterPattern("127.0.0.1", &credentials.AuthConfig{
		Source:       "command",
		Type:         "bearer",
		SourceConfig: &credentials.SourceConfig{Command: "comando-tavily-inexistente-xyz"},
	}); err != nil {
		t.Fatalf("registro de credencial: %v", err)
	}
	provider := &tavilyProvider{credMgr: credMgr, endpointOverride: "http://127.0.0.1/"}
	client := httpclient.New(&httpclient.Config{CredentialManager: credMgr}, map[string]string{})
	_, err := provider.Search(context.Background(), client, "go", 0, 5)
	if err == nil {
		t.Fatal("esperava erro na resolução")
	}
	if isSearchFallbackable(err) {
		t.Errorf("erro operacional não pode avançar na cadeia: %v", err)
	}
}

func TestTavilyProvider_OffsetAlemDaJanela(t *testing.T) {
	credMgr := credentials.NewManager(nil)
	if err := credMgr.RegisterPattern("127.0.0.1", &credentials.AuthConfig{Source: "static", Type: "bearer", Token: "x"}); err != nil {
		t.Fatalf("registro de credencial: %v", err)
	}
	var chamadas int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		chamadas++
		_, _ = w.Write([]byte(`{"results": []}`))
	}))
	defer srv.Close()
	// Registra credencial para o host fake.
	u, _ := url.Parse(srv.URL)
	if err := credMgr.RegisterPattern(u.Hostname(), &credentials.AuthConfig{Source: "static", Type: "bearer", Token: "x"}); err != nil {
		t.Fatalf("registro de credencial: %v", err)
	}
	client := httpclient.New(&httpclient.Config{CredentialManager: credMgr}, map[string]string{})
	provider := &tavilyProvider{credMgr: credMgr, endpointOverride: srv.URL}
	_, err := provider.Search(context.Background(), client, "go", tavilyMaxWindow, 5)
	if err != errTavilyWindowExceeded {
		t.Errorf("esperava errTavilyWindowExceeded, got %v", err)
	}
	if chamadas != 0 {
		t.Errorf("offset além da janela não pode gastar requisição (chamadas=%d)", chamadas)
	}
	if !isSearchFallbackable(errTavilyWindowExceeded) {
		t.Error("errTavilyWindowExceeded deve avançar na cadeia")
	}
}

func TestTavilyProvider_JanelaAntesDaResolucao(t *testing.T) {
	// Sem credencial alguma e offset além da janela: o short-circuit deve
	// vencer a resolução (não há por que tocar no credmanager).
	credMgr := credentials.NewManager(nil)
	client := httpclient.New(&httpclient.Config{CredentialManager: credMgr}, map[string]string{})
	provider := &tavilyProvider{credMgr: credMgr}
	_, err := provider.Search(context.Background(), client, "go", tavilyMaxWindow, 5)
	if err != errTavilyWindowExceeded {
		t.Errorf("esperava errTavilyWindowExceeded antes da resolução, got %v", err)
	}
}

// trustedTavilyTestContext libera o host do servidor fake no guard anti-SSRF.
func trustedTavilyTestContext(t *testing.T, ctx context.Context, srv *httptest.Server) context.Context {
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

func TestTavilyProvider_BuscaComCredencial(t *testing.T) {
	var gotAuth, gotMethod string
	var gotBody tavilySearchRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotMethod = r.Method
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results": [{"title": "Go", "url": "https://go.dev", "content": "Linguagem Go"}]}`))
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	credMgr := credentials.NewManager(nil)
	if err := credMgr.RegisterPattern(u.Hostname(), &credentials.AuthConfig{Source: "static", Type: "bearer", Token: "tvly-teste"}); err != nil {
		t.Fatalf("registro de credencial: %v", err)
	}
	client := httpclient.New(&httpclient.Config{CredentialManager: credMgr}, map[string]string{})
	provider := &tavilyProvider{credMgr: credMgr, endpointOverride: srv.URL}

	ctx := trustedTavilyTestContext(t, context.Background(), srv)
	results, err := provider.Search(ctx, client, "linguagem go", 0, 5)
	if err != nil {
		t.Fatalf("Search falhou: %v", err)
	}
	if gotMethod != "POST" {
		t.Errorf("esperava POST, got %q", gotMethod)
	}
	if gotAuth != "Bearer tvly-teste" {
		t.Errorf("header Authorization ausente/incorreto: %q", gotAuth)
	}
	if gotBody.Query != "linguagem go" || gotBody.MaxResults != 5 || gotBody.SearchDepth != "basic" {
		t.Errorf("corpo POST incorreto: %+v", gotBody)
	}
	if len(results) != 1 || results[0].Title != "Go" || results[0].Snippet != "Linguagem Go" {
		t.Errorf("resultado incorreto: %+v", results)
	}
}

func TestTavilyProvider_OffsetViraMaxResultsMaior(t *testing.T) {
	var gotBody tavilySearchRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results": [
			{"title": "A", "url": "https://a.dev", "content": ""},
			{"title": "B", "url": "https://b.dev", "content": ""},
			{"title": "C", "url": "https://c.dev", "content": ""}
		]}`))
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	credMgr := credentials.NewManager(nil)
	if err := credMgr.RegisterPattern(u.Hostname(), &credentials.AuthConfig{Source: "static", Type: "bearer", Token: "x"}); err != nil {
		t.Fatalf("registro de credencial: %v", err)
	}
	client := httpclient.New(&httpclient.Config{CredentialManager: credMgr}, map[string]string{})
	provider := &tavilyProvider{credMgr: credMgr, endpointOverride: srv.URL}

	ctx := trustedTavilyTestContext(t, context.Background(), srv)
	results, err := provider.Search(ctx, client, "go", 2, 5)
	if err != nil {
		t.Fatalf("Search falhou: %v", err)
	}
	if gotBody.MaxResults != 7 {
		t.Errorf("esperava max_results=offset+pedido (7), got %d", gotBody.MaxResults)
	}
	if len(results) != 1 || results[0].Title != "C" {
		t.Errorf("fatia local do offset incorreta: %+v", results)
	}
}

func TestWebSearch_CadeiaBraveTavilyFallback(t *testing.T) {
	// Brave e Tavily sem credencial => a cadeia atravessa ambos e o
	// DuckDuckGo (mock) responde.
	credMgr := credentials.NewManager(nil)
	tool := NewWebSearch(credMgr)
	tool.fallback = &mockSearchProvider{results: []SearchResult{
		{Title: "Fallback", URL: "https://exemplo.dev", Snippet: "via cadeia"},
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
	if out.Provider != "MockSearch" || out.Count != 1 {
		t.Errorf("cadeia não chegou ao fallback: %+v", out)
	}
}

func TestWebSearch_CadeiaBrave401CaiNaTavily(t *testing.T) {
	// Caminho primário da cadeia: Brave responde 401 e a Tavily atende.
	var braveChamadas int
	var braveToken string
	braveSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		braveChamadas++
		braveToken = r.Header.Get("X-Subscription-Token")
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer braveSrv.Close()
	var tavilyChamadas int
	var tavilyAuth string
	tavilySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tavilyChamadas++
		tavilyAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results": [{"title": "Go", "url": "https://go.dev", "content": "Linguagem Go"}]}`))
	}))
	defer tavilySrv.Close()

	credMgr := credentials.NewManager(nil)
	if err := credMgr.RegisterPattern("127.0.0.1", &credentials.AuthConfig{Source: "static", Type: "bearer", Token: "chave-cadeia"}); err != nil {
		t.Fatalf("registro de credencial: %v", err)
	}
	tool := NewWebSearch(credMgr)
	tool.brave = &braveProvider{credMgr: credMgr, endpointOverride: braveSrv.URL}
	tool.tavily = &tavilyProvider{credMgr: credMgr, endpointOverride: tavilySrv.URL}
	tool.fallback = &mockSearchProvider{results: []SearchResult{
		{Title: "Nunca", URL: "https://nunca.dev", Snippet: "não deve ser alcançado"},
	}}

	ctx := trustedTavilyTestContext(t, context.Background(), braveSrv)
	ctx = trustedTavilyTestContext(t, ctx, tavilySrv)
	results, providerName, err := tool.searchWithFallback(ctx, "go", 0, 5)
	if err != nil {
		t.Fatalf("cadeia falhou: %v", err)
	}
	if braveChamadas != 1 || braveToken != "chave-cadeia" {
		t.Errorf("Brave deveria ser tentada primeiro com a chave (chamadas=%d token=%q)", braveChamadas, braveToken)
	}
	if tavilyChamadas != 1 || tavilyAuth != "Bearer chave-cadeia" {
		t.Errorf("Tavily deveria atender em seguida (chamadas=%d auth=%q)", tavilyChamadas, tavilyAuth)
	}
	if providerName != "Tavily" {
		t.Errorf("esperava provider Tavily, got %q", providerName)
	}
	if len(results) != 1 || results[0].Title != "Go" {
		t.Errorf("resultado da Tavily não chegou: %+v", results)
	}
}

func TestWebSearch_CadeiaTavily429CaiNoFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	credMgr := credentials.NewManager(nil)
	if err := credMgr.RegisterPattern(u.Hostname(), &credentials.AuthConfig{Source: "static", Type: "bearer", Token: "x"}); err != nil {
		t.Fatalf("registro de credencial: %v", err)
	}
	tool := NewWebSearch(credMgr)
	tool.tavily = &tavilyProvider{credMgr: credMgr, endpointOverride: srv.URL}
	tool.fallback = &mockSearchProvider{results: []SearchResult{
		{Title: "Fallback", URL: "https://exemplo.dev", Snippet: "via 429"},
	}}

	ctx := trustedTavilyTestContext(t, context.Background(), srv)
	// Brave não tem credencial (api.search.brave.com sem registro) => pula;
	// Tavily 429 => avança; mock responde.
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
