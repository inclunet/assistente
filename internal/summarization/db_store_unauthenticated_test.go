package summarization

import (
	"context"
	"errors"
	"testing"

	"assistente/internal/database"
)

// TestDBSummarizationStore_UnauthenticatedErrors garante que o repositório
// de sumarização falha fechado (fail-closed) quando o contexto não traz
// userID. Cobre o vetor identificado no re-review do AEP-0052: callers
// distraídos que chamem o store com ctx vazio (ou sessão expirada
// mid-flight) NÃO podem ler/escrever resumos cross-user.
//
// A guarda nas funções database.*WithContext já é fail-closed, mas o
// store mantém uma checagem explícita por defesa em camadas — igual ao
// padrão de chat/tasklist/providers DBStores.
func TestDBSummarizationStore_UnauthenticatedErrors(t *testing.T) {
	store := NewDBStore()
	ctx := context.Background()

	cases := []struct {
		name string
		call func() error
	}{
		{"GetMessages", func() error {
			_, err := store.GetMessages(ctx, "conv-x")
			return err
		}},
		{"GetConversationSummary", func() error {
			_, _, err := store.GetConversationSummary(ctx, "conv-x")
			return err
		}},
		{"IsSummarizingInProgress", func() error {
			_, err := store.IsSummarizingInProgress(ctx, "conv-x")
			return err
		}},
		{"SetSummarizingInProgress", func() error {
			return store.SetSummarizingInProgress(ctx, "conv-x", true)
		}},
		{"UpdateConversationSummary", func() error {
			return store.UpdateConversationSummary(ctx, "conv-x", "summary", "msg-y")
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
