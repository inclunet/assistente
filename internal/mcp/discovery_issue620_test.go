package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestBuildResourceBasesDeepPathIsDeterministicAndSafe(t *testing.T) {
	got := buildResourceBases("https://example.com/api/./2.0/../mcp/sql/?token=secret#fragment")
	want := []string{
		"https://example.com/api/mcp/sql",
		"https://example.com/api/mcp",
		"https://example.com/api",
		"https://example.com",
	}

	if !slices.Equal(got, want) {
		t.Fatalf("bases:\n got: %v\nwant: %v", got, want)
	}
	for _, candidate := range buildPRMCandidates("https://example.com/api/./2.0/../mcp/sql/?token=secret#fragment") {
		if strings.Contains(candidate, "..") || strings.Contains(candidate, "token=") || strings.Contains(candidate, "#") {
			t.Fatalf("candidato não saneado: %q", candidate)
		}
	}
	for _, candidate := range buildPRMCandidates("https://example.com/api/%2e%2e/admin/mcp") {
		if strings.Contains(strings.ToLower(candidate), "%2e") || strings.Contains(candidate, "..") {
			t.Fatalf("traversal codificado preservado: %q", candidate)
		}
	}
}

func TestBuildResourceBasesLimitsDepthAndKeepsOrigin(t *testing.T) {
	rawURL := "https://example.com/" + strings.Repeat("segment/", maxDiscoveryBases+20)
	bases := buildResourceBases(rawURL)

	if len(bases) != maxDiscoveryBases {
		t.Fatalf("quantidade de bases = %d, esperava %d", len(bases), maxDiscoveryBases)
	}
	if bases[0] != strings.TrimRight(rawURL, "/") {
		t.Fatalf("recurso completo não veio primeiro: %q", bases[0])
	}
	if bases[len(bases)-1] != "https://example.com" {
		t.Fatalf("origin não veio por último: %q", bases[len(bases)-1])
	}
	seen := make(map[string]struct{}, len(bases))
	for _, base := range bases {
		if _, exists := seen[base]; exists {
			t.Fatalf("base duplicada: %q", base)
		}
		seen[base] = struct{}{}
	}
}

func TestBuildPRMCandidatesDeepPathOrderAndDeduplication(t *testing.T) {
	got := buildPRMCandidates("https://example.com/api/2.0/mcp/sql/")
	want := []string{
		"https://example.com/.well-known/oauth-protected-resource/api/2.0/mcp/sql",
		"https://example.com/api/2.0/mcp/sql/.well-known/oauth-protected-resource",
		"https://example.com/.well-known/oauth-protected-resource/api/2.0/mcp",
		"https://example.com/api/2.0/mcp/.well-known/oauth-protected-resource",
		"https://example.com/.well-known/oauth-protected-resource/api/2.0",
		"https://example.com/api/2.0/.well-known/oauth-protected-resource",
		"https://example.com/.well-known/oauth-protected-resource/api",
		"https://example.com/api/.well-known/oauth-protected-resource",
		"https://example.com/.well-known/oauth-protected-resource",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("candidatos PRM:\n got: %v\nwant: %v", got, want)
	}
}

func TestDiscoverOAuthWithoutPRMFindsOIDCAtResourceAncestor(t *testing.T) {
	var mu sync.Mutex
	var requested []string
	var serverURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requested = append(requested, r.URL.Path)
		mu.Unlock()
		if r.URL.Path == "/api/2.0/.well-known/openid-configuration" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"issuer":                 serverURL + "/api/2.0",
				"authorization_endpoint": serverURL + "/authorize",
				"token_endpoint":         serverURL + "/token",
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()
	serverURL = server.URL

	result := DiscoverOAuth(server.URL + "/api/2.0/mcp/sql?x=1#fragment")
	if !result.Found || result.MetadataType != "oidc" {
		t.Fatalf("esperava OIDC descoberto, resultado: %+v", result)
	}
	if result.ProtectedResourceFound {
		t.Fatal("PRM não deveria ter sido encontrado")
	}
	if !result.AuthorizationServerFound {
		t.Fatal("metadata do authorization server deveria ter sido encontrada")
	}
	if !result.ManualCompletionRequired {
		t.Fatal("OIDC sem registration_endpoint exige conclusão manual")
	}

	runtimeDiscovery, err := discoverOAuthEndpoints(server.URL + "/api/2.0/mcp/sql")
	if err != nil {
		t.Fatalf("discovery usado pelo fluxo OAuth também deve funcionar sem PRM: %v", err)
	}
	if runtimeDiscovery.TokenEndpoint != server.URL+"/token" {
		t.Fatalf("token endpoint runtime: %q", runtimeDiscovery.TokenEndpoint)
	}

	mu.Lock()
	defer mu.Unlock()
	if !slices.Contains(requested, "/api/2.0/.well-known/openid-configuration") {
		t.Fatalf("candidato OIDC ancestral não consultado: %v", requested)
	}
	for _, requestedPath := range requested {
		if strings.Contains(requestedPath, "..") {
			t.Fatalf("request com traversal: %q", requestedPath)
		}
	}
}

