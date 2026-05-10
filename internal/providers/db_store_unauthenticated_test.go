package providers

import (
	"context"
	"errors"
	"testing"

	"assistente/internal/database"
)

// Estes testes garantem que as leituras do repositório falham fechado
// (fail-closed) quando o contexto não traz userID. O cenário simula a chamada
// feita antes do login — se o registry global de provedores fosse populado
// nesse cenário, dados de todos os usuários ficariam visíveis até o reinício
// do processo (vetor levantado na revisão do AEP-0052).
//
// Save é intencionalmente exceção: o wizard de boas-vindas (bootstrap
// pré-AEP-0052) cria provedores antes do primeiro login. Esses registros
// ficam órfãos (user_id="") até AdoptLegacyData os atribuir, e Load nunca os
// devolve sem userID.

func TestDBStore_UnauthenticatedErrors(t *testing.T) {
	store := NewDBStore()
	ctx := context.Background()

	cases := []struct {
		name string
		call func() error
	}{
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
