package chat

import (
	"context"
	"errors"
	"testing"

	"assistente/internal/database"
)

// Estes testes garantem que o repositório falha fechado (fail-closed) quando o
// contexto não traz userID. O cenário simula a chamada feita antes do login ou
// por um caller que esqueceu de propagar o contexto autenticado — exatamente o
// vetor de ataque levantado na revisão do AEP-0052.

func TestDBMessageStore_UnauthenticatedErrors(t *testing.T) {
	store := NewDBMessageStore()
	ctx := context.Background()

	cases := []struct {
		name string
		call func() error
	}{
		{"CreateMessage", func() error {
			_, err := store.CreateMessage(ctx, database.MessageOptions{ConversationID: "x"})
			return err
		}},
		{"GetMessage", func() error {
			_, err := store.GetMessage(ctx, "x")
			return err
		}},
		{"GetMessages", func() error {
			_, err := store.GetMessages(ctx, "x", nil)
			return err
		}},
		{"GetConversationSummary", func() error {
			_, _, err := store.GetConversationSummary(ctx, "x")
			return err
		}},
		{"GetDetailedTokenStats", func() error {
			_, err := store.GetDetailedTokenStats(ctx, "x", "")
			return err
		}},
		{"GetContextWindowUsage", func() error {
			_, _, err := store.GetContextWindowUsage(ctx, "x", 1024)
			return err
		}},
		{"GetRecentMessagesTokenCount", func() error {
			_, err := store.GetRecentMessagesTokenCount(ctx, "x", 10)
			return err
		}},
		{"GetTurnTokenStats", func() error {
			_, err := store.GetTurnTokenStats(ctx, "x", "y")
			return err
		}},
		{"AddAssistantToolMessage", func() error {
			_, err := store.AddAssistantToolMessage(ctx, "x", "y", "", "", "", "")
			return err
		}},
		{"AddToolResultMessage", func() error {
			_, err := store.AddToolResultMessage(ctx, "x", "y", "", "")
			return err
		}},
		{"SearchMessages", func() error {
			_, err := store.SearchMessages(ctx, "q", 10)
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

func TestDBConversationStore_UnauthenticatedErrors(t *testing.T) {
	store := NewDBConversationStore()
	ctx := context.Background()

	cases := []struct {
		name string
		call func() error
	}{
		{"GetConversationInfo", func() error {
			_, err := store.GetConversationInfo(ctx, "x")
			return err
		}},
		{"UpdateConversation", func() error {
			return store.UpdateConversation(ctx, "x", "t", "")
		}},
		{"UpdateConversationChannel", func() error {
			return store.UpdateConversationChannel(ctx, "x", "ch", "c")
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
