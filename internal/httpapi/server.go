package httpapi

import (
	"assistente/internal/logging"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync/atomic"

	"assistente/internal/auth"
	"assistente/internal/commandcatalog"
	"assistente/internal/commandexecution"
	"assistente/internal/credentials"
)

type Server struct {
	vault                 *auth.VaultService
	ids                   *auth.IdentityService
	session               *auth.SessionService
	sessions              func() *auth.SessionService
	mode                  string
	external              *auth.ExternalAuthenticator
	externalIdentityAdmin *auth.ExternalIdentityAdminService
	externalIdentities    *auth.ExternalIdentityRepository
	externalCommands      map[string]*commandexecution.ExternalService
	mux                   *http.ServeMux

	// jwksCache (B20 do review) absorve picos de tráfego em
	// /.well-known/jwks.json sem segurar lock no signer a cada request.
	// Atualizado via Compare-and-Store quando cacheTTL expira.
	jwksCache atomic.Pointer[jwksCacheEntry]

	// authLimiter throttle agressivo em /auth/login e /auth/refresh para
	// reduzir brute-force/credential-stuffing antes de chegar nas
	// validações Argon2 (caras de propósito). M21 do review.
	authLimiter *rateLimiter
	// jwksLimiter mais permissivo em /.well-known/jwks.json — endpoint
	// público e cacheado, mas vale ter um teto para não esgotar
	// goroutines/sockets em DoS rude.
	jwksLimiter *rateLimiter
}

type Config struct {
	Vault                 *auth.VaultService
	IDs                   *auth.IdentityService
	Session               *auth.SessionService
	Sessions              func() *auth.SessionService
	Mode                  string
	External              *auth.ExternalAuthenticator
	ExternalIdentityAdmin *auth.ExternalIdentityAdminService
	ExternalIdentities    *auth.ExternalIdentityRepository
	ExternalCommands      map[string]*commandexecution.ExternalService
	// AuthRate / AuthBurst e JWKSRate / JWKSBurst permitem ajustar os
	// limites por deploy. Defaults conservadores aplicados quando não
	// configurados — evitam que um teste/integração local "sem cargo"
	// fique mais frágil que o setup atual.
	AuthRate  float64
	AuthBurst float64
	JWKSRate  float64
	JWKSBurst float64
}

func New(cfg Config) *Server {
	externalCommands, validExternalCommands := copyAndValidateExternalCommands(cfg.ExternalCommands)
	if !validExternalCommands {
		logging.Errorf(context.Background(), "httpapi.server", "[httpapi] invalid_external_command_services")
		externalCommands = nil
	}
	authRate := cfg.AuthRate
	if authRate <= 0 {
		authRate = 5
	}
	authBurst := cfg.AuthBurst
	if authBurst <= 0 {
		authBurst = 10
	}
	jwksRate := cfg.JWKSRate
	if jwksRate <= 0 {
		jwksRate = 50
	}
	jwksBurst := cfg.JWKSBurst
	if jwksBurst <= 0 {
		jwksBurst = 100
	}
	s := &Server{
		vault:                 cfg.Vault,
		ids:                   cfg.IDs,
		session:               cfg.Session,
		sessions:              cfg.Sessions,
		mode:                  cfg.Mode,
		external:              cfg.External,
		externalIdentityAdmin: cfg.ExternalIdentityAdmin,
		externalIdentities:    cfg.ExternalIdentities,
		externalCommands:      externalCommands,
		mux:                   http.NewServeMux(),
		authLimiter:           newRateLimiter(authRate, authBurst),
		jwksLimiter:           newRateLimiter(jwksRate, jwksBurst),
	}
	if s.mode == "" {
		s.mode = "local"
	}
	s.routes()
	return s
}

func copyAndValidateExternalCommands(input map[string]*commandexecution.ExternalService) (map[string]*commandexecution.ExternalService, bool) {
	if len(input) == 0 {
		return nil, true
	}
	expected := map[string]commandcatalog.Source{
		"palette": commandcatalog.Palette,
		"ui":      commandcatalog.UI,
		"chat":    commandcatalog.Chat,
	}
	copyOfServices := make(map[string]*commandexecution.ExternalService, len(input))
	var sharedSecurity *commandexecution.ExternalService
	for key, service := range input {
		wantSource, ok := expected[key]
		if !ok || service == nil || service.Source() != wantSource {
			return nil, false
		}
		if sharedSecurity != nil && !sharedSecurity.SharesSecurityWith(service) {
			return nil, false
		}
		if sharedSecurity == nil {
			sharedSecurity = service
		}
		copyOfServices[key] = service
	}
	return copyOfServices, true
}

