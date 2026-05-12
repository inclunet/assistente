package httpapi

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"assistente/internal/auth"
	"assistente/internal/database"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupHTTPAPITestServer(t *testing.T) *Server {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&database.User{}, &database.Session{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	ids := auth.NewIdentityService(db)
	user, err := ids.CreateLocalUser(context.Background(), auth.CreateUserParams{
		Username: "admin",
		Password: "secret-password",
		Admin:    true,
	})
	if err != nil || user == nil {
		t.Fatalf("create user: %v", err)
	}
	sessions, err := auth.NewSessionService(db, auth.SessionConfig{
		Issuer:   "test",
		Audience: "client",
	})
	if err != nil {
		t.Fatalf("new session service: %v", err)
	}
	t.Cleanup(func() {
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			_ = sqlDB.Close()
		}
	})

	return New(Config{IDs: ids, Session: sessions})
}

func TestAuthLoginRefreshMeAndLogout(t *testing.T) {
	server := setupHTTPAPITestServer(t)

	login := requestJSON(t, server, http.MethodPost, "/auth/login", map[string]string{
		"username": "admin",
		"password": "secret-password",
	})
	if login.Code != http.StatusOK {
		t.Fatalf("login status = %d body=%s", login.Code, login.Body.String())
	}
	var pair struct {
		AccessToken  string `json:"accessToken"`
		RefreshToken string `json:"refreshToken"`
	}
	if err := json.Unmarshal(login.Body.Bytes(), &pair); err != nil {
		t.Fatalf("decode login: %v", err)
	}
	if pair.AccessToken == "" || pair.RefreshToken == "" {
		t.Fatalf("missing tokens: %+v", pair)
	}

	meReq := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	meReq.Header.Set("Authorization", "Bearer "+pair.AccessToken)
	me := httptest.NewRecorder()
	server.Handler().ServeHTTP(me, meReq)
	if me.Code != http.StatusOK {
		t.Fatalf("me status = %d body=%s", me.Code, me.Body.String())
	}

	refresh := requestJSON(t, server, http.MethodPost, "/auth/refresh", map[string]string{
		"refreshToken": pair.RefreshToken,
	})
	if refresh.Code != http.StatusOK {
		t.Fatalf("refresh status = %d body=%s", refresh.Code, refresh.Body.String())
	}
	var refreshed struct {
		RefreshToken string `json:"refreshToken"`
	}
	if err := json.Unmarshal(refresh.Body.Bytes(), &refreshed); err != nil {
		t.Fatalf("decode refresh: %v", err)
	}
	if refreshed.RefreshToken == pair.RefreshToken {
		t.Fatal("refresh token was not rotated")
	}

	logout := requestJSON(t, server, http.MethodPost, "/auth/logout", map[string]string{
		"refreshToken": refreshed.RefreshToken,
	})
	if logout.Code != http.StatusNoContent {
		t.Fatalf("logout status = %d body=%s", logout.Code, logout.Body.String())
	}
}

func TestJWKSUsesDynamicSessionProvider(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() {
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			_ = sqlDB.Close()
		}
	})

	var sessions *auth.SessionService
	server := New(Config{Sessions: func() *auth.SessionService { return sessions }})

	before := httptest.NewRecorder()
	server.Handler().ServeHTTP(before, httptest.NewRequest(http.MethodGet, "/.well-known/jwks.json", nil))
	if before.Code != http.StatusServiceUnavailable {
		t.Fatalf("JWKS before session status = %d, want %d", before.Code, http.StatusServiceUnavailable)
	}

	sessions, err = auth.NewSessionService(db, auth.SessionConfig{
		Issuer:   "test",
		Audience: "client",
	})
	if err != nil {
		t.Fatalf("new session service: %v", err)
	}

	after := httptest.NewRecorder()
	server.Handler().ServeHTTP(after, httptest.NewRequest(http.MethodGet, "/.well-known/jwks.json", nil))
	if after.Code != http.StatusOK {
		t.Fatalf("JWKS after session status = %d body=%s", after.Code, after.Body.String())
	}
}

func TestValidateBindSecurity(t *testing.T) {
	if err := ValidateBindSecurity("127.0.0.1:8080", false, false); err != nil {
		t.Fatalf("localhost should allow HTTP: %v", err)
	}
	if err := ValidateBindSecurity("0.0.0.0:8080", false, false); err == nil {
		t.Fatal("non-localhost bind without TLS should fail")
	}
	if err := ValidateBindSecurity("0.0.0.0:8080", true, false); err != nil {
		t.Fatalf("TLS should allow non-localhost bind: %v", err)
	}
}

