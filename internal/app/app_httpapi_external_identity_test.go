package app

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"assistente/internal/auth"
	"assistente/internal/config"
	"assistente/internal/database"
)

func TestHTTPAPIExternalIdentityEnrollmentUsesConfiguredProductionAssembly(t *testing.T) {
	a := readyCommandProduct(t)
	db := database.DB()
	if err := db.AutoMigrate(&auth.ExternalIdentityMapping{}); err != nil {
		t.Fatal(err)
	}
	if err := database.MigrateExternalIdentityAdminAudit(db); err != nil {
		t.Fatal(err)
	}
	signer, err := auth.NewTokenSigner()
	if err != nil {
		t.Fatal(err)
	}
	jwks := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(signer.JWKSet())
	}))
	t.Cleanup(jwks.Close)
	cfg := config.DefaultAuthConfig()
	cfg.Mode = "external"
	cfg.External.Issuer = "https://idp.example"
	cfg.External.Audience = "assistente"
	cfg.External.JWKSURL = jwks.URL
	request := func(handler http.Handler, token string, want int) {
		t.Helper()
		r := httptest.NewRequest(http.MethodPost, "/auth/external/identities/bootstrap", strings.NewReader(`{}`))
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("bootstrap status=%d want=%d body=%s", w.Code, want, w.Body.String())
		}
	}
	handler, err := a.newHTTPAPIHandler(cfg)
	if err != nil {
		t.Fatal(err)
	}
	request(handler, "unused", http.StatusServiceUnavailable)
	cfg.External.IdentityAdminScopes = []string{"identity:admin"}
	handler, err = a.newHTTPAPIHandler(cfg)
	if err != nil {
		t.Fatal(err)
	}
	encodedKey, err := signer.ExportPrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	key, err := base64.RawURLEncoding.DecodeString(encodedKey)
	if err != nil {
		t.Fatal(err)
	}
	header, err := json.Marshal(map[string]string{"alg": "EdDSA", "kid": signer.JWKSet().Keys[0].KeyID})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	sign := func(subject string) string {
		t.Helper()
		claims, err := json.Marshal(map[string]any{
			"iss": cfg.External.Issuer, "aud": cfg.External.Audience, "sub": subject,
			"iat": now.Unix(), "exp": now.Add(time.Minute).Unix(), "scope": "identity:admin",
		})
		if err != nil {
			t.Fatal(err)
		}
		payload := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(claims)
		return payload + "." + base64.RawURLEncoding.EncodeToString(ed25519.Sign(ed25519.PrivateKey(key), []byte(payload)))
	}
	token := sign(a.currentUserID)
	me := func(token string, want int) {
		t.Helper()
		r := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("me status=%d want=%d body=%s", w.Code, want, w.Body.String())
		}
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("resposta de identidade pode ser cacheada")
		}
		if want == http.StatusOK {
			var result struct {
				UserID    string `json:"userId"`
				SessionID string `json:"sessionId"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if result.UserID != a.currentUserID || result.SessionID != "" {
				t.Fatalf("identidade incorreta: %+v", result)
			}
		}
	}
	me(token, http.StatusServiceUnavailable)
	request(handler, token, http.StatusCreated)
	me(token, http.StatusOK)
	remoteToken := sign("remote-operator")
	me(remoteToken, http.StatusUnauthorized)
	create := httptest.NewRequest(http.MethodPost, "/auth/external/identities", strings.NewReader(`{"issuer":"`+cfg.External.Issuer+`","subject":"remote-operator","userId":"`+a.currentUserID+`"}`))
	create.Header.Set("Authorization", "Bearer "+token)
	created := httptest.NewRecorder()
	handler.ServeHTTP(created, create)
	if created.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", created.Code, created.Body.String())
	}
	me(remoteToken, http.StatusOK)
	if err := auth.NewExternalIdentityRepository(db).Revoke(context.Background(), cfg.External.Issuer, "remote-operator"); err != nil {
		t.Fatal(err)
	}
	me(remoteToken, http.StatusUnauthorized)
	me(token, http.StatusOK)
	var rows []database.ExternalIdentityAdminAudit
	if err := db.Where("action = ?", "bootstrap").Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].ActorUserID != a.currentUserID || rows[0].UserID != a.currentUserID || rows[0].Action != "bootstrap" {
		t.Fatalf("wrong audit from production assembly: %+v", rows)
	}
	request(handler, token, http.StatusConflict)
	// Desabilitar novas operações administrativas não desfaz a migração feita.
	cfg.External.IdentityAdminScopes = nil
	handler, err = a.newHTTPAPIHandler(cfg)
	if err != nil {
		t.Fatal(err)
	}
	request(handler, token, http.StatusServiceUnavailable)
	me(token, http.StatusOK)
}
