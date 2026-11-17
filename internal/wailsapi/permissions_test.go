package wailsapi

import (
	"errors"
	"testing"

	"assistente/controllers"
	"assistente/internal/testutil/authorizationfixture"
)

func TestPermissionsRequireAuthenticationAndAdmin(t *testing.T) {
	api := NewPermissions()
	if _, err := api.GetPermissions(); err == nil {
		t.Fatal("API sem wiring")
	}
	repo, _ := authorizationfixture.Open(t, t.TempDir())
	denied := errors.New("sem sessão")
	AttachPermissions(api, stubSession{err: denied, adminErr: denied}, &controllers.PermissionsController{Repository: repo})
	if _, err := api.GetPermissions(); !errors.Is(err, denied) {
		t.Fatalf("listagem sem auth: %v", err)
	}
	if err := api.RevokePermission("id"); !errors.Is(err, denied) {
		t.Fatalf("revogação sem auth: %v", err)
	}
	if _, err := api.GetPermissionMigration(); !errors.Is(err, denied) {
		t.Fatalf("origens sem auth: %v", err)
	}
	if err := api.AssignPermissionOrigin("id", "user"); !errors.Is(err, denied) {
		t.Fatalf("atribuição sem auth: %v", err)
	}
	AttachPermissions(api, stubSession{ctx: authorizationfixture.Context()}, &controllers.PermissionsController{Repository: repo})
	if result, err := api.GetPermissions(); err != nil || result.Rules == nil {
		t.Fatalf("lista autenticada: %+v %v", result, err)
	}
}
