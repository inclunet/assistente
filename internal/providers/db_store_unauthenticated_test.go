package providers

import (
	"context"
	"errors"
	"testing"

	"assistente/internal/database"
	"assistente/internal/llm"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// Estes testes garantem que o repositório falha fechado (fail-closed) quando
// o contexto não traz userID nem o marcador de bootstrap. Cobrem os caminhos
// de leitura E o Save — antes da resposta ao review do AEP-0052, Save
// aceitava silenciosamente gravar provedores órfãos sem qualquer prova de
// intenção do caller. Agora exige userID OU WithBootstrap explícito.

func TestDBStore_UnauthenticatedErrors(t *testing.T) {
	store := NewDBStore()
	ctx := context.Background()

	cases := []struct {
		name string
		call func() error
	}{
		{"Save", func() error {
			return store.Save(ctx, []*llm.ProviderConfig{{ID: "x", Name: "X", Type: llm.ProviderOpenAI}})
		}},
		{"Load", func() error {
			_, err := store.Load(ctx)
			return err
		}},
		{"SetDefault", func() error {
			return store.SetDefault(ctx, "x")
		}},
		{"GetDefault", func() error {
			_, err := store.GetDefault(ctx)
			return err
		}},
		{"Get", func() error {
			_, err := store.Get(ctx, "x")
			return err
		}},
		{"Count", func() error {
			_, err := store.Count(ctx)
			return err
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.call()
			if !errors.Is(err, database.ErrUserScopeRequired) {
				t.Fatalf("esperava ErrUserScopeRequired, obteve %v", err)
			}
		})
	}
}

// TestDBStore_Save_AcceptsBootstrap valida o caminho de bootstrap pré-login:
// quando o ctx não carrega userID mas foi explicitamente marcado por
// WithBootstrap (CLI setup, wizard pré-AEP-0052), Save grava o provedor
// como órfão (user_id="") para ser adotado depois.
func TestDBStore_Save_AcceptsBootstrap(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("falha ao criar banco em memória: %v", err)
	}
	if err := db.AutoMigrate(&database.LLMProvider{}); err != nil {
		t.Fatalf("falha ao migrar tabela: %v", err)
	}
	database.SetDB(db)

	store := NewDBStore()
	ctx := database.WithBootstrap(context.Background())
	err = store.Save(ctx, []*llm.ProviderConfig{{
		ID:      "bootstrap-prov",
		Name:    "Bootstrap",
		Type:    llm.ProviderOpenAI,
		BaseURL: "https://api.openai.com/v1",
	}})
	if err != nil {
		t.Fatalf("Save com WithBootstrap falhou: %v", err)
	}

	var got database.LLMProvider
	if err := db.Where("id = ?", "bootstrap-prov").First(&got).Error; err != nil {
		t.Fatalf("provedor de bootstrap não persistido: %v", err)
	}
	if got.UserID != "" {
		t.Errorf("provedor de bootstrap deveria nascer órfão (user_id=\"\"), got %q", got.UserID)
	}
}
