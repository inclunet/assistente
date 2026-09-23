package app

import (
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
	claims, err := json.Marshal(map[string]any{
		"iss": cfg.External.Issuer, "aud": cfg.External.Audience, "sub": a.currentUserID,
		"iat": now.Unix(), "exp": now.Add(time.Minute).Unix(), "scope": "identity:admin",
	})
	if err != nil {
		t.Fatal(err)
	}
	payload := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(claims)
	token := payload + "." + base64.RawURLEncoding.EncodeToString(ed25519.Sign(ed25519.PrivateKey(key), []byte(payload)))
	request(handler, token, http.StatusCreated)
	var rows []database.ExternalIdentityAdminAudit
	if err := db.Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].ActorUserID != a.currentUserID || rows[0].UserID != a.currentUserID || rows[0].Action != "bootstrap" {
		t.Fatalf("wrong audit from production assembly: %+v", rows)
	}
	request(handler, token, http.StatusConflict)
}
