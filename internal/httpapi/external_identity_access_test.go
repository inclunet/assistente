package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"assistente/internal/auth"
	"assistente/internal/database"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

const externalAccessIssuer = "https://identity.example.test"
const externalAccessAdminScope = "assistente:identity:admin"

type externalIdentityAccessFixture struct {
	t            *testing.T
	issuer       string
	db           *gorm.DB
	admin        *database.User
	target       *database.User
	signer       *auth.TokenSigner
	external     *auth.ExternalAuthenticator
	repo         *auth.ExternalIdentityRepository
	adminService *auth.ExternalIdentityAdminService
	bootstrapped bool
}

func newExternalIdentityAccessFixture(t *testing.T) *externalIdentityAccessFixture {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "external-identities.db")), &gorm.Config{})
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
	admin, err := ids.CreateLocalUser(context.Background(), auth.CreateUserParams{Username: "external-admin", Password: "fixture-password", Admin: true})
	if err != nil {
		t.Fatal(err)
	}
	target, err := ids.CreateLocalUser(context.Background(), auth.CreateUserParams{Username: "external-target", Password: "fixture-password"})
	if err != nil {
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
	external := auth.NewExternalAuthenticator(auth.ExternalAuthConfig{
		Issuer: externalAccessIssuer, Audience: "assistente", JWKSURL: jwks.URL,
		AllowedAlgorithms: []string{"EdDSA"}, RoleClaim: "groups",
	})
	repo := auth.NewExternalIdentityRepository(db)
	adminService, err := auth.NewExternalIdentityAdminService(external, repo, auth.ExternalIdentityAdminConfig{
		Issuer: externalAccessIssuer, AdminScopes: []string{externalAccessAdminScope},
	})
	if err != nil {
		t.Fatal(err)
	}
	return &externalIdentityAccessFixture{t: t, issuer: externalAccessIssuer, db: db, admin: admin, target: target, signer: signer, external: external, repo: repo, adminService: adminService}
}

func (f *externalIdentityAccessFixture) token(subject, issuer, scope string, roles []string) string {
	f.t.Helper()
	now := time.Now()
	token, err := signExternalToken(f.t, f.signer, map[string]any{
		"iss": issuer, "aud": "assistente", "sub": subject,
		"iat": now.Unix(), "exp": now.Add(time.Minute).Unix(),
		"scope": scope, "groups": roles,
	})
	if err != nil {
		f.t.Fatal(err)
	}
	return token
}

func (f *externalIdentityAccessFixture) bootstrap() {
	f.t.Helper()
	if _, err := f.adminService.BootstrapLegacy(context.Background(), f.token(f.admin.ID, externalAccessIssuer, externalAccessAdminScope, []string{"admin"})); err != nil {
		f.t.Fatalf("bootstrap issuer using the real enrollment service: %v", err)
	}
	f.bootstrapped = true
}

func (f *externalIdentityAccessFixture) mapIdentity(subject, userID string) {
	f.t.Helper()
	if !f.bootstrapped {
		f.t.Fatal("fixture must bootstrap issuer before mapping another identity")
	}
	_, err := f.adminService.CreateMapped(context.Background(), f.token(f.admin.ID, externalAccessIssuer, externalAccessAdminScope, []string{"admin"}), auth.ExternalIdentityMappingParams{
		Issuer: externalAccessIssuer, Subject: subject, UserID: userID,
	})
	if err != nil {
		f.t.Fatalf("map external identity %q: %v", subject, err)
	}
}

func (f *externalIdentityAccessFixture) server() *Server {
	return New(Config{Mode: "external", External: f.external, ExternalIdentities: f.repo, AuthBurst: 100})
}

func (f *externalIdentityAccessFixture) serverWithReadScope() *Server {
	external := auth.NewExternalAuthenticator(auth.ExternalAuthConfig{
		Issuer: externalAccessIssuer, Audience: "assistente", JWKSURL: f.externalJWKSURL(),
		AllowedAlgorithms: []string{"EdDSA"}, RequiredScopes: []string{"assistente:read"}, RoleClaim: "groups",
	})
	return New(Config{Mode: "external", External: external, ExternalIdentities: f.repo, AuthBurst: 100})
}

