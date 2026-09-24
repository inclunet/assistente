package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"assistente/internal/auth"
	"assistente/internal/database"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestExternalIdentityRoutesBootstrapAndCreateWithSignedTokens(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "identities.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.Exec("PRAGMA foreign_keys = ON").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&database.User{}, &auth.ExternalIdentityMapping{}); err != nil {
		t.Fatal(err)
	}
	if err := database.MigrateExternalIdentityAdminAudit(db); err != nil {
		t.Fatal(err)
	}
	ids := auth.NewIdentityService(db)
	admin, err := ids.CreateLocalUser(context.Background(), auth.CreateUserParams{Username: "operator", Password: "test-password", Admin: true})
	if err != nil {
		t.Fatal(err)
	}
	target, err := ids.CreateLocalUser(context.Background(), auth.CreateUserParams{Username: "target", Password: "test-password"})
	if err != nil {
		t.Fatal(err)
	}
	signer, err := auth.NewTokenSigner()
	if err != nil {
		t.Fatal(err)
	}
	jwks := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _ = json.NewEncoder(w).Encode(signer.JWKSet()) }))
	t.Cleanup(jwks.Close)
	const issuer = "https://idp.example"
	const scope = "assistente:identity:admin"
	external := auth.NewExternalAuthenticator(auth.ExternalAuthConfig{Issuer: issuer, Audience: "assistente", JWKSURL: jwks.URL, AllowedAlgorithms: []string{"EdDSA"}})
	repo := auth.NewExternalIdentityRepository(db)
	service, err := auth.NewExternalIdentityAdminService(external, repo, auth.ExternalIdentityAdminConfig{Issuer: issuer, AdminScopes: []string{scope}})
	if err != nil {
		t.Fatal(err)
	}
	server := New(Config{Mode: "external", External: external, ExternalIdentities: repo, ExternalIdentityAdmin: service, AuthBurst: 100})
	sign := func(subject, tokenIssuer, tokenScope string) string {
		t.Helper()
		now := time.Now()
		token, err := signExternalToken(t, signer, map[string]any{"iss": tokenIssuer, "aud": "assistente", "sub": subject, "iat": now.Unix(), "exp": now.Add(time.Minute).Unix(), "scope": tokenScope, "roles": []string{"admin"}})
		if err != nil {
			t.Fatal(err)
		}
		return token
	}
	request := func(path, token, body string, want int) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		server.Handler().ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s status=%d want=%d body=%s", path, w.Code, want, w.Body.String())
		}
		return w
	}
	const bootstrap = "/auth/external/identities/bootstrap"
	const create = "/auth/external/identities"
	adminToken := sign(admin.ID, issuer, scope)
	targetBody, _ := json.Marshal(map[string]string{"issuer": issuer, "subject": "remote-target", "userId": target.ID})
	request(create, adminToken, string(targetBody), http.StatusForbidden)
	request(bootstrap, adminToken, `{"userId":"injected"}`, http.StatusBadRequest)
	request(bootstrap, adminToken, `{} {}`, http.StatusBadRequest)
	request(bootstrap, adminToken, `null`, http.StatusBadRequest)
	request(bootstrap, adminToken, `[]`, http.StatusBadRequest)
	request(bootstrap, adminToken, `{"padding":"`+strings.Repeat("x", 9000)+`"}`, http.StatusBadRequest)
	request(bootstrap, "invalid-token", `{}`, http.StatusForbidden)
	request(bootstrap, sign(admin.ID, "https://wrong.example", scope), `{}`, http.StatusForbidden)
	request(bootstrap, sign(admin.ID, issuer, "read"), `{}`, http.StatusForbidden)
	request(bootstrap, sign("not-a-local-id", issuer, scope), `{}`, http.StatusForbidden)
	request(bootstrap, adminToken, `{}`, http.StatusCreated)
	request(bootstrap, adminToken, `{}`, http.StatusConflict)
	result := request(create, adminToken, string(targetBody), http.StatusCreated)
	if result.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("identity response is cacheable")
	}
	var mapping auth.ExternalIdentityMapping
	if err := json.Unmarshal(result.Body.Bytes(), &mapping); err != nil {
		t.Fatal(err)
	}
	if mapping.UserID != target.ID || mapping.Subject != "remote-target" || mapping.Issuer != issuer {
		t.Fatalf("wrong target mapping: %+v", mapping)
	}
	request(create, adminToken, string(targetBody), http.StatusConflict)
	var auditCount int64
	if err := db.Model(&database.ExternalIdentityAdminAudit{}).Count(&auditCount).Error; err != nil {
		t.Fatal(err)
	}
	if auditCount != 2 {
		t.Fatalf("audit rows=%d, want bootstrap + create only", auditCount)
	}
	// Cadastro administrativo mantém o executor de comandos fechado sem o
	// readiness do host, enquanto o middleware HTTP usa o vínculo já cadastrado.
	authenticator := auth.NewExternalCommandAuthenticator(external, repo)
	if _, err := authenticator.Authenticate(context.Background(), sign("remote-target", issuer, "read")); !errors.Is(err, auth.ErrExternalIdentityNotReady) {
		t.Fatalf("registration published readiness: %v", err)
	}
	me := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	me.Header.Set("Authorization", "Bearer "+sign("remote-target", issuer, "read"))
	meResponse := httptest.NewRecorder()
	server.Handler().ServeHTTP(meResponse, me)
	var principal map[string]string
	if err := json.Unmarshal(meResponse.Body.Bytes(), &principal); err != nil {
		t.Fatal(err)
	}
	if meResponse.Code != http.StatusOK || principal["userId"] != target.ID || principal["role"] != "admin" {
		t.Fatalf("mapped middleware identity = %s, want local user %s and external role admin", meResponse.Body.String(), target.ID)
	}
	if err := repo.Revoke(context.Background(), issuer, admin.ID); err != nil {
		t.Fatal(err)
	}
	request(create, adminToken, strings.Replace(string(targetBody), "remote-target", "other-target", 1), http.StatusForbidden)
	if _, err := repo.Resolve(context.Background(), issuer, "other-target"); !errors.Is(err, auth.ErrExternalIdentityNotMapped) {
		t.Fatalf("revoked admin created mapping: %v", err)
	}
}

func TestExternalIdentityRoutesRequireConfiguredExternalModeAndBearer(t *testing.T) {
	for _, tc := range []struct {
		name string
		cfg  Config
		want int
	}{
		{"local", Config{}, http.StatusNotFound},
		{"disabled", Config{Mode: "external"}, http.StatusServiceUnavailable},
		{"noBearer", Config{Mode: "external", ExternalIdentityAdmin: &auth.ExternalIdentityAdminService{}}, http.StatusUnauthorized},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, path := range []string{"/auth/external/identities/bootstrap", "/auth/external/identities"} {
				r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{}`))
				r.Header.Set("Authorization", "Basic invalid")
				w := httptest.NewRecorder()
				New(tc.cfg).Handler().ServeHTTP(w, r)
				if w.Code != tc.want {
					t.Fatalf("%s status=%d want=%d", path, w.Code, tc.want)
				}
			}
		})
	}
}
