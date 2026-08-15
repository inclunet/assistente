package wailsapi

import (
	"errors"
	"testing"

	"assistente/controllers"
	"assistente/internal/chat"
)

func TestConversationsNotWired(t *testing.T) {
	t.Parallel()
	api := NewConversations()
	if _, err := api.GetConversations(); !errors.Is(err, ErrConversationsNotWired) {
		t.Fatalf("GetConversations: got %v", err)
	}
	if _, err := api.CreateConversation("t", ""); !errors.Is(err, ErrConversationsNotWired) {
		t.Fatalf("CreateConversation: got %v", err)
	}
	if err := api.DeleteConversation("id"); !errors.Is(err, ErrConversationsNotWired) {
		t.Fatalf("DeleteConversation: got %v", err)
	}
	if _, err := api.GetMessages("id", nil); !errors.Is(err, ErrConversationsNotWired) {
		t.Fatalf("GetMessages: got %v", err)
	}
	if _, err := api.GetConversationMessageWindow(chat.MessageWindowRequest{}); !errors.Is(err, ErrConversationsNotWired) {
		t.Fatalf("GetConversationMessageWindow: got %v", err)
	}
	if _, err := api.SearchConversationHistory("q", 10); !errors.Is(err, ErrConversationsNotWired) {
		t.Fatalf("SearchConversationHistory: got %v", err)
	}
	if _, err := api.GetEffectiveModel(); !errors.Is(err, ErrConversationsNotWired) {
		t.Fatalf("GetEffectiveModel: got %v", err)
	}
}

func TestConversationsNilControllerIsNotWired(t *testing.T) {
	t.Parallel()
	api := NewConversations()
	AttachConversations(api, stubSession{}, nil)
	if _, err := api.GetConversations(); !errors.Is(err, ErrConversationsNotWired) {
		t.Fatalf("GetConversations com ctrl nil: got %v", err)
	}
}

// TestConversationsUsesWithUserNotRequireAuth cobre o fail-closed da borda:
// sem contexto autenticado o domínio não roda (MsgRepo nil panica se chamado)
// e o erro da sessão sobe como veio.
func TestConversationsUsesWithUserNotRequireAuth(t *testing.T) {
	t.Parallel()
	semAuth := errors.New("sessão não autenticada")
	api := NewConversations()
	AttachConversations(api, stubSession{err: semAuth}, controllers.NewConversationsController(controllers.ConversationsControllerConfig{}))

	casos := []struct {
		nome string
		fn   func() error
	}{
		{"GetConversations", func() error {
			_, err := api.GetConversations()
			return err
		}},
		{"CreateConversation", func() error {
			_, err := api.CreateConversation("t", "")
			return err
		}},
		{"EnsureConversation", func() error {
			_, err := api.EnsureConversation("t")
			return err
		}},
		{"GetConversation", func() error {
			_, err := api.GetConversation("id")
			return err
		}},
		{"GetConversationsPage", func() error {
			_, err := api.GetConversationsPage(10, 0)
			return err
		}},
		{"GetConversationsByIDs", func() error {
			_, err := api.GetConversationsByIDs([]string{"id"})
			return err
		}},
		{"GetMessages", func() error {
			_, err := api.GetMessages("id", nil)
			return err
		}},
		{"GetRecentMessages", func() error {
			_, err := api.GetRecentMessages("id", 10)
			return err
		}},
		{"GetMessagesBefore", func() error {
			_, err := api.GetMessagesBefore("id", "before", 10)
			return err
		}},
		{"GetConversationMessageWindow", func() error {
			_, err := api.GetConversationMessageWindow(chat.MessageWindowRequest{
				ConversationID: "id",
				Limit:          10,
			})
			return err
		}},
		{"GetConversationInfo", func() error {
			_, err := api.GetConversationInfo("id")
			return err
		}},
		{"GetConversationWithThreads", func() error {
			_, err := api.GetConversationWithThreads("id")
			return err
		}},
		{"GetMessageChildren", func() error {
			_, err := api.GetMessageChildren("id")
			return err
		}},
		{"UpdateConversation", func() error {
			return api.UpdateConversation("id", "t", "")
		}},
		{"DeleteConversation", func() error {
			return api.DeleteConversation("id")
		}},
		{"DeleteMessage", func() error {
			return api.DeleteMessage("id")
		}},
		{"UpdateMessage", func() error {
			return api.UpdateMessage("id", "c")
		}},
		{"UpdateConversationModel", func() error {
			return api.UpdateConversationModel("id", "m")
		}},
		{"CreateMessage", func() error {
			_, err := api.CreateMessage("id", "user", "hi")
			return err
		}},
		{"AddMessage", func() error {
			_, err := api.AddMessage("id", "user", "hi")
			return err
		}},
		{"AddMessageWithMedia", func() error {
			_, err := api.AddMessageWithMedia("id", "user", "hi", "")
			return err
		}},
		{"AddMessageWithTokens", func() error {
			_, err := api.AddMessageWithTokens("id", "user", "hi", 1, 1, 2, "m")
			return err
		}},
		{"AddMessageWithTokensAndMedia", func() error {
			_, err := api.AddMessageWithTokensAndMedia("id", "user", "hi", "", 1, 1, 2, "m")
			return err
		}},
		{"AddChildMessage", func() error {
			_, err := api.AddChildMessage("id", "parent", "user", "hi", "m")
			return err
		}},
		{"GetAllTokenStats", func() error {
			_, err := api.GetAllTokenStats()
			return err
		}},
		{"GetConversationSummary", func() error {
			_, err := api.GetConversationSummary("id")
			return err
		}},
		{"RenameConversation", func() error {
			return api.RenameConversation("id", "t")
		}},
		{"ClearConversation", func() error {
			return api.ClearConversation("id")
		}},
		{"DeleteMessages", func() error {
			return api.DeleteMessages("id", []string{"m"})
		}},
		{"SearchConversationHistory", func() error {
			_, err := api.SearchConversationHistory("q", 10)
			return err
		}},
		{"RebuildSearchIndex", func() error {
			return api.RebuildSearchIndex()
		}},
		{"SetConversationModel", func() error {
			return api.SetConversationModel("id", "m")
		}},
		{"GetEffectiveModel", func() error {
			_, err := api.GetEffectiveModel()
			return err
		}},
	}
	for _, c := range casos {
		c := c
		t.Run(c.nome, func(t *testing.T) {
			t.Parallel()
			if err := c.fn(); !errors.Is(err, semAuth) {
				t.Fatalf("erro = %v, quer o da sessão", err)
			}
		})
	}
}
