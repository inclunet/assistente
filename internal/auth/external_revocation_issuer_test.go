package auth

import (
	"context"
	"errors"
	"testing"
)

func TestPreparedExternalRevocationStaysWithinConfiguredIssuer(t *testing.T) {
	db := setupAuthTestDB(t)
	if err := db.AutoMigrate(&ExternalIdentityMapping{}); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	user, err := NewIdentityService(db).CreateLocalUser(ctx, CreateUserParams{Username: "revocation-target", Password: "unused-password"})
	if err != nil {
		t.Fatal(err)
	}
	repo := NewExternalIdentityRepository(db)
	for _, issuer := range []string{"issuer-a", "issuer-b"} {
		if _, err := repo.Create(ctx, ExternalIdentityMappingParams{Issuer: issuer, Subject: "same-subject", UserID: user.ID}); err != nil {
			t.Fatal(err)
		}
	}
	verifier := commandExternalVerifierStub{claims: map[string]*ExternalClaims{
		"admin-a": {Issuer: "issuer-a", Subject: "admin", Scope: "identity:admin"},
		"admin-b": {Issuer: "issuer-b", Subject: "admin", Scope: "identity:admin"},
	}}
	service, err := NewExternalIdentityAdminService(verifier, repo, ExternalIdentityAdminConfig{Issuer: "issuer-a", AdminScopes: []string{"identity:admin"}})
	if err != nil {
		t.Fatal(err)
	}
	replica, err := NewExternalIdentityAdminService(verifier, repo, ExternalIdentityAdminConfig{Issuer: "issuer-a", AdminScopes: []string{"identity:admin"}})
	if err != nil {
		t.Fatal(err)
	}
	otherIssuer, err := NewExternalIdentityAdminService(verifier, repo, ExternalIdentityAdminConfig{Issuer: "issuer-b", AdminScopes: []string{"identity:admin"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, pair := range [][2]string{{"admin-a", "issuer-b"}, {"admin-b", "issuer-a"}} {
		if _, err := service.PrepareRevoke(ctx, pair[0], pair[1], "same-subject"); !errors.Is(err, ErrExternalAdministratorRequired) {
			t.Fatalf("revogação entre issuers aceita: %v", err)
		}
	}
	if _, err := service.Create(ctx, "admin-a", ExternalIdentityMappingParams{Issuer: "issuer-b", Subject: "new-subject", UserID: user.ID}); !errors.Is(err, ErrExternalAdministratorRequired) {
		t.Fatalf("Create aceitou issuer externo ao serviço: %v", err)
	}
	if _, err := service.PrepareRevoke(ctx, "admin-a", "issuer-b", "same-subject"); !errors.Is(err, ErrExternalAdministratorRequired) {
		t.Fatalf("preparação aceitou issuer externo ao serviço: %v", err)
	}
	proofFromB, err := otherIssuer.PrepareRevoke(ctx, "admin-b", "issuer-b", "same-subject")
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.Revoke(ctx, "issuer-b", "same-subject"); err != nil {
		t.Fatal(err)
	}
	if err := service.Enable(ctx, "admin-a", "issuer-b", "same-subject"); !errors.Is(err, ErrExternalAdministratorRequired) {
		t.Fatalf("Enable aceitou issuer externo ao serviço: %v", err)
	}
	if _, err := service.PrepareRevoke(ctx, "admin-b", "issuer-a", "same-subject"); !errors.Is(err, ErrExternalAdministratorRequired) {
		t.Fatalf("claims de outro issuer aceitaram operação: %v", err)
	}
	if err := service.RevokePrepared(ctx, proofFromB); !errors.Is(err, ErrExternalAdministratorRequired) {
		t.Fatalf("prova de issuer-b foi aceita pelo serviço issuer-a: %v", err)
	}
	if _, err := repo.Resolve(ctx, "issuer-b", "same-subject"); !errors.Is(err, ErrExternalIdentityRevoked) {
		t.Fatalf("prova transferida alterou vínculo issuer-b: %v", err)
	}
	if err := repo.Enable(ctx, "issuer-b", "same-subject"); err != nil {
		t.Fatal(err)
	}
	prepared, err := service.PrepareRevoke(ctx, "admin-a", "issuer-a", "same-subject")
	if err != nil {
		t.Fatal(err)
	}
	// Provas são opacas a consumidores, mas transferíveis entre réplicas do
	// mesmo issuer: o limite é a configuração do issuer, não o endereço do serviço.
	if err := replica.RevokePrepared(ctx, prepared); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Resolve(ctx, "issuer-a", "same-subject"); err == nil {
		t.Fatal("vínculo alvo não foi revogado")
	}
	if _, err := repo.Resolve(ctx, "issuer-b", "same-subject"); err != nil {
		t.Fatalf("outro issuer foi alterado: %v", err)
	}
	if _, err := repo.Create(ctx, ExternalIdentityMappingParams{Issuer: "issuer-a", Subject: "prepared-revoke", UserID: user.ID}); err != nil {
		t.Fatal(err)
	}
	second, err := service.PrepareRevoke(ctx, "admin-a", "issuer-a", "prepared-revoke")
	if err != nil {
		t.Fatal(err)
	}
	if err := service.RevokePrepared(ctx, second); err != nil {
		t.Fatalf("revogação preparada do issuer configurado falhou: %v", err)
	}
	if _, err := repo.Resolve(ctx, "issuer-a", "prepared-revoke"); !errors.Is(err, ErrExternalIdentityRevoked) {
		t.Fatalf("vínculo preparado não revogado: %v", err)
	}
}

func TestExternalIdentityAdminRequiresValidIssuerAtConstruction(t *testing.T) {
	db := setupAuthTestDB(t)
	if err := db.AutoMigrate(&ExternalIdentityMapping{}); err != nil {
		t.Fatal(err)
	}
	repo := NewExternalIdentityRepository(db)
	verifier := commandExternalVerifierStub{claims: map[string]*ExternalClaims{}}
	for _, issuer := range []string{"", " ", "issuer\x00suffix"} {
		if _, err := NewExternalIdentityAdminService(verifier, repo, ExternalIdentityAdminConfig{Issuer: issuer, AdminScopes: []string{"identity:admin"}}); !errors.Is(err, ErrExternalIssuerRequired) {
			t.Errorf("issuer %q aceito ou erro inesperado: %v", issuer, err)
		}
	}
}
