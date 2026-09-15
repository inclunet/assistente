package auth

import (
	"context"
	"errors"
	"testing"

	"assistente/internal/database"
)

type commandExternalVerifierStub struct {
	claims map[string]*ExternalClaims
	err    error
}

func (v commandExternalVerifierStub) Validate(_ context.Context, token string) (*ExternalClaims, error) {
	if v.err != nil {
		return nil, v.err
	}
	return v.claims[token], nil
}

func TestExternalIdentityAdminRequiresScopeAndStoresExactMapping(t *testing.T) {
	db := setupAuthTestDB(t)
	if err := db.AutoMigrate(&ExternalIdentityMapping{}); err != nil {
		t.Fatalf("migrate external mapping: %v", err)
	}
	identity := NewIdentityService(db)
	admin, err := identity.CreateLocalUser(context.Background(), CreateUserParams{Username: "admin", Password: "unused-password", Admin: true})
	if err != nil {
		t.Fatal(err)
	}
	target, err := identity.CreateLocalUser(context.Background(), CreateUserParams{Username: "target", Password: "unused-password"})
	if err != nil {
		t.Fatal(err)
	}
	verifier := commandExternalVerifierStub{claims: map[string]*ExternalClaims{
		"admin-token": {Issuer: "https://idp.example", Subject: "operator", Scope: "assistente:identity:admin"},
		"user-token":  {Issuer: "https://idp.example", Subject: "external-subject", Scope: "assistente:commands"},
		"no-admin":    {Issuer: "https://idp.example", Subject: "user", Scope: "assistente:commands"},
	}}
	repo := NewExternalIdentityRepository(db)
	service, err := NewExternalIdentityAdminService(verifier, repo, ExternalIdentityAdminConfig{AdminScopes: []string{"assistente:identity:admin"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Create(context.Background(), "admin-token", ExternalIdentityMappingParams{
		Issuer: " https://idp.example ", Subject: "external-subject", UserID: target.ID,
	}); !errors.Is(err, ErrExternalIdentityNotMapped) {
		t.Fatalf("issuer com whitespace deveria ser rejeitado: %v", err)
	}
	mapping, err := service.Create(context.Background(), "admin-token", ExternalIdentityMappingParams{
		Issuer: "https://idp.example", Subject: "external-subject", UserID: target.ID,
	})
	if err != nil || mapping.Issuer != "https://idp.example" || mapping.Subject != "external-subject" || mapping.UserID != target.ID || !mapping.Enabled {
		t.Fatalf("mapping não exato: %+v, err=%v", mapping, err)
	}
	if _, err := service.Create(context.Background(), "no-admin", ExternalIdentityMappingParams{Issuer: "other", Subject: "sub", UserID: admin.ID}); !errors.Is(err, ErrExternalAdminScopeRequired) {
		t.Fatalf("expected admin scope rejection, got %v", err)
	}
	if _, err := service.Create(context.Background(), "admin-token", ExternalIdentityMappingParams{Issuer: "https://idp.example", Subject: "external-subject", UserID: admin.ID}); !errors.Is(err, ErrExternalIdentityAlreadyMapped) {
		t.Fatalf("expected exact issuer/subject uniqueness, got %v", err)
	}
}

func TestExternalCommandAuthenticatorRelêMappingAndRevocation(t *testing.T) {
	db := setupAuthTestDB(t)
	if err := db.AutoMigrate(&ExternalIdentityMapping{}); err != nil {
		t.Fatal(err)
	}
	user, err := NewIdentityService(db).CreateLocalUser(context.Background(), CreateUserParams{Username: "mapped", Password: "unused-password"})
	if err != nil {
		t.Fatal(err)
	}
	verifier := commandExternalVerifierStub{claims: map[string]*ExternalClaims{
		"token": {Issuer: "issuer-a", Subject: "same-subject", Scope: "read"},
	}}
	repo := NewExternalIdentityRepository(db)
	if _, err := repo.Create(context.Background(), ExternalIdentityMappingParams{Issuer: "issuer-a", Subject: "same-subject", UserID: user.ID}); err != nil {
		t.Fatal(err)
	}
	authenticator := NewExternalCommandAuthenticator(verifier, repo)
	if _, err := authenticator.Authenticate(context.Background(), "token"); !errors.Is(err, ErrExternalIdentityNotReady) {
		t.Fatalf("middleware sem readiness explícita deveria falhar fechado: %v", err)
	}
	authenticator.SetReadiness(repo.CheckReadiness)
	principal, err := authenticator.Authenticate(context.Background(), "token")
	if err != nil {
		t.Fatalf("authenticate mapped token: %v", err)
	}
	if principal.UserID != user.ID || principal.Issuer != "issuer-a" || principal.Subject != "same-subject" || principal.AuthContextID == "" {
		t.Fatalf("principal não derivado: %+v", principal)
	}
	if err := repo.Revoke(context.Background(), "issuer-a", "same-subject"); err != nil {
		t.Fatal(err)
	}
	if _, err := authenticator.Authenticate(context.Background(), "token"); !errors.Is(err, ErrUnauthenticatedExternalCommand) {
		t.Fatalf("revogado ainda autenticou: %v", err)
	}
	if _, err := authenticator.Authenticate(context.Background(), "unknown"); !errors.Is(err, ErrUnauthenticatedExternalCommand) {
		t.Fatalf("token não mapeado não foi rejeitado: %v", err)
	}
}

func TestExternalIdentityRejectsInactiveOrNonUUIDTarget(t *testing.T) {
	db := setupAuthTestDB(t)
	if err := db.AutoMigrate(&ExternalIdentityMapping{}); err != nil {
		t.Fatal(err)
	}
	repo := NewExternalIdentityRepository(db)
	if _, err := repo.Create(context.Background(), ExternalIdentityMappingParams{Issuer: "issuer", Subject: "subject", UserID: "not-a-uuid"}); !errors.Is(err, ErrExternalTargetUserRequired) {
		t.Fatalf("expected canonical user id rejection, got %v", err)
	}
	if _, err := repo.Resolve(context.Background(), "issuer", "subject"); !errors.Is(err, ErrExternalIdentityNotMapped) {
		t.Fatalf("expected missing mapping, got %v", err)
	}
	var count int64
	if err := db.Model(&database.User{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("fixture inesperadamente criou usuário: %d", count)
	}
}