func TestDiscoverOAuthRepresentsPRMWithoutAuthorizationServerMetadata(t *testing.T) {
	var serverURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/oauth-protected-resource/deep/mcp" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"resource":      serverURL + "/deep/mcp",
				"resource_name": "Recurso parcial",
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()
	serverURL = server.URL

	result := DiscoverOAuth(server.URL + "/deep/mcp")
	if result.Found || result.Status != "partial" || !result.ProtectedResourceFound {
		t.Fatalf("resultado parcial inesperado: %+v", result)
	}
	if !result.ManualCompletionRequired || result.ResourceName != "Recurso parcial" {
		t.Fatalf("resultado parcial incompleto: %+v", result)
	}
}

func TestDiscoverOAuthPreservesPartialMetadataWhenAttemptBudgetEnds(t *testing.T) {
	var serverURL string
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path == "/.well-known/oauth-protected-resource/deep/mcp" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"resource":         serverURL + "/deep/mcp",
				"resource_name":    "Recurso preservado",
				"scopes_supported": []string{"read"},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()
	serverURL = server.URL

	budget := newDiscoveryBudgetWithLimits(context.Background(), time.Second, 2, 3)
	defer budget.close()
	result := discoverOAuthWithBudget(server.URL+"/deep/mcp", budget)

	if result.Status != "partial" || !result.ProtectedResourceFound ||
		result.ResourceName != "Recurso preservado" || !slices.Equal(result.Scopes, []string{"read"}) {
		t.Fatalf("metadata parcial perdida ao esgotar orçamento: %+v", result)
	}
	if requests != 2 {
		t.Fatalf("orçamento de tentativas não respeitado: requests=%d", requests)
	}
	if !slices.ContainsFunc(result.ResponseHints, func(hint DiscoveryResponseHint) bool {
		return hint.Classification == "budget_exhausted"
	}) {
		t.Fatalf("hint de orçamento ausente: %+v", result.ResponseHints)
	}
}

func TestDiscoverOAuthCancellationStopsInFlightRequest(t *testing.T) {
	requestStarted := make(chan struct{})
	requestStopped := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		close(requestStarted)
		<-r.Context().Done()
		close(requestStopped)
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	resultCh := make(chan OAuthDiscoveryResult, 1)
	go func() {
		resultCh <- discoverOAuthContext(ctx, server.URL+"/deep/mcp")
	}()

	select {
	case <-requestStarted:
	case <-time.After(time.Second):
		t.Fatal("request de discovery não iniciou")
	}
	cancel()

	select {
	case result := <-resultCh:
		if result.Status != "not_found" || !slices.ContainsFunc(result.ResponseHints, func(hint DiscoveryResponseHint) bool {
			return hint.Classification == "budget_exhausted"
		}) {
			t.Fatalf("cancelamento não retornou resultado seguro: %+v", result)
		}
	case <-time.After(time.Second):
		t.Fatal("discovery não respeitou cancelamento")
	}
	select {
	case <-requestStopped:
	case <-time.After(time.Second):
		t.Fatal("handler permaneceu bloqueado após cancelamento")
	}
}

func TestDiscoveryBudgetStopsAfterRepeatedNetworkErrors(t *testing.T) {
	budget := newDiscoveryBudgetWithLimits(context.Background(), time.Second, 10, 3)
	defer budget.close()

	for attempt := 1; attempt <= 3; attempt++ {
		if err := budget.beginAttempt(); err != nil {
			t.Fatalf("tentativa %d recusada cedo: %v", attempt, err)
		}
		err := budget.finishAttempt(context.DeadlineExceeded)
		if attempt < 3 && err != nil {
			t.Fatalf("orçamento terminou na tentativa %d: %v", attempt, err)
		}
		if attempt == 3 && err == nil {
			t.Fatal("orçamento não terminou após três erros de rede consecutivos")
		}
	}
	if budget.attempts != 3 {
		t.Fatalf("tentativas registradas = %d, esperava 3", budget.attempts)
	}
}

