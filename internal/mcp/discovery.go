package mcp

import (
	"assistente/internal/logging"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

const (
	discoveryBodyLimit = 4 * 1024
	maxDiscoveryHints  = 24
	maxDiscoveryBases  = 16
)

// OAuthDiscoveryResult contém os metadados OAuth descobertos de um servidor MCP.
type OAuthDiscoveryResult struct {
	Found                    bool                    `json:"found"`
	Status                   string                  `json:"status"`
	ProtectedResourceFound   bool                    `json:"protectedResourceFound"`
	AuthorizationServerFound bool                    `json:"authorizationServerFound"`
	MetadataType             string                  `json:"metadataType,omitempty"`
	ManualCompletionRequired bool                    `json:"manualCompletionRequired"`
	AuthType                 AuthType                `json:"authType"`
	AuthURL                  string                  `json:"authUrl"`
	TokenURL                 string                  `json:"tokenUrl"`
	Scopes                   []string                `json:"scopes"`
	ClientID                 string                  `json:"clientId,omitempty"`
	RegistrationURL          string                  `json:"registrationUrl,omitempty"`
	ResourceName             string                  `json:"resourceName,omitempty"`
	SupportsPKCE             bool                    `json:"supportsPkce"`
	ResponseHints            []DiscoveryResponseHint `json:"responseHints,omitempty"`
	Error                    string                  `json:"error,omitempty"`
}

// DiscoveryResponseHint contém apenas dados limitados e saneados de uma
// resposta de discovery que não pôde ser usada como metadata.
type DiscoveryResponseHint struct {
	StatusCode      int    `json:"statusCode"`
	Classification  string `json:"classification"`
	WWWAuthenticate string `json:"wwwAuthenticate,omitempty"`
	Location        string `json:"location,omitempty"`
	JSONError       string `json:"jsonError,omitempty"`
	BodyTruncated   bool   `json:"bodyTruncated,omitempty"`
}

var discoveryHTTPClient = &http.Client{Timeout: 5 * time.Second}

// DiscoverOAuth consulta os endpoints well-known de um servidor MCP para
// preencher automaticamente a configuração de autenticação OAuth.
//
// Segue a spec MCP Authorization (RFC 9728 + RFC 8414/OIDC Discovery):
// 1. GET protected resource metadata (resource URL → origin fallback)
// 2. GET auth server metadata (issuer URL → RFC 8414 path → origin fallback)
func DiscoverOAuth(serverURL string) OAuthDiscoveryResult {
	origin, err := extractOrigin(serverURL)
	if err != nil {
		return OAuthDiscoveryResult{Status: "not_found", Error: err.Error()}
	}

	resourceForLog := origin
	if bases := buildResourceBases(serverURL); len(bases) > 0 {
		resourceForLog = bases[0]
	}
	logging.Infof(context.Background(), "mcp.discovery", "[MCP:discovery] Tentando discovery OAuth para %s", resourceForLog)

	var resourceName string
	var resourceScopes []string
	var hints []DiscoveryResponseHint
	resourceBaseURL := serverURL

	prm, prmHints, err := fetchProtectedResourceMetadataDetailed(serverURL)
	hints = appendHints(hints, prmHints...)
	protectedResourceFound := err == nil && prm != nil
	authServerBases := make([]string, 0)
	if err == nil && prm != nil {
		authServerBases = canonicalAuthorizationServerBases(prm.AuthorizationServers)
		if prm.Resource != "" {
			resourceBaseURL = prm.Resource
		}
		resourceName = prm.ResourceName
		resourceScopes = prm.ScopesSupported
		logging.Infof(context.Background(), "mcp.discovery", "[MCP:discovery] Protected Resource Metadata encontrado")
	} else {
		logging.Infof(context.Background(), "mcp.discovery", "[MCP:discovery] Protected Resource Metadata não encontrado para o recurso")
	}

	if len(authServerBases) == 0 {
		authServerBases = buildResourceBases(resourceBaseURL)
		if len(authServerBases) == 0 && resourceBaseURL != serverURL {
			authServerBases = buildResourceBases(serverURL)
		}
	}

	asm, metadataType, asmHints, err := fetchAuthServerMetadataFromBases(authServerBases)
	hints = appendHints(hints, asmHints...)
	if err != nil || asm == nil {
		logging.Warnf(context.Background(), "mcp.discovery", "[MCP:discovery] Authorization Server Metadata não encontrado")
		status := "not_found"
		if protectedResourceFound {
			status = "partial"
		}
		return OAuthDiscoveryResult{
			Status:                   status,
			ProtectedResourceFound:   protectedResourceFound,
			ManualCompletionRequired: true,
			Scopes:                   resourceScopes,
			ResourceName:             resourceName,
			ResponseHints:            hints,
			Error:                    err.Error(),
		}
	}

	logging.Infof(context.Background(), "mcp.discovery", "[MCP:discovery] Authorization Server Metadata encontrado (%s)", metadataType)

	scopes := resourceScopes
	if len(scopes) == 0 {
		scopes = asm.ScopesSupported
	}

	supportsPKCE := slices.Contains(asm.CodeChallengeMethodsSupported, "S256")

	authType := AuthOAuth2PKCE
	if !supportsPKCE && slices.Contains(asm.GrantTypesSupported, "client_credentials") {
		authType = AuthOAuth2ClientCredentials
	}

	return OAuthDiscoveryResult{
		Found:                    true,
		Status:                   "complete",
		ProtectedResourceFound:   protectedResourceFound,
		AuthorizationServerFound: true,
		MetadataType:             metadataType,
		ManualCompletionRequired: asm.RegistrationEndpoint == "",
		AuthType:                 authType,
		AuthURL:                  asm.AuthorizationEndpoint,
		TokenURL:                 asm.TokenEndpoint,
		Scopes:                   scopes,
		RegistrationURL:          asm.RegistrationEndpoint,
		ResourceName:             resourceName,
		SupportsPKCE:             supportsPKCE,
		ResponseHints:            hints,
	}
}

// protectedResourceMetadata representa a resposta de
// GET /.well-known/oauth-protected-resource (RFC 9728 / MCP spec).
type protectedResourceMetadata struct {
	AuthorizationServers []string `json:"authorization_servers"`
	ScopesSupported      []string `json:"scopes_supported"`
	ResourceName         string   `json:"resource_name"`
	Resource             string   `json:"resource"`
}

// authServerMetadata representa a resposta de
// GET /.well-known/oauth-authorization-server (RFC 8414).
type authServerMetadata struct {
	Issuer                        string   `json:"issuer"`
	AuthorizationEndpoint         string   `json:"authorization_endpoint"`
	TokenEndpoint                 string   `json:"token_endpoint"`
	RegistrationEndpoint          string   `json:"registration_endpoint"`
	DeviceAuthorizationEndpoint   string   `json:"device_authorization_endpoint"`
	ScopesSupported               []string `json:"scopes_supported"`
	GrantTypesSupported           []string `json:"grant_types_supported"`
	CodeChallengeMethodsSupported []string `json:"code_challenge_methods_supported"`
	ResponseTypesSupported        []string `json:"response_types_supported"`
}

// fetchProtectedResourceMetadata tenta descobrir metadata do recurso protegido
// conforme RFC 9728. Para recurso, ancestrais e origin, tenta primeiro a
// localização normativa /.well-known/oauth-protected-resource{path} e depois
// o fallback relativo {base}/.well-known/oauth-protected-resource, sempre com
// ordem determinística e deduplicação.
func fetchProtectedResourceMetadata(mcpURL string) (*protectedResourceMetadata, error) {
	result, _, err := fetchProtectedResourceMetadataDetailed(mcpURL)
	return result, err
}

func fetchProtectedResourceMetadataDetailed(mcpURL string) (*protectedResourceMetadata, []DiscoveryResponseHint, error) {
	candidates := buildPRMCandidates(mcpURL)
	var hints []DiscoveryResponseHint
	for _, candidateURL := range candidates {
		logging.Debugf(context.Background(), "mcp.discovery", "[MCP:discovery] PRM: tentando %s", candidateURL)
		var result protectedResourceMetadata
		attempt, err := fetchJSON(candidateURL, &result)
		if attempt.hint != nil {
			hints = appendHints(hints, *attempt.hint)
		}
		if err == nil && (result.Resource != "" || len(result.AuthorizationServers) > 0) {
			logging.Infof(context.Background(), "mcp.discovery", "[MCP:discovery] PRM: encontrado em %s", candidateURL)
			return &result, hints, nil
		}
	}
	return nil, hints, fmt.Errorf("protected resource metadata not found (tentou %d URLs)", len(candidates))
}

func buildPRMCandidates(mcpURL string) []string {
	bases := buildResourceBases(mcpURL)
	seen := make(map[string]struct{})
	var candidates []string
	add := func(s string) {
		if _, ok := seen[s]; !ok {
			seen[s] = struct{}{}
			candidates = append(candidates, s)
		}
	}

	for _, base := range bases {
		u, err := url.Parse(base)
		if err != nil {
			continue
		}
		origin := u.Scheme + "://" + u.Host
		cleanPath := strings.TrimRight(u.EscapedPath(), "/")
		// RFC 9728 §3.1: o path do recurso vem depois do well-known.
		if cleanPath != "" {
			add(origin + "/.well-known/oauth-protected-resource" + cleanPath)
			// Compatibilidade com servidores MCP que publicam o well-known
			// relativo ao diretório do recurso.
			add(base + "/.well-known/oauth-protected-resource")
		} else {
			add(origin + "/.well-known/oauth-protected-resource")
		}
	}
	return candidates
}

// buildResourceBases devolve recurso, ancestrais e origin, nessa ordem. A URL é
// normalizada sem query/fragment e segmentos "."/".." nunca viram candidatos.
func buildResourceBases(rawURL string) []string {
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
		return nil
	}
	u.RawQuery = ""
	u.Fragment = ""
	u.RawFragment = ""

	cleanPath := canonicalEscapedPath(u.Path)
	origin := u.Scheme + "://" + u.Host
	seen := make(map[string]struct{})
	var bases []string
	for {
		base := origin + cleanPath
		if _, ok := seen[base]; !ok {
			seen[base] = struct{}{}
			bases = append(bases, base)
		}
		if cleanPath == "" {
			break
		}
		if len(bases) >= maxDiscoveryBases-1 {
			if _, ok := seen[origin]; !ok {
				bases = append(bases, origin)
			}
			break
		}
		cleanPath = path.Dir(cleanPath)
		if cleanPath == "/" || cleanPath == "." {
			cleanPath = ""
		}
	}
	return bases
}

