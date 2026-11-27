package controllers

import (
	"context"
	"errors"
	"testing"
	"time"

	"assistente/internal/authorization"
	"assistente/internal/database"
	"assistente/internal/nettrust"
	"assistente/internal/testutil/authorizationfixture"
	"assistente/internal/tools/invocationctx"
	"assistente/internal/trustscope"
	"gorm.io/gorm"
)

func TestPermissionsNetworkManagementIsolatesAndRevokesSessions(t *testing.T) {
	dir := t.TempDir()
	repo, _ := authorizationfixture.Open(t, dir)
	m := nettrust.NewManager(trustscope.Options{Repository: repo, HomeDir: func() string { return dir }, WorkDir: func() string { return "" }})
	ctx := invocationctx.With(authorizationfixture.Context(), invocationctx.InvocationContext{ConversationID: "conversation"})
	created := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	for _, scope := range []nettrust.Scope{nettrust.ScopeGlobal, nettrust.ScopeSession} {
		if err := m.Add(ctx, nettrust.AllowlistEntry{Host: string(scope) + ".internal", Scope: scope, CreatedAt: created}); err != nil {
			t.Fatal(err)
		}
	}
	c := PermissionsController{Repository: repo, Sessions: []SessionPermissions{m}}
	snapshot, err := c.List(ctx)
	if err != nil || len(snapshot.Rules) != 2 {
		t.Fatalf("lista: %+v %v", snapshot, err)
	}
	other := database.WithUserID(context.Background(), "another-user")
	if result, err := c.List(other); err != nil || len(result.Rules) != 0 {
		t.Fatalf("vazamento: %+v %v", result, err)
	}
	for _, row := range snapshot.Rules {
		if row.CreatedAt != created.Format(time.RFC3339) {
			t.Fatalf("perdeu data: %+v", row)
		}
		if err := c.Revoke(other, row.ID); !errors.Is(err, authorization.ErrNotFound) {
			t.Fatalf("revogação cruzada: %v", err)
		}
		if err := c.Revoke(ctx, row.ID); err != nil {
			t.Fatal(err)
		}
		if match := m.Match(ctx, row.Resource, "443"); match.Err != nil || match.Allowed {
			t.Fatalf("continua autorizado: %+v", match)
		}
		if err := c.Revoke(ctx, row.ID); !errors.Is(err, authorization.ErrNotFound) {
			t.Fatalf("revogação repetida: %v", err)
		}
	}
	after, err := c.List(ctx)
	if err != nil || len(after.Rules) != 1 || after.Rules[0].RevokedAt == "" {
		t.Fatalf("histórico da revogação: %+v %v", after, err)
	}
}

func TestPermissionsMigrationFailureDoesNotHideKnownRules(t *testing.T) {
	repo, _ := authorizationfixture.Open(t, t.TempDir())
	ctx := authorizationfixture.Context()
	if err := repo.Upsert(ctx, database.AuthorizationRule{Domain: "network", Scope: "global", Effect: "allow", Origin: "consent", SelectorKey: "known", SelectorVersion: 1, Selector: `{"host":"known.internal","scope":"global"}`}); err != nil {
		t.Fatal(err)
	}
	c := PermissionsController{Repository: repo, Prepare: func(context.Context) []error {
		return []error{
			&PermissionPreparationError{Domain: "network", Profile: "padrao", Err: authorization.ErrLegacyOwnerPending},
			errors.New("detalhe técnico que não deve ir para a UI"),
		}
	}}
	result, err := c.List(ctx)
	if err != nil || len(result.Rules) != 1 || len(result.MigrationErrors) != 2 {
		t.Fatalf("estado incompleto: %+v %v", result, err)
	}
	if result.MigrationErrors[0] != (PermissionDiagnostic{Code: "owner_pending", Domain: "network", Profile: "padrao"}) || result.MigrationErrors[1].Code != "source_unavailable" {
		t.Fatalf("diagnósticos sem contrato traduzível: %+v", result.MigrationErrors)
	}
	if err := c.Revoke(ctx, result.Rules[0].ID); err != nil {
		t.Fatal(err)
	}
}

func TestPermissionsManagePluginHookAlongsideNetwork(t *testing.T) {
	repo, db := authorizationfixture.Open(t, t.TempDir())
	if err := db.AutoMigrate(&database.PluginInstallation{}); err != nil {
		t.Fatal(err)
	}
	ctx := authorizationfixture.Context()
	owner, err := database.RequireUserID(ctx)
	if err != nil {
		t.Fatal(err)
	}
	row := database.PluginInstallation{UserID: owner, Name: "revisor", Origin: "catalogo", Digest: "teste"}
	if err := db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	hook := authorization.PluginHook{Name: row.Name, Origin: row.Origin, InstallationID: row.ID, Digest: row.Digest, Component: "PreToolUse"}
	if err := repo.RememberPluginHook(ctx, hook, authorization.ScopeGlobal, "", "allow", func(_ *gorm.DB, commit func() error) error { return commit() }); err != nil {
		t.Fatal(err)
	}
	if err := repo.Upsert(ctx, database.AuthorizationRule{Domain: "network", Scope: "global", Effect: "allow", Origin: "consent", SelectorKey: "known", SelectorVersion: 1, Selector: `{"host":"known.internal"}`}); err != nil {
		t.Fatal(err)
	}
	c := PermissionsController{Repository: repo}
	other := row
	other.ID = ""
	other.Origin = "outra loja"
	if err := db.Create(&other).Error; err != nil {
		t.Fatal(err)
	}
	sibling := hook
	sibling.Origin, sibling.InstallationID = other.Origin, other.ID
	if err := repo.RememberPluginHook(ctx, sibling, authorization.ScopeGlobal, "", "allow", func(_ *gorm.DB, commit func() error) error { return commit() }); err != nil {
		t.Fatal(err)
	}
	result, err := c.List(ctx)
	if err != nil || len(result.Rules) != 3 {
		t.Fatalf("listagem: %+v, %v", result, err)
	}
	for _, view := range result.Rules {
		if view.Domain == authorization.PluginHookDomain && view.Resource != "revisor (outra loja) — PreToolUse" {
			if view.Resource != "revisor (catalogo) — PreToolUse" || view.ResourceKind != row.Digest {
				t.Fatalf("identidade: %+v", view)
			}
			if err := c.Revoke(ctx, view.ID); err != nil {
				t.Fatal(err)
			}
		}
	}
	ok, err := repo.PluginHookAllowed(invocationctx.WithVerifiedWorkspaceID(ctx, "w"), hook)
	if err != nil || ok {
		t.Fatalf("hook revogado: %v, %v", ok, err)
	}
	ok, err = repo.PluginHookAllowed(invocationctx.WithVerifiedWorkspaceID(ctx, "w"), sibling)
	if err != nil || !ok {
		t.Fatalf("outra instalação: %v, %v", ok, err)
	}
}