func (s *Server) sessionService() *auth.SessionService {
	if s.sessions != nil {
		return s.sessions()
	}
	return s.session
}

func (s *Server) Handler() http.Handler {
	return s.mux
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /vault/status", s.handleVaultStatus)
	s.mux.HandleFunc("POST /vault/setup", s.handleVaultSetup)
	s.mux.HandleFunc("POST /vault/unlock", s.handleVaultUnlock)
	s.mux.HandleFunc("POST /auth/login", s.rateLimit(s.authLimiter, "auth.login", s.handleLogin))
	s.mux.HandleFunc("POST /auth/refresh", s.rateLimit(s.authLimiter, "auth.refresh", s.handleRefresh))
	s.mux.HandleFunc("POST /auth/logout", s.handleLogout)
	s.mux.HandleFunc("GET /auth/me", s.handleMe)
	s.mux.HandleFunc("POST /auth/external/identities/bootstrap", s.rateLimit(s.authLimiter, "auth.external.bootstrap", s.handleExternalIdentityBootstrap))
	s.mux.HandleFunc("POST /auth/external/identities", s.rateLimit(s.authLimiter, "auth.external.identity.create", s.handleExternalIdentityCreate))
	s.mux.Handle("POST /auth/external/identities/revoke", s.noStoreRateLimit(s.authLimiter, "auth.external.identity.revoke", s.handleExternalIdentityRevoke))
	s.mux.Handle("POST /commands/{source}/execute", s.noStoreRateLimit(s.authLimiter, "commands.execute", s.handleExternalCommandExecute))
	s.mux.Handle("GET /commands/{source}/invocations/{id}", s.noStoreRateLimit(s.authLimiter, "commands.lookup", s.handleExternalCommandLookup))
	s.mux.HandleFunc("GET /.well-known/jwks.json", s.rateLimit(s.jwksLimiter, "auth.jwks", s.handleJWKS))
}