func canonicalAuthorizationServerBases(values []string) []string {
	seen := make(map[string]struct{})
	bases := make([]string, 0, len(values))
	for _, value := range values {
		candidates := buildResourceBases(value)
		if len(candidates) == 0 {
			continue
		}
		base := candidates[0]
		if _, exists := seen[base]; exists {
			continue
		}
		seen[base] = struct{}{}
		bases = append(bases, base)
	}
	return bases
}

func canonicalEscapedPath(rawPath string) string {
	cleanPath := path.Clean("/" + strings.TrimPrefix(rawPath, "/"))
	if cleanPath == "/" || cleanPath == "." {
		return ""
	}
	segments := strings.Split(strings.TrimPrefix(cleanPath, "/"), "/")
	for index, segment := range segments {
		segments[index] = url.PathEscape(segment)
	}
	return "/" + strings.Join(segments, "/")
}

// fetchAuthServerMetadataFromBases tenta descobrir metadata do authorization
// server (RFC 8414) ou do provider OIDC.
// Para cada base, tenta primeiro RFC 8414 e OIDC Discovery; em seguida,
// localizações legadas compatíveis. As bases são processadas em ordem e
// deduplicadas globalmente.
func fetchAuthServerMetadataFromBases(bases []string) (*authServerMetadata, string, []DiscoveryResponseHint, error) {
	var hints []DiscoveryResponseHint
	candidateTypes := make(map[string]string)
	var candidates []string
	for _, base := range bases {
		for _, candidate := range buildASMCandidateDetails(base) {
			if _, exists := candidateTypes[candidate.url]; exists {
				continue
			}
			candidateTypes[candidate.url] = candidate.metadataType
			candidates = append(candidates, candidate.url)
		}
	}
	for _, candidateURL := range candidates {
		logging.Debugf(context.Background(), "mcp.discovery", "[MCP:discovery] ASM: tentando %s", candidateURL)
		var result authServerMetadata
		attempt, err := fetchJSON(candidateURL, &result)
		if attempt.hint != nil {
			hints = appendHints(hints, *attempt.hint)
		}
		if err == nil && validAuthServerMetadata(&result) {
			logging.Infof(context.Background(), "mcp.discovery", "[MCP:discovery] ASM: encontrado em %s", candidateURL)
			return &result, candidateTypes[candidateURL], hints, nil
		}
	}
	return nil, "", hints, fmt.Errorf("auth server metadata not found (tentou %d URLs)", len(candidates))
}

