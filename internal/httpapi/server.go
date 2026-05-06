package httpapi

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"

	"assistente/internal/auth"
	"assistente/internal/credentials"
)

type Server struct {
	vault    *auth.VaultService
	ids      *auth.IdentityService
	session  *auth.SessionService
	mode     string
	external *auth.ExternalAuthenticator
	mux      *http.ServeMux
}

type Config struct {
	Vault    *auth.VaultService
	IDs      *auth.IdentityService
	Session  *auth.SessionService
	Mode     string
	External *auth.ExternalAuthenticator
}

func New(cfg Config) *Server {
	s := &Server{
		vault:    cfg.Vault,
		ids:      cfg.IDs,
		session:  cfg.Session,
		mode:     cfg.Mode,
		external: cfg.External,
		mux:      http.NewServeMux(),
	}
	if s.mode == "" {
		s.mode = "local"
	}
	s.routes()
	return s
}

func (s *Server) Handler() http.Handler {
	return s.mux
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /vault/status", s.handleVaultStatus)
	s.mux.HandleFunc("POST /vault/setup", s.handleVaultSetup)
	s.mux.HandleFunc("POST /vault/unlock", s.handleVaultUnlock)
	s.mux.HandleFunc("POST /auth/login", s.handleLogin)
	s.mux.HandleFunc("POST /auth/refresh", s.handleRefresh)
	s.mux.HandleFunc("POST /auth/logout", s.handleLogout)
	s.mux.HandleFunc("GET /auth/me", s.handleMe)
	s.mux.HandleFunc("GET /.well-known/jwks.json", s.handleJWKS)
}

func (s *Server) handleVaultStatus(w http.ResponseWriter, r *http.Request) {
	status, err := s.vault.Status(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
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
		writeError(w, http.StatusUnauthorized, err)
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
		writeError(w, status, err)
		return
	}
	pair, err := s.session.IssueSession(r.Context(), user, req.ClientLabel)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, pair)
}

func (s *Server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	if s.mode == "external" {
		writeError(w, http.StatusNotFound, errors.New("refresh local indisponível em auth.mode=external"))
		return
	}
	var req struct {
		RefreshToken string `json:"refreshToken"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	pair, err := s.session.Refresh(r.Context(), req.RefreshToken)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
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
	if err := s.session.Logout(r.Context(), req.RefreshToken); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
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

func (s *Server) handleJWKS(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.session.JWKSet())
}

type principal struct {
	UserID    string
	SessionID string
	Role      string
}

func (s *Server) requireAccess(w http.ResponseWriter, r *http.Request) (*principal, bool) {
	token := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	if token == "" {
		writeError(w, http.StatusUnauthorized, errors.New("access token obrigatório"))
		return nil, false
	}
	if s.mode == "external" {
		if s.external == nil {
			writeError(w, http.StatusUnauthorized, errors.New("auth.mode=external sem validador configurado"))
			return nil, false
		}
		claims, err := s.external.Validate(r.Context(), token)
		if err != nil {
			writeError(w, http.StatusUnauthorized, err)
			return nil, false
		}
		role := "user"
		if len(claims.Roles) > 0 {
			role = claims.Roles[0]
		}
		return &principal{UserID: claims.Subject, Role: role}, true
	}
	claims, err := s.session.VerifyAccessToken(token)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
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
