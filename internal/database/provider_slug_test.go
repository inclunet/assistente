package database

import (
	"errors"
	"testing"

	"gorm.io/gorm"
)

func TestProviderSlugResolvesInOwnerAfterRenameAndMigration(t *testing.T) {
	db := newMigratorTestDB(t)
	if err := db.AutoMigrate(&LLMProvider{}); err != nil {
		t.Fatal(err)
	}
	for _, record := range []struct{ id, owner string }{{"legacy-a", "a"}, {"legacy-b", "a"}, {"legacy-c", "b"}} {
		if err := db.Exec("INSERT INTO llm_providers (id,user_id,name,type,base_url) VALUES (?,?,?,'openai','')", record.id, record.owner, "Revisão").Error; err != nil {
			t.Fatal(err)
		}
	}
	repo := NewProviderRepository(db)
	for range 2 {
		if err := migrateProviderSlugs(db); err != nil {
			t.Fatal(err)
		}
		for _, tc := range []struct{ owner, slug, id string }{{"a", "revisao", "legacy-a"}, {"a", "revisao-2", "legacy-b"}, {"b", "revisao", "legacy-c"}} {
			ctx := WithUserID(t.Context(), tc.owner)
			p, err := repo.GetBySlug(ctx, tc.slug)
			if err != nil || p.ID != tc.id {
				t.Fatalf("busca %s/%s: %v %v", tc.owner, tc.slug, p, err)
			}
			p.Name = "Novo nome"
			p.Slug = ""
			if err := repo.SaveLLMProvider(ctx, p); err != nil {
				t.Fatal(err)
			}
		}
	}
	ctx := WithUserID(t.Context(), "a")
	if _, err := repo.GetBySlug(ctx, "legacy-c"); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("slug ausente: %v", err)
	}
	if _, err := repo.GetBySlug(t.Context(), "revisao"); !errors.Is(err, ErrUserScopeRequired) {
		t.Fatalf("conta ausente: %v", err)
	}
	if err := repo.SaveLLMProvider(WithBootstrap(t.Context()), &LLMProvider{ID: "legacy-a"}); !errors.Is(err, ErrProviderCrossUser) {
		t.Fatalf("bootstrap sobrescreveu provedor: %v", err)
	}
	conflict := LLMProvider{ID: "collision", Name: "Diferente", Slug: "revisao"}
	if err := repo.SaveLLMProvider(ctx, &conflict); err == nil {
		t.Fatal("aceitou slug já ocupado")
	}
	fresh := LLMProvider{ID: "revisao", Name: "Revisão"}
	if err := repo.SaveLLMProvider(ctx, &fresh); err != nil || fresh.Slug != "revisao-3" {
		t.Fatalf("slug gerado: %q, %v", fresh.Slug, err)
	}
	p, err := repo.GetBySlug(ctx, "revisao")
	if err != nil || p.ID != "legacy-a" {
		t.Fatalf("confundiu slug com ID: %v %v", p, err)
	}
}