func buildASMCandidates(authServerBase string) []string {
	details := buildASMCandidateDetails(authServerBase)
	result := make([]string, 0, len(details))
	for _, detail := range details {
		result = append(result, detail.url)
	}
	return result
}

type asmCandidate struct {
	url          string
	metadataType string
}

func buildASMCandidateDetails(authServerBase string) []asmCandidate {
	bases := buildResourceBases(authServerBase)
	if len(bases) == 0 {
		return nil
	}
	base := bases[0]
	u, _ := url.Parse(base)
	origin := u.Scheme + "://" + u.Host
	issuerPath := strings.TrimRight(u.EscapedPath(), "/")

	seen := make(map[string]struct{})
	var candidates []asmCandidate
	add := func(rawURL, metadataType string) {
		if _, ok := seen[rawURL]; !ok {
			seen[rawURL] = struct{}{}
			candidates = append(candidates, asmCandidate{url: rawURL, metadataType: metadataType})
		}
	}

	// RFC 8414 §3: sufixo well-known antes do path do issuer.
	add(origin+"/.well-known/oauth-authorization-server"+issuerPath, "oauth")
	// OIDC Discovery §4.1: openid-configuration é anexado ao issuer.
	add(base+"/.well-known/openid-configuration", "oidc")
	if issuerPath != "" {
		// Fallbacks compatíveis já aceitos pelo Assistente.
		add(base+"/.well-known/oauth-authorization-server", "oauth")
		add(origin+"/.well-known/openid-configuration"+issuerPath, "oidc")
		add(origin+"/.well-known/oauth-authorization-server", "oauth")
		add(origin+"/.well-known/openid-configuration", "oidc")
	}
	return candidates
}

