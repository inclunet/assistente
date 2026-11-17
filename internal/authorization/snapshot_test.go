package authorization_test

import (
	"assistente/internal/database"
	"assistente/internal/testutil/authorizationfixture"
	"gorm.io/gorm"
	"testing"
)

func TestEscoposNaoMisturamEstadosAntesEDepoisDaRevogacao(t *testing.T) {
	repo, db := authorizationfixture.Open(t, t.TempDir())
	ctx := authorizationfixture.Context()
	grant := networkRule()
	grant.Scope = "profile"
	grant.ScopeKey = "p"
	if err := repo.Upsert(ctx, grant); err != nil {
		t.Fatal(err)
	}
	changed := false
	if err := db.Callback().Query().After("gorm:query").Register("test:change-after-snapshot", func(tx *gorm.DB) {
		if changed || tx.Statement.Table != "authorization_rules" {
			return
		}
		changed = true
		if err := repo.Revoke(ctx, "network", "profile", "p", grant.SelectorKey); err != nil {
			t.Fatal(err)
		}
		deny := networkRule()
		deny.Effect = "deny"
		if err := repo.Upsert(ctx, deny); err != nil {
			t.Fatal(err)
		}
	}); err != nil {
		t.Fatal(err)
	}
	scopes := map[string]string{"profile": "p", "global": ""}
	before, err := repo.ListScopes(ctx, "network", scopes)
	if err != nil || len(before) != 1 || before[0].Effect != "allow" || before[0].Scope != "profile" {
		t.Fatalf("snapshot misturado: %+v %v", before, err)
	}
	after, err := repo.ListScopes(ctx, "network", scopes)
	if err != nil || len(after) != 1 || after[0].Effect != "deny" || after[0].Scope != "global" {
		t.Fatalf("revogação não observada: %+v %v", after, err)
	}
	foreign, err := repo.ListScopes(database.WithUserID(ctx, "another-user"), "network", scopes)
	if err != nil || len(foreign) != 0 {
		t.Fatalf("vazou snapshot: %+v %v", foreign, err)
	}
}
