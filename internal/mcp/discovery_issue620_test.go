package mcp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
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