func TestExternalModeValidatesBearerJWT(t *testing.T) {
	signer, err := auth.NewTokenSigner()
	if err != nil {
		t.Fatalf("new signer: %v", err)
	}
	jwksServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(signer.JWKSet())
	}))
	defer jwksServer.Close()

	external := auth.NewExternalAuthenticator(auth.ExternalAuthConfig{
		Issuer:            "https://idp.example.com",
		Audience:          "assistente",
		JWKSURL:           jwksServer.URL,
		AllowedAlgorithms: []string{"EdDSA"},
	})
	server := New(Config{Mode: "external", External: external})
	now := time.Now()
	token, err := signer.SignAccessToken(auth.AccessClaims{
		Issuer:    "https://idp.example.com",
		Audience:  "assistente",
		Subject:   "external-user",
		IssuedAt:  now.Unix(),
		ExpiresAt: now.Add(time.Minute).Unix(),
		Role:      "user",
	})
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("me status = %d body=%s", rec.Code, rec.Body.String())
	}

	login := requestJSON(t, server, http.MethodPost, "/auth/login", map[string]string{"username": "x"})
	if login.Code != http.StatusNotFound {
		t.Fatalf("external login status = %d", login.Code)
	}
}

func TestExternalModeUsesConfiguredRoleClaimAndScopes(t *testing.T) {
	signer, err := auth.NewTokenSigner()
	if err != nil {
		t.Fatalf("new signer: %v", err)
	}
	jwksServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(signer.JWKSet())
	}))
	defer jwksServer.Close()

	external := auth.NewExternalAuthenticator(auth.ExternalAuthConfig{
		Issuer:            "https://idp.example.com",
		Audience:          "assistente",
		JWKSURL:           jwksServer.URL,
		AllowedAlgorithms: []string{"EdDSA"},
		RequiredScopes:    []string{"assistente:read"},
		RoleClaim:         "groups",
	})
	server := New(Config{Mode: "external", External: external})

	now := time.Now()
	token, err := signExternalToken(t, signer, map[string]any{
		"iss":    "https://idp.example.com",
		"aud":    "assistente",
		"sub":    "external-admin",
		"iat":    now.Unix(),
		"exp":    now.Add(time.Minute).Unix(),
		"scope":  "profile assistente:read",
		"groups": []string{"admin", "operator"},
	})
	if err != nil {
		t.Fatalf("sign external token: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("me status = %d body=%s", rec.Code, rec.Body.String())
	}
	var me struct {
		UserID string `json:"userId"`
		Role   string `json:"role"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &me); err != nil {
		t.Fatalf("decode me: %v", err)
	}
	if me.UserID != "external-admin" || me.Role != "admin" {
		t.Fatalf("me = %+v, want external-admin/admin", me)
	}

	noScope, err := signExternalToken(t, signer, map[string]any{
		"iss":    "https://idp.example.com",
		"aud":    "assistente",
		"sub":    "external-admin",
		"iat":    now.Unix(),
		"exp":    now.Add(time.Minute).Unix(),
		"scope":  "profile",
		"groups": []string{"admin"},
	})
	if err != nil {
		t.Fatalf("sign no-scope token: %v", err)
	}
	req = httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+noScope)
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("missing scope status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func signExternalToken(t *testing.T, signer *auth.TokenSigner, claims map[string]any) (string, error) {
	t.Helper()
	privateEncoded, err := signer.ExportPrivateKey()
	if err != nil {
		return "", err
	}
	privateRaw, err := base64.RawURLEncoding.DecodeString(privateEncoded)
	if err != nil {
		return "", err
	}
	jwks := signer.JWKSet()
	header := map[string]string{"alg": "EdDSA", "kid": jwks.Keys[0].KeyID, "typ": "JWT"}
	headerJSON, err := json.Marshal(header)
	if err != nil {
		return "", err
	}
	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	signingInput := base64.RawURLEncoding.EncodeToString(headerJSON) + "." + base64.RawURLEncoding.EncodeToString(claimsJSON)
	signature := ed25519.Sign(ed25519.PrivateKey(privateRaw), []byte(signingInput))
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

func requestJSON(t *testing.T, server *Server, method, path string, payload any) *httptest.ResponseRecorder {
	t.Helper()

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	return rec
}
