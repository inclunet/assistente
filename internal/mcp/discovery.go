package mcp

import (
	"assistente/internal/logging"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
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
	discoveryErrorBodyLimit    = 4 * 1024
	discoveryMetadataBodyLimit = 64 * 1024
	maxDiscoveryHints          = 24
	maxDiscoveryBases          = 16

	discoveryTotalTimeout       = 12 * time.Second
	maxDiscoveryAttempts        = 128
	maxConsecutiveNetworkErrors = 3
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

type discoveryBudget struct {
	ctx                      context.Context
	cancel                   context.CancelFunc
	maxAttempts              int
	maxConsecutiveNetErrors  int
	attempts                 int
	consecutiveNetworkErrors int
	exhaustedErr             error
}

func newDiscoveryBudget(parent context.Context) *discoveryBudget {
	return newDiscoveryBudgetWithLimits(
		parent,
		discoveryTotalTimeout,
		maxDiscoveryAttempts,
		maxConsecutiveNetworkErrors,
	)
}

func newDiscoveryBudgetWithLimits(
	parent context.Context,
	timeout time.Duration,
	maxAttempts int,
	maxConsecutiveNetErrors int,
) *discoveryBudget {
	ctx, cancel := context.WithTimeout(parent, timeout)
	return &discoveryBudget{
		ctx:                     ctx,
		cancel:                  cancel,
		maxAttempts:             maxAttempts,
		maxConsecutiveNetErrors: maxConsecutiveNetErrors,
	}
}

func (budget *discoveryBudget) beginAttempt() error {
	if budget.exhaustedErr != nil {
		return budget.exhaustedErr
	}
	if err := budget.ctx.Err(); err != nil {
		budget.exhaustedErr = fmt.Errorf("orçamento global de discovery esgotado: %w", err)
		return budget.exhaustedErr
	}
	if budget.attempts >= budget.maxAttempts {
		budget.exhaustedErr = fmt.Errorf("orçamento global de discovery esgotado após %d tentativas", budget.attempts)
		return budget.exhaustedErr
	}
	budget.attempts++
	return nil
}

func (budget *discoveryBudget) finishAttempt(err error) error {
	if budget.exhaustedErr != nil {
		return budget.exhaustedErr
	}
	if ctxErr := budget.ctx.Err(); ctxErr != nil {
		budget.exhaustedErr = fmt.Errorf("orçamento global de discovery esgotado: %w", ctxErr)
		return budget.exhaustedErr
	}
	if err == nil || !isTransientDiscoveryError(err) {
		budget.consecutiveNetworkErrors = 0
		return nil
	}
	budget.consecutiveNetworkErrors++
	if budget.consecutiveNetworkErrors >= budget.maxConsecutiveNetErrors {
		budget.exhaustedErr = fmt.Errorf(
			"discovery abortado após %d erros de rede consecutivos",
			budget.consecutiveNetworkErrors,
		)
		return budget.exhaustedErr
	}
	return nil
}

func (budget *discoveryBudget) close() {
	budget.cancel()
}

func isTransientDiscoveryError(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		err = urlErr.Err
	}
	var netErr net.Error
	return errors.As(err, &netErr)
}

func discoveryBudgetHint() DiscoveryResponseHint {
	return DiscoveryResponseHint{Classification: "budget_exhausted"}
}

func appendDiscoveryBudgetHint(hints []DiscoveryResponseHint) []DiscoveryResponseHint {
	if slices.ContainsFunc(hints, func(hint DiscoveryResponseHint) bool {
		return hint.Classification == "budget_exhausted"
	}) {
		return hints
	}
	hint := discoveryBudgetHint()
	if len(hints) >= maxDiscoveryHints {
		hints[len(hints)-1] = hint
		return hints
	}
	return append(hints, hint)
}

// DiscoverOAuth consulta os endpoints well-known de um servidor MCP para
// preencher automaticamente a configuração de autenticação OAuth.
//
// Segue a spec MCP Authorization (RFC 9728 + RFC 8414/OIDC Discovery):
// 1. GET protected resource metadata (recurso → ancestrais → origin)
// 2. GET auth server metadata (bases do PRM ou recurso → ancestrais → origin)
func DiscoverOAuth(serverURL string) OAuthDiscoveryResult {
	return discoverOAuthContext(context.Background(), serverURL)
}

