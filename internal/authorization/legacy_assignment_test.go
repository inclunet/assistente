package authorization_test

import (
	"context"
	"errors"
	"testing"

	"assistente/internal/authorization"
	"assistente/internal/database"
	"assistente/internal/testutil/authorizationfixture"
)

func TestLegacyOriginRequiresAdminAssignmentAndNeverChangesOwner(t *testing.T) {
	repo, db := authorizationfixture.Open(t, t.TempDir())
	if err := db.AutoMigrate(&database.User{}); err != nil {
		t.Fatal(err)
	}
	for _, user := range []database.User{
		{UUIDModel: database.UUIDModel{ID: "admin"}, Username: "admin", Role: database.UserRoleAdmin, IsActive: true},
		{UUIDModel: database.UUIDModel{ID: "first"}, Username: "first", Role: database.UserRoleUser, IsActive: true},
		{UUIDModel: database.UUIDModel{ID: "owner"}, Username: "owner", Role: database.UserRoleUser, IsActive: true},
	} {
		if err := db.Create(&user).Error; err != nil {
			t.Fatal(err)
		}
	}
	admin := database.WithUserID(context.Background(), "admin")
	first := database.WithUserID(context.Background(), "first")
	owner := database.WithUserID(context.Background(), "owner")
	if allowed, err := repo.AuthorizeLegacyImport(first, "network", "known-origin", false); allowed || !errors.Is(err, authorization.ErrLegacyOwnerPending) {
		t.Fatalf("primeiro login: %v %v", allowed, err)
	}
	if _, err := repo.PendingOrigins(first); err == nil {
		t.Fatal("usuário comum leu origens administrativas")
	}
	pending, err := repo.PendingOrigins(admin)
	if err != nil || len(pending) != 1 {
		t.Fatalf("origens: %+v %v", pending, err)
	}
	if err := repo.AssignOrigin(first, pending[0].ID, "first"); err == nil {
		t.Fatal("atribuição sem admin")
	}
	if err := repo.AssignOrigin(admin, pending[0].ID, "missing"); err == nil {
		t.Fatal("atribuiu a usuário inexistente")
	}
	if err := repo.AssignOrigin(admin, pending[0].ID, "owner"); err != nil {
		t.Fatal(err)
	}
	if allowed, err := repo.AuthorizeLegacyImport(first, "network", "known-origin", true); err != nil || allowed {
		t.Fatalf("adoção automática ignorou dono: %v %v", allowed, err)
	}
	if allowed, err := repo.AuthorizeLegacyImport(owner, "network", "known-origin", false); err != nil || !allowed {
		t.Fatalf("dono explícito: %v %v", allowed, err)
	}
	if err := repo.AssignOrigin(admin, pending[0].ID, "first"); !errors.Is(err, authorization.ErrNotFound) {
		t.Fatalf("reatribuiu origem: %v", err)
	}
	if imported, err := repo.Imported(owner, "network", "known-origin"); err != nil || imported {
		t.Fatalf("atribuição confundida com importação: %v %v", imported, err)
	}
}