func (s *Server) handleVaultStatus(w http.ResponseWriter, r *http.Request) {
	status, err := s.vault.Status(r.Context())
	if err != nil {
		s.writeInternalErr(r.Context(), w, "vault.status", err)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) handleVaultSetup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		MasterPassword string `json:"masterPassword"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	recoveryKey, err := s.vault.Setup(r.Context(), req.MasterPassword)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"recoveryKey": recoveryKey})
}

func (s *Server) handleVaultUnlock(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Kind   string `json:"kind"`
		Secret string `json:"secret"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := s.vault.Unlock(r.Context(), req.Kind, req.Secret); err != nil {
		// Mensagem genérica para que kind/secret específicos não vazem
		// pelo erro do unlock. O log mantém o detalhe.
		s.writeAuthErr(r.Context(), w, "vault.unlock", http.StatusUnauthorized, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"unlocked": true})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if s.mode == "external" {
		writeError(w, http.StatusNotFound, errors.New("login local indisponível em auth.mode=external"))
		return
	}
	var req struct {
		Username    string `json:"username"`
		Password    string `json:"password"`
		ClientLabel string `json:"clientLabel"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	user, err := s.ids.AuthenticateLocal(r.Context(), req.Username, req.Password)
	if err != nil {
		status := http.StatusUnauthorized
		if errors.Is(err, auth.ErrInactiveUser) {
			status = http.StatusForbidden
		}
		s.writeAuthErr(r.Context(), w, "auth.login", status, err)
		return
	}
	session := s.sessionService()
	if session == nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("serviço de sessão indisponível"))
		return
	}
	pair, err := session.IssueSession(r.Context(), user, extractClientLabel(req.ClientLabel))
	if err != nil {
		s.writeInternalErr(r.Context(), w, "auth.login.issue", err)
		return
	}
	writeJSON(w, http.StatusOK, pair)
}

func (s *Server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	if s.mode == "external" {
		writeError(w, http.StatusNotFound, errors.New("refresh local indisponível em auth.mode=external"))
		return
	}
	// B19 do review: o refresh vem por JSON body porque a API HTTP do
	// assistente é hoje local-only (loopback) e o cliente Wails persiste
	// o refresh token em keyring do SO — não há cookie httpOnly neste
	// canal. Quando a API for exposta para clientes web tradicionais, o
	// suporte a cookie httpOnly + CSRF token deve substituir o body
	// (issue de roadmap pós-AEP-0052).
	var req struct {
		RefreshToken string `json:"refreshToken"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	session := s.sessionService()
	if session == nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("serviço de sessão indisponível"))
		return
	}
	pair, err := session.Refresh(r.Context(), req.RefreshToken)
	if err != nil {
		s.writeAuthErr(r.Context(), w, "auth.refresh", http.StatusUnauthorized, err)
		return
	}
	writeJSON(w, http.StatusOK, pair)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if s.mode == "external" {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	var req struct {
		RefreshToken string `json:"refreshToken"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	session := s.sessionService()
	if session == nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("serviço de sessão indisponível"))
		return
	}
	// M23 do review: logout é best-effort do ponto de vista do cliente
	// (sempre retorna 204). Quando a revogação falha, logamos com nível
	// estruturado para alertas / dashboards: o token JWT continua
	// válido até expirar, então a falha é importante para investigação.
	if err := session.Logout(r.Context(), req.RefreshToken); err != nil {
		logging.Errorf(r.Context(), "httpapi.server", "[httpapi] op=auth.logout status=revoke_failed err=%v", err)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	principal, ok := s.requireAccess(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"userId":    principal.UserID,
		"sessionId": principal.SessionID,
		"role":      principal.Role,
	})
}

// handleJWKS é endpoint público sem auth — vetor clássico de DoS por
// contention no signer mutex (B20). O cache em jwksCache + Cache-Control
// resolve as duas frentes: zero lock no signer no caminho quente e
// downstream/CDN podem reusar o resultado por jwksCacheTTL.
func (s *Server) handleJWKS(w http.ResponseWriter, r *http.Request) {
	entry, err := s.jwksFromCacheOrSigner()
	if err != nil {
		if errors.Is(err, errSessionUnavailable) {
			writeError(w, http.StatusServiceUnavailable, err)
			return
		}
		s.writeInternalErr(r.Context(), w, "auth.jwks", err)
		return
	}
	if etag := r.Header.Get("If-None-Match"); etag != "" && etag == entry.etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=300")
	w.Header().Set("ETag", entry.etag)
	_, _ = w.Write(entry.payload)
}

type principal struct {
	UserID    string
	SessionID string
	Role      string
}

func (s *Server) requireAccess(w http.ResponseWriter, r *http.Request) (*principal, bool) {
	parts := strings.Fields(r.Header.Get("Authorization"))
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "access token obrigatório"})
		return nil, false
	}
	token := parts[1]
	if s.mode == "external" {
		if s.external == nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "validador externo indisponível"})
			return nil, false
		}
		claims, err := s.external.Validate(r.Context(), token)
		if err != nil {
			s.writeAuthErr(r.Context(), w, "auth.access.external", http.StatusUnauthorized, err)
			return nil, false
		}
		// A adoção é fail-closed: schema e bootstrap do issuer precisam existir.
		// Mesmo sub igual a users.id exige vínculo explícito, sem fallback legado.
		if err := s.externalIdentities.CheckIssuerReadiness(r.Context(), claims.Issuer); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "mapeamento externo indisponível"})
			return nil, false
		}
		mapping, err := s.externalIdentities.Resolve(r.Context(), claims.Issuer, claims.Subject)
		if err != nil {
			if errors.Is(err, auth.ErrExternalIdentityNotMapped) || errors.Is(err, auth.ErrExternalIdentityRevoked) {
				s.writeAuthErr(r.Context(), w, "auth.access.external.mapping", http.StatusUnauthorized, auth.ErrUnauthenticatedExternalCommand)
			} else {
				logging.Errorf(r.Context(), "httpapi.server", "[httpapi] op=auth.access.external.mapping status=503 err=%v", err)
				writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "mapeamento externo indisponível"})
			}
			return nil, false
		}
		role := "user"
		if len(claims.Roles) > 0 {
			role = claims.Roles[0]
		}
		return &principal{UserID: mapping.UserID, Role: role}, true
	}
	session := s.sessionService()
	if session == nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("serviço de sessão indisponível"))
		return nil, false
	}
	claims, err := session.VerifyAccessToken(token)
	if err != nil {
		s.writeAuthErr(r.Context(), w, "auth.access.local", http.StatusUnauthorized, err)
		return nil, false
	}
	return &principal{UserID: claims.Subject, SessionID: claims.SessionID, Role: claims.Role}, true
}

func ValidateBindSecurity(bindAddr string, tlsEnabled bool, devInsecure bool) error {
	host, _, err := net.SplitHostPort(bindAddr)
	if err != nil {
		host = bindAddr
	}
	ip := net.ParseIP(host)
	isLocalhost := host == "localhost" || (ip != nil && ip.IsLoopback())
	if !isLocalhost && !tlsEnabled && !devInsecure {
		return errors.New("HTTPS é obrigatório quando o bind não é localhost")
	}
	return nil
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	defer func() { _ = r.Body.Close() }()
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, err error) {
	if errors.Is(err, credentials.ErrKeyWrapNotFound) {
		status = http.StatusNotFound
	}
	writeJSON(w, status, map[string]string{"error": err.Error()})
}