func validAuthServerMetadata(metadata *authServerMetadata) bool {
	if metadata == nil || metadata.TokenEndpoint == "" {
		return false
	}
	return metadata.AuthorizationEndpoint != "" ||
		slices.Contains(metadata.GrantTypesSupported, "client_credentials")
}

type fetchAttempt struct {
	hint *DiscoveryResponseHint
}

func fetchJSON(rawURL string, target any) (fetchAttempt, error) {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return fetchAttempt{}, err
	}
	req.Header.Set("Accept", "application/json")

	redirects := make([]DiscoveryResponseHint, 0, 2)
	client := *discoveryHTTPClient
	originalRedirectPolicy := client.CheckRedirect
	client.CheckRedirect = func(next *http.Request, via []*http.Request) error {
		statusCode := 0
		if next.Response != nil {
			statusCode = next.Response.StatusCode
		}
		redirects = append(redirects, DiscoveryResponseHint{
			StatusCode:     statusCode,
			Classification: "redirect",
			Location:       sanitizeURLHint(next.URL.String()),
		})
		if len(via) >= 5 {
			return fmt.Errorf("muitos redirects de discovery")
		}
		if next.URL.User != nil || (next.URL.Scheme != "http" && next.URL.Scheme != "https") {
			return fmt.Errorf("redirect de discovery inseguro")
		}
		if len(via) > 0 && via[0].URL.Scheme == "https" && next.URL.Scheme != "https" {
			return fmt.Errorf("downgrade HTTPS em discovery")
		}
		if originalRedirectPolicy != nil {
			return originalRedirectPolicy(next, via)
		}
		return nil
	}

	resp, err := client.Do(req)
	if err != nil {
		return fetchAttempt{}, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, readErr := io.ReadAll(io.LimitReader(resp.Body, discoveryBodyLimit+1))
	if readErr != nil {
		return fetchAttempt{}, readErr
	}
	truncated := len(body) > discoveryBodyLimit
	if truncated {
		body = body[:discoveryBodyLimit]
	}

	if resp.StatusCode != http.StatusOK {
		hint := &DiscoveryResponseHint{
			StatusCode:      resp.StatusCode,
			Classification:  classifyDiscoveryStatus(resp.StatusCode),
			WWWAuthenticate: sanitizeWWWAuthenticate(resp.Header.Get("WWW-Authenticate")),
			Location:        sanitizeResponseLocation(resp),
			JSONError:       extractSafeJSONError(body),
			BodyTruncated:   truncated,
		}
		if hint.Location == "" && len(redirects) > 0 {
			hint.Location = redirects[len(redirects)-1].Location
		}
		return fetchAttempt{hint: hint}, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	if truncated {
		hint := &DiscoveryResponseHint{
			StatusCode:     resp.StatusCode,
			Classification: "invalid_metadata",
			BodyTruncated:  true,
		}
		if len(redirects) > 0 {
			hint.Location = redirects[len(redirects)-1].Location
		}
		return fetchAttempt{hint: hint}, fmt.Errorf("metadata excede %d bytes", discoveryBodyLimit)
	}

	attempt := fetchAttempt{}
	if len(redirects) > 0 {
		redirectHint := redirects[len(redirects)-1]
		attempt.hint = &redirectHint
	}
	if err := json.Unmarshal(body, target); err != nil {
		hint := &DiscoveryResponseHint{
			StatusCode:     resp.StatusCode,
			Classification: "invalid_metadata",
		}
		if attempt.hint != nil {
			hint.Location = attempt.hint.Location
		}
		return fetchAttempt{hint: hint}, err
	}
	return attempt, nil
}

func classifyDiscoveryStatus(status int) string {
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return "authorization_required"
	case status >= 300 && status < 400:
		return "redirect"
	case status == http.StatusNotFound || status == http.StatusGone:
		return "not_found"
	case status == http.StatusTooManyRequests:
		return "rate_limited"
	case status >= 500:
		return "server_error"
	default:
		return "http_error"
	}
}