func discoverOAuthContext(ctx context.Context, serverURL string) OAuthDiscoveryResult {
	budget := newDiscoveryBudget(ctx)
	defer budget.close()
	return discoverOAuthWithBudget(serverURL, budget)
}

func discoverOAuthWithBudget(serverURL string, budget *discoveryBudget) OAuthDiscoveryResult {
	origin, err := extractOrigin(serverURL)
	if err != nil {
		return OAuthDiscoveryResult{Status: "not_found", Error: err.Error()}
	}

	resourceForLog := origin
	if bases := buildResourceBases(serverURL); len(bases) > 0 {
		resourceForLog = bases[0]
	}
	logging.Infof(budget.ctx, "mcp.discovery", "[MCP:discovery] Tentando discovery OAuth para %s", resourceForLog)

	var resourceName string
	var resourceScopes []string
	var hints []DiscoveryResponseHint
	resourceBaseURL := serverURL

	prm, prmHints, err := fetchProtectedResourceMetadataDetailedWithBudget(budget, serverURL)
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
		logging.Infof(budget.ctx, "mcp.discovery", "[MCP:discovery] Protected Resource Metadata encontrado")
	} else {
		logging.Infof(budget.ctx, "mcp.discovery", "[MCP:discovery] Protected Resource Metadata não encontrado para o recurso")
	}

	if len(authServerBases) == 0 {
		authServerBases = buildResourceBases(resourceBaseURL)
		if len(authServerBases) == 0 && resourceBaseURL != serverURL {
			authServerBases = buildResourceBases(serverURL)
		}
	}

	asm, metadataType, asmHints, err := fetchAuthServerMetadataFromBasesWithBudget(budget, authServerBases)
	hints = appendHints(hints, asmHints...)
	if err != nil || asm == nil {
		logging.Warnf(budget.ctx, "mcp.discovery", "[MCP:discovery] Authorization Server Metadata não encontrado")
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

	logging.Infof(budget.ctx, "mcp.discovery", "[MCP:discovery] Authorization Server Metadata encontrado (%s)", metadataType)

	scopes := append([]string(nil), resourceScopes...)
	for _, scope := range asm.ScopesSupported {
		if !slices.Contains(scopes, scope) {
			scopes = append(scopes, scope)
		}
	}

	supportsPKCE := slices.Contains(asm.CodeChallengeMethodsSupported, "S256")

	authType := AuthOAuth2PKCE
	if asm.AuthorizationEndpoint == "" ||
		(!supportsPKCE && slices.Contains(asm.GrantTypesSupported, "client_credentials")) {
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

// fetchProtectedResourceMetadataDetailed tenta descobrir metadata do recurso
// protegido conforme RFC 9728. Para recurso, ancestrais e origin, tenta
// primeiro a localização normativa /.well-known/oauth-protected-resource{path}
// e depois o fallback relativo {base}/.well-known/oauth-protected-resource,
// sempre com ordem determinística e deduplicação.
func fetchProtectedResourceMetadataDetailed(mcpURL string) (*protectedResourceMetadata, []DiscoveryResponseHint, error) {
	budget := newDiscoveryBudget(context.Background())
	defer budget.close()
	return fetchProtectedResourceMetadataDetailedWithBudget(budget, mcpURL)
}

func fetchProtectedResourceMetadataDetailedWithBudget(
	budget *discoveryBudget,
	mcpURL string,
) (*protectedResourceMetadata, []DiscoveryResponseHint, error) {
	candidates := buildPRMCandidates(mcpURL)
	var hints []DiscoveryResponseHint
	for _, candidateURL := range candidates {
		if err := budget.beginAttempt(); err != nil {
			hints = appendDiscoveryBudgetHint(hints)
			return nil, hints, err
		}
		logging.Debugf(budget.ctx, "mcp.discovery", "[MCP:discovery] PRM: tentando %s", candidateURL)
		var result protectedResourceMetadata
		attempt, err := fetchJSONContext(budget.ctx, candidateURL, &result)
		if attempt.hint != nil {
			hints = appendHints(hints, *attempt.hint)
		}
		// RFC 9728 §2 exige o identificador do recurso. Outros membros isolados
		// são hints insuficientes e não transformam a resposta em PRM válido.
		if err == nil && result.Resource != "" {
			logging.Infof(budget.ctx, "mcp.discovery", "[MCP:discovery] PRM: encontrado em %s", candidateURL)
			return &result, hints, nil
		}
		if budgetErr := budget.finishAttempt(err); budgetErr != nil {
			hints = appendDiscoveryBudgetHint(hints)
			return nil, hints, budgetErr
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

// fetchAuthServerMetadataFromBasesWithBudget tenta descobrir metadata do
// authorization server (RFC 8414) ou do provider OIDC. Para cada base, tenta
// primeiro RFC 8414 e OIDC Discovery; em seguida, localizações legadas
// compatíveis. As bases são processadas em ordem e deduplicadas globalmente.
func fetchAuthServerMetadataFromBasesWithBudget(
	budget *discoveryBudget,
	bases []string,
) (*authServerMetadata, string, []DiscoveryResponseHint, error) {
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
		if err := budget.beginAttempt(); err != nil {
			hints = appendDiscoveryBudgetHint(hints)
			return nil, "", hints, err
		}
		logging.Debugf(budget.ctx, "mcp.discovery", "[MCP:discovery] ASM: tentando %s", candidateURL)
		var result authServerMetadata
		attempt, err := fetchJSONContext(budget.ctx, candidateURL, &result)
		if attempt.hint != nil {
			hints = appendHints(hints, *attempt.hint)
		}
		if err == nil && validAuthServerMetadata(&result) {
			logging.Infof(budget.ctx, "mcp.discovery", "[MCP:discovery] ASM: encontrado em %s", candidateURL)
			return &result, candidateTypes[candidateURL], hints, nil
		}
		if budgetErr := budget.finishAttempt(err); budgetErr != nil {
			hints = appendDiscoveryBudgetHint(hints)
			return nil, "", hints, budgetErr
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
	// grant_types_supported é opcional na RFC 8414. Um token endpoint sem
	// authorization endpoint ainda é utilizável pelo fluxo client credentials.
	return metadata != nil && metadata.TokenEndpoint != ""
}

type fetchAttempt struct {
	hint *DiscoveryResponseHint
}

func fetchJSON(rawURL string, target any) (fetchAttempt, error) {
	return fetchJSONContext(context.Background(), rawURL, target)
}

func fetchJSONContext(ctx context.Context, rawURL string, target any) (fetchAttempt, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return fetchAttempt{}, err
	}
	req.Header.Set("Accept", "application/json")

	redirects := make([]DiscoveryResponseHint, 0, 2)
	client := *discoveryHTTPClient
	originalRedirectPolicy := client.CheckRedirect
	client.CheckRedirect = func(next *http.Request, via []*http.Request) error {
		redirects = append(redirects, DiscoveryResponseHint{
			StatusCode:     discoveryRedirectStatus(next, via),
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
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		attempt := fetchAttempt{}
		if len(redirects) > 0 {
			redirectHint := redirects[len(redirects)-1]
			attempt.hint = &redirectHint
		}
		return attempt, err
	}
	defer func() { _ = resp.Body.Close() }()

	bodyLimit := discoveryMetadataBodyLimit
	if resp.StatusCode != http.StatusOK {
		bodyLimit = discoveryErrorBodyLimit
	}
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, int64(bodyLimit+1)))
	if readErr != nil {
		return fetchAttempt{}, readErr
	}
	truncated := len(body) > bodyLimit
	if truncated {
		body = body[:bodyLimit]
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
		return fetchAttempt{hint: hint}, fmt.Errorf("metadata excede %d bytes", discoveryMetadataBodyLimit)
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

func discoveryRedirectStatus(next *http.Request, via []*http.Request) int {
	if next != nil && next.Response != nil {
		return next.Response.StatusCode
	}
	if len(via) > 0 {
		previous := via[len(via)-1]
		if previous != nil && previous.Response != nil {
			return previous.Response.StatusCode
		}
	}
	return 0
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

var sensitiveHintPattern = regexp.MustCompile(`(?i)((?:access_|refresh_|id_)?token|secret|password|cookie|credential)\s*=\s*("[^"]*"|[^,\s]+)`)

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
	parsed.ForceQuery = false
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