func TestDiscoverOAuthInfersClientCredentialsWithoutGrantList(t *testing.T) {
	var serverURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/oauth-protected-resource/mcp":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"resource":              serverURL + "/mcp",
				"authorization_servers": []string{serverURL},
			})
		case "/.well-known/oauth-authorization-server":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"issuer":         serverURL,
				"token_endpoint": serverURL + "/token",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	serverURL = server.URL

	result := DiscoverOAuth(server.URL + "/mcp")
	if !result.Found || result.AuthType != AuthOAuth2ClientCredentials ||
		result.AuthURL != "" || result.TokenURL != server.URL+"/token" {
		t.Fatalf("metadata token-only não inferiu client credentials: %+v", result)
	}
	runtimeResult, err := discoverOAuthEndpoints(server.URL + "/mcp")
	if err != nil || runtimeResult.TokenEndpoint != server.URL+"/token" {
		t.Fatalf("runtime rejeitou metadata token-only: result=%+v err=%v", runtimeResult, err)
	}
}

func TestProtectedResourceMetadataRequiresResourceIdentifier(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"resource_name":    "Metadata incompleta",
			"scopes_supported": []string{"read"},
		})
	}))
	defer server.Close()

	result, _, err := fetchProtectedResourceMetadataDetailed(server.URL + "/mcp")
	if err == nil || result != nil {
		t.Fatalf("PRM sem resource foi aceito: result=%+v err=%v", result, err)
	}
}

func TestDiscoverOAuthUsesCanonicalPRMResourceAsAuthorizationBase(t *testing.T) {
	var serverURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/oauth-protected-resource/input":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"resource":              serverURL + "/canonical/deep/mcp/../resource?tenant=secret#fragment",
				"authorization_servers": []string{"not-a-url", "https://user:secret@example.test/issuer"},
			})
		case "/canonical/deep/.well-known/openid-configuration":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"issuer":                 serverURL + "/canonical/deep",
				"authorization_endpoint": serverURL + "/authorize",
				"token_endpoint":         serverURL + "/token",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	serverURL = server.URL

	result := DiscoverOAuth(server.URL + "/input")
	if !result.Found || result.TokenURL != server.URL+"/token" {
		t.Fatalf("base canônica do PRM não usada: %+v", result)
	}
	runtimeResult, err := discoverOAuthEndpoints(server.URL + "/input?ignored=1#fragment")
	if err != nil {
		t.Fatalf("runtime discovery falhou: %v", err)
	}
	if runtimeResult.Resource != server.URL+"/canonical/deep/resource" {
		t.Fatalf("resource RFC 8707 não normalizado: %q", runtimeResult.Resource)
	}
}

