package authorization_test

import (
	"context"
	"errors"
	"testing"

	"assistente/internal/authorization"
	"assistente/internal/database"
	"assistente/internal/testutil/authorizationfixture"
)

func networkRule() database.AuthorizationRule {
	return database.AuthorizationRule{Domain: "network", Scope: "global", SelectorKey: "example.test", Selector: `{"host":"example.test","scope":"global"}`, SelectorVersion: 1, Effect: "allow", Origin: "consent"}
}

func TestImportacaoAtomicaNaoRessuscitaRevogacao(t *testing.T) {
	repo, _ := authorizationfixture.Open(t, t.TempDir())
	ctx := authorizationfixture.Context()
	rules := []database.AuthorizationRule{networkRule()}
	changed := errors.New("origem alterada")
	if err := repo.Import(ctx, "network", "legacy", "digest", rules, func() error { return changed }); !errors.Is(err, changed) {
		t.Fatalf("importação: %v", err)
	}
	if done, err := repo.Imported(ctx, "network", "legacy"); err != nil || done {
		t.Fatalf("marcador sobreviveu ao rollback: %v, %v", done, err)
	}
	if rows, err := repo.List(ctx, "network", "global", ""); err != nil || len(rows) != 0 {
		t.Fatalf("regras sobreviveram ao rollback: %v, %v", rows, err)
	}
	verify := func() error { return nil }
	if err := repo.Import(ctx, "network", "legacy", "digest", rules, verify); err != nil {
		t.Fatal(err)
	}
	if err := repo.Revoke(ctx, "network", "global", "", "example.test"); err != nil {
		t.Fatal(err)
	}
	if err := repo.Import(ctx, "network", "legacy", "new-digest", rules, verify); err != nil {
		t.Fatal(err)
	}
	if rows, err := repo.List(ctx, "network", "global", ""); err != nil || len(rows) != 0 {
		t.Fatalf("reimportação reativou regra: %v, %v", rows, err)
	}
}

func TestRepositorioIsolaUsuarioERecusaEscoposTemporarios(t *testing.T) {
	repo, _ := authorizationfixture.Open(t, t.TempDir())
	ctx := authorizationfixture.Context()
	rule := networkRule()
	rule.UserID = "outro-usuario"
	if err := repo.Upsert(ctx, rule); err != nil {
		t.Fatal(err)
	}
	other := database.WithUserID(context.Background(), "outro-usuario")
	if rows, err := repo.List(other, "network", "global", ""); err != nil || len(rows) != 0 {
		t.Fatalf("vazamento entre usuários: %v, %v", rows, err)
	}
	if err := repo.Revoke(other, "network", "global", "", rule.SelectorKey); !errors.Is(err, authorization.ErrNotFound) {
		t.Fatalf("revogação cruzada: %v", err)
	}
	if _, err := repo.List(context.Background(), "network", "global", ""); err == nil {
		t.Fatal("leitura sem usuário")
	}
	for _, scope := range []string{"once", "session"} {
		rule.Scope = scope
		if err := repo.Upsert(ctx, rule); err == nil {
			t.Fatalf("persistiu %s", scope)
		}
	}
	if rows, err := repo.List(ctx, "network", "global", ""); err != nil || len(rows) != 1 {
		t.Fatalf("regra do proprietário perdida: %v, %v", rows, err)
	}
}

func TestAtualizacaoInvalidaMantemPoliticaAnterior(t *testing.T) {
	repo, _ := authorizationfixture.Open(t, t.TempDir())
	ctx := authorizationfixture.Context()
	policy := database.AuthorizationPolicy{Domain: "shell", Slug: "restrita", Source: "home", DefaultEffect: "deny"}
	rule := networkRule()
	rule.Effect = "deny"
	if err := repo.SavePolicy(ctx, policy, []database.AuthorizationRule{rule}); err != nil {
		t.Fatal(err)
	}
	rows, err := repo.Policies(ctx, "shell")
	if err != nil || len(rows) != 1 {
		t.Fatalf("políticas: %v, %v", rows, err)
	}
	policy.DefaultEffect = "ask"
	rule.Selector = "inválido"
	if err := repo.SavePolicy(ctx, policy, []database.AuthorizationRule{rule}); err == nil {
		t.Fatal("aceitou seletor inválido")
	}
	snapshot, rules, err := repo.PolicySnapshot(ctx, rows[0].ID)
	if err != nil || snapshot.DefaultEffect != "deny" || snapshot.Revision != 1 || len(rules) != 1 || rules[0].Effect != "deny" {
		t.Fatalf("atualização parcial: %+v, %+v, %v", snapshot, rules, err)
	}
}
