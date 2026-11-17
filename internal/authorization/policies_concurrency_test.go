package authorization_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"assistente/internal/database"
	"assistente/internal/testutil/authorizationfixture"
	"gorm.io/gorm"
)

func TestConcurrentFirstBindingRetriesWholeTransaction(t *testing.T) {
	repo, db := authorizationfixture.Open(t, t.TempDir())
	sql, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sql.SetMaxOpenConns(4)
	ctx, cancel := context.WithTimeout(authorizationfixture.Context(), 10*time.Second)
	defer cancel()
	if err := repo.SavePolicy(ctx, database.AuthorizationPolicy{Domain: "shell", Slug: "test", Source: "test", DefaultEffect: "ask"}, nil); err != nil {
		t.Fatal(err)
	}
	policies, err := repo.Policies(ctx, "shell")
	if err != nil || len(policies) != 1 {
		t.Fatalf("políticas: %+v %v", policies, err)
	}
	// As duas transações leem a ausência do vínculo antes de qualquer escrita.
	// Uma perde a promoção do snapshot WAL a writer e precisa recomeçar.
	var readers atomic.Int32
	ready := make(chan struct{})
	if err := db.Callback().Query().After("gorm:query").Register("test:binding_snapshot", func(tx *gorm.DB) {
		if tx.Statement.Table != "authorization_policy_bindings" {
			return
		}
		n := readers.Add(1)
		if n == 2 {
			close(ready)
		}
		if n <= 2 {
			select {
			case <-ready:
			case <-ctx.Done():
				_ = tx.AddError(ctx.Err())
			}
		}
	}); err != nil {
		t.Fatal(err)
	}
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() { results <- repo.BindPolicy(ctx, policies[0].ID, "perfil") }()
	}
	for i := 0; i < 2; i++ {
		if err := <-results; err != nil {
			t.Errorf("vínculo concorrente: %v", err)
		}
	}
	var count int64
	if err := db.Model(&database.AuthorizationPolicyBinding{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("vínculos=%d, esperado um", count)
	}
	if readers.Load() < 3 {
		t.Fatal("cenário não exercitou a reexecução da transação")
	}
}