func TestDiscoveryNon200HintsAreBoundedAndSanitized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "oauth-protected-resource") {
			w.Header().Set("WWW-Authenticate", `Bearer realm="mcp", resource_metadata="https://example.test/meta?token=secret", token="secret-token"`)
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"invalid_request","error_description":"token=secret-value"}`))
			return
		}
		w.Header().Set("Location", "https://user:password@example.test/login?access_token=secret#fragment")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(strings.Repeat("x", discoveryBodyLimit+100)))
	}))
	defer server.Close()

	result := DiscoverOAuth(server.URL + "/deep/mcp")
	if result.Found {
		t.Fatal("401/403 não podem ser tratados como metadata válida")
	}

	var saw401, saw403Truncated bool
	for _, hint := range result.ResponseHints {
		serialized, err := json.Marshal(hint)
		if err != nil {
			t.Fatal(err)
		}
		text := string(serialized)
		if strings.Contains(text, "secret-token") || strings.Contains(text, "secret-value") ||
			strings.Contains(text, "access_token=secret") || strings.Contains(text, "user:password") {
			t.Fatalf("hint vazou dado sensível: %s", text)
		}
		if hint.StatusCode == http.StatusUnauthorized && hint.Classification == "authorization_required" {
			saw401 = true
		}
		if hint.StatusCode == http.StatusForbidden && hint.BodyTruncated {
			saw403Truncated = true
		}
	}
	if !saw401 || !saw403Truncated {
		t.Fatalf("hints esperados não encontrados: %+v", result.ResponseHints)
	}
	if len(result.ResponseHints) > maxDiscoveryHints {
		t.Fatalf("hints sem limite: %d", len(result.ResponseHints))
	}
}

func TestDiscoveryRecordsTruncatedSuccessfulBodyWithoutExposingIt(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("<html>" + strings.Repeat("sensitive", discoveryBodyLimit) + "</html>"))
	}))
	defer server.Close()

	var target authServerMetadata
	attempt, err := fetchJSON(server.URL, &target)
	if err == nil {
		t.Fatal("body 200 acima do limite não pode ser aceito como metadata")
	}
	if attempt.hint == nil || attempt.hint.StatusCode != http.StatusOK ||
		attempt.hint.Classification != "invalid_metadata" || !attempt.hint.BodyTruncated {
		t.Fatalf("hint de truncamento ausente: %+v", attempt.hint)
	}
	if attempt.hint.JSONError != "" || attempt.hint.WWWAuthenticate != "" {
		t.Fatalf("conteúdo do body 200 não deve ser exposto: %+v", attempt.hint)
	}
}

func TestDiscoveryRecordsInvalidSuccessfulJSONWithoutExposingBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("<html>token=secret</html>"))
	}))
	defer server.Close()

	var target authServerMetadata
	attempt, err := fetchJSON(server.URL, &target)
	if err == nil {
		t.Fatal("HTML curto não pode ser aceito como metadata")
	}
	if attempt.hint == nil || attempt.hint.StatusCode != http.StatusOK ||
		attempt.hint.Classification != "invalid_metadata" {
		t.Fatalf("hint de JSON inválido ausente: %+v", attempt.hint)
	}
	if attempt.hint.JSONError != "" || attempt.hint.WWWAuthenticate != "" {
		t.Fatalf("body inválido não deve ser exposto: %+v", attempt.hint)
	}
}

func TestDiscoveryResolvesAndSanitizesRelativeLocationHint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", "/login?access_token=secret#fragment")
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	var target authServerMetadata
	attempt, err := fetchJSON(server.URL+"/metadata", &target)
	if err == nil {
		t.Fatal("401 não pode ser aceito como metadata")
	}
	if attempt.hint == nil || attempt.hint.Location != server.URL+"/login" {
		t.Fatalf("Location relativo não resolvido/saneado: %+v", attempt.hint)
	}
}

func TestSanitizeURLHintAcceptsOnlyHTTP(t *testing.T) {
	if got := sanitizeURLHint("ftp://example.test/metadata?token=secret"); got != "" {
		t.Fatalf("esquema inesperado exposto no hint: %q", got)
	}
	if got := sanitizeURLHint("HTTPS://user:secret@example.test/metadata?token=secret#fragment"); got != "https://example.test/metadata" {
		t.Fatalf("URL HTTPS não foi saneada: %q", got)
	}
	if got := sanitizeURLHint("https://example.test/metadata?"); got != "https://example.test/metadata" {
		t.Fatalf("query vazia foi preservada no hint: %q", got)
	}
}

func TestSanitizeWWWAuthenticateRedactsOAuthTokens(t *testing.T) {
	got := sanitizeWWWAuthenticate(`Bearer access_token=access-secret, refresh_token="refresh-secret", id_token=id-secret`)
	for _, secret := range []string{"access-secret", "refresh-secret", "id-secret"} {
		if strings.Contains(got, secret) {
			t.Fatalf("token OAuth vazou no hint: %q", got)
		}
	}
	for _, key := range []string{"access_token", "refresh_token", "id_token"} {
		if !strings.Contains(got, key+`="[redacted]"`) {
			t.Fatalf("chave %s não foi redigida explicitamente: %q", key, got)
		}
	}
}

func TestDiscoveryRedirectStatusFallsBackToPreviousRequest(t *testing.T) {
	next := &http.Request{}
	previous := &http.Request{Response: &http.Response{StatusCode: http.StatusTemporaryRedirect}}

	if got := discoveryRedirectStatus(next, []*http.Request{previous}); got != http.StatusTemporaryRedirect {
		t.Fatalf("status do redirect = %d, esperava %d", got, http.StatusTemporaryRedirect)
	}
}

func TestDiscoveryPreservesHintWhenRedirectIsRejected(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", "ftp://example.test/metadata?token=secret")
		w.WriteHeader(http.StatusFound)
	}))
	defer server.Close()

	var target authServerMetadata
	attempt, err := fetchJSON(server.URL, &target)
	if err == nil {
		t.Fatal("redirect com esquema inseguro não foi rejeitado")
	}
	if attempt.hint == nil || attempt.hint.StatusCode != http.StatusFound ||
		attempt.hint.Classification != "redirect" || attempt.hint.Location != "" {
		t.Fatalf("hint de redirect rejeitado ausente ou inseguro: %+v", attempt.hint)
	}
}

func TestDiscoveryFollowsSafeRedirect(t *testing.T) {
	var serverURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/oauth-authorization-server/deep/mcp":
			http.Redirect(w, r, "/metadata", http.StatusTemporaryRedirect)
		case "/metadata":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"issuer":                 serverURL + "/deep/mcp",
				"authorization_endpoint": serverURL + "/authorize",
				"token_endpoint":         serverURL + "/token",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	serverURL = server.URL

	result := DiscoverOAuth(server.URL + "/deep/mcp")
	if !result.Found || result.TokenURL != server.URL+"/token" {
		t.Fatalf("redirect seguro não seguido: %+v", result)
	}
	if !slices.ContainsFunc(result.ResponseHints, func(hint DiscoveryResponseHint) bool {
		return hint.Classification == "redirect" && hint.StatusCode == http.StatusTemporaryRedirect &&
			hint.Location == server.URL+"/metadata"
	}) {
		t.Fatalf("redirect seguro não registrado nos hints: %+v", result.ResponseHints)
	}
}