func (f *externalIdentityAccessFixture) externalJWKSURL() string {
	// The authenticator exposes no endpoint accessor; retain a fresh test JWKS
	// endpoint backed by the same signer for tests with scope restrictions.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(f.signer.JWKSet())
	}))
	f.t.Cleanup(server.Close)
	return server.URL
}

func externalMeRequest(server *Server, authorization string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	return rec
}

func TestExternalIdentityAccessRequiresConfiguredRepoAndIssuerBootstrap(t *testing.T) {
	f := newExternalIdentityAccessFixture(t)
	token := f.token(f.admin.ID, externalAccessIssuer, "", []string{"admin"})
	withoutRepo := New(Config{Mode: "external", External: f.external})
	if got := externalMeRequest(withoutRepo, "Bearer "+token).Code; got != http.StatusServiceUnavailable {
		t.Fatalf("request without mapping repository status=%d want=%d", got, http.StatusServiceUnavailable)
	}
	if got := externalMeRequest(f.server(), "Bearer "+token).Code; got != http.StatusServiceUnavailable {
		t.Fatalf("request before issuer bootstrap status=%d want=%d", got, http.StatusServiceUnavailable)
	}
}

func TestExternalIdentityAccessUsesMappedLocalUserAndExternalRoles(t *testing.T) {
	f := newExternalIdentityAccessFixture(t)
	f.bootstrap()
	const subject = "subject-does-not-equal-local-uuid"
	f.mapIdentity(subject, f.target.ID)
	server := f.server()
	rec := externalMeRequest(server, "Bearer "+f.token(subject, externalAccessIssuer, "files:read", []string{"operator", "admin"}))
	if rec.Code != http.StatusOK {
		t.Fatalf("mapped access status=%d body=%s", rec.Code, rec.Body.String())
	}
	var me struct {
		UserID    string `json:"userId"`
		SessionID string `json:"sessionId"`
		Role      string `json:"role"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &me); err != nil {
		t.Fatal(err)
	}
	if me.UserID != f.target.ID || me.UserID == subject {
		t.Fatalf("userId=%q, want mapped local UUID %q, distinct from subject", me.UserID, f.target.ID)
	}
	if me.Role != "operator" {
		t.Fatalf("role=%q, want first role from external claims, not local user role", me.Role)
	}
	if me.SessionID != "" {
		t.Fatalf("external sessionId=%q, want empty", me.SessionID)
	}
	commands := auth.NewExternalCommandAuthenticator(f.external, f.repo)
	if _, err := commands.Authenticate(context.Background(), f.token(subject, externalAccessIssuer, "files:read", []string{"operator"})); err == nil || err != auth.ErrExternalIdentityNotReady {
		t.Fatalf("external command auth err=%v, want readiness disabled", err)
	}
}

func TestExternalIdentityAccessStorageFailureReturnsSanitizedServiceUnavailable(t *testing.T) {
	f := newExternalIdentityAccessFixture(t)
	f.bootstrap()
	f.mapIdentity("storage-failure-subject", f.target.ID)
	server := f.server()
	token := f.token("storage-failure-subject", externalAccessIssuer, "files:read", []string{"reader"})
	if got := externalMeRequest(server, "Bearer "+token).Code; got != http.StatusOK {
		t.Fatalf("baseline request status=%d want=%d", got, http.StatusOK)
	}

	const callbackName = "httpapi:fail_external_identity_mapping_read"
	if err := f.db.Callback().Query().Before("gorm:query").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Table == "external_identity_mappings" {
			_ = tx.AddError(errors.New("synthetic SQL failure: external_identity_mappings unavailable"))
		}
	}); err != nil {
		t.Fatalf("register mapping query failure: %v", err)
	}
	t.Cleanup(func() { _ = f.db.Callback().Query().Remove(callbackName) })

	rec := externalMeRequest(server, "Bearer "+token)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("mapping query failure status=%d want=%d body=%s", rec.Code, http.StatusServiceUnavailable, rec.Body.String())
	}
	const sanitizedMessage = "mapeamento externo indisponível"
	if rec.Body.String() != "{\"error\":\""+sanitizedMessage+"\"}\n" {
		t.Fatalf("mapping query failure leaked details: %s", rec.Body.String())
	}
}

func TestExternalIdentityAccessRejectsUnmappedRevokedAndInactiveIdentities(t *testing.T) {
	t.Run("unmapped does not JIT provision", func(t *testing.T) {
		f := newExternalIdentityAccessFixture(t)
		f.bootstrap()
		const subject = "not-yet-mapped"
		rec := externalMeRequest(f.server(), "Bearer "+f.token(subject, externalAccessIssuer, "", nil))
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status=%d want=%d body=%s", rec.Code, http.StatusUnauthorized, rec.Body.String())
		}
		var count int64
		if err := f.db.Model(&auth.ExternalIdentityMapping{}).Where("issuer = ? AND subject = ?", externalAccessIssuer, subject).Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("request provisioned %d mapping rows", count)
		}
		rec = externalMeRequest(f.server(), "Bearer "+f.token(f.target.ID, externalAccessIssuer, "", nil))
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("unmapped subject equal to local UUID status=%d want=%d body=%s", rec.Code, http.StatusUnauthorized, rec.Body.String())
		}
		if err := f.db.Model(&auth.ExternalIdentityMapping{}).Where("issuer = ? AND subject = ?", externalAccessIssuer, f.target.ID).Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("UUID-equal subject request provisioned %d mapping rows", count)
		}
	})
	t.Run("revoked", func(t *testing.T) {
		f := newExternalIdentityAccessFixture(t)
		f.bootstrap()
		f.mapIdentity("revoked-subject", f.target.ID)
		if err := f.repo.Revoke(context.Background(), externalAccessIssuer, "revoked-subject"); err != nil {
			t.Fatal(err)
		}
		rec := externalMeRequest(f.server(), "Bearer "+f.token("revoked-subject", externalAccessIssuer, "", nil))
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status=%d want=%d", rec.Code, http.StatusUnauthorized)
		}
	})
	t.Run("inactive local user", func(t *testing.T) {
		f := newExternalIdentityAccessFixture(t)
		f.bootstrap()
		f.mapIdentity("inactive-subject", f.target.ID)
		if err := f.db.Model(&database.User{}).Where("id = ?", f.target.ID).Update("is_active", false).Error; err != nil {
			t.Fatal(err)
		}
		rec := externalMeRequest(f.server(), "Bearer "+f.token("inactive-subject", externalAccessIssuer, "", nil))
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status=%d want=%d", rec.Code, http.StatusUnauthorized)
		}
	})
	t.Run("wrong issuer", func(t *testing.T) {
		f := newExternalIdentityAccessFixture(t)
		f.bootstrap()
		f.mapIdentity("issuer-subject", f.target.ID)
		rec := externalMeRequest(f.server(), "Bearer "+f.token("issuer-subject", "https://other-issuer.example.test", "", nil))
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status=%d want=%d", rec.Code, http.StatusUnauthorized)
		}
	})
}

func TestExternalIdentityAccessRequiresBearerAuthorization(t *testing.T) {
	f := newExternalIdentityAccessFixture(t)
	f.bootstrap()
	f.mapIdentity("bearer-subject", f.target.ID)
	token := f.token("bearer-subject", externalAccessIssuer, "", nil)
	for _, tc := range []struct {
		name          string
		authorization string
	}{
		{name: "raw", authorization: token},
		{name: "basic", authorization: "Basic " + token},
		{name: "missing", authorization: ""},
		{name: "malformed bearer", authorization: "Bearer"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := externalMeRequest(f.server(), tc.authorization).Code; got != http.StatusUnauthorized {
				t.Fatalf("authorization %q status=%d want=%d", tc.authorization, got, http.StatusUnauthorized)
			}
		})
	}
}