var sensitiveHintPattern = regexp.MustCompile(`(?i)(token|secret|password|cookie|credential)\s*=\s*("[^"]*"|[^,\s]+)`)

func sanitizeWWWAuthenticate(value string) string {
	value = strings.TrimSpace(strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, value))
	value = sensitiveHintPattern.ReplaceAllString(value, `$1="[redacted]"`)
	if len(value) > 512 {
		value = value[:512]
	}
	return value
}

func sanitizeURLHint(value string) string {
	if value == "" {
		return ""
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" ||
		(!strings.EqualFold(parsed.Scheme, "http") && !strings.EqualFold(parsed.Scheme, "https")) {
		return ""
	}
	parsed.User = nil
	parsed.RawQuery = ""
	parsed.Fragment = ""
	parsed.RawFragment = ""
	result := parsed.String()
	if len(result) > 512 {
		return ""
	}
	return result
}

func sanitizeResponseLocation(resp *http.Response) string {
	if resp == nil {
		return ""
	}
	location, err := resp.Location()
	if err != nil {
		return ""
	}
	return sanitizeURLHint(location.String())
}

func extractSafeJSONError(body []byte) string {
	var payload struct {
		Error            any `json:"error"`
		ErrorDescription any `json:"error_description"`
	}
	if len(body) == 0 || json.Unmarshal(body, &payload) != nil {
		return ""
	}
	parts := make([]string, 0, 2)
	if value := safeScalar(payload.Error); value != "" {
		parts = append(parts, "error="+strconv.Quote(value))
	}
	if value := safeScalar(payload.ErrorDescription); value != "" {
		parts = append(parts, "error_description="+strconv.Quote(value))
	}
	result := strings.Join(parts, ", ")
	if len(result) > 512 {
		result = result[:512]
	}
	return sensitiveHintPattern.ReplaceAllString(result, `$1="[redacted]"`)
}

func safeScalar(value any) string {
	text, ok := value.(string)
	if !ok {
		return ""
	}
	text = strings.TrimSpace(strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, text))
	if len(text) > 256 {
		text = text[:256]
	}
	return text
}

func appendHints(existing []DiscoveryResponseHint, hints ...DiscoveryResponseHint) []DiscoveryResponseHint {
	for _, hint := range hints {
		if len(existing) >= maxDiscoveryHints {
			break
		}
		if hint.WWWAuthenticate == "" && hint.Location == "" && hint.JSONError == "" && !hint.BodyTruncated &&
			hint.Classification == "not_found" {
			continue
		}
		existing = append(existing, hint)
	}
	return existing
}

func extractOrigin(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("URL inválida: %w", err)
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
		return "", fmt.Errorf("URL precisa de scheme e host, sem credenciais embutidas")
	}
	return u.Scheme + "://" + u.Host, nil
}
